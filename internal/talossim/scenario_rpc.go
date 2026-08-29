package talossim

// The scenarios whose fault lives in RPC semantics.
//
// These four differ from the connection-shaped ones in that the call arrives,
// is answered, and the answer is wrong in a specific, documented way. That
// makes them the easier half to fake badly: any status code can be returned
// from anywhere. What keeps them honest is that each reproduces the shape the
// research recorded rather than a convenient approximation -- an apply is
// refused with InvalidArgument because that is what Talos does with a config it
// will not take (PITFALLS.md P11: "the node is unaffected"; Talos does not
// brick on invalid config), and etcd_down is deliberately asymmetric because
// "the node is gone" and "one subsystem is gone" are different facts a client
// has to be able to tell apart.

import (
	"context"
	"fmt"
	"strings"

	"github.com/cosi-project/runtime/pkg/state"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/siderolabs/talos/pkg/machinery/resources/k8s"
)

// rejectApplyMessage is shaped like a real Talos validation rejection.
//
// The wording matters more than it looks: the expected client behaviour is
// that the upstream message survives to the operator, so a placeholder here
// would make plan 02-05's assertion about carrying it through vacuous.
const rejectApplyMessage = "configuration validation failed: " +
	"machine.install.disk: \"/dev/nonexistent\" is not a block device on this machine"

// rpcScenarioGate answers a call the way a scenario says to, instead of
// letting the handler answer it.
//
// It is keyed on the method name rather than implemented inside each handler
// for the reason the package's single-accessor rule exists: a fault that has to
// be remembered in nine places is a fault that will be forgotten in one.
func (s *Server) rpcScenarioGate(fullMethod string) error {
	method := shortMethod(fullMethod)

	if _, ok := s.activeScenario(ScenarioRejectApply); ok && method == "ApplyConfiguration" {
		return status.Error(codes.InvalidArgument, "talossim: "+rejectApplyMessage)
	}

	// Every Etcd* RPC and nothing else. Version, Hostname and ServiceList keep
	// answering, and that asymmetry is the scenario: a client that opened its
	// breaker here would be treating a reachable node as an unreachable one.
	if _, ok := s.activeScenario(ScenarioEtcdDown); ok && strings.HasPrefix(method, "Etcd") {
		return status.Errorf(codes.Unavailable,
			"talossim: etcd is not serving on %s: connection refused on 127.0.0.1:2379",
			s.node.snapshot().Hostname)
	}

	return nil
}

// startSecondBootstrapAlreadyExists puts the node into the state that makes
// the client's next Bootstrap a second one.
//
// The scenario TRANS-07 names is a Bootstrap that returns AlreadyExists, and
// the case that produces it on hardware is a node that was already bootstrapped
// before this client arrived -- most sharply, a Bootstrap whose success was
// committed and whose reply was lost, which a client that retried would then
// see refused. Injection therefore performs the first bootstrap itself. The
// refusal it causes is the node's ordinary, uninjected behaviour (see
// nodeState.bootstrap): the scenario supplies the precondition, not a special
// case, so a client that reconnects still sees the second call rejected.
func (s *Server) startSecondBootstrapAlreadyExists(_ Scenario) (func(), error) {
	before := s.node.snapshot().Bootstrapped
	s.node.setBootstrapped(true)

	return func() { s.node.setBootstrapped(before) }, nil
}

// startK8sDown removes the node's Kubernetes resources from COSI.
//
// Removal rather than an error on the read is what makes the failure
// attributable: a COSI Get that returns a transport error says the node is
// unreachable, and a not-found says Kubernetes is not running on a node that
// is answering fine. Those are different problems with different fixes, and a
// simulator that conflated them would train the product to conflate them too.
func (s *Server) startK8sDown(_ Scenario) (func(), error) {
	ctx := context.Background()

	md := k8s.NewNodename(k8s.NamespaceName, k8s.NodenameID).Metadata()
	if err := s.cosi.Destroy(ctx, md); err != nil && !state.IsNotFoundError(err) {
		return nil, fmt.Errorf("talossim: k8s_down: remove %s: %w", k8s.NodenameType, err)
	}

	return func() {
		// Re-seeded from scratch rather than from the value that was removed:
		// a resource carries a version, and putting a destroyed one back would
		// either be refused or would resurrect a version COSI had retired.
		_ = s.seedKubernetes(context.Background())
	}, nil
}

// kubernetesIsDown reports whether the k8s_down scenario is active.
//
// ServiceList consults it so that the service view and the resource view agree:
// a node whose Kubernetes resources have vanished while its kubelet still
// reports Running would be a state no real node reaches, and the product would
// have no way to notice the difference.
func (s *Server) kubernetesIsDown() bool {
	_, ok := s.activeScenario(ScenarioK8sDown)
	return ok
}
