package talos_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// machineryClient is the import the seam exists to contain. It is spelled out
// here rather than imported, because importing it in this file would be the
// very thing the test forbids everywhere else.
const machineryClient = "github.com/siderolabs/talos/pkg/machinery/client"

// endpointFormat matches a format string that is building a network endpoint:
// a verb, a colon, then a port as a verb or as a literal number. It is
// deliberately narrow. "%s:%d" and "%s:50000" are endpoints; "%s: %w" and
// "user:%s" are not, and flagging them would make this guard something people
// switch off.
var endpointFormat = regexp.MustCompile(`%[sqv]:(%[dsqv]|[0-9]{2,5})`)

// TestNoAddressAboveTheSeam is the architecture guard for internal/talos.
//
// The seam is identity-shaped: everything above it speaks talos.Target, and
// Dialer.Resolve is the single place identity becomes an address. That property
// is only worth anything if nothing above the seam reaches around it, and it is
// the property the whole SideroLink retrofit rests on -- a tunnel resolves the
// same Target to an overlay address, and any caller that built a host:port
// itself has to be found and rewritten first.
//
// It is the same shape as TestNoDirectFileAccessOutsideFsstore: a review
// catches this on the day someone looks; a test catches it on the day it is
// written.
//
// Exempt, and why:
//
//   - internal/talos — the seam itself. It owns the machinery client and it
//     owns the one call to net.JoinHostPort in the repository, because that is
//     what Dialer.Resolve is for.
//   - _test.go files — a test legitimately constructs an address to point a
//     dialer at a simulated node.
func TestNoAddressAboveTheSeam(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")
	exempt := filepath.Join("internal", "talos")

	var violations []string
	scanned := 0

	for _, top := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			if rel == exempt || strings.HasPrefix(rel, exempt+string(filepath.Separator)) {
				return nil
			}

			scanned++
			violations = append(violations, inspect(t, p, rel)...)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", top, err)
		}
	}

	// A guard that scans nothing passes for the wrong reason. If the relative
	// path to the repository root ever breaks, this catches it instead of
	// letting the architecture rule quietly stop being enforced.
	if scanned < 15 {
		t.Fatalf("scanned only %d source files; the guard is not reaching the tree (root %q)", scanned, root)
	}
	t.Logf("scanned %d production source files outside %s", scanned, exempt)

	if len(violations) > 0 {
		t.Fatalf("node addressing or the machinery client appears outside internal/talos:\n  %s\n\n"+
			"A machine is reached through talos.Target and a talos.Dialer, never through an address "+
			"a caller built. Resolving identity to an address is Dialer.Resolve's job -- that is what "+
			"lets a SideroLink tunnel resolve the same Target to an overlay address with no change here.",
			strings.Join(violations, "\n  "))
	}
}

func inspect(t *testing.T, path, rel string) []string {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly|parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	var found []string
	for _, imp := range file.Imports {
		value, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if value == machineryClient || strings.HasPrefix(value, machineryClient+"/") {
			found = append(found, fmt.Sprintf("%s:%d imports %s",
				rel, fset.Position(imp.Pos()).Line, value))
		}
	}

	full, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	ast.Inspect(full, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		line := fset.Position(call.Pos()).Line

		switch {
		case pkg.Name == "net" && sel.Sel.Name == "JoinHostPort":
			found = append(found, fmt.Sprintf("%s:%d calls net.JoinHostPort", rel, line))
		case pkg.Name == "fmt" && sel.Sel.Name == "Sprintf" && len(call.Args) > 0:
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if endpointFormat.MatchString(lit.Value) {
				found = append(found, fmt.Sprintf("%s:%d builds an endpoint with fmt.Sprintf(%s)", rel, line, lit.Value))
			}
		}
		return true
	})

	return found
}

// TestSeamGuardRecognisesAViolation is the negative control for the guard
// above.
//
// Today no package outside internal/talos violates the rule, so
// TestNoAddressAboveTheSeam passes whether its classification is right or
// vacuous. This pins the classification itself, the same way
// TestGuardRecognisesTheRootModule pins the dependency guard's.
func TestSeamGuardRecognisesAViolation(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		source string
		want   int
	}{
		"machinery client import": {source: `package x

import _ "` + machineryClient + `"
`, want: 1},
		"JoinHostPort": {source: `package x

import "net"

func f(h string) string { return net.JoinHostPort(h, "50000") }
`, want: 1},
		"Sprintf endpoint": {source: `package x

import "fmt"

func f(h string, p int) string { return fmt.Sprintf("%s:%d", h, p) }
`, want: 1},
		"Sprintf that is not an endpoint": {source: `package x

import "fmt"

func f(err error) string { return fmt.Sprintf("talos: %s: %v", "context", err) }
`, want: 0},
		"clean file": {source: `package x

import "context"

func f(ctx context.Context) error { return ctx.Err() }
`, want: 0},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "sample.go")
			if err := os.WriteFile(path, []byte(tc.source), 0o600); err != nil {
				t.Fatalf("write sample: %v", err)
			}
			got := inspect(t, path, "sample.go")
			if len(got) != tc.want {
				t.Errorf("inspect reported %d violation(s) %v, want %d", len(got), got, tc.want)
			}
		})
	}
}

// machineryClientConstructor is the call that turns dial options into a live
// connection. Everything the dry-run gate and the deadline policy do is
// installed in the options handed to it, so a second call site is a second
// connection with a different policy on it.
const machineryClientConstructor = "New"

// TestEveryClientConstructionRoutesThroughTheSharedPath is the bypass proof for
// D-03, and the half of it that TestNoAddressAboveTheSeam cannot make.
//
// That guard proves no package outside internal/talos can construct a machinery
// client at all -- it cannot even import the package. This one proves the
// remaining case: that inside internal/talos, where the import is legal, there
// is still exactly one place a connection is made. Both halves are needed,
// because "no mutation reaches a node" is only true if every connection to a
// node carries the interceptors, and an interceptor chain is a property of the
// options passed at construction rather than of the type that comes back.
//
// It also pins where the two client types are built, so a future constructor
// that skipped dial -- and therefore skipped the gate, the deadline policy and
// the retry loop -- fails here rather than in production.
func TestEveryClientConstructionRoutesThroughTheSharedPath(t *testing.T) {
	t.Parallel()

	files, sites := inspectConstruction(t, filepath.Join("..", "..", "internal", "talos"))

	// A walk that reached nothing passes for the wrong reason. The negative
	// control below proves this threshold is doing work rather than being
	// trivially satisfied.
	if files < 8 {
		t.Fatalf("inspected only %d production file(s) in internal/talos; the guard is not reaching the package", files)
	}
	t.Logf("inspected %d production files and %d construction site(s) in internal/talos", files, len(sites))

	want := map[string]string{
		"client.New":         "dial",
		"&ClusterClient":     "NewClusterClient",
		"&MaintenanceClient": "NewMaintenanceClient",
	}

	seen := map[string]int{}
	for _, s := range sites {
		seen[s.what]++
		allowed, known := want[s.what]
		if !known {
			continue
		}
		if s.fn != allowed {
			t.Errorf("%s constructs %s inside %s, not inside %s; "+
				"a connection made anywhere but dial does not carry the dry-run gate, "+
				"the deadline policy or the retry loop", s.where, s.what, s.fn, allowed)
		}
	}

	for what := range want {
		if seen[what] != 1 {
			t.Errorf("found %d construction site(s) for %s, want exactly 1; "+
				"a second one is a second policy", seen[what], what)
		}
	}
}

// TestConstructionGuardIsNotVacuous is the negative control the plan asks for:
// point the walk at an empty tree and confirm it reports nothing, which is what
// makes the file threshold above a real assertion rather than decoration.
func TestConstructionGuardIsNotVacuous(t *testing.T) {
	t.Parallel()

	files, sites := inspectConstruction(t, t.TempDir())
	if files != 0 || len(sites) != 0 {
		t.Fatalf("an empty tree reported %d file(s) and %d site(s); the walk is not reading what it is pointed at",
			files, len(sites))
	}

	// And a tree with a violation in it is seen. Without this, a walk that
	// silently classified nothing would satisfy the case above too.
	dir := t.TempDir()
	source := `package talos

import "github.com/siderolabs/talos/pkg/machinery/client"

func somewhereElse() (*ClusterClient, error) {
	c, err := client.New(nil)
	if err != nil {
		return nil, err
	}
	return &ClusterClient{conn: &conn{c: c}}, nil
}
`
	if err := os.WriteFile(filepath.Join(dir, "sample.go"), []byte(source), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	files, sites = inspectConstruction(t, dir)
	if files != 1 {
		t.Fatalf("inspected %d file(s), want 1", files)
	}
	found := map[string]string{}
	for _, s := range sites {
		found[s.what] = s.fn
	}
	if found["client.New"] != "somewhereElse" || found["&ClusterClient"] != "somewhereElse" {
		t.Errorf("the guard did not attribute the violations to their function: %v", found)
	}
}

// constructionSite is one place a connection or a client value is built.
type constructionSite struct {
	what  string // "client.New", "&ClusterClient", "&MaintenanceClient"
	fn    string // the enclosing function's name
	where string // file:line
}

// inspectConstruction walks a directory's production Go files and reports every
// place a machinery client or one of the two client types is constructed,
// together with the function that does it.
func inspectConstruction(t *testing.T, dir string) (int, []constructionSite) {
	t.Helper()

	files := 0
	var sites []constructionSite

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		// A file excluded from every ordinary build -- the compile-failure
		// fixture -- is not production code and does not construct anything at
		// runtime.
		if hasBuildConstraint(file) {
			continue
		}
		files++

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				at := func() string {
					return fmt.Sprintf("%s:%d", name, fset.Position(n.Pos()).Line)
				}
				switch node := n.(type) {
				case *ast.CallExpr:
					sel, ok := node.Fun.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					pkg, ok := sel.X.(*ast.Ident)
					if !ok || pkg.Name != "client" || sel.Sel.Name != machineryClientConstructor {
						return true
					}
					sites = append(sites, constructionSite{what: "client.New", fn: fn.Name.Name, where: at()})
				case *ast.CompositeLit:
					ident, ok := node.Type.(*ast.Ident)
					if !ok {
						return true
					}
					if ident.Name == "ClusterClient" || ident.Name == "MaintenanceClient" {
						sites = append(sites, constructionSite{what: "&" + ident.Name, fn: fn.Name.Name, where: at()})
					}
				}
				return true
			})
		}
	}

	return files, sites
}

// hasBuildConstraint reports whether a file carries a //go:build line, which is
// how clusteronly_fixture.go excludes itself from every ordinary build.
func hasBuildConstraint(file *ast.File) bool {
	for _, group := range file.Comments {
		for _, c := range group.List {
			if strings.HasPrefix(c.Text, "//go:build") {
				return true
			}
		}
	}
	return false
}
