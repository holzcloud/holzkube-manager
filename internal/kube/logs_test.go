package kube_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// Logs, events and container detail (2026-09-19).
//
// The claim under all of this: the screen should answer "why is it broken", not
// only "it is broken". Each test below is one of the questions an operator
// actually arrives with.

func crashingCluster(t *testing.T) (*kubesim.Server, *kube.Client) {
	t.Helper()

	return newCluster(t, kubesim.Options{
		Pods: []kubesim.Pod{{
			Namespace:      "default",
			Name:           "api-7c9",
			Node:           "cp-1",
			OwnerKind:      "ReplicaSet",
			Phase:          corev1.PodRunning,
			Ready:          0,
			ContainerNames: []string{"api", "log-shipper"},
			Image:          "ghcr.io/holzcloud/api:2026.09.17",
			CrashLoop:      true,
			LastExitCode:   137,
			LastReason:     "OOMKilled",
			Restarts:       14,
			Logs: map[string]string{
				"api":         "", // restarting: nothing yet
				"log-shipper": "shipping\n",
			},
			PreviousLogs: map[string]string{
				"api": "listening on :8080\nallocating cache\nfatal: out of memory\n",
			},
		}},
		Events: []kubesim.Event{
			{
				Namespace: "default", Type: "Warning", Reason: "BackOff", Kind: "Pod", Name: "api-7c9",
				Message: "Back-off restarting failed container api", Count: 14,
				LastSeen: time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC),
			},
			{
				Namespace: "default", Type: "Normal", Reason: "Pulled", Kind: "Pod", Name: "api-7c9",
				Message:  "Container image already present on machine",
				LastSeen: time.Date(2026, 9, 19, 11, 0, 0, 0, time.UTC),
			},
			{
				Namespace: "kube-system", Type: "Warning", Reason: "FailedScheduling", Kind: "Pod",
				Name: "something-else", Message: "0/2 nodes are available",
				LastSeen: time.Date(2026, 9, 19, 11, 30, 0, 0, time.UTC),
			},
		},
	})
}

// TestTheLogOfTheContainerThatDiedIsWhatDiagnosesACrashLoop.
//
// This is the test the whole feature exists for. A pod in CrashLoopBackOff is,
// when you look at it, waiting to start again: its CURRENT container has printed
// nothing. Everything that explains the crash is in the run that already ended.
// A log view without `previous` answers every crash loop with an empty box.
func TestTheLogOfTheContainerThatDiedIsWhatDiagnosesACrashLoop(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := crashingCluster(t)

	current, err := client.PodLogs(ctx, "default", "api-7c9", kube.LogOptions{Container: "api"})
	if err != nil {
		t.Fatalf("PodLogs: %v", err)
	}
	if len(current.Lines) != 0 {
		t.Errorf("the running container printed %v; it has not started yet", current.Lines)
	}

	previous, err := client.PodLogs(ctx, "default", "api-7c9",
		kube.LogOptions{Container: "api", Previous: true})
	if err != nil {
		t.Fatalf("PodLogs(previous): %v", err)
	}
	if !previous.Previous {
		t.Error("the answer does not say which of the two logs it is, so a screen could label " +
			"the dead container's output as the running one's")
	}
	last := previous.Lines[len(previous.Lines)-1]
	if !strings.Contains(last, "out of memory") {
		t.Errorf("last line = %q, want the one that explains the crash", last)
	}
}

// TestAPodWithSeveralContainersIsNotGuessedAt: picking the first would show a
// sidecar's log and let somebody conclude the application printed nothing.
func TestAPodWithSeveralContainersIsNotGuessedAt(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := crashingCluster(t)

	_, err := client.PodLogs(ctx, "default", "api-7c9", kube.LogOptions{})
	if !errors.Is(err, kube.ErrNoSuchContainer) {
		t.Fatalf("err = %v, want a refusal naming the containers", err)
	}
	// And it names them, because the repair is picking one.
	if !strings.Contains(err.Error(), "api") || !strings.Contains(err.Error(), "log-shipper") {
		t.Errorf("reason = %q, want both container names", err)
	}

	if _, err := client.PodLogs(ctx, "default", "api-7c9",
		kube.LogOptions{Container: "nope"}); !errors.Is(err, kube.ErrNoSuchContainer) {
		t.Errorf("a container that is not there = %v, want ErrNoSuchContainer", err)
	}
}

// TestAMissingLogIsAnAnswerRatherThanAnError: a previous container that never
// existed is the ordinary state of a pod that has simply never crashed.
func TestAMissingLogIsAnAnswerRatherThanAnError(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := crashingCluster(t)

	log, err := client.PodLogs(ctx, "default", "api-7c9",
		kube.LogOptions{Container: "log-shipper", Previous: true})
	if err != nil {
		t.Fatalf("a container with no previous run came back as an error: %v", err)
	}
	if len(log.Lines) != 1 || !strings.Contains(log.Lines[0], "no such log") {
		t.Errorf("lines = %v, want one sentence saying the cluster has no such log", log.Lines)
	}
}

// TestALogLongerThanTheCapKeepsTheEnd: the last line is the one that explains
// the failure, so the cut can only be made at the beginning.
func TestALogLongerThanTheCapKeepsTheEnd(t *testing.T) {
	t.Parallel()

	// Both ends are marked, and the FIRST marker is what makes this test able to
	// tell the two cuts apart. Asserting only that the last line survived does
	// not: measured, a 21-byte last line fits into the space freed by dropping
	// a 121-byte line, so it survives a cut made at the wrong end too. That
	// version of this test passed against a deliberately reversed
	// implementation, which is the whole reason the first line is checked.
	var b strings.Builder
	b.WriteString("first: the beginning\n")
	for i := 0; b.Len() <= kube.MaxLogBytes+4096; i++ {
		b.WriteString(strings.Repeat("x", 120))
		b.WriteString("\n")
	}
	body := b.String() + "fatal: the last line\n"

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{Pods: []kubesim.Pod{{
		Namespace: "default", Name: "chatty", ContainerNames: []string{"app"},
		Logs: map[string]string{"app": body},
	}}})

	log, err := client.PodLogs(ctx, "default", "chatty", kube.LogOptions{Container: "app"})
	if err != nil {
		t.Fatalf("PodLogs: %v", err)
	}
	if !log.Truncated {
		t.Error("a log over the cap was not reported as cut, which is how half a log is read as " +
			"the whole one")
	}
	if got := log.Lines[len(log.Lines)-1]; got != "fatal: the last line" {
		t.Errorf("last line = %q; the cut has to be at the START, because the end is the part "+
			"that explains the failure", got)
	}
	if log.Lines[0] == "first: the beginning" {
		t.Error("the beginning survived, so the cut was made at the end -- which throws away the " +
			"only part of a log anybody opens it for")
	}
}

// TestContainerDetailSaysWhichContainerAndHowItEnded.
func TestContainerDetailSaysWhichContainerAndHowItEnded(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := crashingCluster(t)

	containers, err := client.ContainersOf(ctx, "default", "api-7c9")
	if err != nil {
		t.Fatalf("ContainersOf: %v", err)
	}
	if len(containers) != 2 {
		t.Fatalf("containers = %+v", containers)
	}

	api := containers[0]
	if api.Name != "api" || api.Image != "ghcr.io/holzcloud/api:2026.09.17" {
		t.Errorf("container = %+v, want the name and the image it runs", api)
	}
	if api.State != "waiting" || api.Reason != "CrashLoopBackOff" {
		t.Errorf("state = %q/%q", api.State, api.Reason)
	}
	// The diagnosis, which belongs to the run that ended.
	if api.LastExitCode != 137 || api.LastReason != "OOMKilled" {
		t.Errorf("last run = %d/%q, want the exit code and reason", api.LastExitCode, api.LastReason)
	}
	if !api.HasPrevious {
		t.Error("has_previous is false, so a screen would not offer the log that explains this")
	}

	if got := kube.ExplainContainer(api); !strings.Contains(got, "more memory") {
		t.Errorf("explanation = %q, want the sentence about the memory limit", got)
	}
}

// TestEventsAreFilteredByTheServerAndReadOldestFirst.
func TestEventsAreFilteredByTheServerAndReadOldestFirst(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := crashingCluster(t)

	all, err := client.Events(ctx, "")
	if err != nil {
		t.Fatalf("Events: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("events = %d, want every namespace's", len(all))
	}
	// Oldest first: the sequence is the story, and a screen that reversed them
	// would put the consequence above the cause.
	if all[0].LastSeen > all[len(all)-1].LastSeen {
		t.Errorf("events are newest first: %q then %q", all[0].LastSeen, all[len(all)-1].LastSeen)
	}

	about, err := client.EventsAbout(ctx, "default", "Pod", "api-7c9")
	if err != nil {
		t.Fatalf("EventsAbout: %v", err)
	}
	if len(about) != 2 {
		t.Fatalf("events about the pod = %+v, want only its own", about)
	}
	for _, e := range about {
		if e.Object != "Pod/api-7c9" {
			t.Errorf("event about %q leaked into this pod's list", e.Object)
		}
	}
	if about[len(about)-1].Reason != "BackOff" || about[len(about)-1].Count != 14 {
		t.Errorf("newest = %+v, want the BackOff seen 14 times", about[len(about)-1])
	}
}

// TestASecretIsRefusedRatherThanRedacted.
//
// A Secret's data is base64, not encryption. Redacting the values was the
// obvious alternative and it is worse: it teaches that looking at Secrets here
// is safe, and the first field the redaction misses -- a new stringData, an
// annotation somebody stuffed a token into -- is a credential on a screen that
// promised it was not. Refusing the kind has no such failure mode.
func TestASecretIsRefusedRatherThanRedacted(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{})

	_, err := client.Describe(ctx, "v1", "Secret", "default", "database")
	if !errors.Is(err, kube.ErrRefusedKind) {
		t.Fatalf("err = %v, want ErrRefusedKind", err)
	}
	// And the refusal says where the value does belong, because "no" without a
	// next step sends somebody to kubectl for the same thing.
	if !strings.Contains(err.Error(), "base64") {
		t.Errorf("reason = %q, want it to say why base64 is not protection", err)
	}
}

// TestDescribingAnObjectLeavesOutTheBookkeepingAndSaysSo.
func TestDescribingAnObjectLeavesOutTheBookkeepingAndSaysSo(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, client := newCluster(t, kubesim.Options{
		Objects: []string{"configmaps/default/settings"},
	})

	described, err := client.Describe(ctx, "v1", "ConfigMap", "default", "settings")
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if described.Kind != "ConfigMap" || described.Name != "settings" {
		t.Errorf("described = %+v", described)
	}
	if !strings.Contains(described.YAML, "name: settings") {
		t.Errorf("yaml = %q, want the object", described.YAML)
	}
	// A kind the cluster does not have is refused by name rather than with a
	// stack of discovery words.
	if _, err := client.Describe(ctx, "cert-manager.io/v1", "Certificate", "default",
		"tls"); !errors.Is(err, kube.ErrManifestInvalid) {
		t.Errorf("an unknown kind = %v, want ErrManifestInvalid", err)
	}
}
