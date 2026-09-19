// Package kubesim is an in-process Kubernetes API server, enough of one to
// hold this product's use of it to account.
//
// # Why not client-go's fake clientset
//
// Because it is not a server. `fake.NewSimpleClientset` intercepts calls inside
// the client: there is no HTTP, no TLS, no client certificate and no
// authorisation, so a test against it proves that this product calls the
// functions it calls. Half of what goes wrong here is the path -- a certificate
// minted with the wrong group, an endpoint read out of the wrong field, a
// server that refuses the connection -- and a fake without the path cannot see
// any of it.
//
// # Why not envtest
//
// A real kube-apiserver and etcd are the truth, and they are two binaries CI
// would download and the operator's Pi would need for arm64. That is worth
// having later, as the Tier 1 and Tier 2 rungs are for Talos. It is not worth
// having instead of something that runs everywhere in a second.
//
// # What this is, and the rule it is built under
//
// A real TLS listener that verifies client certificates against a real
// certificate authority, serving real Kubernetes JSON -- marshalled from
// k8s.io/api types, so the wire shape is upstream's and not this file's
// opinion -- over state that CHANGES. A cordon changes what the next list
// returns. That last property is the one talossim had to be taught this week
// (ledger 3, and the CA rotation): a fake that records a call and drops its
// effect lets a test pass that hardware would fail.
package kubesim

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Options configure a simulated cluster.
type Options struct {
	// Version is what /version answers. It defaults to a release inside the
	// range internal/compat supports, because a simulator that answered
	// something unsupported would make every test about the version check.
	Version string

	// Pods are the cluster's pods. A test describes what it needs; the server
	// renders the Kubernetes shape.
	Pods []Pod

	// Namespaces the cluster has. Empty means the two every cluster has.
	Namespaces []string

	// Deployments the cluster has, described the way a test thinks of them.
	Deployments []Deployment

	// Nodes are the cluster's nodes. An empty list is a legitimate cluster --
	// an API server that is up before any kubelet registered -- and tests use
	// it, so it is not filled in by default.
	Nodes []Node

	// RequireClientCertificate is true by default: this fake exists partly to
	// prove the product authenticates. A test can turn it off to assert what
	// happens when it does not.
	AllowAnonymous bool

	// Listener lets a test decide the address BEFORE the server starts.
	//
	// It exists because of a chicken and an egg: the product reads the API
	// server's URL out of a Talos machine configuration, so a test has to know
	// that URL before it can generate the cluster -- and the cluster's
	// Kubernetes authority is what this server then has to verify against. With
	// a listener opened by the test, both are known in the right order.
	Listener net.Listener

	// AuthorityCrt and AuthorityKey let a test hand this server the Kubernetes
	// authority of a cluster that already exists -- talossim's derived bundle,
	// say -- rather than one it generated itself.
	//
	// It is what makes an end-to-end test possible at all: the product mints
	// its certificate from the authority the STORE holds, so a fake with an
	// authority of its own would refuse a correctly working product. Both must
	// be set together, or neither.
	AuthorityCrt []byte
	AuthorityKey []byte
}

// Node is a node as a test wants to describe it, rather than the ninety fields
// Kubernetes uses to say it.
type Node struct {
	Name             string
	Ready            corev1.ConditionStatus
	Unschedulable    bool
	Roles            []string
	KubeletVersion   string
	OSImage          string
	InternalAddress  string
	ContainerRuntime string
}

// Pod is a pod as a test wants to describe it.
type Pod struct {
	Namespace  string
	Name       string
	Node       string
	Phase      corev1.PodPhase
	Ready      int
	Containers int
	Restarts   int32
	WaitReason string

	// The four properties a drain has to treat differently, described the way a
	// test thinks about them rather than as owner references and annotations.
	//
	// OwnerKind is the controller that would recreate this pod -- "ReplicaSet",
	// "DaemonSet", "StatefulSet" -- or empty for a bare pod nothing owns.
	OwnerKind string

	// Mirror marks a static pod: the control plane's own on Talos, owned by the
	// kubelet, which the API server cannot evict.
	Mirror bool

	// LocalData gives the pod an emptyDir volume, whose contents go with it.
	LocalData bool

	// BudgetRefuses makes an eviction of this pod answer 429, which is what a
	// PodDisruptionBudget looks like from the client's side.
	BudgetRefuses bool
}

// Deployment is a deployment as a test wants to describe it.
type Deployment struct {
	Namespace string
	Name      string
	Desired   int32
	Ready     int32
	Image     string

	// RestartedAt is the annotation `kubectl rollout restart` writes. A test
	// reads it back, so a rollout restart is proven by what it wrote rather
	// than by having been called.
	RestartedAt string
}

// Server is a simulated cluster's API server.
type Server struct {
	srv *httptest.Server

	caCrt []byte
	caKey []byte

	mu         sync.Mutex
	version    string
	nodes      []Node
	pods       []Pod
	namespaces []string

	// calls counts what arrived, per method and path, so a test can assert
	// that a mutation reached the server exactly once -- the property
	// talossim's call counter exists for.
	calls map[string]int

	// anonymous records whether a request arrived with no client certificate,
	// which is what lets a test prove the product authenticates rather than
	// assume it.
	anonymous int

	// identities are the common names the peers presented, in arrival order.
	identities []string

	// evicted is what evictions actually removed, in order, so a test can
	// assert what a drain moved rather than what it reported moving.
	evicted []string

	// deleted is what a DELETE removed, kept apart from evicted because the
	// two are different operations: a test about restarting a pod must not
	// pass because something was evicted, and the reverse.
	deleted []string

	deployments []Deployment
}

// New starts one.
func New(opts Options) (*Server, error) {
	if opts.Version == "" {
		opts.Version = "v1.34.1"
	}

	caCrt, caKey, caCert, caSigner, err := authorityFrom(opts)
	if err != nil {
		return nil, err
	}

	s := &Server{
		caCrt:       caCrt,
		caKey:       caKey,
		version:     opts.Version,
		nodes:       append([]Node(nil), opts.Nodes...),
		deployments: append([]Deployment(nil), opts.Deployments...),
		pods:        append([]Pod(nil), opts.Pods...),
		namespaces:  namespacesOf(opts),
		calls:       map[string]int{},
	}

	serverCrt, err := serverCertificate(caCert, caSigner)
	if err != nil {
		return nil, err
	}

	pool := x509.NewCertPool()
	pool.AddCert(caCert)

	clientAuth := tls.RequireAndVerifyClientCert
	if opts.AllowAnonymous {
		clientAuth = tls.VerifyClientCertIfGiven
	}

	s.srv = httptest.NewUnstartedServer(s.routes())
	if opts.Listener != nil {
		_ = s.srv.Listener.Close()
		s.srv.Listener = opts.Listener
	}
	s.srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCrt},
		ClientAuth:   clientAuth,
		ClientCAs:    pool,
		MinVersion:   tls.VersionTLS12,
	}
	s.srv.StartTLS()
	return s, nil
}

// Close stops the server.
func (s *Server) Close() { s.srv.Close() }

// Endpoint is the URL the product should connect to.
func (s *Server) Endpoint() string { return s.srv.URL }

// AuthorityPEM is the cluster's Kubernetes certificate authority, certificate
// and key -- the pair internal/kube mints its own certificate from, and the
// pair the store holds for a real cluster.
func (s *Server) AuthorityPEM() (crt, key []byte) { return s.caCrt, s.caKey }

// Calls reports how many requests arrived for one method and path prefix.
func (s *Server) Calls(key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[key]
}

// AnonymousRequests reports how many requests arrived with no client
// certificate.
func (s *Server) AnonymousRequests() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.anonymous
}

// Identities are the common names the server saw, so a test can assert that
// this product introduces itself as itself rather than as admin.
func (s *Server) Identities() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.identities...)
}

// SetUnschedulable is what a scenario uses to put a node into a state the
// product then has to read correctly.
func (s *Server) SetUnschedulable(name string, unschedulable bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.nodes {
		if s.nodes[i].Name == name {
			s.nodes[i].Unschedulable = unschedulable
			return nil
		}
	}
	return fmt.Errorf("kubesim: no node named %q", name)
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/version", s.record(s.serveVersion))
	mux.HandleFunc("/api/v1/nodes", s.record(s.serveNodes))
	mux.HandleFunc("/api/v1/nodes/", s.record(s.serveNode))
	mux.HandleFunc("/api/v1/namespaces", s.record(s.serveNamespaces))
	mux.HandleFunc("/apis/apps/v1/deployments", s.record(s.serveDeployments))
	mux.HandleFunc("/apis/apps/v1/namespaces/", s.record(s.serveAppsNamespaced))
	mux.HandleFunc("/api/v1/pods", s.record(s.servePods))
	// The namespaced list, which is the path client-go uses when a namespace is
	// named: /api/v1/namespaces/{ns}/pods.
	mux.HandleFunc("/api/v1/namespaces/", s.record(s.serveNamespaced))
	// Everything else answers a Kubernetes-shaped 404 rather than Go's plain
	// text, because a client that has to parse an error is a client whose
	// behaviour on an unimplemented route is part of what is being tested.
	mux.HandleFunc("/", s.record(s.serveNotFound))
	return mux
}

// record notes the request and its identity before handing it on.
func (s *Server) record(next func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.calls[r.Method+" "+r.URL.Path]++
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			s.anonymous++
		} else {
			s.identities = append(s.identities, r.TLS.PeerCertificates[0].Subject.CommonName)
		}
		s.mu.Unlock()

		next(w, r)
	}
}

func (s *Server) serveVersion(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	version := s.version
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]string{
		"major":      "1",
		"minor":      "34",
		"gitVersion": version,
		"platform":   "linux/amd64",
	})
}

func (s *Server) serveNodes(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	nodes := append([]Node(nil), s.nodes...)
	s.mu.Unlock()

	list := corev1.NodeList{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "NodeList"},
		ListMeta: metav1.ListMeta{ResourceVersion: "1"},
	}
	for _, n := range nodes {
		list.Items = append(list.Items, renderNode(n))
	}

	writeJSON(w, http.StatusOK, list)
}

// namespacesOf is what a cluster has when a test did not say.
func namespacesOf(opts Options) []string {
	if len(opts.Namespaces) > 0 {
		return append([]string(nil), opts.Namespaces...)
	}

	// Every cluster has these two, plus whatever the pods are in: a screen that
	// offered a namespace list missing the namespace of a pod it just listed
	// would be its own kind of wrong.
	out := []string{"default", "kube-system"}
	for _, p := range opts.Pods {
		if p.Namespace == "" || slices.Contains(out, p.Namespace) {
			continue
		}
		out = append(out, p.Namespace)
	}
	return out
}

func (s *Server) serveNamespaces(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	names := append([]string(nil), s.namespaces...)
	s.mu.Unlock()

	list := corev1.NamespaceList{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "NamespaceList"},
		ListMeta: metav1.ListMeta{ResourceVersion: "1"},
	}
	for _, name := range names {
		list.Items = append(list.Items, corev1.Namespace{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"},
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
		})
	}
	writeJSON(w, http.StatusOK, list)
}

// serveNamespaced routes /api/v1/namespaces/{ns}/{resource}.
func (s *Server) serveNamespaced(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/namespaces/")
	ns, tail, ok := strings.Cut(rest, "/")
	if !ok {
		s.serveNotFound(w, r)
		return
	}

	switch parts := strings.Split(tail, "/"); {
	case len(parts) == 1 && parts[0] == "pods":
		s.writePods(w, r, ns)
	case len(parts) == 2 && parts[0] == "pods":
		s.servePod(w, r, ns, parts[1])
	case len(parts) == 3 && parts[0] == "pods" && parts[2] == "eviction":
		s.evictPod(w, r, ns, parts[1])
	default:
		s.serveNotFound(w, r)
	}
}

func (s *Server) servePods(w http.ResponseWriter, r *http.Request) { s.writePods(w, r, "") }

// writePods answers a pod list, filtered to one namespace when named and to one
// node when the request carries the field selector a drain sends.
//
// The selector is honoured rather than ignored, and that is not politeness: a
// fake that returned every pod in the cluster would let a drain of one node
// evict another node's pods and call it a success.
func (s *Server) writePods(w http.ResponseWriter, r *http.Request, namespace string) {
	// The field selector a drain sends is honoured rather than ignored, and
	// that is not politeness: a fake that answered with every pod in the
	// cluster would let a drain of one node evict another node's pods and
	// report success.
	node := ""
	for _, term := range strings.Split(r.URL.Query().Get("fieldSelector"), ",") {
		if name, ok := strings.CutPrefix(strings.TrimSpace(term), "spec.nodeName="); ok {
			node = name
		}
	}

	s.mu.Lock()
	pods := append([]Pod(nil), s.pods...)
	s.mu.Unlock()

	list := corev1.PodList{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "PodList"},
		ListMeta: metav1.ListMeta{ResourceVersion: "1"},
	}
	for _, p := range pods {
		if namespace != "" && p.Namespace != namespace {
			continue
		}
		if node != "" && p.Node != node {
			continue
		}

		list.Items = append(list.Items, renderPod(p))
	}

	writeJSON(w, http.StatusOK, list)
}

// renderNode is a test's node in the shape Kubernetes sends it.
//
// One function, because the list and the single-node answer a patch returns
// must not describe the same node differently -- a client that read the patch's
// body would otherwise see a node the list never shows.
func renderNode(n Node) corev1.Node {
	ready := n.Ready
	if ready == "" {
		ready = corev1.ConditionTrue
	}
	labels := map[string]string{}
	for _, role := range n.Roles {
		labels["node-role.kubernetes.io/"+role] = ""
	}

	return corev1.Node{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Node"},
		ObjectMeta: metav1.ObjectMeta{
			Name:              n.Name,
			Labels:            labels,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Hour).UTC()),
		},
		Spec: corev1.NodeSpec{Unschedulable: n.Unschedulable},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: ready}},
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: n.InternalAddress},
			},
			NodeInfo: corev1.NodeSystemInfo{
				KubeletVersion:          n.KubeletVersion,
				OSImage:                 n.OSImage,
				ContainerRuntimeVersion: n.ContainerRuntime,
			},
		},
	}
}

// serveNode is one node: the patch a cordon is, and nothing else.
//
// Only spec.unschedulable is applied, deliberately: a fake that applied any
// patch would accept patches a real API server rejects, and the product sends
// exactly this one.
func (s *Server) serveNode(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/v1/nodes/")
	if name == "" || strings.Contains(name, "/") || r.Method != http.MethodPatch {
		s.serveNotFound(w, r)
		return
	}

	var patch struct {
		Spec struct {
			Unschedulable *bool `json:"unschedulable"`
		} `json:"spec"`
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err != nil || json.Unmarshal(body, &patch) != nil || patch.Spec.Unschedulable == nil {
		writeJSON(w, http.StatusBadRequest, metav1.Status{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"},
			Status:   "Failure",
			Message:  "kubesim applies only patches that set spec.unschedulable",
			Reason:   metav1.StatusReasonBadRequest,
			Code:     http.StatusBadRequest,
		})
		return
	}

	if err := s.SetUnschedulable(name, *patch.Spec.Unschedulable); err != nil {
		s.serveNotFound(w, r)
		return
	}
	s.writeNode(w, name)
}

// writeNode answers one node, which is a patch's response body.
func (s *Server) writeNode(w http.ResponseWriter, name string) {
	s.mu.Lock()
	var found *Node
	for i := range s.nodes {
		if s.nodes[i].Name == name {
			node := s.nodes[i]
			found = &node
			break
		}
	}
	s.mu.Unlock()

	if found == nil {
		writeJSON(w, http.StatusNotFound, metav1.Status{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"},
			Status:   "Failure",
			Reason:   metav1.StatusReasonNotFound,
			Code:     http.StatusNotFound,
		})
		return
	}
	writeJSON(w, http.StatusOK, renderNode(*found))
}

// evictPod is the call a drain makes, and the one place a PodDisruptionBudget's
// refusal is modelled.
func (s *Server) evictPod(w http.ResponseWriter, r *http.Request, namespace, name string) {
	if r.Method != http.MethodPost {
		s.serveNotFound(w, r)
		return
	}

	s.mu.Lock()
	index := -1
	for i := range s.pods {
		if s.pods[i].Namespace == namespace && s.pods[i].Name == name {
			index = i
			break
		}
	}
	if index < 0 {
		s.mu.Unlock()
		s.serveNotFound(w, r)
		return
	}
	if s.pods[index].BudgetRefuses {
		s.mu.Unlock()
		// 429 with a Status body: what a real API server answers when a budget
		// would be violated, and what client-go turns into IsTooManyRequests.
		writeJSON(w, http.StatusTooManyRequests, metav1.Status{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"},
			Status:   "Failure",
			Message: "Cannot evict pod as it would violate the pod's disruption budget: " +
				"the budget allows no further disruption",
			Reason: metav1.StatusReasonTooManyRequests,
			Code:   http.StatusTooManyRequests,
		})
		return
	}

	// The pod is really gone, which is what makes a drain measurable: the next
	// list does not contain it.
	s.pods = append(s.pods[:index:index], s.pods[index+1:]...)
	s.evicted = append(s.evicted, namespace+"/"+name)
	s.mu.Unlock()

	writeJSON(w, http.StatusCreated, metav1.Status{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"},
		Status:   "Success",
		Code:     http.StatusCreated,
	})
}

// renderPod is a test's pod in the shape Kubernetes sends it.
//
// One function, for the reason renderNode is one: the list and the single-pod
// read a restart makes must describe the same pod identically, or the product's
// classification would depend on which call it came from.
func renderPod(p Pod) corev1.Pod {
	containers := p.Containers
	if containers == 0 {
		containers = 1
	}
	statuses := make([]corev1.ContainerStatus, 0, containers)
	for i := range containers {
		st := corev1.ContainerStatus{
			Name:  fmt.Sprintf("container-%d", i),
			Ready: i < p.Ready,
		}
		// The restart count a test gives is the POD's, and Kubernetes carries
		// it per container -- so it goes on the first one only. Putting it on
		// each container made a two-container pod report double, and the
		// client was right to sum them: that is what kubectl shows.
		if i == 0 {
			st.RestartCount = p.Restarts
		}
		if p.WaitReason != "" && !st.Ready {
			st.State = corev1.ContainerState{
				Waiting: &corev1.ContainerStateWaiting{Reason: p.WaitReason},
			}
		}
		statuses = append(statuses, st)
	}

	phase := p.Phase
	if phase == "" {
		phase = corev1.PodRunning
	}

	meta := metav1.ObjectMeta{
		Name:              p.Name,
		Namespace:         p.Namespace,
		CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Minute).UTC()),
	}
	if p.OwnerKind != "" {
		// A controller reference: what a drain and a restart both read to
		// decide whether anything would recreate this pod.
		controller := true
		meta.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: "apps/v1",
			Kind:       p.OwnerKind,
			Name:       p.Name + "-owner",
			UID:        types.UID(p.Namespace + "/" + p.Name + "/owner"),
			Controller: &controller,
		}}
	}
	if p.Mirror {
		meta.Annotations = map[string]string{corev1.MirrorPodAnnotationKey: "true"}
	}

	spec := corev1.PodSpec{NodeName: p.Node}
	if p.LocalData {
		spec.Volumes = []corev1.Volume{{
			Name:         "scratch",
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		}}
	}

	return corev1.Pod{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: meta,
		Spec:       spec,
		Status:     corev1.PodStatus{Phase: phase, ContainerStatuses: statuses},
	}
}

// servePod is one pod: the read a restart makes first, and the delete that IS
// the restart.
//
// The read is here because the product refuses to delete a pod no controller
// owns, and it can only know that by asking. A fake without it would leave that
// refusal untested.
func (s *Server) servePod(w http.ResponseWriter, r *http.Request, namespace, name string) {
	s.mu.Lock()
	index := -1
	for i := range s.pods {
		if s.pods[i].Namespace == namespace && s.pods[i].Name == name {
			index = i
			break
		}
	}
	if index < 0 {
		s.mu.Unlock()
		s.serveNotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		pod := s.pods[index]
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, renderPod(pod))

	case http.MethodDelete:
		s.pods = append(s.pods[:index:index], s.pods[index+1:]...)
		s.deleted = append(s.deleted, namespace+"/"+name)
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, metav1.Status{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"},
			Status:   "Success",
			Code:     http.StatusOK,
		})

	default:
		s.mu.Unlock()
		s.serveNotFound(w, r)
	}
}

// Deleted is what a DELETE removed, in order.
func (s *Server) Deleted() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.deleted...)
}

// serveDeployments answers the cluster-wide deployment list.
func (s *Server) serveDeployments(w http.ResponseWriter, _ *http.Request) {
	s.writeDeployments(w, "")
}

// serveAppsNamespaced routes the apps/v1 paths this product uses: the
// namespaced deployment list, the scale subresource, and the patch a rollout
// restart sends.
func (s *Server) serveAppsNamespaced(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/apis/apps/v1/namespaces/")
	ns, tail, ok := strings.Cut(rest, "/")
	if !ok {
		s.serveNotFound(w, r)
		return
	}

	parts := strings.Split(tail, "/")
	switch {
	case len(parts) == 1 && parts[0] == "deployments":
		s.writeDeployments(w, ns)
	case len(parts) == 3 && parts[0] == "deployments" && parts[2] == "scale":
		s.serveScale(w, r, ns, parts[1])
	case len(parts) == 2 && parts[0] == "deployments" && r.Method == http.MethodPatch:
		s.restartDeployment(w, r, ns, parts[1])
	default:
		s.serveNotFound(w, r)
	}
}

func (s *Server) writeDeployments(w http.ResponseWriter, namespace string) {
	s.mu.Lock()
	deployments := append([]Deployment(nil), s.deployments...)
	s.mu.Unlock()

	list := appsv1.DeploymentList{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "DeploymentList"},
		ListMeta: metav1.ListMeta{ResourceVersion: "1"},
	}
	for _, d := range deployments {
		if namespace != "" && d.Namespace != namespace {
			continue
		}
		list.Items = append(list.Items, renderDeployment(d))
	}
	writeJSON(w, http.StatusOK, list)
}

// serveScale is the subresource `kubectl scale` uses. It exists so a client can
// change the count without owning the rest of the spec, and the count really
// changes here: the next read says the new number.
func (s *Server) serveScale(w http.ResponseWriter, r *http.Request, namespace, name string) {
	s.mu.Lock()
	index := -1
	for i := range s.deployments {
		if s.deployments[i].Namespace == namespace && s.deployments[i].Name == name {
			index = i
			break
		}
	}
	if index < 0 {
		s.mu.Unlock()
		s.serveNotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		current := s.deployments[index]
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, renderScale(current))

	case http.MethodPut:
		var scale autoscalingv1.Scale
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		if err != nil || json.Unmarshal(body, &scale) != nil {
			s.mu.Unlock()
			// The message names the format, because this is exactly where a
			// client that negotiated protobuf shows up: client-go does that on
			// its own for this subresource unless the content type is pinned,
			// and a bare 400 sent the reader looking in the wrong place.
			writeJSON(w, http.StatusBadRequest, metav1.Status{
				TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"},
				Status:   "Failure",
				Message:  "kubesim reads JSON; this body is not JSON",
				Reason:   metav1.StatusReasonBadRequest,
				Code:     http.StatusBadRequest,
			})
			return
		}

		s.deployments[index].Desired = scale.Spec.Replicas
		current := s.deployments[index]
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, renderScale(current))

	default:
		s.mu.Unlock()
		s.serveNotFound(w, r)
	}
}

// restartDeployment applies the one patch a rollout restart sends: the
// timestamp annotation on the pod template.
func (s *Server) restartDeployment(w http.ResponseWriter, r *http.Request, namespace, name string) {
	var patch struct {
		Spec struct {
			Template struct {
				Metadata struct {
					Annotations map[string]string `json:"annotations"`
				} `json:"metadata"`
			} `json:"template"`
		} `json:"spec"`
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	stamp := ""
	if err == nil && json.Unmarshal(body, &patch) == nil {
		stamp = patch.Spec.Template.Metadata.Annotations["kubectl.kubernetes.io/restartedAt"]
	}
	if stamp == "" {
		// Only the patch a rollout restart sends is applied: a fake that took
		// any patch would accept one a real API server rejects.
		writeJSON(w, http.StatusBadRequest, metav1.Status{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"},
			Status:   "Failure",
			Message:  "kubesim applies only the restartedAt annotation patch",
			Reason:   metav1.StatusReasonBadRequest,
			Code:     http.StatusBadRequest,
		})
		return
	}

	s.mu.Lock()
	index := -1
	for i := range s.deployments {
		if s.deployments[i].Namespace == namespace && s.deployments[i].Name == name {
			index = i
			break
		}
	}
	if index < 0 {
		s.mu.Unlock()
		s.serveNotFound(w, r)
		return
	}
	s.deployments[index].RestartedAt = stamp
	current := s.deployments[index]
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, renderDeployment(current))
}

// RestartedAt is the annotation a rollout restart wrote.
func (s *Server) RestartedAt(namespace, name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.deployments {
		if d.Namespace == namespace && d.Name == name {
			return d.RestartedAt
		}
	}
	return ""
}

// DesiredReplicas is the count as this server now holds it.
func (s *Server) DesiredReplicas(namespace, name string) int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.deployments {
		if d.Namespace == namespace && d.Name == name {
			return d.Desired
		}
	}
	return -1
}

func renderDeployment(d Deployment) appsv1.Deployment {
	replicas := d.Desired
	template := corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: d.Image}}},
	}
	if d.RestartedAt != "" {
		template.ObjectMeta.Annotations = map[string]string{
			"kubectl.kubernetes.io/restartedAt": d.RestartedAt,
		}
	}

	return appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:              d.Name,
			Namespace:         d.Namespace,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Hour).UTC()),
		},
		Spec: appsv1.DeploymentSpec{Replicas: &replicas, Template: template},
		Status: appsv1.DeploymentStatus{
			Replicas:          d.Desired,
			ReadyReplicas:     d.Ready,
			UpdatedReplicas:   d.Ready,
			AvailableReplicas: d.Ready,
		},
	}
}

func renderScale(d Deployment) autoscalingv1.Scale {
	return autoscalingv1.Scale{
		TypeMeta:   metav1.TypeMeta{APIVersion: "autoscaling/v1", Kind: "Scale"},
		ObjectMeta: metav1.ObjectMeta{Name: d.Name, Namespace: d.Namespace},
		Spec:       autoscalingv1.ScaleSpec{Replicas: d.Desired},
		Status:     autoscalingv1.ScaleStatus{Replicas: d.Ready},
	}
}

// Evicted is what evictions actually removed, in order.
func (s *Server) Evicted() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.evicted...)
}

func (s *Server) serveNotFound(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, metav1.Status{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"},
		Status:   "Failure",
		Message:  "the path " + r.URL.Path + " is not served by kubesim",
		Reason:   metav1.StatusReasonNotFound,
		Code:     http.StatusNotFound,
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// authorityFrom uses the cluster's own authority when a test supplied one, and
// generates one otherwise.
func authorityFrom(opts Options) ([]byte, []byte, *x509.Certificate, crypto.Signer, error) {
	switch {
	case len(opts.AuthorityCrt) == 0 && len(opts.AuthorityKey) == 0:
		return newAuthority()
	case len(opts.AuthorityCrt) == 0 || len(opts.AuthorityKey) == 0:
		return nil, nil, nil, nil, errors.New("kubesim: an authority needs both its certificate " +
			"and its key; one of the two is missing")
	}

	certBlock, _ := pem.Decode(opts.AuthorityCrt)
	if certBlock == nil {
		return nil, nil, nil, nil, errors.New("kubesim: the supplied authority certificate is not PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("kubesim: the supplied authority certificate: %w", err)
	}

	keyBlock, _ := pem.Decode(opts.AuthorityKey)
	if keyBlock == nil {
		return nil, nil, nil, nil, errors.New("kubesim: the supplied authority key is not PEM")
	}

	// Talos writes this key as RSA; the listener's own certificate is then
	// signed by it, which is all this needs. An ECDSA key is accepted too, so
	// a test can hand over either.
	if rsaKey, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes); err == nil {
		return opts.AuthorityCrt, opts.AuthorityKey, cert, rsaKey, nil
	}
	ecKey, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("kubesim: the supplied authority key is in no "+
			"format this fake reads: %w", err)
	}
	return opts.AuthorityCrt, opts.AuthorityKey, cert, ecKey, nil
}

// newAuthority builds the cluster's Kubernetes CA.
func newAuthority() (crtPEM, keyPEM []byte, cert *x509.Certificate, signer crypto.Signer, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("kubesim: authority key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("kubesim: serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "kubernetes", Organization: []string{"kubesim"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("kubesim: authority certificate: %w", err)
	}
	cert, err = x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("kubesim: parse authority: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("kubesim: encode authority key: %w", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
		cert, key, nil
}

// serverCertificate issues the listener's own certificate, for 127.0.0.1 --
// which is where httptest listens, and the name the client will verify.
func serverCertificate(ca *x509.Certificate, caKey crypto.Signer) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("kubesim: server key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("kubesim: serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "kubernetes", Organization: []string{"kubesim"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"kubernetes", "localhost"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("kubesim: server certificate: %w", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("kubesim: parse server certificate: %w", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, nil
}
