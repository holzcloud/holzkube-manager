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

	// simulatorPackage is the in-process fake Talos node. It is a normal
	// package in the product module rather than a separate one (D-07), which is
	// what makes a guard necessary: nothing but this test stops an import of it
	// from reaching the binary.
	simulatorPackage = rootModule + "/internal/talossim"

	// cosiRuntime is the resource runtime the machinery client's COSI adapter
	// speaks. It arrives with machinery rather than by choice, so its version is
	// pinned and asserted rather than left to resolution.
	cosiRuntime = "github.com/cosi-project/runtime"

	// The pinned upstream versions. Changing either of these numbers is a
	// decision about which Talos API surface holzkube speaks, so it must be a
	// visible edit here and not a side effect of somebody running `go get -u`.
	machineryVersion = "v1.13.9"
	cosiVersion      = "v1.14.1"
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

		// The import paths phase 2 actually added. Before this task the table
		// pinned the classification against paths nobody imported yet; these
		// are the real ones, so a future change to offendingPackages is
		// measured against what the product compiles.
		"github.com/siderolabs/talos/pkg/machinery/api/machine": false,
		"github.com/siderolabs/talos/pkg/machinery/constants":   false,
		"github.com/cosi-project/runtime/pkg/state":             false,
		"google.golang.org/grpc/test/bufconn":                   false,
		"github.com/holzcloud/holzkube/internal/talos":          false,
		"github.com/holzcloud/holzkube/internal/talossim":       false,

		"github.com/holzcloud/holzkube/internal/store":      false,
		"github.com/siderolabs/talos-metal-agent/pkg/thing": false,
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

// TestModuleGraphExcludesTalosRoot closes the gap TestBinaryDependencyWeight
// leaves open.
//
// That guard walks `go list -deps ./cmd/holzkubed`, which is package-level: it
// stays green while a test-only package under internal/ pulls the Talos root
// module into go.mod, into go.sum, and into the compile of every `go test
// ./...`. The module graph is the level at which "we do not depend on that"
// actually means something, so it is asserted here as well (D-07).
func TestModuleGraphExcludesTalosRoot(t *testing.T) {
	for _, mod := range moduleGraph(t) {
		if mod == talosRoot {
			t.Fatalf("the module graph contains %s.\n\n"+
				"Only %s may be required by the product module. Anything that needs the root "+
				"module belongs in the separate module under sandbox/ -- see sandbox/README.md.",
				talosRoot, talosMachinery)
		}
	}
}

// TestPinnedUpstreamVersions asserts the versions that are actually resolved,
// not the versions go.mod asks for.
//
// Reading go.mod would prove nothing: minimal version selection can raise a
// requirement above the written one when some other dependency asks for more,
// and that is exactly the drift worth catching -- machinery and the COSI
// runtime define the wire surface holzkube speaks to a cluster.
func TestPinnedUpstreamVersions(t *testing.T) {
	want := map[string]string{
		talosMachinery: machineryVersion,
		cosiRuntime:    cosiVersion,
	}

	got := map[string]string{}
	for _, line := range goList(t, "-m", "all") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if _, ok := want[fields[0]]; ok {
			got[fields[0]] = fields[1]
		}
	}

	for mod, wantVersion := range want {
		switch gotVersion, ok := got[mod]; {
		case !ok:
			t.Errorf("%s is not in the module graph at all; the pin cannot be asserted", mod)
		case gotVersion != wantVersion:
			t.Errorf("%s resolves to %s, but the pin is %s.\n"+
				"A version moved without a decision. Either update the pin in this file "+
				"deliberately, or find the dependency that raised it.", mod, gotVersion, wantVersion)
		}
	}
}

// TestSimulatorIsNotInTheProduct keeps the fake Talos node out of the shipped
// binary (T-02-02).
//
// A simulator that answers Version and Hostname inside a real deployment is
// indistinguishable from a real node to everything above the transport seam,
// which is precisely what makes it useful in a test and dangerous in
// production. It lives in the product module so that the production client's
// tests can import it without crossing a module boundary; this test is the
// price of that choice.
func TestSimulatorIsNotInTheProduct(t *testing.T) {
	for _, pkg := range goList(t, "-deps", "./cmd/holzkubed") {
		if pkg == simulatorPackage || strings.HasPrefix(pkg, simulatorPackage+"/") {
			t.Fatalf("cmd/holzkubed depends on %s.\n\n"+
				"The simulated Talos node must never be reachable from the product binary: "+
				"in a real deployment it is indistinguishable from a real machine. "+
				"It belongs in _test.go files and in packages the binary does not import.", pkg)
		}
	}
}

// moduleGraph returns the module paths of `go list -m all`.
func moduleGraph(t *testing.T) []string {
	t.Helper()

	var mods []string
	for _, line := range goList(t, "-m", "all") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		mods = append(mods, fields[0])
	}
	return mods
}
