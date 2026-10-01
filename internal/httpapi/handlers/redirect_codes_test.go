package handlers

// Every code a sign-in or a re-authentication can be sent back with has a
// sentence on the page and a row in the contract.
//
// These codes are not problem codes: they ride in the query string of a
// redirect (?sso_error=, ?sudo_error=), because the routes are navigations and
// a problem document would render as raw JSON in the address bar. So
// TestEveryProblemCodeIsInTheContract never saw them, and a new one could be
// minted that the page answers with "The sign-in did not complete." and the
// contract does not mention.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// redirectCodes reads oidc.go for the codes it can redirect with: the string
// literals handed to failSignIn and refuseCallback (sign-in) and to failSudo
// (re-authentication), and those bindCode returns, which go to failSignIn.
func redirectCodes(t *testing.T) (signIn, sudo []string) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "oidc.go", nil, 0)
	if err != nil {
		t.Fatalf("parse oidc.go: %v", err)
	}
	literal := func(e ast.Expr) (string, bool) {
		lit, ok := e.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return "", false
		}
		v, err := strconv.Unquote(lit.Value)
		return v, err == nil && v != ""
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			fn, ok := n.Fun.(*ast.Ident)
			if !ok || len(n.Args) == 0 {
				return true
			}
			code, ok := literal(n.Args[len(n.Args)-1])
			if !ok {
				return true
			}
			switch fn.Name {
			case "failSignIn", "refuseCallback":
				signIn = append(signIn, code)
			case "failSudo":
				sudo = append(sudo, code)
			}
		case *ast.FuncDecl:
			if n.Name.Name != "bindCode" {
				return true
			}
			ast.Inspect(n.Body, func(m ast.Node) bool {
				if ret, ok := m.(*ast.ReturnStmt); ok && len(ret.Results) == 1 {
					if code, ok := literal(ret.Results[0]); ok {
						signIn = append(signIn, code)
					}
				}
				return true
			})
			return false
		}
		return true
	})
	if len(signIn) == 0 || len(sudo) == 0 {
		t.Fatalf("found %d sign-in and %d sudo codes, so this test proves nothing", len(signIn), len(sudo))
	}
	return signIn, sudo
}

func readRepoFile(t *testing.T, parts ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(append([]string{"..", "..", ".."}, parts...)...))
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Join(parts...), err)
	}
	return string(b)
}

func TestEveryRedirectCodeIsExplainedAndInTheContract(t *testing.T) {
	t.Parallel()
	signIn, sudo := redirectCodes(t)
	contract := readRepoFile(t, "docs", "api-contract.md")
	login := readRepoFile(t, "web", "src", "routes", "login.tsx")
	resume := readRepoFile(t, "web", "src", "components", "ResumeAfterProvider.tsx")

	// A key of the page's code-to-sentence table, quoted or bare.
	isKey := func(page, code string) bool {
		return regexp.MustCompile(`(?m)^\s*'?` + regexp.QuoteMeta(code) + `'?:`).MatchString(page)
	}
	inContract := func(param, code string) bool {
		return regexp.MustCompile("`" + regexp.QuoteMeta(code) + "`").MatchString(contract) ||
			regexp.MustCompile(regexp.QuoteMeta(param+"="+code)+`\b`).MatchString(contract)
	}

	for _, code := range signIn {
		if !isKey(login, code) {
			t.Errorf("sso_error=%s has no sentence in web/src/routes/login.tsx", code)
		}
		if !inContract("sso_error", code) {
			t.Errorf("sso_error=%s is not in docs/api-contract.md", code)
		}
	}
	for _, code := range sudo {
		if !isKey(resume, code) {
			t.Errorf("sudo_error=%s has no sentence in web/src/components/ResumeAfterProvider.tsx", code)
		}
		if !inContract("sudo_error", code) {
			t.Errorf("sudo_error=%s is not in docs/api-contract.md", code)
		}
	}
}
