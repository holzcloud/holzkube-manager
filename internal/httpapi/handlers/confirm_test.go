package handlers

// An internal test, because what it walks is the table itself. The behaviour
// it guards -- that no confirmable action silently defaults to "no typing
// needed" -- cannot be checked from outside without a live server per action,
// and the point is to catch the omission at the table rather than at the
// fifth route somebody adds.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/host/hostaction"
	"github.com/holzcloud/holzkube-manager/internal/model"
)

// TestEveryConfirmableActionDecidesOnTypedPhrase is the guard the missing rule
// needed.
//
// node.remove-from-cluster shipped in phase 9 with a confirmation token, a
// destructive marking and a dialog that asked the operator to type the
// hostname -- and a server that issued the token to anybody who asked without
// one, because the rule was `if action == node.reset` and nobody widened it.
// The browser was the only thing enforcing it.
//
// So the question is asked of the table rather than of a condition: an action
// that is not in it gets no token at all.
func TestEveryConfirmableActionDecidesOnTypedPhrase(t *testing.T) {
	t.Parallel()

	want := map[string]bool{
		string(model.JobReboot):   false,
		string(model.JobShutdown): false,
		string(model.JobReset):    true,
		ActionRemoveFromCluster:   true,
	}

	for action, needs := range want {
		got, known := typedPhrase[action]
		if !known {
			t.Errorf("%s has no entry in typedPhrase, so the confirm route refuses it outright. "+
				"That is the safe direction and it is still a bug: the action exists and cannot "+
				"be confirmed", action)
			continue
		}
		if got != needs {
			t.Errorf("typedPhrase[%s] = %v, want %v", action, got, needs)
		}
	}

	for action := range typedPhrase {
		if _, ok := want[action]; !ok {
			t.Errorf("typedPhrase carries %q, which this test does not know about. A new "+
				"confirmable action needs a line here saying whether typing the hostname is "+
				"required and why -- which is the decision this table exists to force", action)
		}
	}
}

// TestEveryHostActionRequiresTyping holds the host confirm route's own table
// (Phase 13, D-09).
//
// Every host action requires typing the hostname -- unlike a node's reboot and
// shutdown -- because there is exactly one host and it is the machine this page
// runs on: after the click it is gone. The check, which takes nothing away,
// requires it too: HACT-05 wants it for every host action. So hostTypedPhrase has exactly one entry
// per hostaction.Actions(), and every one is true.
//
// And it is a separate table for a reason this test holds too: typedPhrase is
// the node confirm route's, and a host action there would make
// POST /api/v1/machines/{id}/confirm issue host-action tokens.
func TestEveryHostActionRequiresTyping(t *testing.T) {
	t.Parallel()

	for _, a := range hostaction.Actions() {
		name := hostActionName(a)
		needs, known := hostTypedPhrase[name]
		if !known {
			t.Errorf("%s has no entry in hostTypedPhrase, so the host confirm route refuses it and "+
				"its button can never work", name)
			continue
		}
		if !needs {
			t.Errorf("hostTypedPhrase[%s] = false: a host action without the typed hostname. Every "+
				"host action requires it (D-09, HACT-05)", name)
		}
	}
	if got, want := len(hostTypedPhrase), len(hostaction.Actions()); got != want {
		t.Errorf("hostTypedPhrase has %d entries and there are %d host actions; an entry for no "+
			"action is a token the host confirm route issues for nothing", got, want)
	}

	for action := range typedPhrase {
		if strings.HasPrefix(action, "host.") {
			t.Errorf("typedPhrase carries %q: the node confirm route would issue a token for a "+
				"host action. Host actions belong in hostTypedPhrase alone", action)
		}
		if _, ok := hostTypedPhrase[action]; ok {
			t.Errorf("%q is in both typedPhrase and hostTypedPhrase", action)
		}
	}
}

// TestEveryConfirmedActionIsInTheTable is the half a hand-written list cannot
// hold on its own.
//
// The table above says what the confirm route will issue. This reads the
// handler sources for the actions that are *checked* -- every
// Confirmer.Check whose intent names a machine-scoped action -- and asserts the
// two sets agree. An action checked somewhere and absent here is an action
// whose confirmation dialog cannot produce a valid token; one in the table and
// checked nowhere is a token that authorises nothing.
func TestEveryConfirmedActionIsInTheTable(t *testing.T) {
	t.Parallel()

	// The files are walked one at a time rather than through parser.ParseDir,
	// which Go 1.26 deprecates: a directory of .go files is not a package and
	// the helper had to guess which one it was looking at. Here there is only
	// one package and the test files are the ones to skip, which is a rule this
	// loop can state.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read the handler package directory: %v", err)
	}

	fset := token.NewFileSet()
	checked := map[string]bool{}
	// hostChecks counts the Check call sites in host.go on their own. Their
	// action is `hostActionName(a)`, which actionValue cannot resolve, so
	// without this count a host action route that stopped checking its token
	// would leave this test exactly as green as before.
	hostChecks := 0

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}

		{
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				// CheckOnce is Check that spends the token (the host routes).
				if !ok || (sel.Sel.Name != "Check" && sel.Sel.Name != "CheckOnce") {
					return true
				}
				if name == "host.go" {
					hostChecks++
				}
				// The second argument is the jobs.Intent literal; its Action
				// field is what the token has to have been issued for.
				for _, arg := range call.Args {
					lit, ok := arg.(*ast.CompositeLit)
					if !ok {
						continue
					}
					for _, el := range lit.Elts {
						kv, ok := el.(*ast.KeyValueExpr)
						if !ok {
							continue
						}
						if key, ok := kv.Key.(*ast.Ident); !ok || key.Name != "Action" {
							continue
						}
						if v := actionValue(kv.Value); v != "" {
							checked[v] = true
						}
					}
				}
				return true
			})
		}
	}

	if len(checked) == 0 {
		t.Fatal("no Confirmer.Check call sites were found, so this test proves nothing")
	}
	if hostChecks == 0 {
		t.Error("no Confirmer.Check call site was found in host.go: the host action routes place " +
			"an order without checking the confirmation token they are handed")
	}

	for action := range checked {
		// The provisioning and upgrade paths issue their tokens from their own
		// routes, with their own typed phrases -- the install disk and the
		// target version. They are machine- and cluster-scoped respectively
		// and do not go through the machine confirm route at all.
		if action == "node.provision" || strings.HasPrefix(action, "cluster.upgrade-") {
			continue
		}
		// A host action's token comes from the host confirm route, whose table
		// is hostTypedPhrase; typedPhrase must not know it.
		if strings.HasPrefix(action, "host.") {
			if _, ok := hostTypedPhrase[action]; !ok {
				t.Errorf("%s is checked against a confirmation token and has no entry in "+
					"hostTypedPhrase, so the host confirm route refuses to issue it", action)
			}
			continue
		}
		if _, ok := typedPhrase[action]; !ok {
			t.Errorf("%s is checked against a confirmation token and has no entry in typedPhrase, "+
				"so the route that issues that token refuses to issue it. The dialog for this "+
				"action cannot work", action)
		}
	}
}

// actionValue resolves the handful of expression shapes an Action field takes
// in this package: a string literal, a named constant, or a conversion of one.
//
// A call expression stays unresolved -- which is every host action site,
// `hostActionName(a)`. Those are covered by TestEveryHostActionRequiresTyping,
// which walks the actions themselves, and by the hostChecks count above, which
// notices a host route that stopped checking at all.
func actionValue(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			return strings.Trim(v.Value, `"`)
		}
	case *ast.Ident:
		switch v.Name {
		case "ActionRemoveFromCluster":
			return ActionRemoveFromCluster
		}
	case *ast.CallExpr:
		// string(kind) and the like: the value is not knowable statically, and
		// those sites are the upgrade kinds the loop above skips by name.
		return ""
	case *ast.SelectorExpr:
		return ""
	}
	return ""
}
