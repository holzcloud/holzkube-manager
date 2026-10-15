package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// TestTheDaemonWiresTheDriftWatch holds six pieces of main.go that nothing else
// holds, each on its own and each with a message that says what a daemon
// without it does. They are read from the syntax tree, so a comment that
// mentions a call counts for nothing.
//
// Every one of them can be missing with a clean build: a missing piece leaves
// the variable used somewhere else, and Deps is copied by value into every
// ...Routes(deps) call, so a field set after the literal is nil in every
// handler with no compile error. That is why this reads the file.
func TestTheDaemonWiresTheDriftWatch(t *testing.T) {
	t.Parallel()

	const file = "main.go"
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}

	names := map[string]string{}
	for _, path := range []string{
		"github.com/holzcloud/holzkube-manager/internal/configdrift",
		"github.com/holzcloud/holzkube-manager/internal/httpapi",
		"github.com/holzcloud/holzkube-manager/internal/httpapi/handlers",
		"github.com/holzcloud/holzkube-manager/internal/provision",
		"context",
	} {
		names[path] = importName(t, f, path, file)
	}
	drift := names["github.com/holzcloud/holzkube-manager/internal/configdrift"]
	api := names["github.com/holzcloud/holzkube-manager/internal/httpapi"]
	handlerPkg := names["github.com/holzcloud/holzkube-manager/internal/httpapi/handlers"]
	prov := names["github.com/holzcloud/holzkube-manager/internal/provision"]
	ctxPkg := names["context"]

	// The variable the service is assigned to: everything below is about that
	// variable and not about a service somebody else built.
	var news []*ast.CallExpr
	svc := ""
	ast.Inspect(f, func(n ast.Node) bool {
		if as, ok := n.(*ast.AssignStmt); ok && len(as.Rhs) == 1 {
			if call, ok := as.Rhs[0].(*ast.CallExpr); ok && isSelectorCall(call, drift, "New") {
				if id, ok := as.Lhs[0].(*ast.Ident); ok {
					svc = id.Name
				}
			}
		}
		if call, ok := n.(*ast.CallExpr); ok && isSelectorCall(call, drift, "New") {
			news = append(news, call)
		}
		return true
	})

	// Piece 1: the service is built, over the reader of the config domain and
	// the store of baselines.
	if len(news) != 1 {
		t.Fatalf("%s calls %s.New %d times, want exactly once: a daemon without the service watches no node's configuration", file, drift, len(news))
	}
	if svc == "" {
		t.Fatalf("the service of %s.New at %s is not assigned to a variable, so nothing below can be wired to it", drift, fset.Position(news[0].Pos()))
	}
	if len(news[0].Args) != 1 {
		t.Fatalf("%s.New at %s takes %d arguments, want the one Deps literal", drift, fset.Position(news[0].Pos()), len(news[0].Args))
	}
	lit, ok := news[0].Args[0].(*ast.CompositeLit)
	if !ok {
		t.Fatalf("the argument of %s.New at %s is not a literal: the guard reads the fields it sets", drift, fset.Position(news[0].Pos()))
	}
	read := field(lit, "Read")
	if read == nil || !isSelectorName(read, "ReadSnapshot") {
		t.Errorf("%s.New does not set Read to the ReadSnapshot of the configuration service: "+
			"the watch would read nothing, or something that is not the redacting reader", drift)
	}
	if field(lit, "Baselines") == nil {
		t.Errorf("%s.New does not set Baselines: a watch with no store of baselines has nothing to compare with", drift)
	}

	// Pieces 2 and 3: the calls on the inventory, one each, with the service's
	// own method as the argument.
	if observers := callsNamed(f, "SetApplyObserver"); len(observers) != 1 {
		t.Errorf("%s calls SetApplyObserver %d times, want exactly once: "+
			"without it the watch does not see an apply of this daemon and reports every one as drift", file, len(observers))
	} else if len(observers[0].Args) != 1 || !isMethodCall(observers[0].Args[0], svc, "ApplyObserver") {
		t.Errorf("the argument of SetApplyObserver at %s is not %s.ApplyObserver(): "+
			"another observer, or none, means an own apply is read as a change by somebody else",
			fset.Position(observers[0].Pos()), svc)
	}
	if views := callsNamed(f, "SetConfigDrift"); len(views) != 1 {
		t.Errorf("%s calls SetConfigDrift %d times, want exactly once: "+
			"without it the cluster card never says anything about configuration drift", file, len(views))
	} else if len(views[0].Args) != 1 || !isSelectorOn(views[0].Args[0], svc, "HealthFor") {
		t.Errorf("the argument of SetConfigDrift at %s is not %s.HealthFor: the card would show another view or none",
			fset.Position(views[0].Pos()), svc)
	}

	// Piece 4: the loop runs, on a context derived from the background one.
	// Not on the context of a request and not on one with a deadline: a
	// background loop with a deadline stops reading when it runs out and says
	// nothing (the lesson of the certificate watch, quick task 261014-gl2), and
	// a Talos client refuses a call with no deadline at all, which is why the
	// watch puts a budget on every read itself.
	var runs []*ast.GoStmt
	ast.Inspect(f, func(n ast.Node) bool {
		if g, ok := n.(*ast.GoStmt); ok && isMethodCall(g.Call, svc, "Run") {
			runs = append(runs, g)
		}
		return true
	})
	switch {
	case len(runs) != 1:
		t.Errorf("%s starts %s.Run in a goroutine %d times, want exactly once: without it nothing is ever read", file, svc, len(runs))
	case len(runs[0].Call.Args) == 0:
		t.Errorf("%s.Run at %s is given no context", svc, fset.Position(runs[0].Pos()))
	default:
		id, ok := runs[0].Call.Args[0].(*ast.Ident)
		if !ok {
			t.Errorf("the context of %s.Run at %s is not a variable made by context.WithCancel(context.Background()): "+
				"a context with a deadline would end the watch without a word", svc, fset.Position(runs[0].Pos()))
			break
		}
		if !derivedFromBackground(f, id.Name, ctxPkg) {
			t.Errorf("the context %s of %s.Run at %s is not made by %s.WithCancel(%s.Background()): "+
				"the watch must live as long as the daemon and not as long as a deadline or a request",
				id.Name, svc, fset.Position(runs[0].Pos()), ctxPkg, ctxPkg)
		}
	}

	// Piece 5: the two fields, in the literals, where a copy of Deps sees them.
	if drifts := literalFields(f, api, "Deps", "Drift"); len(drifts) != 1 {
		t.Errorf("the %s.Deps literal sets Drift %d times, want once: "+
			"a field set after the literal is nil in every handler, and the routes answer 502", api, len(drifts))
	} else if id, ok := drifts[0].(*ast.Ident); !ok || id.Name != svc {
		t.Errorf("Drift in the %s.Deps literal is not the service %s that is run and observed: the routes would ask another one", api, svc)
	}
	if forgets := literalFields(f, prov, "Deps", "ForgetConfig"); len(forgets) != 1 {
		t.Errorf("the %s.Deps literal sets ForgetConfig %d times, want once: "+
			"without it a machine that is reset and set up again is compared with the baseline of its old life", prov, len(forgets))
	} else if !isSelectorOn(forgets[0], svc, "Forget") {
		t.Errorf("ForgetConfig in the %s.Deps literal is not %s.Forget: the provisioning forgets nothing of the service that reads", prov, svc)
	}

	// Piece 6: the routes are in the table.
	found := false
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "routeTable" {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok && isSelectorCall(call, handlerPkg, "ConfigDriftRoutes") {
				found = true
			}
			return true
		})
	}
	if !found {
		t.Errorf("routeTable does not call %s.ConfigDriftRoutes: the routes of the drift watch would be answered with 404", handlerPkg)
	}
}

// importName is the name main.go gives an import.
func importName(t *testing.T, f *ast.File, path, file string) string {
	t.Helper()
	for _, imp := range f.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil || p != path {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		for i := len(p) - 1; i >= 0; i-- {
			if p[i] == '/' {
				return p[i+1:]
			}
		}
		return p
	}
	t.Fatalf("%s does not import %s", file, path)
	return ""
}

// isSelectorCall is a call of pkg.name(...).
func isSelectorCall(call *ast.CallExpr, pkg, name string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == pkg
}

// isMethodCall is a call of x.name(...) where e is that call.
func isMethodCall(e ast.Expr, x, name string) bool {
	call, ok := e.(*ast.CallExpr)
	return ok && isSelectorCall(call, x, name)
}

// isSelectorName is a selector expression ending in name, on anything.
func isSelectorName(e ast.Expr, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == name
}

// isSelectorOn is the selector x.name, without a call.
func isSelectorOn(e ast.Expr, x, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == x
}

// callsNamed is every call of a method or function with the name.
func callsNamed(f *ast.File, name string) []*ast.CallExpr {
	var out []*ast.CallExpr
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == name {
				out = append(out, call)
			}
		}
		return true
	})
	return out
}

// field is the value a composite literal gives to the key, or nil.
func field(lit *ast.CompositeLit, key string) ast.Expr {
	for _, el := range lit.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if id, ok := kv.Key.(*ast.Ident); ok && id.Name == key {
			return kv.Value
		}
	}
	return nil
}

// literalFields is the value of the key in every literal of the type pkg.typ.
func literalFields(f *ast.File, pkg, typ, key string) []ast.Expr {
	var out []ast.Expr
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		sel, ok := lit.Type.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != typ {
			return true
		}
		if x, ok := sel.X.(*ast.Ident); !ok || x.Name != pkg {
			return true
		}
		if v := field(lit, key); v != nil {
			out = append(out, v)
		}
		return true
	})
	return out
}

// derivedFromBackground says whether the variable is assigned the context of
// ctxPkg.WithCancel(ctxPkg.Background()) and of nothing else: no deadline, no
// timeout, no request in its ancestry.
func derivedFromBackground(f *ast.File, name, ctxPkg string) bool {
	ok := false
	ast.Inspect(f, func(n ast.Node) bool {
		as, isAssign := n.(*ast.AssignStmt)
		if !isAssign || len(as.Rhs) != 1 || len(as.Lhs) == 0 {
			return true
		}
		id, isIdent := as.Lhs[0].(*ast.Ident)
		if !isIdent || id.Name != name {
			return true
		}
		call, isCall := as.Rhs[0].(*ast.CallExpr)
		if !isCall || !isSelectorCall(call, ctxPkg, "WithCancel") || len(call.Args) != 1 {
			return true
		}
		if parent, isCall := call.Args[0].(*ast.CallExpr); isCall && isSelectorCall(parent, ctxPkg, "Background") {
			ok = true
		}
		return true
	})
	return ok
}
