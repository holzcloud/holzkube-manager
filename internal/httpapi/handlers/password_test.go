package handlers

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryPasswordCheckAsksWhetherTheAccountHasOne.
//
// A service account has no password: only a SHA-256 token hash, and
// PasswordHash empty. Handed that empty string, auth.Verify does not return
// false -- an empty hash is undecodable rather than wrong -- it returns an
// error, and an error on a password path becomes 500 internal.unexpected with
// a log line about an unexpected internal condition. There is nothing
// unexpected about it, and the product has had a sentence for it
// (auth.ErrNotAPerson) since service accounts shipped.
//
// Both password-taking routes were reachable by a service account and both
// answered 500. They need only RoleReader, and a bearer token satisfies the
// CSRF check and the sudo window, so every gate in front of them hands the
// request through. Fixing the two is not the answer: the next route that takes
// a password is the one that will not look.
//
// So this walks the package's own source. Every function that calls
// auth.Verify must also ask whether the account it is verifying is a person.
// It is a source-level guard, like the file-access scan in
// internal/store/fsstore, and it is deliberately crude: it does not check the
// order or the branch, only that the question is asked in the same function.
// A guard that tried to prove the control flow would be a small static
// analyser nobody maintains; this one is three rules and it fails for the
// right reason.
func TestEveryPasswordCheckAsksWhetherTheAccountHasOne(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}

	var offenders []string
	scanned, verifiers := 0, 0

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		scanned++

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}

			verifies, asks := false, false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if sel.Sel.Name == "IsService" {
					asks = true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if ok && pkg.Name == "auth" && sel.Sel.Name == "Verify" {
					verifies = true
				}
				return true
			})

			if verifies {
				verifiers++
				if !asks {
					offenders = append(offenders,
						name+":"+itoa(fset.Position(fn.Pos()).Line)+": "+fn.Name.Name)
				}
			}
		}
	}

	// A guard that scans nothing passes for the wrong reason, and a guard that
	// finds no password check is a guard watching a package that has stopped
	// having one -- which is a thing to notice, not a pass.
	if scanned < 5 {
		t.Fatalf("scanned only %d source files; the guard is not reading the package", scanned)
	}
	if verifiers == 0 {
		t.Fatal("no function in this package calls auth.Verify. Either the password path moved " +
			"and this guard should move with it, or it stopped existing and that is worth saying " +
			"out loud rather than passing quietly.")
	}

	if len(offenders) > 0 {
		t.Fatalf("these functions check a password without asking whether the account has one:\n  %s\n\n"+
			"A service account's PasswordHash is empty, and auth.Verify against an empty hash "+
			"returns an error rather than false -- which becomes 500 internal.unexpected for a "+
			"request that has a good answer. Refuse with notAPerson() first.",
			strings.Join(offenders, "\n  "))
	}
}
