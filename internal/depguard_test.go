// Package internal holds no code. It exists so that the module boundary between
// the product binary and the sandbox can be asserted by a test that actually
// runs, rather than by a note in a document.
package internal

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	// talosRoot is the heavy module: the Docker and QEMU provisioners live here
	// and drag in a large part of an operating system.
	talosRoot = "github.com/siderolabs/talos"

	// talosMachinery is the only part of it the product is allowed to reach:
	// the API types and the client, which are light and have a normal
	// dependency tree.
	talosMachinery = "github.com/siderolabs/talos/pkg/machinery"

	rootModule = "github.com/holzcloud/holzkube"
)

// TestBinaryDependencyWeight fails if the product binary ever depends on the
// Talos root module.
//
// Phase 1 has no Talos dependency at all, so this passes trivially today. It is
// written now on purpose: phase 2 adds the first one, and a guard added after
// the mistake has to be paid for with a refactor rather than with a red test.
// Getting it wrong yields a ~200 MB binary whose supply-chain surface includes
// every dependency of a container runtime, in a tool that holds cluster PKI.
func TestBinaryDependencyWeight(t *testing.T) {
	offenders := offendingPackages(goList(t, "-deps", "./cmd/holzkubed"))

	if len(offenders) > 0 {
		t.Fatalf("cmd/holzkubed depends on %d package(s) of the Talos root module:\n  %s\n\n"+
			"Only %s may be imported by the product. Anything that needs the root module "+
			"belongs in the separate module under sandbox/ -- see sandbox/README.md.",
			len(offenders), strings.Join(offenders, "\n  "), talosMachinery)
	}
}

// offendingPackages returns the packages of the Talos root module that are not
// part of pkg/machinery.
func offendingPackages(deps []string) []string {
	var offenders []string
	for _, pkg := range deps {
		if pkg != talosRoot && !strings.HasPrefix(pkg, talosRoot+"/") {
			continue
		}
		if pkg == talosMachinery || strings.HasPrefix(pkg, talosMachinery+"/") {
			continue
		}
		offenders = append(offenders, pkg)
	}
	return offenders
}

// TestGuardRecognisesTheRootModule is the negative control. Phase 1 has no Talos
// dependency, so TestBinaryDependencyWeight passes whether the classification is
// right or vacuous; this pins the classification itself against the imports
// phase 2 will actually add.
func TestGuardRecognisesTheRootModule(t *testing.T) {
	cases := map[string]bool{
		"github.com/siderolabs/talos":                                     true,
		"github.com/siderolabs/talos/pkg/provision":                       true,
		"github.com/siderolabs/talos/pkg/provision/providers/docker":      true,
		"github.com/siderolabs/talos/pkg/provision/providers/qemu":        true,
		"github.com/siderolabs/talos/pkg/machinery":                       false,
		"github.com/siderolabs/talos/pkg/machinery/client":                false,
		"github.com/siderolabs/talos/pkg/machinery/config/types/v1alpha1": false,
		"github.com/holzcloud/holzkube/internal/store":                    false,
		"github.com/siderolabs/talos-metal-agent/pkg/thing":               false,
		"net/http": false,
	}

	for pkg, wantOffending := range cases {
		got := len(offendingPackages([]string{pkg})) == 1
		if got != wantOffending {
			t.Errorf("offendingPackages(%q) reports offending=%v, want %v", pkg, got, wantOffending)
		}
	}
}

// TestSandboxIsASeparateModule asserts the boundary the guard above relies on:
// a nested go.mod is what keeps the root module's package patterns from reaching
// into sandbox/ in the first place.
func TestSandboxIsASeparateModule(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "sandbox", "go.mod"))
	if err != nil {
		t.Fatalf("sandbox/go.mod: %v", err)
	}
	var module string
	for _, line := range strings.Split(string(raw), "\n") {
		if after, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			module = strings.TrimSpace(after)
			break
		}
	}
	if module == "" {
		t.Fatal("sandbox/go.mod declares no module path")
	}
	if module == rootModule {
		t.Fatalf("sandbox declares the root module path %q; it would not be a separate module", module)
	}

	for _, pkg := range goList(t, "./...") {
		if strings.HasPrefix(pkg, rootModule+"/sandbox") {
			t.Errorf("the root module lists %s; sandbox/ is meant to be invisible to the product build", pkg)
		}
	}
}

// goList runs `go list` in the repository root and returns its lines.
//
// It skips rather than fails when the toolchain cannot answer -- but never
// silently: the skip states which command could not run and why, so a skipped
// guard is visible in the test output instead of looking like a pass.
func goList(t *testing.T, args ...string) []string {
	t.Helper()

	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("skipping the dependency guard: the go tool is not on PATH, so `go list` cannot be run: %v", err)
	}

	cmd := exec.Command(goBin, append([]string{"list"}, args...)...) //nolint:gosec // args are constants in this file
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("skipping the dependency guard: `go list %s` failed (an incomplete module cache with no network will do this): %v\n%s",
			strings.Join(args, " "), err, out)
	}

	var pkgs []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			pkgs = append(pkgs, line)
		}
	}
	if len(pkgs) == 0 {
		t.Fatalf("`go list %s` returned nothing", strings.Join(args, " "))
	}
	return pkgs
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("%s does not look like the repository root: %v", root, err)
	}
	return root
}
