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
	authv1 "k8s.io/api/authorization/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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

	// ReplicaSets, for the sweep. Owner names the Deployment behind it; a newer
	// one of the same owner is what a rollback would return to and is kept.
	ReplicaSets []ReplicaSet

	// Objects the cluster already holds, as "resource/namespace/name" (a
	// cluster-scoped object leaves the namespace empty: "namespaces//web"). They
	// are what makes a manifest plan's difference between "create" and "update"
	// measurable: the product asks the cluster, and this is the cluster's
	// answer.
	Objects []string

	// Services the cluster has, and what each answers when it is reached
	// through the API server's proxy. ProxyBodies is keyed
	// "namespace/service:port/path"; a body for a key that is not there is a
	// 404 from the proxy, which is what a real service does for a path it does
	// not serve.
	Services    []Service
	ProxyBodies map[string]string

	// The other workload kinds, described the way a test thinks of them.
	StatefulSets []StatefulSet
	DaemonSets   []DaemonSet
	Jobs         []Job
	CronJobs     []CronJob

	// The non-workload resources, described the way a test thinks of them.
	Resources []Resource

	// Storage: the cluster-scoped half the claim list cannot show.
	Volumes        []Volume
	StorageClasses []StorageClass

	// Namespaces beyond their names, with what constrains them.
	NamespaceFixtures []NamespaceFixture
	Quotas            []Quota
	LimitRanges       []LimitRange

	// The cluster's own kinds.
	CustomResourceDefinitions []CustomResourceDefinition

	// Access control: who may do what, and the bindings that grant nothing.
	Roles           []Role
	RoleBindings    []RoleBinding
	ServiceAccounts []ServiceAccount

	// Networking: what is behind a Service, and what is allowed to reach it.
	EndpointSlices  []EndpointSlice
	NetworkPolicies []NetworkPolicy
	IngressClasses  []IngressClass

	// Usage is what metrics-server would report, keyed "node/<name>" or
	// "pod/<namespace>/<name>", as "cpu,memory" -- e.g. "250m,512Mi".
	//
	// A nil map is a cluster with NO metrics-server, which is most clusters:
	// Talos does not ship one. That case is the default here deliberately, so a
	// product that assumed the numbers exist fails in the ordinary test rather
	// than on somebody's cluster.
	Usage map[string]string

	// Events the cluster has to report.
	Events []Event

	// Impersonation, as a cluster's RBAC looks from the client's side.
	//
	// AllowImpersonated lists the users this fake's RBAC permits. A request
	// that arrives impersonating anybody else is answered 403 -- which is the
	// case the product's promise turns on: it must NOT then retry as its own
	// certificate.
	AllowImpersonated []string

	// DenyVerbs are "verb:resource" pairs the impersonated user may not do,
	// answered through SelfSubjectAccessReview. It is how a test builds the
	// realistic case: an identity the cluster knows, with fewer rights than the
	// product needs.
	DenyVerbs []string

	// ConflictOn are objects whose apply answers 409, keyed the same way. It is
	// how a test reaches the case that matters most: another field manager owns
	// a field this apply sets, and the product must report that rather than
	// force it.
	ConflictOn []string

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

// Service is a service as a test wants to describe it.
type Service struct {
	Namespace string
	Name      string
	Type      string
	ClusterIP string
	Ports     []ServicePort

	// Selector as written. Empty is a Service whose endpoints are managed by
	// hand, which is a real kind and looks broken to a screen that assumes
	// every Service selects pods.
	Selector map[string]string

	// ExternalName for that type; LoadBalancerIP for an address a controller
	// assigned. An empty one on a LoadBalancer is a Service waiting for a
	// controller Talos does not ship.
	ExternalName   string
	LoadBalancerIP string
}

// ServicePort is one port of a service.
type ServicePort struct {
	Name string
	Port int32
}

// Node is a node as a test wants to describe it, rather than the ninety fields
// Kubernetes uses to say it.
type Node struct {
	Name          string
	Ready         corev1.ConditionStatus
	Unschedulable bool
	Roles         []string

	// Taints and Conditions as a test describes them: "key=value:Effect" and
	// "Type=Status,Reason". They exist so the detail screen's two hardest
	// claims can be measured -- that a pressure condition is bad when TRUE, and
	// that NoExecute is a different day from NoSchedule.
	Taints     []string
	Conditions []string

	// Allocatable, as Kubernetes quantities: "4", "8Gi", pods "110".
	CPUAllocatable    string
	MemoryAllocatable string
	PodCapacity       string
	KubeletVersion    string
	OSImage           string
	InternalAddress   string
	ContainerRuntime  string
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

	// Labels the pod carries, which is what a Service selector and a
	// NetworkPolicy selector both match on. Without them a test cannot build the
	// case those screens exist for: a selector that matches nothing.
	Labels map[string]string

	// ServiceAccount the pod runs as. Empty means "default", which is what
	// every pod that names none runs as -- and the reason a permission granted
	// to that account reaches things nobody intended.
	ServiceAccount string

	// Claims are the PersistentVolumeClaims this pod mounts, by name in its own
	// namespace. What makes "who is using this claim" measurable: nothing but
	// the pods knows it.
	Claims []string

	// BudgetRefuses makes an eviction of this pod answer 429, which is what a
	// PodDisruptionBudget looks like from the client's side.
	BudgetRefuses bool

	// ContainerNames gives the pod several containers. Empty means one called
	// after the pod, which is the ordinary shape and keeps every existing test
	// unchanged.
	ContainerNames []string

	// Image, and what the containers are doing. These exist so a test can build
	// the case the container detail was written for: a container that is
	// waiting in CrashLoopBackOff whose PREVIOUS run exited with a code.
	Image        string
	CrashLoop    bool
	LastExitCode int32
	LastReason   string

	// Logs are what each container said, keyed by container name, and
	// PreviousLogs what the run before this one said. The second is the one
	// that matters: a pod in CrashLoopBackOff has produced nothing yet in its
	// current container.
	Logs         map[string]string
	PreviousLogs map[string]string
}

// StatefulSet is one as a test describes it.
//
// Separate types for the four kinds rather than one with a kind field, because
// the numbers mean different things: a DaemonSet's count is how many nodes
// match, a Job's is how many pods finished, and a CronJob has none at all
// between runs. A single struct would invite a test to set "ready" on a
// CronJob, which is the confusion the product is being built not to have.
type StatefulSet struct {
	Namespace string
	Name      string
	Desired   int32
	Ready     int32
	Updated   int32
	Image     string
}

// DaemonSet is one as a test describes it: how many nodes match, how many are ready.
type DaemonSet struct {
	Namespace string
	Name      string
	// Scheduled is how many nodes match; Ready how many of those are up.
	Scheduled int32
	Ready     int32
	Image     string

	// NodeSelector is the selector its pod template carries. A patch that
	// changes it changes it here, so a stop that adds a key and a start that
	// removes it are read back rather than taken on trust.
	NodeSelector map[string]string
}

// Job is one as a test describes it: its numbers are succeeded and failed.
type Job struct {
	Namespace string
	Name      string
	Succeeded int32
	Failed    int32
	Active    int32
	Suspended bool
	Image     string
}

// CronJob is one as a test describes it: it has no pods between runs.
type CronJob struct {
	Namespace string
	Name      string
	Schedule  string
	Suspended bool
	LastRun   time.Time
	Image     string
}

// Volume is one PersistentVolume as a test describes it.
//
// Separate from Resource because the fields that matter are the ones no claim
// carries: the reclaim policy, which decides whether the data survives, and the
// phase Released, which is a volume holding data nothing claims.
type Volume struct {
	Name          string
	Capacity      string
	Phase         corev1.PersistentVolumePhase
	Claim         string // "namespace/name", empty when nothing holds it
	StorageClass  string
	ReclaimPolicy corev1.PersistentVolumeReclaimPolicy
	AccessModes   []corev1.PersistentVolumeAccessMode
	CSIDriver     string
	LocalPath     string
}

// StorageClass is one as a test describes it.
type StorageClass struct {
	Name            string
	Provisioner     string
	Default         bool
	ReclaimPolicy   corev1.PersistentVolumeReclaimPolicy
	WaitForConsumer bool
	AllowsExpansion bool
}

// EndpointSlice is what is actually behind a Service.
//
// A test says how many addresses and how many of them are ready, because the
// claim worth measuring is that a Service with three endpoints of which none are
// ready is reported as broken -- it is excluded from load balancing entirely and
// it looks healthier than having none.
type EndpointSlice struct {
	Namespace string
	Service   string
	Addresses int
	NotReady  int
}

// NetworkPolicy is one as a test describes it.
type NetworkPolicy struct {
	Namespace string
	Name      string
	// Selector as written. Empty means every pod in the namespace, which is the
	// case that surprises people.
	Selector map[string]string
	// HasEgress adds an egress rule, so the derived policy types are both.
	HasEgress bool
	// Types overrides the derived list, for a policy that names them.
	Types []networkingv1.PolicyType
}

// IngressClass is one as a test describes it.
type IngressClass struct {
	Name       string
	Controller string
	Default    bool
}

// NamespaceFixture is one namespace as a test describes it.
//
// Terminating and the conditions that explain it are the point: kubectl shows the
// word and nothing about what is holding the namespace, and a fake that could not
// render a stuck one could not measure the finding.
type NamespaceFixture struct {
	Name string
	// Terminating makes it the stuck case; Holding is what the API server says is
	// keeping it, as the condition message.
	Terminating bool
	Holding     string
	Finalizers  []string
}

// Quota is one ResourceQuota as a test describes it.
//
// Used and Hard as plain strings per resource, keyed "cpu", "memory",
// "count/pods" -- because the claim worth measuring is that a FULL quota is named
// and a quota on object counts does not trigger the LimitRange warning.
type Quota struct {
	Namespace string
	Name      string
	Used      map[string]string
	Hard      map[string]string
}

// LimitRange only has to exist for the namespace screen's purposes.
type LimitRange struct {
	Namespace string
	Name      string
}

// CustomResourceDefinition is one of the cluster's own kinds as a test describes
// it.
type CustomResourceDefinition struct {
	Group  string
	Kind   string
	Plural string
	Scope  string
	// Versions served, and which one stores. NotServed is a version present in
	// the definition that the API server does not answer.
	Versions  []string
	Stored    string
	NotServed []string
	// Established false is a kind every manifest naming it is refused for.
	Established bool
}

// Role is a Role or a ClusterRole as a test describes it.
//
// Rules as three lists, because the claim worth measuring is that wildcard verbs
// AND wildcard groups AND wildcard resources in ONE rule is administrative, while
// wildcards spread across several rules are not -- "* verbs on configmaps" and
// "get on *" are both ordinary, and a check that ORed them would report half the
// cluster's built-in roles as administrative and therefore report nothing.
type Role struct {
	// Namespace empty makes it a ClusterRole.
	Namespace string
	Name      string
	Verbs     []string
	APIGroups []string
	Resources []string
	// NonResourceURLs for a rule about /healthz rather than about anything in
	// the cluster, which is not administration of it.
	NonResourceURLs []string
	// BuiltIn marks it the way Kubernetes marks its own seventy.
	BuiltIn bool
}

// RoleBinding is a RoleBinding or a ClusterRoleBinding as a test describes it.
type RoleBinding struct {
	// Namespace empty makes it a ClusterRoleBinding.
	Namespace string
	Name      string
	// RoleKind is "Role" or "ClusterRole"; RoleName may deliberately name one
	// that does not exist, which is the case this exists for.
	RoleKind string
	RoleName string
	Subjects []BindingSubject
}

// BindingSubject is one subject of a binding.
type BindingSubject struct {
	// Kind is User, Group or ServiceAccount.
	Kind      string
	Namespace string
	Name      string
}

// ServiceAccount is one as a test describes it.
type ServiceAccount struct {
	Namespace string
	Name      string
}

// Resource is one non-volume, non-workload object, as a test describes it.
//
// One struct with a Kind field here, unlike the workload kinds: these are
// LISTED rather than acted on, and the fields a test sets are the same shape for
// all of them -- a phase, a size, a couple of keys.
type Resource struct {
	Kind      string
	Namespace string
	Name      string

	// Keys for a ConfigMap or a Secret.
	Keys []string

	// Phase and Size for a PersistentVolumeClaim.
	Phase string
	Size  string

	// StorageClass and VolumeName for a claim, so the storage screen can say
	// what it is bound to and whether its class allows growing it.
	StorageClass string
	VolumeName   string

	// Host and Address for an Ingress. An empty address is an Ingress no
	// controller has claimed, which is the "why does this URL not work" case.
	Host    string
	Address string

	// Current, Min and Max for a HorizontalPodAutoscaler, plus what it targets.
	Current    int32
	Min        int32
	Max        int32
	TargetKind string
	TargetName string

	// Healthy, Desired and Allowed for a PodDisruptionBudget. Allowed zero is
	// why a drain refuses.
	Healthy int32
	Desired int32
	Allowed int32
}

// ReplicaSet is one as a test describes it: which Deployment owns it, how many
// it runs, and how old it is relative to its siblings.
type ReplicaSet struct {
	Namespace string
	Name      string
	Owner     string
	Replicas  int32
	Age       time.Duration
}

// Event is one thing the cluster reported, as a test describes it.
type Event struct {
	Namespace string
	Type      string
	Reason    string
	Message   string
	Kind      string
	Name      string
	Count     int32
	LastSeen  time.Time
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

	// applied are the objects server-side apply wrote, keyed by
	// "resource/namespace/name", with the document as it arrived. A test reads
	// them back, so an apply is proven by what the cluster now holds rather
	// than by a call having been made.
	applied map[string]map[string]any

	// managers records the field manager every apply named, and forced the
	// objects an apply overrode a conflict on.
	managers   []string
	forced     []string
	conflictOn []string

	services     []Service
	volumes      []Volume
	classes      []StorageClass
	slices       []EndpointSlice
	policies     []NetworkPolicy
	ingclasses   []IngressClass
	nsfixtures   []NamespaceFixture
	quotas       []Quota
	limitranges  []LimitRange
	crds         []CustomResourceDefinition
	roles        []Role
	bindings     []RoleBinding
	accounts     []ServiceAccount
	statefulSets []StatefulSet
	daemonSets   []DaemonSet
	jobs         []Job
	cronJobs     []CronJob
	resources    []Resource
	replicaSets  []ReplicaSet
	annotations  map[string]map[string]string
	usage        map[string]string
	restarted    map[string]string
	events       []Event

	allowImpersonated []string
	denyVerbs         []string

	// impersonated records the identity every request arrived as, in order. It
	// is what proves the product acted as the operator rather than as itself --
	// a claim nothing else can check, because both requests succeed.
	impersonated []string
	proxyBodies  map[string]string

	// proxied records every path a proxy request asked for, as
	// "namespace/service:port/path". It is what proves the product built the URL
	// it meant to: a traversal that slipped through would show up here as a path
	// nobody asked for, and nowhere else.
	proxied []string

	// graces records the grace period each pod DELETE asked for, keyed
	// "namespace/name", and nil when it asked for none. A force-stop and a stop
	// both delete; the grace period is the whole difference, so it has to be
	// readable back.
	graces map[string]*int64
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
		caCrt:             caCrt,
		caKey:             caKey,
		version:           opts.Version,
		nodes:             append([]Node(nil), opts.Nodes...),
		deployments:       append([]Deployment(nil), opts.Deployments...),
		applied:           map[string]map[string]any{},
		conflictOn:        append([]string(nil), opts.ConflictOn...),
		services:          append([]Service(nil), opts.Services...),
		volumes:           append([]Volume(nil), opts.Volumes...),
		classes:           append([]StorageClass(nil), opts.StorageClasses...),
		slices:            append([]EndpointSlice(nil), opts.EndpointSlices...),
		policies:          append([]NetworkPolicy(nil), opts.NetworkPolicies...),
		ingclasses:        append([]IngressClass(nil), opts.IngressClasses...),
		nsfixtures:        append([]NamespaceFixture(nil), opts.NamespaceFixtures...),
		quotas:            append([]Quota(nil), opts.Quotas...),
		limitranges:       append([]LimitRange(nil), opts.LimitRanges...),
		crds:              append([]CustomResourceDefinition(nil), opts.CustomResourceDefinitions...),
		roles:             append([]Role(nil), opts.Roles...),
		bindings:          append([]RoleBinding(nil), opts.RoleBindings...),
		accounts:          append([]ServiceAccount(nil), opts.ServiceAccounts...),
		events:            append([]Event(nil), opts.Events...),
		statefulSets:      append([]StatefulSet(nil), opts.StatefulSets...),
		daemonSets:        append([]DaemonSet(nil), opts.DaemonSets...),
		jobs:              append([]Job(nil), opts.Jobs...),
		cronJobs:          append([]CronJob(nil), opts.CronJobs...),
		resources:         append([]Resource(nil), opts.Resources...),
		replicaSets:       append([]ReplicaSet(nil), opts.ReplicaSets...),
		annotations:       map[string]map[string]string{},
		usage:             opts.Usage,
		allowImpersonated: append([]string(nil), opts.AllowImpersonated...),
		denyVerbs:         append([]string(nil), opts.DenyVerbs...),
		proxyBodies:       opts.ProxyBodies,
		pods:              append([]Pod(nil), opts.Pods...),
		namespaces:        namespacesOf(opts),
		calls:             map[string]int{},
	}

	for _, key := range opts.Objects {
		resource, rest, ok := strings.Cut(key, "/")
		namespace, name, ok2 := strings.Cut(rest, "/")
		if !ok || !ok2 || name == "" {
			return nil, fmt.Errorf(
				"kubesim: %q is not resource/namespace/name", key)
		}
		metadata := map[string]any{"name": name}
		if namespace != "" {
			metadata["namespace"] = namespace
		}
		object := map[string]any{"kind": "ConfigMap", "apiVersion": "v1", "metadata": metadata}
		if resource == "namespaces" {
			object["kind"] = "Namespace"
		}
		s.applied[key] = object
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
	// Discovery, which the manifest path needs: an arbitrary manifest names
	// kinds this binary has never heard of, so the product asks the CLUSTER
	// which resource a kind lives at. A fake without discovery would make that
	// path untestable and the product's own mapper unexercised.
	mux.HandleFunc("/apis/authorization.k8s.io/v1/selfsubjectaccessreviews",
		s.record(s.serveAccessReview))
	mux.HandleFunc("/apis/authorization.k8s.io/v1", s.record(s.serveAuthResources))
	mux.HandleFunc("/api", s.record(s.serveAPIVersions))
	mux.HandleFunc("/apis", s.record(s.serveAPIGroups))
	mux.HandleFunc("/api/v1", s.record(s.serveCoreResources))
	mux.HandleFunc("/apis/apps/v1", s.record(s.serveAppsResources))
	mux.HandleFunc("/apis/apps/v1/deployments", s.record(s.serveDeployments))
	mux.HandleFunc("/apis/apps/v1/replicasets", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeReplicaSets(w, "")
	}))
	mux.HandleFunc("/apis/apps/v1/statefulsets", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeStatefulSets(w, "")
	}))
	mux.HandleFunc("/apis/apps/v1/daemonsets", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeDaemonSets(w, "")
	}))
	mux.HandleFunc("/apis/metrics.k8s.io/v1beta1/nodes", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeUsage(w, "node", "")
	}))
	mux.HandleFunc("/apis/metrics.k8s.io/v1beta1/pods", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeUsage(w, "pod", "")
	}))
	mux.HandleFunc("/apis/metrics.k8s.io/v1beta1/namespaces/", s.record(func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/apis/metrics.k8s.io/v1beta1/namespaces/")
		ns, tail, ok := strings.Cut(rest, "/")
		if !ok || tail != "pods" {
			s.serveNotFound(w, r)
			return
		}
		s.writeUsage(w, "pod", ns)
	}))
	mux.HandleFunc("/apis/batch/v1", s.record(s.serveBatchResources))
	mux.HandleFunc("/apis/networking.k8s.io/v1", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeGroupResources(w, "networking.k8s.io/v1",
			metav1.APIResource{Name: "ingresses", SingularName: "ingress", Namespaced: true,
				Kind: "Ingress", Verbs: metav1.Verbs{"get", "list", "delete"}})
	}))
	mux.HandleFunc("/apis/autoscaling/v2", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeGroupResources(w, "autoscaling/v2",
			metav1.APIResource{Name: "horizontalpodautoscalers", SingularName: "horizontalpodautoscaler",
				Namespaced: true, Kind: "HorizontalPodAutoscaler",
				Verbs: metav1.Verbs{"get", "list", "delete"}})
	}))
	mux.HandleFunc("/apis/policy/v1", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeGroupResources(w, "policy/v1",
			metav1.APIResource{Name: "poddisruptionbudgets", SingularName: "poddisruptionbudget",
				Namespaced: true, Kind: "PodDisruptionBudget",
				Verbs: metav1.Verbs{"get", "list", "delete"}})
	}))
	mux.HandleFunc("/apis/networking.k8s.io/v1/", s.record(s.serveGroupNamespaced))
	mux.HandleFunc("/apis/autoscaling/v2/", s.record(s.serveGroupNamespaced))
	mux.HandleFunc("/apis/policy/v1/", s.record(s.serveGroupNamespaced))
	mux.HandleFunc("/apis/batch/v1/jobs", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeJobs(w, "")
	}))
	mux.HandleFunc("/apis/batch/v1/cronjobs", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeCronJobs(w, "")
	}))
	mux.HandleFunc("/apis/batch/v1/namespaces/", s.record(s.serveBatchNamespaced))
	mux.HandleFunc("/apis/apps/v1/namespaces/", s.record(s.serveAppsNamespaced))
	mux.HandleFunc("/api/v1/resourcequotas", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeQuotas(w, "")
	}))
	mux.HandleFunc("/api/v1/limitranges", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeLimitRanges(w, "")
	}))
	mux.HandleFunc("/apis/apiextensions.k8s.io/v1/customresourcedefinitions", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeCRDs(w)
	}))
	mux.HandleFunc("/apis/rbac.authorization.k8s.io/v1/roles", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeRoles(w, "", false)
	}))
	mux.HandleFunc("/apis/rbac.authorization.k8s.io/v1/clusterroles", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeRoles(w, "", true)
	}))
	mux.HandleFunc("/apis/rbac.authorization.k8s.io/v1/rolebindings", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeBindings(w, "", false)
	}))
	mux.HandleFunc("/apis/rbac.authorization.k8s.io/v1/clusterrolebindings", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeBindings(w, "", true)
	}))
	mux.HandleFunc("/apis/rbac.authorization.k8s.io/v1/namespaces/", s.record(s.serveRBACNamespaced))
	mux.HandleFunc("/api/v1/serviceaccounts", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeServiceAccounts(w, "")
	}))
	mux.HandleFunc("/api/v1/persistentvolumes", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeVolumes(w)
	}))
	mux.HandleFunc("/apis/storage.k8s.io/v1/storageclasses", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeStorageClasses(w)
	}))
	mux.HandleFunc("/apis/discovery.k8s.io/v1/endpointslices", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeEndpointSlices(w, "")
	}))
	mux.HandleFunc("/apis/discovery.k8s.io/v1/namespaces/", s.record(s.serveDiscoveryNamespaced))
	mux.HandleFunc("/apis/networking.k8s.io/v1/networkpolicies", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeNetworkPolicies(w, "")
	}))
	mux.HandleFunc("/apis/networking.k8s.io/v1/ingressclasses", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeIngressClasses(w)
	}))
	mux.HandleFunc("/api/v1/services", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeServices(w, "")
	}))
	mux.HandleFunc("/api/v1/events", s.record(func(w http.ResponseWriter, r *http.Request) {
		s.writeEvents(w, r, "")
	}))
	mux.HandleFunc("/api/v1/configmaps", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeResources(w, "ConfigMap", "")
	}))
	mux.HandleFunc("/api/v1/secrets", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeResources(w, "Secret", "")
	}))
	mux.HandleFunc("/api/v1/persistentvolumeclaims", s.record(func(w http.ResponseWriter, _ *http.Request) {
		s.writeResources(w, "PersistentVolumeClaim", "")
	}))
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

		// Every recorded request passes the impersonation check, so a 403 for a
		// user this fake's RBAC does not know reaches the product from wherever
		// it asked -- which is what makes "it never retries as the admin"
		// something a test can watch rather than a sentence in a comment.
		if !s.checkImpersonation(w, r) {
			return
		}

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
	s.mu.Lock()
	fixtures := append([]NamespaceFixture(nil), s.nsfixtures...)
	s.mu.Unlock()
	detailed := map[string]NamespaceFixture{}
	for _, fixture := range fixtures {
		detailed[fixture.Name] = fixture
		// A fixture names a namespace whether or not the plain list did: a test
		// about a stuck namespace should not also have to list it twice.
		if !contains(names, fixture.Name) {
			names = append(names, fixture.Name)
		}
	}

	for _, name := range names {
		rendered := corev1.Namespace{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"},
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
		}
		if fixture, ok := detailed[name]; ok && fixture.Terminating {
			rendered.Status.Phase = corev1.NamespaceTerminating
			if fixture.Holding != "" {
				// The condition is where the API server says WHAT is holding it,
				// and it is the whole answer kubectl does not show.
				rendered.Status.Conditions = []corev1.NamespaceCondition{{
					Type:    corev1.NamespaceContentRemaining,
					Status:  corev1.ConditionTrue,
					Message: fixture.Holding,
				}}
			}
			for _, finalizer := range fixture.Finalizers {
				rendered.Spec.Finalizers = append(rendered.Spec.Finalizers,
					corev1.FinalizerName(finalizer))
			}
		}
		list.Items = append(list.Items, rendered)
	}
	writeJSON(w, http.StatusOK, list)
}

// contains is the small membership test the namespace merge needs.
func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// serveNamespaced routes /api/v1/namespaces/{ns}/{resource}.
func (s *Server) serveNamespaced(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/namespaces/")
	ns, tail, ok := strings.Cut(rest, "/")
	if !ok {
		// /api/v1/namespaces/{name}: the namespace itself, which is
		// cluster-scoped. A manifest that creates one comes here, and sending it
		// to the namespaced path instead is exactly the mistake the scope in
		// discovery exists to prevent.
		s.serveObject(w, r, "namespaces", "", rest)
		return
	}

	switch parts := strings.Split(tail, "/"); {
	case len(parts) == 1 && parts[0] == "pods":
		s.writePods(w, r, ns)
	case len(parts) == 2 && parts[0] == "pods":
		s.servePod(w, r, ns, parts[1])
	case len(parts) == 3 && parts[0] == "pods" && parts[2] == "log":
		s.servePodLog(w, r, ns, parts[1])
	case len(parts) == 1 && parts[0] == "events":
		s.writeEvents(w, r, ns)
	case len(parts) == 3 && parts[0] == "pods" && parts[2] == "eviction":
		s.evictPod(w, r, ns, parts[1])
	case len(parts) == 1 && parts[0] == "configmaps":
		s.writeResources(w, "ConfigMap", ns)
	case len(parts) == 1 && parts[0] == "secrets":
		s.writeResources(w, "Secret", ns)
	case len(parts) == 1 && parts[0] == "persistentvolumeclaims":
		s.writeResources(w, "PersistentVolumeClaim", ns)
	case len(parts) == 2 && parts[0] == "configmaps":
		s.serveObject(w, r, "configmaps", ns, parts[1])
	case len(parts) == 1 && parts[0] == "services":
		s.writeServices(w, ns)
	case len(parts) == 1 && parts[0] == "serviceaccounts":
		s.writeServiceAccounts(w, ns)
	case len(parts) >= 2 && parts[0] == "services":
		s.serveService(w, r, ns, parts[1], parts[2:])
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
	// And the label selector an app's pods are found by, for the same reason:
	// a fake that ignored it would let a force-stop of one app kill another's
	// pods and report success.
	selector := r.URL.Query().Get("labelSelector")

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
		if !matchesLabels(p.Labels, selector) {
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
		Spec: corev1.NodeSpec{Unschedulable: n.Unschedulable, Taints: taintsOf(n.Taints)},
		Status: corev1.NodeStatus{
			Allocatable: allocatableOf(n),
			Conditions:  append([]corev1.NodeCondition{{Type: corev1.NodeReady, Status: ready}}, conditionsOf(n.Conditions)...),
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
	if name == "" || strings.Contains(name, "/") {
		s.serveNotFound(w, r)
		return
	}
	// A plain get, which the node detail reads. It was not served before,
	// because until then nothing asked for one node.
	if r.Method == http.MethodGet {
		s.writeNode(w, name)
		return
	}
	if r.Method != http.MethodPatch {
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
	// A test may name the containers; otherwise they are numbered as before, so
	// every test written against the old shape still describes the same pod.
	names := p.ContainerNames
	if len(names) == 0 {
		names = make([]string, containers)
		for i := range containers {
			names[i] = fmt.Sprintf("container-%d", i)
		}
	}
	containers = len(names)

	statuses := make([]corev1.ContainerStatus, 0, containers)
	for i := range containers {
		st := corev1.ContainerStatus{
			Name:  names[i],
			Image: p.Image,
			Ready: i < p.Ready,
		}
		// The case the container detail exists for: waiting to be restarted,
		// with the diagnosis belonging to the run that already ended.
		if p.CrashLoop {
			st.State = corev1.ContainerState{
				Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
			}
			st.LastTerminationState = corev1.ContainerState{
				Terminated: &corev1.ContainerStateTerminated{
					ExitCode:   p.LastExitCode,
					Reason:     p.LastReason,
					FinishedAt: metav1.NewTime(time.Now().Add(-time.Minute).UTC()),
				},
			}
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
		Labels:            p.Labels,
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

	spec := corev1.PodSpec{NodeName: p.Node, ServiceAccountName: p.ServiceAccount}
	for _, name := range names {
		spec.Containers = append(spec.Containers, corev1.Container{Name: name, Image: p.Image})
	}
	if p.LocalData {
		spec.Volumes = append(spec.Volumes, corev1.Volume{
			Name:         "scratch",
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		})
	}
	for _, claim := range p.Claims {
		spec.Volumes = append(spec.Volumes, corev1.Volume{
			Name: claim,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: claim},
			},
		})
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
		s.recordGrace(namespace+"/"+name, r)
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
	case len(parts) == 2 && parts[0] == "deployments" && r.Method == http.MethodGet:
		s.writeDeployment(w, ns, parts[1])
	case len(parts) == 2 && parts[0] == "statefulsets" && r.Method == http.MethodGet:
		s.writeStatefulSet(w, ns, parts[1])
	case len(parts) == 1 && parts[0] == "replicasets":
		s.writeReplicaSets(w, ns)
	case len(parts) == 2 && parts[0] == "replicasets" && r.Method == http.MethodDelete:
		s.recordDelete(w, "replicasets", ns, parts[1])
	case len(parts) == 1 && parts[0] == "statefulsets":
		s.writeStatefulSets(w, ns)
	case len(parts) == 3 && parts[0] == "statefulsets" && parts[2] == "scale":
		s.serveStatefulSetScale(w, r, ns, parts[1])
	case len(parts) == 2 && parts[0] == "statefulsets" && r.Method == http.MethodPatch:
		s.restartTemplate(w, r, "statefulset", ns, parts[1])
	case len(parts) == 2 && parts[0] == "daemonsets" && r.Method == http.MethodPatch:
		s.patchDaemonSet(w, r, ns, parts[1])
	case len(parts) == 2 && parts[0] == "daemonsets" && r.Method == http.MethodGet:
		s.writeDaemonSet(w, ns, parts[1])
	case len(parts) == 1 && parts[0] == "daemonsets":
		s.writeDaemonSets(w, ns)
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
		Metadata struct {
			Annotations map[string]*string `json:"annotations"`
		} `json:"metadata"`
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

	// A patch that writes annotations on the OBJECT is what a stop sends to
	// remember the replica count. It is applied and kept, because the start
	// reads it back -- a fake that dropped it would let "starting restores what
	// was running" pass while restoring nothing.
	if len(patch.Metadata.Annotations) > 0 {
		s.storeAnnotations("deployment", namespace, name, patch.Metadata.Annotations)
		writeJSON(w, http.StatusOK, metav1.ObjectMeta{Name: name, Namespace: namespace})
		return
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
		template.Annotations = map[string]string{
			"kubectl.kubernetes.io/restartedAt": d.RestartedAt,
		}
	}

	template.Labels = map[string]string{"app": d.Name}

	return appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:              d.Name,
			Namespace:         d.Namespace,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Hour).UTC()),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas, Template: template, Selector: appSelector(d.Name),
		},
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

// Discovery and server-side apply (milestone v1.17, slice 5).
//
// A manifest names kinds this binary has never heard of, so the product asks the
// cluster which resource a kind lives at and what scope it has. That makes
// discovery part of the path under test: a fake without it would leave the
// product's own mapper unexercised, and the first CustomResourceDefinition in a
// real manifest would be the first time anybody found out.
//
// Two kinds are served, and deliberately two: a ConfigMap, which lives in a
// namespace, and a Namespace, which does not. The scope is the half that goes
// wrong -- sending a cluster-scoped object to a namespaced path answers 404 --
// and one kind could not tell the two apart.

func (s *Server) serveAPIVersions(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, metav1.APIVersions{
		TypeMeta: metav1.TypeMeta{Kind: "APIVersions"},
		Versions: []string{"v1"},
	})
}

func (s *Server) serveAPIGroups(w http.ResponseWriter, _ *http.Request) {
	apps := metav1.GroupVersionForDiscovery{GroupVersion: "apps/v1", Version: "v1"}
	batch := metav1.GroupVersionForDiscovery{GroupVersion: "batch/v1", Version: "v1"}
	writeJSON(w, http.StatusOK, metav1.APIGroupList{
		TypeMeta: metav1.TypeMeta{Kind: "APIGroupList", APIVersion: "v1"},
		Groups: []metav1.APIGroup{
			{
				TypeMeta:         metav1.TypeMeta{Kind: "APIGroup", APIVersion: "v1"},
				Name:             "apps",
				Versions:         []metav1.GroupVersionForDiscovery{apps},
				PreferredVersion: apps,
			},
			{
				TypeMeta:         metav1.TypeMeta{Kind: "APIGroup", APIVersion: "v1"},
				Name:             "batch",
				Versions:         []metav1.GroupVersionForDiscovery{batch},
				PreferredVersion: batch,
			},
			group("networking.k8s.io", "networking.k8s.io/v1"),
			group("autoscaling", "autoscaling/v2"),
			group("policy", "policy/v1"),
		},
	})
}

func (s *Server) serveCoreResources(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, metav1.APIResourceList{
		TypeMeta:     metav1.TypeMeta{Kind: "APIResourceList", APIVersion: "v1"},
		GroupVersion: "v1",
		APIResources: []metav1.APIResource{
			{Name: "nodes", SingularName: "node", Namespaced: false, Kind: "Node",
				Verbs: metav1.Verbs{"get", "list", "patch", "update"}},
			{Name: "namespaces", SingularName: "namespace", Namespaced: false, Kind: "Namespace",
				Verbs: metav1.Verbs{"get", "list", "create", "patch", "update"}},
			{Name: "pods", SingularName: "pod", Namespaced: true, Kind: "Pod",
				Verbs: metav1.Verbs{"get", "list", "delete"}},
			{Name: "pods/eviction", SingularName: "", Namespaced: true, Kind: "Eviction",
				Verbs: metav1.Verbs{"create"}},
			{Name: "configmaps", SingularName: "configmap", Namespaced: true, Kind: "ConfigMap",
				Verbs: metav1.Verbs{"get", "list", "create", "patch", "update"}},
			// Offered deliberately, so the product's refusal to render a Secret
			// is proven against a cluster that HAS them. Without this the
			// refusal could be removed and the test would still pass -- on
			// "this cluster does not have Secret", which is a different
			// sentence and no protection at all.
			{Name: "secrets", SingularName: "secret", Namespaced: true, Kind: "Secret",
				Verbs: metav1.Verbs{"get", "list"}},
		},
	})
}

func (s *Server) serveAppsResources(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, metav1.APIResourceList{
		TypeMeta:     metav1.TypeMeta{Kind: "APIResourceList", APIVersion: "v1"},
		GroupVersion: "apps/v1",
		APIResources: []metav1.APIResource{
			{Name: "deployments", SingularName: "deployment", Namespaced: true, Kind: "Deployment",
				Verbs: metav1.Verbs{"get", "list", "patch", "update"}},
			{Name: "deployments/scale", Namespaced: true, Kind: "Scale",
				Verbs: metav1.Verbs{"get", "update"}},
		},
	})
}

// serveObject answers get and apply for one object of a kind this fake stores
// generically, so an apply is proven by what the server now holds.
func (s *Server) serveObject(w http.ResponseWriter, r *http.Request, resource, namespace, name string) {
	key := resource + "/" + namespace + "/" + name

	switch r.Method {
	case http.MethodGet:
		s.mu.Lock()
		object, ok := s.applied[key]
		s.mu.Unlock()
		if !ok {
			s.writeStatus(w, http.StatusNotFound, metav1.StatusReasonNotFound,
				fmt.Sprintf("%s %q not found", resource, name))
			return
		}
		writeJSON(w, http.StatusOK, object)

	case http.MethodPatch:
		if got := r.Header.Get("Content-Type"); got != string(types.ApplyPatchType) {
			// Not politeness: server-side apply is what records a field manager,
			// and a fake that accepted a merge patch here would let this product
			// silently take over somebody else's field while the test passed.
			s.writeStatus(w, http.StatusUnsupportedMediaType, metav1.StatusReasonUnsupportedMediaType,
				fmt.Sprintf("this fake serves apply, and the request was %q", got))
			return
		}
		manager := r.URL.Query().Get("fieldManager")
		if manager == "" {
			s.writeStatus(w, http.StatusUnprocessableEntity, metav1.StatusReasonInvalid,
				"an apply must name a field manager")
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			s.writeStatus(w, http.StatusBadRequest, metav1.StatusReasonBadRequest, err.Error())
			return
		}
		object := map[string]any{}
		if err := json.Unmarshal(body, &object); err != nil {
			s.writeStatus(w, http.StatusBadRequest, metav1.StatusReasonBadRequest, err.Error())
			return
		}

		s.mu.Lock()
		conflicts := slices.Contains(s.conflictOn, key)
		forced := r.URL.Query().Get("force") == "true"
		if !conflicts || forced {
			s.applied[key] = object
			s.managers = append(s.managers, manager)
			if forced {
				s.forced = append(s.forced, key)
			}
		}
		s.mu.Unlock()

		if conflicts && !forced {
			// What a real API server says when another manager owns a field this
			// apply sets, down to the reason, because that reason is what the
			// product branches on.
			s.writeStatus(w, http.StatusConflict, metav1.StatusReasonConflict,
				fmt.Sprintf("Apply failed with 1 conflict: conflict with %q: .data.tuned", "someone-else"))
			return
		}
		writeJSON(w, http.StatusOK, object)

	default:
		s.serveNotFound(w, r)
	}
}

// writeStatus answers with the Status object a real API server sends, so that
// client-go's own apierrors.IsNotFound and IsConflict decide what this is.
func (s *Server) writeStatus(
	w http.ResponseWriter, code int32, reason metav1.StatusReason, message string,
) {
	writeJSON(w, int(code), metav1.Status{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"},
		Status:   metav1.StatusFailure,
		Code:     code,
		Reason:   reason,
		Message:  message,
	})
}

// Applied returns the object server-side apply wrote, or nil.
func (s *Server) Applied(resource, namespace, name string) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.applied[resource+"/"+namespace+"/"+name]
}

// FieldManagers returns the field managers every apply named, in order.
func (s *Server) FieldManagers() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.managers...)
}

// Forced returns the objects an apply overrode a conflict on. A product that
// forced its way past somebody else's field manager would show up here, and
// nowhere else.
func (s *Server) Forced() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.forced...)
}

// Services and the proxy subresource (milestone v1.17, slice 6).

func (s *Server) writeServices(w http.ResponseWriter, namespace string) {
	s.mu.Lock()
	services := append([]Service(nil), s.services...)
	s.mu.Unlock()

	list := corev1.ServiceList{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ServiceList"},
		ListMeta: metav1.ListMeta{ResourceVersion: "1"},
	}
	for _, svc := range services {
		if namespace != "" && svc.Namespace != namespace {
			continue
		}
		rendered := corev1.Service{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
			ObjectMeta: metav1.ObjectMeta{Name: svc.Name, Namespace: svc.Namespace},
			Spec: corev1.ServiceSpec{
				Type:         corev1.ServiceType(svc.Type),
				ClusterIP:    svc.ClusterIP,
				Selector:     svc.Selector,
				ExternalName: svc.ExternalName,
			},
		}
		if svc.LoadBalancerIP != "" {
			rendered.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
				{IP: svc.LoadBalancerIP},
			}
		}
		if rendered.Spec.Type == "" {
			rendered.Spec.Type = corev1.ServiceTypeClusterIP
		}
		for _, p := range svc.Ports {
			rendered.Spec.Ports = append(rendered.Spec.Ports, corev1.ServicePort{
				Name: p.Name, Port: p.Port, Protocol: corev1.ProtocolTCP,
			})
		}
		list.Items = append(list.Items, rendered)
	}
	writeJSON(w, http.StatusOK, list)
}

// serveService answers a get on one service, and its proxy subresource.
//
// The name arrives as the proxy addresses it -- "name:port" -- so this splits it
// the way the API server does. A test can then assert on the port the product
// sent, which is half of what the proxy has to get right.
func (s *Server) serveService(w http.ResponseWriter, r *http.Request, namespace, name string, tail []string) {
	target, _, hasPort := strings.Cut(name, ":")

	s.mu.Lock()
	var found *Service
	for i := range s.services {
		if s.services[i].Namespace == namespace && s.services[i].Name == target {
			found = &s.services[i]
			break
		}
	}
	s.mu.Unlock()

	if found == nil {
		s.writeStatus(w, http.StatusNotFound, metav1.StatusReasonNotFound,
			fmt.Sprintf("services %q not found", target))
		return
	}

	if len(tail) == 0 || tail[0] != "proxy" {
		// A plain get of the service, which is what the product does before it
		// proxies.
		writeJSON(w, http.StatusOK, corev1.Service{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
			ObjectMeta: metav1.ObjectMeta{Name: found.Name, Namespace: found.Namespace},
			Spec:       corev1.ServiceSpec{Type: corev1.ServiceType(found.Type), ClusterIP: found.ClusterIP},
		})
		return
	}

	if !hasPort {
		s.writeStatus(w, http.StatusBadRequest, metav1.StatusReasonBadRequest,
			"the proxy subresource needs a port")
		return
	}
	if r.Method != http.MethodGet {
		// A real API server forwards the method. This one refuses anything else,
		// because the product is meant never to send one -- and a fake that
		// forwarded a POST would let that promise rot quietly.
		s.writeStatus(w, http.StatusMethodNotAllowed, metav1.StatusReasonMethodNotAllowed,
			fmt.Sprintf("this fake proxies GET only, and the request was %s", r.Method))
		return
	}

	path := "/" + strings.Join(tail[1:], "/")
	key := namespace + "/" + name + path

	s.mu.Lock()
	s.proxied = append(s.proxied, key)
	body, ok := s.proxyBodies[key]
	s.mu.Unlock()

	if !ok {
		s.writeStatus(w, http.StatusNotFound, metav1.StatusReasonNotFound,
			fmt.Sprintf("the service answered 404 for %q", path))
		return
	}

	// Deliberately a content type the product must NOT honour: if it ever served
	// a proxied body as the workload described it, this is the header that would
	// have put a workload's markup in the daemon's own origin.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, body)
}

// Proxied returns every path a proxy request asked for, in order.
func (s *Server) Proxied() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.proxied...)
}

// Logs and events (2026-09-19).

// servePodLog answers the log subresource.
//
// It honours `previous` and `container`, because those two are exactly what the
// product has to get right: a fake that returned the same text for both would
// let a screen label the dead container's output as the running one's, and a
// crash loop is diagnosed entirely from the dead one.
func (s *Server) servePodLog(w http.ResponseWriter, r *http.Request, namespace, name string) {
	s.mu.Lock()
	var found *Pod
	for i := range s.pods {
		if s.pods[i].Namespace == namespace && s.pods[i].Name == name {
			found = &s.pods[i]
			break
		}
	}
	s.mu.Unlock()

	if found == nil {
		s.writeStatus(w, http.StatusNotFound, metav1.StatusReasonNotFound,
			fmt.Sprintf("pods %q not found", name))
		return
	}

	container := r.URL.Query().Get("container")
	previous := r.URL.Query().Get("previous") == "true"

	logs := found.Logs
	if previous {
		logs = found.PreviousLogs
	}
	body, ok := logs[container]
	if !ok {
		// What a real API server says when the container never ran: a 400 with
		// a sentence, not a 404. The product turns it into something readable
		// rather than an error, and this is what lets that be tested.
		s.writeStatus(w, http.StatusBadRequest, metav1.StatusReasonBadRequest,
			fmt.Sprintf("previous terminated container %q in pod %q not found", container, name))
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, body)
}

func (s *Server) writeEvents(w http.ResponseWriter, r *http.Request, namespace string) {
	s.mu.Lock()
	events := append([]Event(nil), s.events...)
	s.mu.Unlock()

	// The field selector is honoured rather than ignored, for the reason the
	// pod list's is: a fake that returned every event would let a detail screen
	// show another object's failures under this object's name.
	wantName, wantKind := "", ""
	for _, term := range strings.Split(r.URL.Query().Get("fieldSelector"), ",") {
		key, value, ok := strings.Cut(term, "=")
		if !ok {
			continue
		}
		switch key {
		case "involvedObject.name":
			wantName = value
		case "involvedObject.kind":
			wantKind = value
		}
	}

	list := corev1.EventList{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "EventList"},
		ListMeta: metav1.ListMeta{ResourceVersion: "1"},
	}
	for _, e := range events {
		if namespace != "" && e.Namespace != namespace {
			continue
		}
		if wantName != "" && e.Name != wantName {
			continue
		}
		if wantKind != "" && e.Kind != wantKind {
			continue
		}
		seen := e.LastSeen
		if seen.IsZero() {
			seen = time.Now().UTC()
		}
		count := e.Count
		if count == 0 {
			count = 1
		}
		list.Items = append(list.Items, corev1.Event{
			TypeMeta:       metav1.TypeMeta{APIVersion: "v1", Kind: "Event"},
			ObjectMeta:     metav1.ObjectMeta{Name: e.Reason + "-" + e.Name, Namespace: e.Namespace},
			Type:           e.Type,
			Reason:         e.Reason,
			Message:        e.Message,
			Count:          count,
			FirstTimestamp: metav1.NewTime(seen.Add(-time.Minute)),
			LastTimestamp:  metav1.NewTime(seen),
			InvolvedObject: corev1.ObjectReference{Kind: e.Kind, Name: e.Name, Namespace: e.Namespace},
		})
	}
	writeJSON(w, http.StatusOK, list)
}

// Impersonation and RBAC (2026-09-19).

// checkImpersonation records who a request arrived as and answers 403 when this
// fake's RBAC does not know them.
//
// Recording is half the value. Whether the product impersonated or not, the
// request succeeds against a permissive fake, so a test that only watched the
// answer could not tell the two apart -- and "acts as the operator" is precisely
// the claim being made.
func (s *Server) checkImpersonation(w http.ResponseWriter, r *http.Request) bool {
	user := r.Header.Get("Impersonate-User")

	s.mu.Lock()
	s.impersonated = append(s.impersonated, user)
	allowed := s.allowImpersonated
	s.mu.Unlock()

	if user == "" {
		return true
	}
	if slices.Contains(allowed, user) {
		return true
	}

	s.writeStatus(w, http.StatusForbidden, metav1.StatusReasonForbidden,
		fmt.Sprintf("User %q cannot do this: no RBAC policy matched", user))
	return false
}

// Impersonated returns the identity every request arrived as, in order. An
// empty string is a request that arrived as the client's own certificate.
func (s *Server) Impersonated() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.impersonated...)
}

// serveAccessReview answers SelfSubjectAccessReview for whoever asked.
func (s *Server) serveAccessReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.serveNotFound(w, r)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		s.writeStatus(w, http.StatusBadRequest, metav1.StatusReasonBadRequest, err.Error())
		return
	}
	var review authv1.SelfSubjectAccessReview
	if err := json.Unmarshal(body, &review); err != nil {
		s.writeStatus(w, http.StatusBadRequest, metav1.StatusReasonBadRequest, err.Error())
		return
	}

	attrs := review.Spec.ResourceAttributes
	if attrs == nil {
		attrs = &authv1.ResourceAttributes{}
	}

	user := r.Header.Get("Impersonate-User")
	s.mu.Lock()
	denied := slices.Contains(s.denyVerbs, attrs.Verb+":"+attrs.Resource)
	s.mu.Unlock()

	// Without impersonation the caller is this product's own certificate, which
	// is in system:masters and may do everything. Saying otherwise would make
	// the preview lie about the state the product is in before anybody switches
	// impersonation on.
	allowed := user == "" || !denied

	review.Status = authv1.SubjectAccessReviewStatus{Allowed: allowed}
	if !allowed {
		review.Status.Reason = fmt.Sprintf("no RBAC policy allows %q to %s %s",
			user, attrs.Verb, attrs.Resource)
	}
	review.TypeMeta = metav1.TypeMeta{APIVersion: "authorization.k8s.io/v1", Kind: "SelfSubjectAccessReview"}
	writeJSON(w, http.StatusCreated, review)
}

// serveAuthResources lets discovery find the access-review subresource.
func (s *Server) serveAuthResources(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, metav1.APIResourceList{
		TypeMeta:     metav1.TypeMeta{Kind: "APIResourceList", APIVersion: "v1"},
		GroupVersion: "authorization.k8s.io/v1",
		APIResources: []metav1.APIResource{{
			Name: "selfsubjectaccessreviews", Namespaced: false, Kind: "SelfSubjectAccessReview",
			Verbs: metav1.Verbs{"create"},
		}},
	})
}

// The other workload kinds (2026-09-19).

func (s *Server) writeStatefulSets(w http.ResponseWriter, namespace string) {
	s.mu.Lock()
	rows := append([]StatefulSet(nil), s.statefulSets...)
	s.mu.Unlock()

	list := appsv1.StatefulSetList{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "StatefulSetList"},
		ListMeta: metav1.ListMeta{ResourceVersion: "1"},
	}
	for _, r := range rows {
		if namespace != "" && r.Namespace != namespace {
			continue
		}
		desired := r.Desired
		list.Items = append(list.Items, appsv1.StatefulSet{
			TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "StatefulSet"},
			ObjectMeta: metav1.ObjectMeta{Name: r.Name, Namespace: r.Namespace},
			Spec: appsv1.StatefulSetSpec{
				Replicas: &desired,
				Template: podTemplate(r.Name, r.Image),
				Selector: appSelector(r.Name),
			},
			Status: appsv1.StatefulSetStatus{
				ReadyReplicas:   r.Ready,
				UpdatedReplicas: r.Updated,
				Replicas:        r.Ready,
			},
		})
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) writeDaemonSets(w http.ResponseWriter, namespace string) {
	s.mu.Lock()
	rows := append([]DaemonSet(nil), s.daemonSets...)
	s.mu.Unlock()
	// renderDaemonSet takes the lock itself, for the annotations.

	list := appsv1.DaemonSetList{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "DaemonSetList"},
		ListMeta: metav1.ListMeta{ResourceVersion: "1"},
	}
	for _, r := range rows {
		if namespace != "" && r.Namespace != namespace {
			continue
		}
		list.Items = append(list.Items, s.renderDaemonSet(r))
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) writeJobs(w http.ResponseWriter, namespace string) {
	s.mu.Lock()
	rows := append([]Job(nil), s.jobs...)
	s.mu.Unlock()

	list := batchv1.JobList{
		TypeMeta: metav1.TypeMeta{APIVersion: "batch/v1", Kind: "JobList"},
		ListMeta: metav1.ListMeta{ResourceVersion: "1"},
	}
	for _, r := range rows {
		if namespace != "" && r.Namespace != namespace {
			continue
		}
		list.Items = append(list.Items, renderJob(r, nil))
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) writeCronJobs(w http.ResponseWriter, namespace string) {
	s.mu.Lock()
	rows := append([]CronJob(nil), s.cronJobs...)
	s.mu.Unlock()

	list := batchv1.CronJobList{
		TypeMeta: metav1.TypeMeta{APIVersion: "batch/v1", Kind: "CronJobList"},
		ListMeta: metav1.ListMeta{ResourceVersion: "1"},
	}
	for _, r := range rows {
		if namespace != "" && r.Namespace != namespace {
			continue
		}
		suspend := r.Suspended
		item := batchv1.CronJob{
			TypeMeta:   metav1.TypeMeta{APIVersion: "batch/v1", Kind: "CronJob"},
			ObjectMeta: metav1.ObjectMeta{Name: r.Name, Namespace: r.Namespace},
			Spec: batchv1.CronJobSpec{
				Schedule: r.Schedule,
				Suspend:  &suspend,
				JobTemplate: batchv1.JobTemplateSpec{
					Spec: batchv1.JobSpec{Template: podTemplate(r.Name, r.Image)},
				},
			},
		}
		if !r.LastRun.IsZero() {
			last := metav1.NewTime(r.LastRun)
			item.Status.LastScheduleTime = &last
		}
		list.Items = append(list.Items, item)
	}
	writeJSON(w, http.StatusOK, list)
}

// serveBatchNamespaced routes /apis/batch/v1/namespaces/{ns}/{resource}.
func (s *Server) serveBatchNamespaced(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/apis/batch/v1/namespaces/")
	ns, tail, ok := strings.Cut(rest, "/")
	if !ok {
		s.serveNotFound(w, r)
		return
	}
	switch tail {
	case "jobs":
		s.writeJobs(w, ns)
	case "cronjobs":
		s.writeCronJobs(w, ns)
	default:
		parts := strings.Split(tail, "/")
		switch {
		case len(parts) == 2 && parts[0] == "cronjobs" && r.Method == http.MethodPatch:
			s.patchSuspend(w, r, "cronjob", ns, parts[1])
		case len(parts) == 2 && parts[0] == "jobs" && r.Method == http.MethodPatch:
			s.patchSuspend(w, r, "job", ns, parts[1])
		case len(parts) == 2 && parts[0] == "cronjobs" && r.Method == http.MethodGet:
			s.writeCronJob(w, ns, parts[1])
		case len(parts) == 2 && parts[0] == "jobs" && r.Method == http.MethodGet:
			s.writeJob(w, ns, parts[1])
		case len(parts) == 2 && parts[0] == "jobs" && r.Method == http.MethodDelete:
			s.recordDelete(w, "jobs", ns, parts[1])
		default:
			s.serveNotFound(w, r)
		}
	}
}

func (s *Server) serveBatchResources(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, metav1.APIResourceList{
		TypeMeta:     metav1.TypeMeta{Kind: "APIResourceList", APIVersion: "v1"},
		GroupVersion: "batch/v1",
		APIResources: []metav1.APIResource{
			{Name: "jobs", SingularName: "job", Namespaced: true, Kind: "Job",
				Verbs: metav1.Verbs{"get", "list", "delete"}},
			{Name: "cronjobs", SingularName: "cronjob", Namespaced: true, Kind: "CronJob",
				Verbs: metav1.Verbs{"get", "list", "patch"}},
		},
	})
}

// podTemplate is the one field every kind's template shares that this product
// reads: the first container's image.
func podTemplate(name, image string) corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: name, Image: image}}},
	}
}

// serveStatefulSetScale answers the scale subresource, and CHANGES the state.
//
// A fake that recorded the call and dropped its effect would let a test pass
// that hardware fails -- the rule talossim had to be taught (ledger 3).
func (s *Server) serveStatefulSetScale(w http.ResponseWriter, r *http.Request, namespace, name string) {
	s.mu.Lock()
	var found *StatefulSet
	for i := range s.statefulSets {
		if s.statefulSets[i].Namespace == namespace && s.statefulSets[i].Name == name {
			found = &s.statefulSets[i]
			break
		}
	}
	s.mu.Unlock()

	if found == nil {
		s.writeStatus(w, http.StatusNotFound, metav1.StatusReasonNotFound,
			fmt.Sprintf("statefulsets %q not found", name))
		return
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, scaleOf(namespace, name, found.Desired))
	case http.MethodPut:
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			s.writeStatus(w, http.StatusBadRequest, metav1.StatusReasonBadRequest, err.Error())
			return
		}
		var scale autoscalingv1.Scale
		if err := json.Unmarshal(body, &scale); err != nil {
			s.writeStatus(w, http.StatusBadRequest, metav1.StatusReasonBadRequest, err.Error())
			return
		}
		s.mu.Lock()
		found.Desired = scale.Spec.Replicas
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, scaleOf(namespace, name, scale.Spec.Replicas))
	default:
		s.serveNotFound(w, r)
	}
}

// storeAnnotations keeps what a patch wrote on an object's metadata.
//
// A fake that dropped them would let "starting restores what was running" pass
// while restoring nothing: the count is written as an annotation precisely so
// it outlives the scale to zero.
//
// A null removes the key, which is what a strategic merge patch means by it:
// an enable that removed the disabled mark and a fake that kept it as "" would
// disagree about whether the app is disabled.
func (s *Server) storeAnnotations(kind, namespace, name string, annotations map[string]*string) {
	if len(annotations) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := kind + "/" + namespace + "/" + name
	if s.annotations[key] == nil {
		s.annotations[key] = map[string]string{}
	}
	for k, v := range annotations {
		if v == nil {
			delete(s.annotations[key], k)
			continue
		}
		s.annotations[key][k] = *v
	}
}

// restartTemplate records a rollout restart of a kind that has a pod template.
func (s *Server) restartTemplate(w http.ResponseWriter, r *http.Request, kind, namespace, name string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		s.writeStatus(w, http.StatusBadRequest, metav1.StatusReasonBadRequest, err.Error())
		return
	}
	var patch struct {
		Metadata struct {
			Annotations map[string]*string `json:"annotations"`
		} `json:"metadata"`
		Spec struct {
			Template struct {
				Metadata struct {
					Annotations map[string]string `json:"annotations"`
				} `json:"metadata"`
			} `json:"template"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(body, &patch); err != nil {
		s.writeStatus(w, http.StatusBadRequest, metav1.StatusReasonBadRequest, err.Error())
		return
	}

	if len(patch.Metadata.Annotations) > 0 {
		s.storeAnnotations(kind, namespace, name, patch.Metadata.Annotations)
		writeJSON(w, http.StatusOK, metav1.ObjectMeta{Name: name, Namespace: namespace})
		return
	}

	s.mu.Lock()
	if s.restarted == nil {
		s.restarted = map[string]string{}
	}
	s.restarted[kind+"/"+namespace+"/"+name] =
		patch.Spec.Template.Metadata.Annotations["kubectl.kubernetes.io/restartedAt"]
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, metav1.ObjectMeta{Name: name, Namespace: namespace})
}

// RestartedTemplate returns the timestamp a rollout restart wrote for a kind
// other than a Deployment, or "".
func (s *Server) RestartedTemplate(kind, namespace, name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.restarted[kind+"/"+namespace+"/"+name]
}

func scaleOf(namespace, name string, replicas int32) autoscalingv1.Scale {
	return autoscalingv1.Scale{
		TypeMeta:   metav1.TypeMeta{APIVersion: "autoscaling/v1", Kind: "Scale"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       autoscalingv1.ScaleSpec{Replicas: replicas},
		Status:     autoscalingv1.ScaleStatus{Replicas: replicas},
	}
}

// The non-workload resources (2026-09-19).

// writeResources answers one of the five list endpoints, filtered by kind and
// namespace.
//
// One function rather than five, because the fake's job here is to return the
// handful of fields the product reads -- and five near-identical renderers
// would drift from each other before they drifted from Kubernetes.
func (s *Server) writeResources(w http.ResponseWriter, kind, namespace string) {
	s.mu.Lock()
	rows := append([]Resource(nil), s.resources...)
	s.mu.Unlock()

	keep := make([]Resource, 0, len(rows))
	for _, r := range rows {
		if r.Kind != kind {
			continue
		}
		if namespace != "" && r.Namespace != namespace {
			continue
		}
		keep = append(keep, r)
	}

	switch kind {
	case "ConfigMap":
		list := corev1.ConfigMapList{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMapList"},
		}
		for _, r := range keep {
			data := map[string]string{}
			for _, k := range r.Keys {
				data[k] = "…"
			}
			list.Items = append(list.Items, corev1.ConfigMap{
				TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
				ObjectMeta: metav1.ObjectMeta{Name: r.Name, Namespace: r.Namespace},
				Data:       data,
			})
		}
		writeJSON(w, http.StatusOK, list)

	case "Secret":
		list := corev1.SecretList{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "SecretList"},
		}
		for _, r := range keep {
			data := map[string][]byte{}
			for _, k := range r.Keys {
				// A value the product must never carry out of here. It is set
				// so that a test can watch it NOT appear.
				data[k] = []byte("super-secret-value")
			}
			list.Items = append(list.Items, corev1.Secret{
				TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
				ObjectMeta: metav1.ObjectMeta{Name: r.Name, Namespace: r.Namespace},
				Type:       corev1.SecretTypeOpaque,
				Data:       data,
			})
		}
		writeJSON(w, http.StatusOK, list)

	case "PersistentVolumeClaim":
		list := corev1.PersistentVolumeClaimList{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "PersistentVolumeClaimList"},
		}
		for _, r := range keep {
			claim := corev1.PersistentVolumeClaim{
				TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "PersistentVolumeClaim"},
				ObjectMeta: metav1.ObjectMeta{Name: r.Name, Namespace: r.Namespace},
				Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.PersistentVolumeClaimPhase(r.Phase)},
			}
			if r.StorageClass != "" {
				class := r.StorageClass
				claim.Spec.StorageClassName = &class
			}
			claim.Spec.VolumeName = r.VolumeName
			claim.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
			if r.Size != "" && r.Phase == string(corev1.ClaimBound) {
				claim.Status.Capacity = corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(r.Size),
				}
			} else if r.Size != "" {
				claim.Spec.Resources.Requests = corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(r.Size),
				}
			}
			list.Items = append(list.Items, claim)
		}
		writeJSON(w, http.StatusOK, list)

	case "Ingress":
		list := networkingv1.IngressList{
			TypeMeta: metav1.TypeMeta{APIVersion: "networking.k8s.io/v1", Kind: "IngressList"},
		}
		for _, r := range keep {
			ing := networkingv1.Ingress{
				TypeMeta:   metav1.TypeMeta{APIVersion: "networking.k8s.io/v1", Kind: "Ingress"},
				ObjectMeta: metav1.ObjectMeta{Name: r.Name, Namespace: r.Namespace},
				Spec:       networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{Host: r.Host}}},
			}
			if r.Address != "" {
				ing.Status.LoadBalancer.Ingress = []networkingv1.IngressLoadBalancerIngress{
					{IP: r.Address},
				}
			}
			list.Items = append(list.Items, ing)
		}
		writeJSON(w, http.StatusOK, list)

	case "HorizontalPodAutoscaler":
		list := autoscalingv2.HorizontalPodAutoscalerList{
			TypeMeta: metav1.TypeMeta{APIVersion: "autoscaling/v2", Kind: "HorizontalPodAutoscalerList"},
		}
		for _, r := range keep {
			minimum := r.Min
			list.Items = append(list.Items, autoscalingv2.HorizontalPodAutoscaler{
				TypeMeta:   metav1.TypeMeta{APIVersion: "autoscaling/v2", Kind: "HorizontalPodAutoscaler"},
				ObjectMeta: metav1.ObjectMeta{Name: r.Name, Namespace: r.Namespace},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					MinReplicas: &minimum,
					MaxReplicas: r.Max,
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind: r.TargetKind, Name: r.TargetName,
					},
				},
				Status: autoscalingv2.HorizontalPodAutoscalerStatus{CurrentReplicas: r.Current},
			})
		}
		writeJSON(w, http.StatusOK, list)

	case "PodDisruptionBudget":
		list := policyv1.PodDisruptionBudgetList{
			TypeMeta: metav1.TypeMeta{APIVersion: "policy/v1", Kind: "PodDisruptionBudgetList"},
		}
		for _, r := range keep {
			list.Items = append(list.Items, policyv1.PodDisruptionBudget{
				TypeMeta:   metav1.TypeMeta{APIVersion: "policy/v1", Kind: "PodDisruptionBudget"},
				ObjectMeta: metav1.ObjectMeta{Name: r.Name, Namespace: r.Namespace},
				Status: policyv1.PodDisruptionBudgetStatus{
					CurrentHealthy:     r.Healthy,
					DesiredHealthy:     r.Desired,
					DisruptionsAllowed: r.Allowed,
				},
			})
		}
		writeJSON(w, http.StatusOK, list)

	default:
		s.serveNotFound(w, nil)
	}
}

// group is one entry in the discovery group list.
func group(name, groupVersion string) metav1.APIGroup {
	gv := metav1.GroupVersionForDiscovery{GroupVersion: groupVersion, Version: "v1"}
	if strings.HasSuffix(groupVersion, "/v2") {
		gv.Version = "v2"
	}
	return metav1.APIGroup{
		TypeMeta:         metav1.TypeMeta{Kind: "APIGroup", APIVersion: "v1"},
		Name:             name,
		Versions:         []metav1.GroupVersionForDiscovery{gv},
		PreferredVersion: gv,
	}
}

func (s *Server) writeGroupResources(w http.ResponseWriter, groupVersion string, rs ...metav1.APIResource) {
	writeJSON(w, http.StatusOK, metav1.APIResourceList{
		TypeMeta:     metav1.TypeMeta{Kind: "APIResourceList", APIVersion: "v1"},
		GroupVersion: groupVersion,
		APIResources: rs,
	})
}

// serveGroupNamespaced routes /apis/{group}/{version}/namespaces/{ns}/{resource}
// and the cluster-wide /apis/{group}/{version}/{resource}.
func (s *Server) serveGroupNamespaced(w http.ResponseWriter, r *http.Request) {
	kindOf := map[string]string{
		"ingresses":                "Ingress",
		"horizontalpodautoscalers": "HorizontalPodAutoscaler",
		"poddisruptionbudgets":     "PodDisruptionBudget",
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// apis/{group}/{version}/... -- three segments of prefix.
	if len(parts) < 4 {
		s.serveNotFound(w, r)
		return
	}
	rest := parts[3:]

	switch {
	case len(rest) == 1:
		if kind, ok := kindOf[rest[0]]; ok {
			s.writeResources(w, kind, "")
			return
		}
	case len(rest) == 3 && rest[0] == "namespaces":
		if kind, ok := kindOf[rest[2]]; ok {
			s.writeResources(w, kind, rest[1])
			return
		}
	case len(rest) == 4 && rest[0] == "namespaces" && r.Method == http.MethodDelete:
		if _, ok := kindOf[rest[2]]; ok {
			s.mu.Lock()
			s.deleted = append(s.deleted, rest[2]+"/"+rest[1]+"/"+rest[3])
			s.mu.Unlock()
			writeJSON(w, http.StatusOK, metav1.Status{Status: metav1.StatusSuccess})
			return
		}
	}
	s.serveNotFound(w, r)
}

// taintsOf parses "key=value:Effect", or "key:Effect" when there is no value.
func taintsOf(specs []string) []corev1.Taint {
	out := make([]corev1.Taint, 0, len(specs))
	for _, spec := range specs {
		head, effect, ok := strings.Cut(spec, ":")
		if !ok {
			continue
		}
		key, value, _ := strings.Cut(head, "=")
		out = append(out, corev1.Taint{
			Key: key, Value: value, Effect: corev1.TaintEffect(effect),
		})
	}
	return out
}

// conditionsOf parses "Type=Status,Reason".
func conditionsOf(specs []string) []corev1.NodeCondition {
	out := make([]corev1.NodeCondition, 0, len(specs))
	for _, spec := range specs {
		head, reason, _ := strings.Cut(spec, ",")
		name, status, ok := strings.Cut(head, "=")
		if !ok {
			continue
		}
		out = append(out, corev1.NodeCondition{
			Type:    corev1.NodeConditionType(name),
			Status:  corev1.ConditionStatus(status),
			Reason:  reason,
			Message: reason,
		})
	}
	return out
}

func allocatableOf(n Node) corev1.ResourceList {
	out := corev1.ResourceList{}
	if n.CPUAllocatable != "" {
		out[corev1.ResourceCPU] = resource.MustParse(n.CPUAllocatable)
	}
	if n.MemoryAllocatable != "" {
		out[corev1.ResourceMemory] = resource.MustParse(n.MemoryAllocatable)
	}
	if n.PodCapacity != "" {
		out[corev1.ResourcePods] = resource.MustParse(n.PodCapacity)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// writeUsage answers the metrics API, or says it is not installed.
//
// A nil Usage map answers 404 for the whole group, which is what a cluster
// without metrics-server does -- and that is the default, because most clusters
// are that cluster. A product that assumed the numbers exist therefore fails in
// the ordinary test rather than on somebody's hardware.
func (s *Server) writeUsage(w http.ResponseWriter, kind, namespace string) {
	s.mu.Lock()
	usage := s.usage
	s.mu.Unlock()

	if usage == nil {
		s.writeStatus(w, http.StatusNotFound, metav1.StatusReasonNotFound,
			"the server could not find the requested resource")
		return
	}

	type container struct {
		Name  string            `json:"name"`
		Usage map[string]string `json:"usage"`
	}
	type item struct {
		Metadata   metav1.ObjectMeta `json:"metadata"`
		Usage      map[string]string `json:"usage,omitempty"`
		Containers []container       `json:"containers,omitempty"`
	}
	out := struct {
		Kind  string `json:"kind"`
		Items []item `json:"items"`
	}{Kind: "NodeMetricsList"}
	if kind == "pod" {
		out.Kind = "PodMetricsList"
	}

	for key, value := range usage {
		parts := strings.Split(key, "/")
		if parts[0] != kind {
			continue
		}
		cpu, memory, _ := strings.Cut(value, ",")

		switch kind {
		case "node":
			if len(parts) != 2 {
				continue
			}
			out.Items = append(out.Items, item{
				Metadata: metav1.ObjectMeta{Name: parts[1]},
				Usage:    map[string]string{"cpu": cpu, "memory": memory},
			})
		case "pod":
			if len(parts) != 3 {
				continue
			}
			if namespace != "" && parts[1] != namespace {
				continue
			}
			// Two containers, each with half, so a product that reads only the
			// first understates the pod -- which is the mistake worth catching.
			out.Items = append(out.Items, item{
				Metadata: metav1.ObjectMeta{Namespace: parts[1], Name: parts[2]},
				Containers: []container{
					{Name: "a", Usage: map[string]string{"cpu": halve(cpu), "memory": halve(memory)}},
					{Name: "b", Usage: map[string]string{"cpu": halve(cpu), "memory": halve(memory)}},
				},
			})
		}
	}

	// Sorted, because map iteration is random and a list that reordered itself
	// between two reads would look like the cluster changed.
	slices.SortFunc(out.Items, func(a, b item) int {
		return strings.Compare(a.Metadata.Namespace+"/"+a.Metadata.Name,
			b.Metadata.Namespace+"/"+b.Metadata.Name)
	})
	writeJSON(w, http.StatusOK, out)
}

// halve splits a quantity in two, so a pod's two containers add back up to what
// a test asked for.
func halve(value string) string {
	q, err := resource.ParseQuantity(value)
	if err != nil {
		return value
	}
	if q.Format == resource.BinarySI {
		return resource.NewQuantity(q.Value()/2, resource.BinarySI).String()
	}
	return resource.NewMilliQuantity(q.MilliValue()/2, resource.DecimalSI).String()
}

// Stopping, starting and sweeping (2026-09-20).

// annotationsFor returns the annotations a test's object carries, including the
// ones the product wrote. They are stored per object so a stop that wrote a
// count can be read back by the start -- a fake that dropped them would let
// "start restores what was running" pass while doing nothing.
func (s *Server) annotationsFor(kind, namespace, name string) map[string]string {
	return s.annotations[kind+"/"+namespace+"/"+name]
}

func (s *Server) writeDeployment(w http.ResponseWriter, namespace, name string) {
	s.mu.Lock()
	var found *Deployment
	for i := range s.deployments {
		if s.deployments[i].Namespace == namespace && s.deployments[i].Name == name {
			found = &s.deployments[i]
			break
		}
	}
	annotations := s.annotationsFor("deployment", namespace, name)
	s.mu.Unlock()

	if found == nil {
		s.writeStatus(w, http.StatusNotFound, metav1.StatusReasonNotFound,
			fmt.Sprintf("deployments %q not found", name))
		return
	}
	object := renderDeployment(*found)
	object.Annotations = annotations
	writeJSON(w, http.StatusOK, object)
}

func (s *Server) writeStatefulSet(w http.ResponseWriter, namespace, name string) {
	s.mu.Lock()
	var found *StatefulSet
	for i := range s.statefulSets {
		if s.statefulSets[i].Namespace == namespace && s.statefulSets[i].Name == name {
			found = &s.statefulSets[i]
			break
		}
	}
	annotations := s.annotationsFor("statefulset", namespace, name)
	s.mu.Unlock()

	if found == nil {
		s.writeStatus(w, http.StatusNotFound, metav1.StatusReasonNotFound,
			fmt.Sprintf("statefulsets %q not found", name))
		return
	}
	desired := found.Desired
	writeJSON(w, http.StatusOK, appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "StatefulSet"},
		ObjectMeta: metav1.ObjectMeta{
			Name: found.Name, Namespace: found.Namespace, Annotations: annotations,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &desired, Template: podTemplate(found.Name, found.Image),
			Selector: appSelector(found.Name),
		},
		Status: appsv1.StatefulSetStatus{ReadyReplicas: found.Ready, UpdatedReplicas: found.Updated},
	})
}

func (s *Server) writeCronJob(w http.ResponseWriter, namespace, name string) {
	s.mu.Lock()
	var found *CronJob
	for i := range s.cronJobs {
		if s.cronJobs[i].Namespace == namespace && s.cronJobs[i].Name == name {
			found = &s.cronJobs[i]
			break
		}
	}
	s.mu.Unlock()

	if found == nil {
		s.writeStatus(w, http.StatusNotFound, metav1.StatusReasonNotFound,
			fmt.Sprintf("cronjobs %q not found", name))
		return
	}
	suspend := found.Suspended
	s.mu.Lock()
	annotations := s.annotationsFor("cronjob", namespace, name)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, batchv1.CronJob{
		TypeMeta: metav1.TypeMeta{APIVersion: "batch/v1", Kind: "CronJob"},
		ObjectMeta: metav1.ObjectMeta{
			Name: found.Name, Namespace: found.Namespace, Annotations: annotations,
		},
		Spec: batchv1.CronJobSpec{Schedule: found.Schedule, Suspend: &suspend},
	})
}

func (s *Server) writeReplicaSets(w http.ResponseWriter, namespace string) {
	s.mu.Lock()
	rows := append([]ReplicaSet(nil), s.replicaSets...)
	s.mu.Unlock()

	list := appsv1.ReplicaSetList{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "ReplicaSetList"},
	}
	controller := true
	for _, r := range rows {
		if namespace != "" && r.Namespace != namespace {
			continue
		}
		replicas := r.Replicas
		list.Items = append(list.Items, appsv1.ReplicaSet{
			TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "ReplicaSet"},
			ObjectMeta: metav1.ObjectMeta{
				Name: r.Name, Namespace: r.Namespace,
				CreationTimestamp: metav1.NewTime(time.Now().Add(-r.Age).UTC()),
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "apps/v1", Kind: "Deployment", Name: r.Owner,
					Controller: &controller,
				}},
			},
			Spec:   appsv1.ReplicaSetSpec{Replicas: &replicas},
			Status: appsv1.ReplicaSetStatus{Replicas: replicas},
		})
	}
	writeJSON(w, http.StatusOK, list)
}

// recordDelete removes nothing from the fake's own state and records that the
// call was made, which is what a sweep test checks.
func (s *Server) recordDelete(w http.ResponseWriter, resource, namespace, name string) {
	s.mu.Lock()
	s.deleted = append(s.deleted, resource+"/"+namespace+"/"+name)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, metav1.Status{Status: metav1.StatusSuccess})
}

// patchSuspend applies the suspend patch a stop sends to a CronJob or Job, and
// CHANGES the state -- so a test reads back what a stop did rather than that a
// call happened.
func (s *Server) patchSuspend(w http.ResponseWriter, r *http.Request, kind, namespace, name string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err != nil {
		s.writeStatus(w, http.StatusBadRequest, metav1.StatusReasonBadRequest, err.Error())
		return
	}
	var patch struct {
		Metadata struct {
			Annotations map[string]*string `json:"annotations"`
		} `json:"metadata"`
		Spec struct {
			Suspend *bool `json:"suspend"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(body, &patch); err != nil ||
		(patch.Spec.Suspend == nil && len(patch.Metadata.Annotations) == 0) {
		s.writeStatus(w, http.StatusBadRequest, metav1.StatusReasonBadRequest,
			"kubesim applies only patches that set spec.suspend or metadata.annotations here")
		return
	}
	s.storeAnnotations(kind, namespace, name, patch.Metadata.Annotations)
	if patch.Spec.Suspend == nil {
		writeJSON(w, http.StatusOK, metav1.ObjectMeta{Name: name, Namespace: namespace})
		return
	}

	s.mu.Lock()
	if kind == "cronjob" {
		for i := range s.cronJobs {
			if s.cronJobs[i].Namespace == namespace && s.cronJobs[i].Name == name {
				s.cronJobs[i].Suspended = *patch.Spec.Suspend
			}
		}
	} else {
		for i := range s.jobs {
			if s.jobs[i].Namespace == namespace && s.jobs[i].Name == name {
				s.jobs[i].Suspended = *patch.Spec.Suspend
			}
		}
	}
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, metav1.ObjectMeta{Name: name, Namespace: namespace})
}

// Storage and networking (2026-09-20).
//
// The claim worth building a fake for is the derived one: a Service with
// endpoints none of which are ready, a volume in Released holding data nothing
// claims, a policy whose selector matches no pod. None of those can be measured
// against a fake that only stores what it is told -- they are computed from two
// lists disagreeing, which is exactly what this renders.

func (s *Server) writeVolumes(w http.ResponseWriter) {
	s.mu.Lock()
	volumes := append([]Volume(nil), s.volumes...)
	s.mu.Unlock()

	list := corev1.PersistentVolumeList{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "PersistentVolumeList"},
	}
	for _, v := range volumes {
		rendered := corev1.PersistentVolume{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "PersistentVolume"},
			ObjectMeta: metav1.ObjectMeta{Name: v.Name},
			Spec: corev1.PersistentVolumeSpec{
				StorageClassName:              v.StorageClass,
				PersistentVolumeReclaimPolicy: v.ReclaimPolicy,
				AccessModes:                   v.AccessModes,
			},
			Status: corev1.PersistentVolumeStatus{Phase: v.Phase},
		}
		if v.Capacity != "" {
			rendered.Spec.Capacity = corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(v.Capacity),
			}
		}
		if namespace, name, ok := strings.Cut(v.Claim, "/"); ok {
			rendered.Spec.ClaimRef = &corev1.ObjectReference{Namespace: namespace, Name: name}
		}
		switch {
		case v.CSIDriver != "":
			rendered.Spec.CSI = &corev1.CSIPersistentVolumeSource{Driver: v.CSIDriver}
		case v.LocalPath != "":
			rendered.Spec.Local = &corev1.LocalVolumeSource{Path: v.LocalPath}
		}
		list.Items = append(list.Items, rendered)
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) writeStorageClasses(w http.ResponseWriter) {
	s.mu.Lock()
	classes := append([]StorageClass(nil), s.classes...)
	s.mu.Unlock()

	list := storagev1.StorageClassList{
		TypeMeta: metav1.TypeMeta{APIVersion: "storage.k8s.io/v1", Kind: "StorageClassList"},
	}
	for _, c := range classes {
		rendered := storagev1.StorageClass{
			TypeMeta:    metav1.TypeMeta{APIVersion: "storage.k8s.io/v1", Kind: "StorageClass"},
			ObjectMeta:  metav1.ObjectMeta{Name: c.Name},
			Provisioner: c.Provisioner,
		}
		if c.Default {
			rendered.Annotations = map[string]string{
				"storageclass.kubernetes.io/is-default-class": "true",
			}
		}
		if c.ReclaimPolicy != "" {
			policy := c.ReclaimPolicy
			rendered.ReclaimPolicy = &policy
		}
		mode := storagev1.VolumeBindingImmediate
		if c.WaitForConsumer {
			mode = storagev1.VolumeBindingWaitForFirstConsumer
		}
		rendered.VolumeBindingMode = &mode
		expansion := c.AllowsExpansion
		rendered.AllowVolumeExpansion = &expansion
		list.Items = append(list.Items, rendered)
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) writeEndpointSlices(w http.ResponseWriter, namespace string) {
	s.mu.Lock()
	slices := append([]EndpointSlice(nil), s.slices...)
	s.mu.Unlock()

	list := discoveryv1.EndpointSliceList{
		TypeMeta: metav1.TypeMeta{APIVersion: "discovery.k8s.io/v1", Kind: "EndpointSliceList"},
	}
	for _, slice := range slices {
		if namespace != "" && slice.Namespace != namespace {
			continue
		}
		rendered := discoveryv1.EndpointSlice{
			TypeMeta: metav1.TypeMeta{APIVersion: "discovery.k8s.io/v1", Kind: "EndpointSlice"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      slice.Service + "-abc",
				Namespace: slice.Namespace,
				Labels:    map[string]string{discoveryv1.LabelServiceName: slice.Service},
			},
			AddressType: discoveryv1.AddressTypeIPv4,
		}
		for i := range slice.Addresses {
			// Ready is a POINTER in the API and absent means ready, so the fake
			// sets it explicitly both ways: a test about "none are ready" must
			// not pass because the field was left nil.
			ready := i >= slice.NotReady
			rendered.Endpoints = append(rendered.Endpoints, discoveryv1.Endpoint{
				Addresses:  []string{fmt.Sprintf("10.244.1.%d", i+10)},
				Conditions: discoveryv1.EndpointConditions{Ready: &ready},
			})
		}
		list.Items = append(list.Items, rendered)
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) writeNetworkPolicies(w http.ResponseWriter, namespace string) {
	s.mu.Lock()
	policies := append([]NetworkPolicy(nil), s.policies...)
	s.mu.Unlock()

	list := networkingv1.NetworkPolicyList{
		TypeMeta: metav1.TypeMeta{APIVersion: "networking.k8s.io/v1", Kind: "NetworkPolicyList"},
	}
	for _, p := range policies {
		if namespace != "" && p.Namespace != namespace {
			continue
		}
		rendered := networkingv1.NetworkPolicy{
			TypeMeta:   metav1.TypeMeta{APIVersion: "networking.k8s.io/v1", Kind: "NetworkPolicy"},
			ObjectMeta: metav1.ObjectMeta{Name: p.Name, Namespace: p.Namespace},
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{MatchLabels: p.Selector},
				PolicyTypes: p.Types,
			},
		}
		if p.HasEgress {
			rendered.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{{}}
		}
		list.Items = append(list.Items, rendered)
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) writeIngressClasses(w http.ResponseWriter) {
	s.mu.Lock()
	classes := append([]IngressClass(nil), s.ingclasses...)
	s.mu.Unlock()

	list := networkingv1.IngressClassList{
		TypeMeta: metav1.TypeMeta{APIVersion: "networking.k8s.io/v1", Kind: "IngressClassList"},
	}
	for _, c := range classes {
		rendered := networkingv1.IngressClass{
			TypeMeta:   metav1.TypeMeta{APIVersion: "networking.k8s.io/v1", Kind: "IngressClass"},
			ObjectMeta: metav1.ObjectMeta{Name: c.Name},
			Spec:       networkingv1.IngressClassSpec{Controller: c.Controller},
		}
		if c.Default {
			rendered.Annotations = map[string]string{
				"ingressclass.kubernetes.io/is-default-class": "true",
			}
		}
		list.Items = append(list.Items, rendered)
	}
	writeJSON(w, http.StatusOK, list)
}

// serveDiscoveryNamespaced answers the namespaced endpointslice list.
func (s *Server) serveDiscoveryNamespaced(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/apis/discovery.k8s.io/v1/namespaces/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) == 2 && parts[1] == "endpointslices" {
		s.writeEndpointSlices(w, parts[0])
		return
	}
	s.writeStatus(w, http.StatusNotFound, metav1.StatusReasonNotFound, "not found: "+r.URL.Path)
}

// Access control (2026-09-20).
//
// The cases worth rendering are the ones where two objects disagree: a binding
// naming a role that is not there, a binding naming a service account that was
// never created. Both grant nothing and look exactly like ones that work, because
// RBAC has no referential integrity -- so a fake that refused to store them could
// not measure the finding at all.

func (s *Server) writeRoles(w http.ResponseWriter, namespace string, cluster bool) {
	s.mu.Lock()
	roles := append([]Role(nil), s.roles...)
	s.mu.Unlock()

	rule := func(r Role) rbacv1.PolicyRule {
		return rbacv1.PolicyRule{
			Verbs:           r.Verbs,
			APIGroups:       r.APIGroups,
			Resources:       r.Resources,
			NonResourceURLs: r.NonResourceURLs,
		}
	}
	labelsFor := func(r Role) map[string]string {
		if !r.BuiltIn {
			return nil
		}
		return map[string]string{"kubernetes.io/bootstrapping": "rbac-defaults"}
	}

	if cluster {
		list := rbacv1.ClusterRoleList{
			TypeMeta: metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRoleList"},
		}
		for _, r := range roles {
			if r.Namespace != "" {
				continue
			}
			list.Items = append(list.Items, rbacv1.ClusterRole{
				TypeMeta:   metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRole"},
				ObjectMeta: metav1.ObjectMeta{Name: r.Name, Labels: labelsFor(r)},
				Rules:      []rbacv1.PolicyRule{rule(r)},
			})
		}
		writeJSON(w, http.StatusOK, list)
		return
	}

	list := rbacv1.RoleList{
		TypeMeta: metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleList"},
	}
	for _, r := range roles {
		if r.Namespace == "" || (namespace != "" && r.Namespace != namespace) {
			continue
		}
		list.Items = append(list.Items, rbacv1.Role{
			TypeMeta:   metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role"},
			ObjectMeta: metav1.ObjectMeta{Name: r.Name, Namespace: r.Namespace, Labels: labelsFor(r)},
			Rules:      []rbacv1.PolicyRule{rule(r)},
		})
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) writeBindings(w http.ResponseWriter, namespace string, cluster bool) {
	s.mu.Lock()
	bindings := append([]RoleBinding(nil), s.bindings...)
	s.mu.Unlock()

	subjectsOf := func(b RoleBinding) []rbacv1.Subject {
		out := make([]rbacv1.Subject, 0, len(b.Subjects))
		for _, subject := range b.Subjects {
			out = append(out, rbacv1.Subject{
				Kind: subject.Kind, Namespace: subject.Namespace, Name: subject.Name,
			})
		}
		return out
	}
	refOf := func(b RoleBinding) rbacv1.RoleRef {
		return rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName, Kind: b.RoleKind, Name: b.RoleName,
		}
	}

	if cluster {
		list := rbacv1.ClusterRoleBindingList{
			TypeMeta: metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRoleBindingList"},
		}
		for _, b := range bindings {
			if b.Namespace != "" {
				continue
			}
			list.Items = append(list.Items, rbacv1.ClusterRoleBinding{
				TypeMeta:   metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRoleBinding"},
				ObjectMeta: metav1.ObjectMeta{Name: b.Name},
				RoleRef:    refOf(b), Subjects: subjectsOf(b),
			})
		}
		writeJSON(w, http.StatusOK, list)
		return
	}

	list := rbacv1.RoleBindingList{
		TypeMeta: metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBindingList"},
	}
	for _, b := range bindings {
		if b.Namespace == "" || (namespace != "" && b.Namespace != namespace) {
			continue
		}
		list.Items = append(list.Items, rbacv1.RoleBinding{
			TypeMeta:   metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding"},
			ObjectMeta: metav1.ObjectMeta{Name: b.Name, Namespace: b.Namespace},
			RoleRef:    refOf(b), Subjects: subjectsOf(b),
		})
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) writeServiceAccounts(w http.ResponseWriter, namespace string) {
	s.mu.Lock()
	accounts := append([]ServiceAccount(nil), s.accounts...)
	s.mu.Unlock()

	list := corev1.ServiceAccountList{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ServiceAccountList"},
	}
	for _, a := range accounts {
		if namespace != "" && a.Namespace != namespace {
			continue
		}
		list.Items = append(list.Items, corev1.ServiceAccount{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ServiceAccount"},
			ObjectMeta: metav1.ObjectMeta{Name: a.Name, Namespace: a.Namespace},
		})
	}
	writeJSON(w, http.StatusOK, list)
}

// serveRBACNamespaced answers the namespaced role and binding lists.
func (s *Server) serveRBACNamespaced(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/apis/rbac.authorization.k8s.io/v1/namespaces/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) == 2 {
		switch parts[1] {
		case "roles":
			s.writeRoles(w, parts[0], false)
			return
		case "rolebindings":
			s.writeBindings(w, parts[0], false)
			return
		}
	}
	s.writeStatus(w, http.StatusNotFound, metav1.StatusReasonNotFound, "not found: "+r.URL.Path)
}

// Quotas, limit ranges and the cluster's own kinds (2026-09-20).

func (s *Server) writeQuotas(w http.ResponseWriter, namespace string) {
	s.mu.Lock()
	quotas := append([]Quota(nil), s.quotas...)
	s.mu.Unlock()

	list := corev1.ResourceQuotaList{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ResourceQuotaList"},
	}
	asList := func(values map[string]string) corev1.ResourceList {
		out := corev1.ResourceList{}
		for name, value := range values {
			out[corev1.ResourceName(name)] = resource.MustParse(value)
		}
		return out
	}
	for _, q := range quotas {
		if namespace != "" && q.Namespace != namespace {
			continue
		}
		list.Items = append(list.Items, corev1.ResourceQuota{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ResourceQuota"},
			ObjectMeta: metav1.ObjectMeta{Name: q.Name, Namespace: q.Namespace},
			// On STATUS rather than spec, because that is where the API server
			// reports what is actually in force and what is used -- a product
			// reading spec would report a quota as empty the moment somebody
			// edited it.
			Status: corev1.ResourceQuotaStatus{Hard: asList(q.Hard), Used: asList(q.Used)},
		})
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) writeLimitRanges(w http.ResponseWriter, namespace string) {
	s.mu.Lock()
	ranges := append([]LimitRange(nil), s.limitranges...)
	s.mu.Unlock()

	list := corev1.LimitRangeList{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "LimitRangeList"},
	}
	for _, r := range ranges {
		if namespace != "" && r.Namespace != namespace {
			continue
		}
		list.Items = append(list.Items, corev1.LimitRange{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "LimitRange"},
			ObjectMeta: metav1.ObjectMeta{Name: r.Name, Namespace: r.Namespace},
		})
	}
	writeJSON(w, http.StatusOK, list)
}

// writeCRDs answers the cluster's own kinds, as unstructured objects.
//
// Unstructured rather than typed, because that is how the product reads them --
// through the dynamic client it already carries for the manifest path. A fake that
// answered a typed list would be testing a code path the product does not use.
func (s *Server) writeCRDs(w http.ResponseWriter) {
	s.mu.Lock()
	crds := append([]CustomResourceDefinition(nil), s.crds...)
	s.mu.Unlock()

	items := make([]any, 0, len(crds))
	for _, crd := range crds {
		versions := make([]any, 0, len(crd.Versions)+len(crd.NotServed))
		for _, version := range crd.Versions {
			versions = append(versions, map[string]any{
				"name": version, "served": true, "storage": version == crd.Stored,
			})
		}
		for _, version := range crd.NotServed {
			versions = append(versions, map[string]any{
				"name": version, "served": false, "storage": false,
			})
		}
		conditions := []any{}
		if crd.Established {
			conditions = append(conditions, map[string]any{
				"type": "Established", "status": "True",
			})
		}
		plural := crd.Plural
		if plural == "" {
			plural = strings.ToLower(crd.Kind) + "s"
		}
		items = append(items, map[string]any{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]any{
				"name":              plural + "." + crd.Group,
				"creationTimestamp": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
			},
			"spec": map[string]any{
				"group":    crd.Group,
				"scope":    crd.Scope,
				"names":    map[string]any{"kind": crd.Kind, "plural": plural},
				"versions": versions,
			},
			"status": map[string]any{"conditions": conditions},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"apiVersion": "apiextensions.k8s.io/v1",
		"kind":       "CustomResourceDefinitionList",
		"metadata":   map[string]any{"resourceVersion": "1"},
		"items":      items,
	})
}
