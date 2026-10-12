package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// TestTheDaemonWiresTheKubernetesReleaseSourceWithTheFixedHost holds the one
// line nothing else holds: that the upgrade service gets its Kubernetes release
// source from the fixed address and from nothing the operator can set.
//
// The source is read from the syntax tree and not from the text, so a comment
// that mentions the call, or a string that spells it, counts for nothing. What
// is asked is the shape: one k8srelease.New whose first argument is the
// selector k8srelease.DefaultBaseURL -- which is what keeps a value of the
// configuration from ever flowing into the source (D-04: no flag, no variable,
// no setting) -- and one WithKubernetesReleases with one argument, a selector
// on Newest. A nil there would be a daemon whose every chain with a step on the
// way is blocked, and nothing but a plan would say so.
func TestTheDaemonWiresTheKubernetesReleaseSourceWithTheFixedHost(t *testing.T) {
	t.Parallel()

	const (
		pkgPath = "github.com/holzcloud/holzkube-manager/internal/k8srelease"
		file    = "main.go"
	)

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}

	local := ""
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != pkgPath {
			continue
		}
		local = "k8srelease"
		if imp.Name != nil {
			local = imp.Name.Name
		}
	}
	if local == "" {
		t.Fatalf("%s does not import %s", file, pkgPath)
	}

	var news, withs []*ast.CallExpr
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if x, ok := sel.X.(*ast.Ident); ok && x.Name == local && sel.Sel.Name == "New" {
			news = append(news, call)
		}
		if sel.Sel.Name == "WithKubernetesReleases" {
			withs = append(withs, call)
		}
		return true
	})

	if len(news) != 1 {
		t.Fatalf("%s calls %s.New %d times, want exactly once", file, local, len(news))
	}
	if len(news[0].Args) == 0 || !isSelector(news[0].Args[0], local, "DefaultBaseURL") {
		t.Errorf("the first argument of %s.New at %s is not the constant %s.DefaultBaseURL: "+
			"nothing from the configuration, and no literal, may decide where the releases are read from",
			local, fset.Position(news[0].Pos()), local)
	}

	if len(withs) != 1 {
		t.Fatalf("%s calls WithKubernetesReleases %d times, want exactly once", file, len(withs))
	}
	if len(withs[0].Args) != 1 {
		t.Fatalf("WithKubernetesReleases at %s takes %d arguments, want the one source",
			fset.Position(withs[0].Pos()), len(withs[0].Args))
	}
	arg, ok := withs[0].Args[0].(*ast.SelectorExpr)
	if !ok || arg.Sel.Name != "Newest" {
		t.Errorf("the argument of WithKubernetesReleases at %s is not the Newest method of the client: "+
			"a nil or any other function there leaves the daemon without the source, or with another one",
			fset.Position(withs[0].Pos()))
	}
}

// isSelector reports whether e is the selector pkg.name.
func isSelector(e ast.Expr, pkg, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == pkg
}
