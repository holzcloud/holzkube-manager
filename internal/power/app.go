package power

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/model"
	"github.com/holzcloud/holzkube-manager/internal/store"
)

// Apps: the Kubernetes half of the power model.
//
// An app is addressed by {namespace, kind, name}. The kinds are the five
// workload kinds this product already knows plus a bare pod, which is an app of
// its own because nothing else owns it -- and which has only a stop, because a
// pod nothing owns cannot be brought back once it is gone.
//
// App actions are synchronous and answer with a sentence rather than a job.
// Every one of them is one or two API calls -- a scale, a patch, a handful of
// deletes -- and what happens afterwards is the controller's business and
// visible on the workload itself. A job whose only step is "the API server
// accepted a patch" would be a progress screen for something that is already
// over.

// AppKind is which kind of app a path names.
type AppKind string

// The six.
const (
	KindDeployment  = AppKind(kube.KindDeployment)
	KindStatefulSet = AppKind(kube.KindStatefulSet)
	KindDaemonSet   = AppKind(kube.KindDaemonSet)
	KindJob         = AppKind(kube.KindJob)
	KindCronJob     = AppKind(kube.KindCronJob)
	KindPod         = AppKind("Pod")
)

// AppKinds is every kind an app can be.
func AppKinds() []AppKind {
	return []AppKind{KindDeployment, KindStatefulSet, KindDaemonSet, KindJob, KindCronJob, KindPod}
}

// ParseAppKind reads a kind from a path segment, in any case: "deployment" in
// a hand-typed URL means the same thing as "Deployment" in a generated one.
func ParseAppKind(s string) (AppKind, bool) {
	for _, k := range AppKinds() {
		if strings.EqualFold(s, string(k)) {
			return k, true
		}
	}
	return "", false
}

// AppRef names one app.
type AppRef struct {
	Namespace string
	Kind      AppKind
	Name      string
}

func (r AppRef) workload() kube.WorkloadKind { return kube.WorkloadKind(r.Kind) }

// ErrUnknownAppKind reports a path naming a kind that is not one of the six.
var ErrUnknownAppKind = errors.New("power: that is not a kind of app")

// AppReport answers "what can I do to this app, and why not".
func (s *Service) AppReport(ctx context.Context, cluster model.ClusterID, ref AppRef) (Report, error) {
	client, err := s.deps.Kube(ctx, cluster)
	if err != nil {
		return Report{}, err
	}
	facts, err := s.appFacts(ctx, cluster, client, ref)
	if err != nil {
		return Report{}, err
	}
	return appReport(facts), nil
}

func (s *Service) appFacts(ctx context.Context, cluster model.ClusterID, client *kube.Client, ref AppRef) (appFacts, error) {
	facts := appFacts{Kind: ref.Kind}
	c, err := s.deps.Store.Clusters().Get(ctx, cluster)
	switch {
	case err == nil:
		facts.Locked = c.Locked
	case errors.Is(err, store.ErrNotFound):
		return appFacts{}, fmt.Errorf("%w: cluster %s", ErrNotFound, cluster)
	default:
		return appFacts{}, err
	}

	if ref.Kind == KindPod {
		facts.Pod, err = client.PodPowerOf(ctx, ref.Namespace, ref.Name)
		return facts, err
	}
	facts.Power, err = client.AppPowerOf(ctx, ref.workload(), ref.Namespace, ref.Name)
	return facts, err
}

// RunApp performs one app action and says, in one sentence, what happened.
//
// The same rules as the report decide first, so that a button the screen drew
// as unavailable is refused with the sentence it showed -- and a button it drew
// as available is not refused for a reason the screen never mentioned.
func (s *Service) RunApp(ctx context.Context, cluster model.ClusterID, ref AppRef, a Action) (string, error) {
	client, err := s.deps.Kube(ctx, cluster)
	if err != nil {
		return "", err
	}
	facts, err := s.appFacts(ctx, cluster, client, ref)
	if err != nil {
		return "", err
	}
	if ok, why := appReport(facts).Available(a); !ok {
		return "", &UnavailableError{Action: a, Reason: why}
	}

	if ref.Kind == KindPod {
		// Stop and force-stop are all a bare pod has; the rules refused the
		// rest.
		if err := client.DeleteBarePod(ctx, ref.Namespace, ref.Name, a == ForceStop); err != nil {
			return "", err
		}
		if a == ForceStop {
			return fmt.Sprintf("Deleted %s without waiting for it to finish; a bare pod cannot be started again.", ref.Name), nil
		}
		return fmt.Sprintf("Deleted %s; a bare pod cannot be started again.", ref.Name), nil
	}

	kind := ref.workload()
	switch a {
	case Stop:
		if err := client.Stop(ctx, kind, ref.Namespace, ref.Name); err != nil {
			return "", err
		}
		return stoppedSentence(ref, facts.Power), nil

	case ForceStop:
		if err := client.Stop(ctx, kind, ref.Namespace, ref.Name); err != nil {
			return "", err
		}
		// After the stop, so the controller is no longer making replacements
		// for the pods this kills.
		n, err := client.ForceDeleteAppPods(ctx, kind, ref.Namespace, ref.Name)
		if err != nil {
			return "", err
		}
		return strings.TrimSuffix(stoppedSentence(ref, facts.Power), ".") +
			fmt.Sprintf(", and killed %s without waiting for %s to finish.", pods(n), them(n)), nil

	case Start:
		if err := client.Start(ctx, kind, ref.Namespace, ref.Name); err != nil {
			return "", err
		}
		return startedSentence(ref, facts.Power), nil

	case Disable:
		// The mark first: a stop that fails after it leaves an app marked
		// "stays off" that is still running, which the report then shows as
		// disabled with stop available. The other order would leave an app
		// stopped and unmarked, which the next start brings straight back.
		if err := client.SetAppDisabled(ctx, kind, ref.Namespace, ref.Name, true); err != nil {
			return "", err
		}
		if !facts.Power.Stopped && !facts.Power.Finished {
			if err := client.Stop(ctx, kind, ref.Namespace, ref.Name); err != nil {
				return "", err
			}
		}
		return fmt.Sprintf("Disabled %s: it is stopped and stays stopped until it is enabled.", ref.Name), nil

	case Enable:
		if err := client.SetAppDisabled(ctx, kind, ref.Namespace, ref.Name, false); err != nil {
			return "", err
		}
		if !facts.Power.Stopped {
			return fmt.Sprintf("Enabled %s; it was not stopped, so nothing had to start.", ref.Name), nil
		}
		if err := client.Start(ctx, kind, ref.Namespace, ref.Name); err != nil {
			return "", err
		}
		return fmt.Sprintf("Enabled %s and started it again.", ref.Name), nil

	case Restart:
		if err := client.RolloutRestartWorkload(ctx, kind, ref.Namespace, ref.Name, time.Now()); err != nil {
			return "", err
		}
		return fmt.Sprintf("Restarting %s: its pods are being replaced one at a time, under its own rollout rules.", ref.Name), nil

	case ForceRestart:
		n, err := client.ForceDeleteAppPods(ctx, kind, ref.Namespace, ref.Name)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Killed %s of %s without waiting; its controller is creating new ones.", pods(n), ref.Name), nil
	}
	return "", &UnavailableError{Action: a, Reason: "That is not an action."}
}

func stoppedSentence(ref AppRef, before kube.AppPower) string {
	switch ref.Kind {
	case KindDeployment, KindStatefulSet:
		return fmt.Sprintf("Scaled %s to zero; a start brings back the %d it ran.", ref.Name, before.Desired)
	case KindDaemonSet:
		return fmt.Sprintf("%s now runs on no node; a start puts it back where it ran.", ref.Name)
	case KindCronJob:
		return fmt.Sprintf("Suspended %s; it keeps its schedule and runs nothing until it is started.", ref.Name)
	default:
		return fmt.Sprintf("Suspended %s.", ref.Name)
	}
}

func startedSentence(ref AppRef, before kube.AppPower) string {
	switch ref.Kind {
	case KindDeployment, KindStatefulSet:
		n := before.WouldStartWith
		if n <= 0 {
			// The count nobody wrote down; see kube.Start.
			return fmt.Sprintf("Started %s with 1 replica; nothing recorded how many it ran before.", ref.Name)
		}
		return fmt.Sprintf("Started %s again with the %d it ran before.", ref.Name, n)
	case KindDaemonSet:
		return fmt.Sprintf("%s runs on its nodes again.", ref.Name)
	case KindCronJob:
		return fmt.Sprintf("Resumed %s's schedule.", ref.Name)
	default:
		return fmt.Sprintf("Resumed %s.", ref.Name)
	}
}

func pods(n int) string {
	if n == 1 {
		return "its 1 pod"
	}
	return fmt.Sprintf("its %d pods", n)
}

func them(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}
