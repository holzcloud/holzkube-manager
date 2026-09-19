// Package kube is this product's client for the Kubernetes API.
//
// It exists because the operator asked for Kubernetes and pod management on
// 2026-09-19, which lifts the boundary PROJECT.md drew -- "this product speaks
// the Talos machine API and nothing else". .planning/MILESTONE-v1.17.md records
// that decision, what it costs, and the half of the boundary that still holds:
// the daemon runs OUTSIDE the cluster, and everything that works today without
// Kubernetes has to go on working without it. A cluster whose API server is
// gone is precisely the cluster somebody is holding this tool to repair.
//
// # Where the credentials come from, and why nothing new is stored
//
// The daemon already holds each adopted cluster's Kubernetes certificate
// authority, key included: it derives it from the control-plane node's own
// machine configuration at import (D-01/D-02). So a client certificate is
// minted from that authority on demand and kept in memory for the life of the
// client -- there is no new secret on disk, and a store that was compromised
// was already game over, because the same record holds the authority itself.
//
// The certificate is issued to `holzkube-manager` and not to `admin`, which is
// what the cluster's own audit log will show. That is the whole reason not to
// reuse the admin kubeconfig Talos hands out: a product that acts as `admin`
// makes the cluster's record say nothing about who acted.
//
// # What this package deliberately does not decide
//
// It reaches the API server and answers questions. It holds no policy about
// which operations are allowed, and it does not know about sudo windows or the
// audit archive -- those live where every other mutation in this product lives,
// at the HTTP boundary. A read model comes out of here rather than Kubernetes
// types, for the reason internal/inventory's views exist: a screen that renders
// upstream structs renders whatever upstream changed.
package kube

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
)

// ErrNoKubernetesAuthority reports a cluster whose stored bundle cannot issue a
// client certificate for the Kubernetes API.
//
// Its own error because the repair is specific and nothing else in this package
// produces it: the cluster was adopted from a talosconfig that carried no
// Kubernetes authority key, which is enough to manage nodes over the Talos API
// and not enough to speak to Kubernetes.
var ErrNoKubernetesAuthority = errors.New("kube: this cluster's stored bundle has no Kubernetes authority key")

// ErrNoEndpoint reports a machine configuration that names no Kubernetes
// endpoint.
var ErrNoEndpoint = errors.New("kube: the machine configuration names no Kubernetes endpoint")

// ClientName is the common name every certificate this product mints for the
// Kubernetes API carries.
//
// It is a constant so that the cluster's audit log, the certificate, and this
// product's own audit archive all say the same word.
const ClientName = "holzkube-manager"

// clientGroup is the Kubernetes group the minted certificate claims.
//
// system:masters is cluster-admin by certificate, without an RBAC object to
// bind. It is the honest starting point -- this daemon already holds the Talos
// PKI, so it can reinstall every node in the cluster, and a narrower Kubernetes
// identity would not change what it is capable of. Whether it should
// nonetheless hold less here is a real question and it belongs with the node
// actions, where the first genuinely destructive Kubernetes call arrives.
const clientGroup = "system:masters"

// certValidity is how long a minted client certificate lasts.
//
// Short, and not stored: it is minted per client, so the failure mode the Talos
// client certificate has -- a date on which everything stops at once (D-23) --
// does not exist here. An hour is longer than any operation this product
// performs and shorter than a session.
const certValidity = time.Hour

// CallBudget is the ceiling on one call to a cluster's Kubernetes API.
//
// Exported because cmd/holzkube-managerd/budget_test.go composes every route's
// worst case out of the per-call budgets, and a guard that restated the number
// it guards would guard nothing.
//
// One number rather than a class table, and that is an honest description of
// what this client does today: every call is a list or a get. The evictions and
// applies of the later slices need a decision about their own budgets, and a
// second constant here is where that decision will be visible.
const CallBudget = 60 * time.Second

// Creds are what one cluster's Kubernetes API needs.
type Creds struct {
	// Server is the API server URL, as the cluster's own configuration names
	// it: https://host:6443.
	Server string

	// CACrt is the Kubernetes certificate authority, which is what verifies the
	// API server's certificate.
	CACrt []byte

	// ClientCrt and ClientKey are this product's own, minted from the cluster's
	// authority. They are not stored anywhere.
	ClientCrt []byte
	ClientKey []byte
}

// MintCreds issues this product a client certificate for one cluster.
//
// caCrt and caKey are the cluster's Kubernetes authority as the store holds it.
// The certificate never reaches disk: it is minted for this client and expires
// within the hour.
func MintCreds(server string, caCrt, caKey []byte, now time.Time) (Creds, error) {
	if strings.TrimSpace(server) == "" {
		return Creds{}, ErrNoEndpoint
	}
	if len(caCrt) == 0 || len(caKey) == 0 {
		return Creds{}, fmt.Errorf("%w. It was adopted from a talosconfig that carried no "+
			"Kubernetes authority, which is enough to manage its nodes over the Talos API and "+
			"not enough to reach its Kubernetes API", ErrNoKubernetesAuthority)
	}

	ca, caSigner, err := parseAuthority(caCrt, caKey)
	if err != nil {
		return Creds{}, err
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return Creds{}, fmt.Errorf("kube: generating a client key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return Creds{}, fmt.Errorf("kube: serial number: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: ClientName,
			// Kubernetes reads the certificate's organisations as the user's
			// groups. This is the authorisation decision, made in one place.
			Organization: []string{clientGroup},
		},
		NotBefore:   now.Add(-time.Minute),
		NotAfter:    now.Add(certValidity),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caSigner)
	if err != nil {
		return Creds{}, fmt.Errorf("kube: signing the client certificate: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return Creds{}, fmt.Errorf("kube: encoding the client key: %w", err)
	}

	return Creds{
		Server:    server,
		CACrt:     caCrt,
		ClientCrt: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		ClientKey: pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
	}, nil
}

func parseAuthority(caCrt, caKey []byte) (*x509.Certificate, any, error) {
	certBlock, _ := pem.Decode(caCrt)
	if certBlock == nil {
		return nil, nil, fmt.Errorf("%w: its certificate is not PEM", ErrNoKubernetesAuthority)
	}
	ca, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrNoKubernetesAuthority, err)
	}

	keyBlock, _ := pem.Decode(caKey)
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("%w: its key is not PEM", ErrNoKubernetesAuthority)
	}
	// Talos writes the Kubernetes authority's key as PKCS#1 or PKCS#8
	// depending on its age, and both have to load: a cluster adopted a year
	// ago must not be the one this refuses.
	if signer, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes); err == nil {
		return ca, signer, nil
	}
	if signer, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes); err == nil {
		return ca, signer, nil
	}
	signer, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: its key is in no format this build can read: %w",
			ErrNoKubernetesAuthority, err)
	}
	return ca, signer, nil
}

// EndpointFromMachineConfig reads the Kubernetes endpoint out of a node's own
// machine configuration.
//
// The cluster's own answer rather than a derivation: this product stores the
// address of the control-plane node it was adopted through, and gluing :6443
// onto that is right until a cluster puts its API server behind a virtual
// address or a load balancer -- which is exactly the cluster where being wrong
// costs the most.
//
// TWO PLACES, and the second was found by looking rather than by reading the
// documentation. In the v1alpha1 document the field is
// `cluster.controlPlane.endpoint`. Since Talos 1.14 the configuration is
// multi-document and the same fact lives in a document of its own:
//
//	---
//	apiVersion: v1alpha1
//	kind: KubeClusterConfig
//	clusterName: homelab
//	endpoint: https://192.168.0.100:6443
//
// A version of this function that read only the first place returned "no
// endpoint" for every cluster written by the Talos the operator actually runs
// -- which is to say for their cluster. So every document is read: the typed
// one wins when both are present, because a cluster that carries both has been
// migrated and the typed document is the one Talos itself reads.
func EndpointFromMachineConfig(configYAML []byte) (string, error) {
	dec := yaml.NewDecoder(bytes.NewReader(configYAML))

	var legacy string
	for {
		var doc struct {
			Kind     string `yaml:"kind"`
			Endpoint string `yaml:"endpoint"`
			Cluster  struct {
				ControlPlane struct {
					Endpoint string `yaml:"endpoint"`
				} `yaml:"controlPlane"`
			} `yaml:"cluster"`
		}

		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// A document this build cannot parse is not a reason to fail: the
			// configuration a node is running may carry kinds newer than this
			// binary, and the endpoint may be in one this build does read.
			// Stopping here would make a newer Talos unreadable for a field
			// that is present.
			break
		}

		if doc.Kind == "KubeClusterConfig" && strings.TrimSpace(doc.Endpoint) != "" {
			return strings.TrimSpace(doc.Endpoint), nil
		}
		if endpoint := strings.TrimSpace(doc.Cluster.ControlPlane.Endpoint); endpoint != "" {
			legacy = endpoint
		}
	}

	if legacy != "" {
		return legacy, nil
	}
	return "", ErrNoEndpoint
}

// Client is one cluster's Kubernetes API, as this product uses it.
type Client struct {
	cs *kubernetes.Clientset

	// dyn and mapper exist for the manifest path. An arbitrary manifest names
	// kinds this binary has never heard of -- every cluster with a
	// CustomResourceDefinition has some -- so the typed clientset cannot reach
	// them: the dynamic client sends whatever the document is, and the mapper
	// asks the CLUSTER which resource a kind lives at.
	dyn    dynamic.Interface
	mapper *restmapper.DeferredDiscoveryRESTMapper
}

// New builds a client from credentials.
//
// The rest configuration is assembled by hand rather than from a kubeconfig
// file, and that is deliberate: a kubeconfig is a file with a current context,
// a proxy setting and an exec plugin field, and every one of those is a way for
// something outside this process to decide where this client connects or what
// it runs. Here the three values that matter are passed in.
func New(creds Creds) (*Client, error) {
	if strings.TrimSpace(creds.Server) == "" {
		return nil, ErrNoEndpoint
	}
	if len(creds.CACrt) == 0 {
		return nil, errors.New("kube: no certificate authority to verify the API server against")
	}
	if len(creds.ClientCrt) == 0 || len(creds.ClientKey) == 0 {
		return nil, errors.New("kube: no client certificate to authenticate with")
	}

	cfg := &rest.Config{
		Host: creds.Server,
		TLSClientConfig: rest.TLSClientConfig{
			CAData:   creds.CACrt,
			CertData: creds.ClientCrt,
			KeyData:  creds.ClientKey,
		},
		// A ceiling, not the deadline callers rely on: every call in this
		// package takes its budget from the context, the way the Talos client
		// does (D-04). This is here so that a caller who forgot cannot hold a
		// connection for ever.
		Timeout: CallBudget,

		// JSON, explicitly, and this was measured rather than assumed:
		// client-go negotiates PROTOBUF for some paths on its own -- the scale
		// subresource sends a body starting with the bytes "k8s\x00" -- and a
		// reader expecting JSON gets a parse error with no useful message.
		//
		// Two reasons to pin it. The simulator this product is tested against
		// serves the API's JSON, marshalled from upstream types, and teaching
		// it a second wire format would buy nothing. And an operator debugging
		// this daemon against their own cluster can read JSON off the wire,
		// which is worth more here than the bytes protobuf saves on a fleet of
		// five machines.
		ContentConfig: rest.ContentConfig{
			ContentType:        "application/json",
			AcceptContentTypes: "application/json",
		},

		// The client-side rate limit, raised off its default, and this was
		// MEASURED: client-go throttles itself to 5 requests a second with a
		// burst of 10, and a manifest plan asks the cluster about every object
		// in turn. Twenty objects against an in-process fake on localhost took
		// two seconds -- all of it this limiter -- and a manifest of the size
		// this route accepts would have spent the route's whole budget waiting
		// on a queue inside this process, then reported the CLUSTER as slow.
		//
		// The numbers are kubectl's own. The limit is not removed, because a
		// bug in a loop here must not become a denial of service against
		// somebody's API server.
		QPS:   50,
		Burst: 100,
	}

	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("kube: building the client: %w", err)
	}

	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("kube: building the dynamic client: %w", err)
	}

	// The discovery cache is per client and therefore per call, which is the
	// conservative choice: a cached mapping that outlived a
	// CustomResourceDefinition being installed would refuse a manifest the
	// cluster accepts, and this product's clients are short-lived anyway
	// because the certificate is.
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(
		memory.NewMemCacheClient(cs.Discovery()))

	return &Client{cs: cs, dyn: dyn, mapper: mapper}, nil
}

// ServerVersion is the cheapest question that proves the whole path: DNS, TLS,
// the client certificate, and authorisation.
//
// It is what a connectivity proof uses, and it is a read, so it can be made
// against a production cluster without changing anything -- which is the
// difference between this half of the product and the Talos half.
func (c *Client) ServerVersion(ctx context.Context) (string, error) {
	// client-go's discovery client takes no context, so the deadline is
	// applied by the caller's context through the rest config's transport --
	// and the RESTClient path below is used instead, which does take one.
	body, err := c.cs.Discovery().RESTClient().Get().AbsPath("/version").DoRaw(ctx)
	if err != nil {
		return "", fmt.Errorf("kube: reading the server version: %w", err)
	}

	var info struct {
		GitVersion string `json:"gitVersion"`
	}
	// encoding/json and not the YAML decoder: YAML reads `yaml` tags, so it
	// would look for "gitversion" and find nothing -- silently, leaving the
	// field empty, which is how a decode against the wrong tag set reads as an
	// answer. The CLI met the same thing (v1.16 phase 7) and it cost a column
	// of zeroes that looked real.
	if err := json.Unmarshal(body, &info); err != nil {
		return "", fmt.Errorf("kube: decoding the server version: %w", err)
	}
	if info.GitVersion == "" {
		return "", errors.New("kube: the API server answered /version with no gitVersion")
	}
	return info.GitVersion, nil
}

// Node is one node as Kubernetes sees it.
//
// Kubernetes's view and the inventory's are allowed to disagree, and where they
// do, the disagreement is the information: the inventory knows what the machine
// API says about a machine, and this knows whether the kubelet registered, what
// the scheduler thinks, and whether somebody cordoned it.
type Node struct {
	Name string `json:"name"`

	// Ready is the Ready condition, and Unknown is a real answer: a node whose
	// kubelet stopped reporting is not NotReady, it is unheard from.
	Ready string `json:"ready"`

	// Unschedulable is the cordon, which is a decision somebody made rather
	// than a state the node fell into.
	Unschedulable bool `json:"unschedulable"`

	Roles            []string `json:"roles"`
	KubeletVersion   string   `json:"kubelet_version"`
	OSImage          string   `json:"os_image"`
	CreatedAt        string   `json:"created_at"`
	InternalAddress  string   `json:"internal_address"`
	ContainerRuntime string   `json:"container_runtime"`
}

// Nodes is every node the cluster's API server knows about.
func (c *Client) Nodes(ctx context.Context) ([]Node, error) {
	list, err := c.cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kube: listing nodes: %w", err)
	}

	out := make([]Node, 0, len(list.Items))
	for _, n := range list.Items {
		node := Node{
			Name:             n.Name,
			Ready:            "Unknown",
			Unschedulable:    n.Spec.Unschedulable,
			KubeletVersion:   n.Status.NodeInfo.KubeletVersion,
			OSImage:          n.Status.NodeInfo.OSImage,
			ContainerRuntime: n.Status.NodeInfo.ContainerRuntimeVersion,
			CreatedAt:        n.CreationTimestamp.UTC().Format(time.RFC3339),
		}
		for _, cond := range n.Status.Conditions {
			if cond.Type == "Ready" {
				node.Ready = string(cond.Status)
			}
		}
		for label := range n.Labels {
			if role, ok := strings.CutPrefix(label, "node-role.kubernetes.io/"); ok && role != "" {
				node.Roles = append(node.Roles, role)
			}
		}
		for _, addr := range n.Status.Addresses {
			if addr.Type == "InternalIP" {
				node.InternalAddress = addr.Address
				break
			}
		}
		out = append(out, node)
	}
	return out, nil
}

// Pod is one pod as the cluster reports it.
//
// The fields are the ones somebody looks at when a workload is not running:
// where it is, what phase it claims, how often it has restarted, and whether
// its containers are actually ready. A pod that is Running with 0/2 ready is
// the single most misread state in Kubernetes, which is why both are here.
type Pod struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Node      string `json:"node"`

	// Phase is the pod's own claim: Pending, Running, Succeeded, Failed or
	// Unknown.
	Phase string `json:"phase"`

	// Ready and Containers are the honest version of Phase: 1 of 2 containers
	// ready in a Running pod is a pod that is not working.
	Ready      int `json:"ready"`
	Containers int `json:"containers"`

	// Restarts is the sum over the pod's containers. It is the number that
	// distinguishes "started once and broke" from "breaking in a loop".
	Restarts int32 `json:"restarts"`

	// Reason is what the cluster says is wrong, when it says anything:
	// CrashLoopBackOff, ImagePullBackOff, Evicted. Empty is normal.
	Reason string `json:"reason"`

	CreatedAt string `json:"created_at"`
}

// Pods lists pods, in one namespace or across all of them when namespace is
// empty.
//
// Across all of them is the default the screen uses, because the question an
// operator arrives with is "what is broken" and not "what is broken in
// kube-system".
func (c *Client) Pods(ctx context.Context, namespace string) ([]Pod, error) {
	list, err := c.cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kube: listing pods: %w", err)
	}

	out := make([]Pod, 0, len(list.Items))
	for _, p := range list.Items {
		pod := Pod{
			Namespace:  p.Namespace,
			Name:       p.Name,
			Node:       p.Spec.NodeName,
			Phase:      string(p.Status.Phase),
			Containers: len(p.Status.ContainerStatuses),
			Reason:     p.Status.Reason,
			CreatedAt:  p.CreationTimestamp.UTC().Format(time.RFC3339),
		}
		for _, cs := range p.Status.ContainerStatuses {
			if cs.Ready {
				pod.Ready++
			}
			pod.Restarts += cs.RestartCount

			// A container's own reason beats the pod's, because the pod's is
			// usually empty while a container says CrashLoopBackOff.
			if pod.Reason == "" && cs.State.Waiting != nil {
				pod.Reason = cs.State.Waiting.Reason
			}
		}
		// A pod with no container statuses yet has containers in its spec, and
		// reporting 0 of 0 for a pod that is Pending would read as complete.
		if pod.Containers == 0 {
			pod.Containers = len(p.Spec.Containers)
		}
		out = append(out, pod)
	}
	return out, nil
}

// Namespaces is what the cluster has, so a screen can offer them rather than
// asking somebody to type one.
func (c *Client) Namespaces(ctx context.Context) ([]string, error) {
	list, err := c.cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("kube: listing namespaces: %w", err)
	}
	out := make([]string, 0, len(list.Items))
	for _, ns := range list.Items {
		out = append(out, ns.Name)
	}
	return out, nil
}
