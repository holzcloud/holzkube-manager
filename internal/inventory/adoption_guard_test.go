package inventory_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
)

// generatorSymbols are the ways a fresh secrets bundle can come into
// existence. They are spelled out rather than imported, because importing them
// here would be the very thing this test forbids elsewhere.
var generatorSymbols = []string{"NewBundle", "NewBundleFromKubernetesPKI", "NewInput"}

// TestTheAdoptionPathCannotGenerateSecrets is D-07, held by a test rather than
// by discipline.
//
// The failure it prevents is the worst one this phase can produce. A "generate
// if the derivation failed" fallback creates a cluster record with fresh CA
// material: it looks exactly like the real cluster, is accepted everywhere in
// the UI, and cannot enter a single one of that cluster's nodes. Every later
// phase that generates a joining configuration would then produce a
// configuration no node accepts, and the first sign of it would be a
// provisioning run that never joins.
//
// The check is on the adoption source itself rather than on the package,
// because a later phase will add a create path that legitimately generates --
// and this test must keep passing then, saying what it has always said: the
// adoption path cannot.
func TestTheAdoptionPathCannotGenerateSecrets(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"import.go", "secrets.go"} {
		src, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}

		file, err := parser.ParseFile(token.NewFileSet(), name, src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			for _, sym := range generatorSymbols {
				if sel.Sel.Name == sym {
					t.Errorf("%s calls %s, which mints fresh cluster PKI. "+
						"An adoption that generates its own certificate authority produces a cluster "+
						"that looks right and cannot open a single one of its nodes (D-07).",
						name, sym)
				}
			}
			return true
		})
	}
}
