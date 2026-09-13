package httpapi_test

// Every problem code a client can receive has to be in the contract.
//
// docs/api-contract.md is what a client reads: the taxonomy's own rule is that
// the types are closed and the codes are minted deliberately, in the same
// commit as the route that emits them. A code that exists in Go and not in the
// document is a code a client can be handed and cannot look up -- and two of
// them had accumulated that way before this test existed.

import (
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryProblemCodeIsInTheContract reads the constants rather than a list.
//
// A hand-kept list here would have exactly the failure mode it is meant to
// catch: somebody adds a code, does not add the row, and does not add the list
// entry either. The constants are the source, the document is the claim, and
// the test is the comparison.
func TestEveryProblemCodeIsInTheContract(t *testing.T) {
	t.Parallel()

	contract, err := os.ReadFile(filepath.Join("..", "..", "docs", "api-contract.md"))
	if err != nil {
		t.Fatalf("read the API contract: %v", err)
	}
	doc := string(contract)

	codes := problemCodes(t)
	if len(codes) == 0 {
		t.Fatal("no problem codes were found, so this test proves nothing")
	}

	var missing []string
	for name, value := range codes {
		if !strings.Contains(doc, value) {
			missing = append(missing, name+" ("+value+")")
		}
	}

	if len(missing) > 0 {
		t.Errorf("these problem codes are not mentioned anywhere in docs/api-contract.md:\n  %s\n\n"+
			"The taxonomy's rule is that codes are minted deliberately and the document is where a "+
			"client reads them. A code that exists only in Go is one somebody can be handed and "+
			"cannot look up.", strings.Join(missing, "\n  "))
	}
}

// TestEveryProblemCodeIsEmitted is the other direction.
//
// A code nothing produces is a row in a contract that describes a response
// nobody will ever receive, and it is indistinguishable in review from a code
// whose route was dropped.
func TestEveryProblemCodeIsEmitted(t *testing.T) {
	t.Parallel()

	// Identifiers rather than text, and the difference is the whole test.
	//
	// Two earlier versions were wrong in opposite directions. Skipping
	// problem.go reported CodeClusterLocked as dead -- it is emitted by
	// ClusterLocked(), a constructor in the same file. Counting occurrences in
	// the raw text instead made the test vacuous: every constant here carries a
	// doc comment that opens with its own name, so the count reached two for a
	// code nothing used. A deliberately dead constant was added to check, and
	// the test passed.
	//
	// Tokenising answers both: a name in a comment is not an identifier, and a
	// use anywhere -- including the declaring file -- is.
	uses := identifierUses(t, filepath.Join("..", ".."))

	var dead []string
	for name := range problemCodes(t) {
		if _, exempt := emittedElsewhere[name]; exempt {
			continue
		}
		// Once is the declaration itself.
		if uses[name] < 2 {
			dead = append(dead, name)
		}
	}

	if len(dead) > 0 {
		t.Errorf("these problem codes are declared and never used:\n  %s\n\n"+
			"Either something should be producing them, or they are a route that was dropped and "+
			"left its vocabulary behind.", strings.Join(dead, "\n  "))
	}
}

// emittedElsewhere names the codes this package declares and another package
// produces by value, with the reason.
//
// It is two, and they share one: internal/talos sits below HTTP, so importing
// internal/httpapi from it would invert the layering -- the shape that becomes
// an import cycle the first time the taxonomy needs anything from the seam. It
// therefore spells these two as its own unexported literals, and the
// duplication is held honest from the other side: internal/talos/errors_test.go
// imports this package and asserts the values are equal by identifier.
//
// So the constants here are named only by that test, which is exactly the
// pattern this guard reports -- and reporting it is right. What makes these two
// acceptable is the argument, not the shape.
var emittedElsewhere = map[string]string{
	"CodeUpstreamNodeUnreachable": "internal/talos spells it as codeNodeUnreachable; " +
		"errors_test.go asserts the two are equal",
	"CodeUpstreamNodeTimeout": "internal/talos spells it as codeNodeTimeout; " +
		"errors_test.go asserts the two are equal",
}

// TestNoStaleCodeExemptions keeps that list honest in the other direction.
func TestNoStaleCodeExemptions(t *testing.T) {
	t.Parallel()

	codes := problemCodes(t)
	uses := identifierUses(t, filepath.Join("..", ".."))

	for name, reason := range emittedElsewhere {
		if _, ok := codes[name]; !ok {
			t.Errorf("emittedElsewhere names %q, which is not a problem code", name)
			continue
		}
		if strings.TrimSpace(reason) == "" {
			t.Errorf("%q is exempted with no reason", name)
		}
		if uses[name] >= 2 {
			t.Errorf("%q is exempted as emitted elsewhere and is now used in this repository's "+
				"own sources. The exemption is stale", name)
		}
	}
}

// problemCodes reads the Code* constants out of problem.go.
func problemCodes(t *testing.T) map[string]string {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), "problem.go", nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse problem.go: %v", err)
	}

	out := map[string]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range spec.Names {
			if !strings.HasPrefix(name.Name, "Code") || i >= len(spec.Values) {
				continue
			}
			lit, ok := spec.Values[i].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			out[name.Name] = strings.Trim(lit.Value, `"`)
		}
		return true
	})
	return out
}

// identifierUses counts every identifier token in the repository's Go sources,
// excluding tests.
//
// Skipping the tests matters for the same reason it matters in
// cmd/holzkube-managerd's reachability guard: a code only a test names is a
// code nothing emits, which is the finding rather than the exemption.
func identifierUses(t *testing.T, root string) map[string]int {
	t.Helper()

	uses := map[string]int{}

	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			base := filepath.Base(path)
			if d.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(base, "_test.go") {
				return nil
			}

			raw, err := os.ReadFile(path) //nolint:gosec // a path this test walked itself
			if err != nil {
				return err
			}

			fset := token.NewFileSet()
			var sc scanner.Scanner
			sc.Init(fset.AddFile(path, fset.Base(), len(raw)), raw, nil, 0)
			for {
				_, tok, lit := sc.Scan()
				if tok == token.EOF {
					break
				}
				if tok == token.IDENT {
					uses[lit]++
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	return uses
}
