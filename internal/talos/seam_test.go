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
