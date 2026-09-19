package kube

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// Container detail (2026-09-19).
//
// The pod list says "1/2 ready" and "CrashLoopBackOff". That is enough to know
// something is wrong and not enough to do anything about it. What an operator
// needs next is which container, what image it runs, and how it ended: a
// container killed with exit code 137 is out of memory, one with 1 has thrown,
// and one stuck in ImagePullBackOff never started at all. Those are different
// days of work, and the pod's summary cannot tell them apart.

// Container is one container of a pod, as somebody diagnosing it needs it.
type Container struct {
	Name  string `json:"name"`
	Image string `json:"image"`

	// Init marks a container that runs before the others. A pod stuck in Init
	// is a pod whose interesting log belongs to one of these, and the ordinary
	// container list does not show it.
	Init bool `json:"init"`

	Ready    bool  `json:"ready"`
	Started  bool  `json:"started"`
	Restarts int32 `json:"restarts"`

	// State is running, waiting or terminated, with Reason carrying the
	// cluster's own word for it: CrashLoopBackOff, ImagePullBackOff,
	// OOMKilled, Completed.
	State  string `json:"state"`
	Reason string `json:"reason"`

	// Message is the longer sentence the cluster attaches, when it does. It is
	// where "failed to pull image ... unauthorized" lives.
	Message string `json:"message"`

	// LastState describes the PREVIOUS run, and it is the field this type
	// exists for. A container that is restarting has nothing to say about its
	// current run; how the last one ended is the whole diagnosis.
	LastState      string `json:"last_state"`
	LastReason     string `json:"last_reason"`
	LastExitCode   int32  `json:"last_exit_code"`
	LastFinishedAt string `json:"last_finished_at"`

	// HasPrevious says whether a previous log can be asked for at all, so the
	// screen offers that button only when it would answer.
	HasPrevious bool `json:"has_previous"`

	// Requests and Limits, as written. Empty means the container asked for
	// nothing -- which is itself worth seeing, because a pod with no requests
	// is a pod the scheduler places blind and the kubelet evicts first.
	CPURequest    string `json:"cpu_request"`
	MemoryRequest string `json:"memory_request"`
	CPULimit      string `json:"cpu_limit"`
	MemoryLimit   string `json:"memory_limit"`
}

// containersOf renders a pod's containers, init containers first.
//
// Init first because that is the order they run in, and a pod that is stuck is
// stuck at the earliest one that has not finished.
func containersOf(pod *corev1.Pod) []Container {
	out := make([]Container, 0, len(pod.Spec.InitContainers)+len(pod.Spec.Containers))
	for _, spec := range pod.Spec.InitContainers {
		out = append(out, container(spec, statusFor(pod.Status.InitContainerStatuses, spec.Name), true))
	}
	for _, spec := range pod.Spec.Containers {
		out = append(out, container(spec, statusFor(pod.Status.ContainerStatuses, spec.Name), false))
	}
	return out
}

func statusFor(statuses []corev1.ContainerStatus, name string) *corev1.ContainerStatus {
	for i := range statuses {
		if statuses[i].Name == name {
			return &statuses[i]
		}
	}
	// A container with no status has not been scheduled or reported yet, which
	// is a real state -- a pod stuck Pending has specs and no statuses at all.
	return nil
}

func container(spec corev1.Container, status *corev1.ContainerStatus, init bool) Container {
	out := Container{
		Name:          spec.Name,
		Image:         spec.Image,
		Init:          init,
		CPURequest:    quantity(spec.Resources.Requests, corev1.ResourceCPU),
		MemoryRequest: quantity(spec.Resources.Requests, corev1.ResourceMemory),
		CPULimit:      quantity(spec.Resources.Limits, corev1.ResourceCPU),
		MemoryLimit:   quantity(spec.Resources.Limits, corev1.ResourceMemory),
	}
	if status == nil {
		out.State = "not started"
		return out
	}

	out.Ready = status.Ready
	out.Restarts = status.RestartCount
	if status.Started != nil {
		out.Started = *status.Started
	}
	// The image the kubelet actually runs, when it differs from the one the
	// spec names -- a tag that moved is exactly the case somebody is chasing.
	if status.Image != "" {
		out.Image = status.Image
	}

	switch {
	case status.State.Running != nil:
		out.State = "running"
	case status.State.Waiting != nil:
		out.State = "waiting"
		out.Reason = status.State.Waiting.Reason
		out.Message = status.State.Waiting.Message
	case status.State.Terminated != nil:
		out.State = "terminated"
		out.Reason = status.State.Terminated.Reason
		out.Message = status.State.Terminated.Message
	}

	if last := status.LastTerminationState.Terminated; last != nil {
		out.LastState = "terminated"
		out.LastReason = last.Reason
		out.LastExitCode = last.ExitCode
		if !last.FinishedAt.IsZero() {
			out.LastFinishedAt = last.FinishedAt.UTC().Format(time.RFC3339)
		}
		// A previous log exists exactly when a previous run does.
		out.HasPrevious = true
	}
	return out
}

func quantity(list corev1.ResourceList, name corev1.ResourceName) string {
	value, ok := list[name]
	if !ok {
		return ""
	}
	return value.String()
}

// ExplainContainer is the one-line answer to "what is wrong with it".
//
// It exists so the sentence is written once rather than in the screen, the API
// and whatever reads the API next -- three places that would drift, and two of
// them would be wrong about an exit code.
func ExplainContainer(c Container) string {
	switch {
	// OOMKilled first, and the order is the finding: a container in
	// CrashLoopBackOff whose previous run was OOMKilled would otherwise be
	// explained as "exit code 137", which is true, useless, and the thing
	// somebody has to already know to decode. The more specific cause wins.
	case c.LastReason == "OOMKilled":
		return fmt.Sprintf("The previous run was killed for using more memory than its limit "+
			"allowed, and it has restarted %d times.", c.Restarts)
	case c.Reason == "CrashLoopBackOff":
		return fmt.Sprintf("Started and exited %d times; the cluster is waiting before trying again. "+
			"Its previous run ended with exit code %d.", c.Restarts, c.LastExitCode)
	case c.Reason == "ImagePullBackOff" || c.Reason == "ErrImagePull":
		return "The image could not be pulled. That is a registry, a tag or a credential, not the workload."
	case c.State == "terminated" && c.Reason == "Completed":
		return "It finished and exited cleanly."
	case c.State == "terminated":
		return fmt.Sprintf("It exited with code %d.", c.LastExitCode)
	case c.State == "waiting" && c.Reason != "":
		return c.Reason
	case c.State == "running" && !c.Ready:
		return "Running, and not passing its readiness probe."
	default:
		return ""
	}
}
