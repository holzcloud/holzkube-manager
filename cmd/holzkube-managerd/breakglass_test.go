package main

// The break-glass subcommand.
//
// The minting rules are tested in internal/auth. What is tested here is the
// part that only exists at this layer: that the act leaves a record, and that
// the token comes out somewhere a script can use without picking up prose.

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/auth"
	"github.com/holzcloud/holzkube-manager/internal/httpapi"
)

// TestBreakGlassPrintsTheTokenAndNothingElseOnStdout.
//
// The token goes to stdout alone so `TOKEN=$(… break-glass)` works. If the
// explanation shared that stream, the next thing in the Authorization header
// would be a sentence about expiry.
//
// It runs the built binary rather than calling cmdBreakGlass, because the two
// streams ARE the interface and a function call cannot tell them apart.
func TestBreakGlassPrintsTheTokenAndNothingElseOnStdout(t *testing.T) {
	t.Parallel()

	binary := buildDaemon(t)
	dir := dataDir(t)

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(t.Context(), binary, "break-glass", "--data-dir", dir, "--ttl=5m")
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("break-glass: %v\n%s", err, stderr.String())
	}

	token := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(token, auth.TokenPrefix) {
		t.Fatalf("stdout is %q, want just a token", token)
	}
	if strings.Contains(token, "\n") || strings.Contains(token, " ") {
		t.Errorf("stdout carries more than the token: %q", token)
	}
	// And the explanation IS printed -- on the other stream. A credential handed
	// out with no word about when it dies is the failure this whole design is
	// about.
	for _, want := range []string{"expires", "once", "Revoke"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr never says %q:\n%s", want, stderr.String())
		}
	}
}

// TestBreakGlassLeavesARecordOfItself.
//
// This is what makes the subcommand defensible rather than a back door. The
// access it uses is access somebody already had -- the data directory is the
// authority -- so the thing it must add is the record.
func TestBreakGlassLeavesARecordOfItself(t *testing.T) {
	t.Parallel()

	binary := buildDaemon(t)
	dir := dataDir(t)

	cmd := exec.CommandContext(t.Context(), binary, "break-glass", "--data-dir", dir, "--ttl=5m")
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Run(); err != nil {
		t.Fatalf("break-glass: %v", err)
	}

	records := auditRecords(t, dir)
	var attempt, outcome bool
	for _, rec := range records {
		if rec["action"] != "auth.break-glass" {
			continue
		}
		actor, _ := rec["actor"].(string)
		if !strings.HasPrefix(actor, "local:") {
			t.Errorf("the record's actor is %q, want a local: prefix -- nobody signed in for "+
				"this, and a record that named a person would be a lie", actor)
		}
		switch rec["outcome"] {
		case "attempt":
			attempt = true
		case "ok":
			outcome = true
		}
	}
	if !attempt || !outcome {
		t.Errorf("the archive has attempt=%v outcome=%v for auth.break-glass, want both -- "+
			"a record written only on success leaves a failed attempt invisible, and a "+
			"failed attempt is the one somebody would want to see", attempt, outcome)
	}
}

// TestBreakGlassIsNotAThingTheServerServes.
//
// The whole justification is that this is a subcommand operating on a path:
// whoever can open the directory already holds everything in it. Over the
// network that reasoning does not hold and the same act is a back door, so no
// route may ever carry it.
func TestBreakGlassIsNotAThingTheServerServes(t *testing.T) {
	t.Parallel()

	found := []string{}
	for _, route := range routeTable(httpapi.Deps{}) {
		if strings.Contains(strings.ToLower(route.Pattern), "break-glass") {
			found = append(found, route.Pattern)
		}
	}
	if len(found) > 0 {
		t.Errorf("these routes serve break-glass: %v -- it must stay a subcommand", found)
	}
}

// auditRecords reads the archive a subcommand wrote.
func auditRecords(t *testing.T, dir string) []map[string]any {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(dir, "audit", "*.jsonl"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no audit file under %s (glob err %v)", dir, err)
	}

	out := []map[string]any{}
	for _, name := range matches {
		raw, err := os.ReadFile(name) //nolint:gosec // a path this test just made
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
			if line == "" {
				continue
			}
			rec := map[string]any{}
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				t.Fatalf("parse %s: %v", name, err)
			}
			out = append(out, rec)
		}
	}
	return out
}

// buildDaemon builds the binary under test once per test that asks.
func buildDaemon(t *testing.T) string {
	t.Helper()

	out := filepath.Join(t.TempDir(), "holzkube-managerd")
	build := exec.CommandContext(context.Background(), "go", "build", "-o", out, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, output)
	}
	return out
}
