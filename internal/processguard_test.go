package internal

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// processRule is what TestTheDaemonStartsNoProcess protects, said once for
// every failure message.
const processRule = "holzkube-manager never carries out a host action itself: the daemon " +
	"writes one order file and the root-owned helper (deploy/holzkube-manager-host.sh) " +
	"carries it out. SystemCallFilter=@system-service in the reference unit does not " +
	"block exec -- it includes @process -- so this test is the only thing that keeps " +
	"the daemon from starting a process (Phase 13, success criterion 2, D-11)"

// processGuardExemptDirs are the simulators (D-11). They are safe to leave
// out: TestSimulatorIsNotInTheProduct keeps both out of the daemon binary, and
// the go-list half of TestTheDaemonStartsNoProcess would see their imports if
// they ever got in anyway.
var processGuardExemptDirs = []string{
	filepath.Join("internal", "talossim"),
	filepath.Join("internal", "kubesim"),
}

// processStartSelectors are the calls that start or become another program,
// by import path and name.
var processStartSelectors = map[string]map[string]bool{
	"os":                    {"StartProcess": true},
	"syscall":               {"ForkExec": true, "Exec": true, "StartProcess": true},
	"golang.org/x/sys/unix": {"Exec": true},
}

// execSyscallNames are the raw system call numbers of execve, reachable from
// any package that exports them (syscall, x/sys/unix) through Syscall.
var execSyscallNames = map[string]bool{"SYS_EXECVE": true, "SYS_EXECVEAT": true}

// TestTheDaemonStartsNoProcess: no path from the daemon to a new process.
//
// It is two halves because each misses what the other sees. `go list` sees
// the daemon's real import graph, but only imports: a syscall.ForkExec needs
// no os/exec. The AST scan sees every call form, but only in this module's
// sources. And os/exec is in the binary already through four third-party
// packages (pkg/browser, cni's invoke, gnostic's compiler, client-go's exec
// auth -- none reachable from anything the daemon calls), so the go-list half
// asks only about this module's own packages; a membership test on the whole
// list would be red on day one (RESEARCH R1).
//
// Faults injected and seen red, each as a new non-test file in internal/host:
// exec.Command("systemctl", "reboot").Run() -- both halves red (F7); a
// syscall.ForkExec with no os/exec import -- the AST half red, the go-list
// half green (F8), which is why the AST half exists.
func TestTheDaemonStartsNoProcess(t *testing.T) {
	t.Run("own packages in the daemon import no os/exec", func(t *testing.T) {
		lines := goList(t, "-deps", "-f", `{{.ImportPath}} {{join .Imports " "}}`, "./cmd/holzkube-managerd")
		own := 0
		for _, line := range lines {
			f := strings.Fields(line)
			if len(f) == 0 || !strings.HasPrefix(f[0], rootModule+"/") {
				continue
			}
			own++
			for _, imp := range f[1:] {
				if imp == "os/exec" {
					t.Errorf("%s imports os/exec and is part of cmd/holzkube-managerd.\n\n%s.", f[0], processRule)
				}
			}
		}
		// The daemon's own packages number in the dozens; a handful means the
		// template stopped printing what this loop reads.
		if own < 15 {
			t.Fatalf("only %d of this module's packages were found among the daemon's dependencies; the go list output is not what this guard reads", own)
		}
	})

	t.Run("no source starts a process", func(t *testing.T) {
		root := repoRoot(t)
		var violations []string
		scanned := 0
		for _, top := range []string{"cmd", "internal"} {
			err := filepath.WalkDir(filepath.Join(root, top), func(p string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				rel, err := filepath.Rel(root, p)
				if err != nil {
					return err
				}
				if d.IsDir() {
					for _, ex := range processGuardExemptDirs {
						if rel == ex {
							return filepath.SkipDir
						}
					}
					return nil
				}
				if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
					return nil
				}
				fset := token.NewFileSet()
				file, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
				if err != nil {
					return fmt.Errorf("parse %s: %w", rel, err)
				}
				scanned++
				for _, v := range processStarts(fset, file) {
					violations = append(violations, rel+":"+v)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("walk %s: %v", top, err)
			}
		}
		if scanned < 15 {
			t.Fatalf("only %d source files were scanned; the walk is not seeing the module", scanned)
		}
		if len(violations) > 0 {
			t.Errorf("%d way(s) to start a process in the daemon's sources:\n  %s\n\n%s.",
				len(violations), strings.Join(violations, "\n  "), processRule)
		}
		t.Logf("%d non-test source files scanned", scanned)
	})
}

// processStarts reports every way file has to start a process: the os/exec
// import under any name, the calls in processStartSelectors under any import
// name, the execve system call numbers, and a dot import of a package that
// holds such a call (it would hide the call from the selector check).
func processStarts(fset *token.FileSet, file *ast.File) []string {
	var out []string
	report := func(pos token.Pos, what string) {
		out = append(out, fmt.Sprintf("%d: %s", fset.Position(pos).Line, what))
	}

	// Local name -> import path, so an aliased `sc "syscall"` is still syscall.
	local := map[string]string{}
	for _, spec := range file.Imports {
		p, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := path.Base(p)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		switch {
		case p == "os/exec":
			report(spec.Pos(), `imports "os/exec"`)
		case name == "." && processStartSelectors[p] != nil:
			report(spec.Pos(), fmt.Sprintf("dot-imports %q, which hides its process calls from this guard", p))
		}
		local[name] = p
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			if pkg, ok := x.X.(*ast.Ident); ok {
				if names := processStartSelectors[local[pkg.Name]]; names[x.Sel.Name] {
					report(x.Pos(), fmt.Sprintf("uses %s.%s", local[pkg.Name], x.Sel.Name))
				}
			}
		case *ast.Ident:
			// Every SYS_EXECVE, selected from a package or not: the selector's
			// Sel is an Ident too, so this one case covers both.
			if execSyscallNames[x.Name] {
				report(x.Pos(), "names "+x.Name)
			}
		}
		return true
	})
	return out
}

// TestProcessGuardRecognisesAProcessStart is the negative control: a guard
// that finds nothing proves nothing until it has found something. Each source
// here starts a process one way, and the classifier must name it; the last
// one starts none and uses the same words harmlessly.
func TestProcessGuardRecognisesAProcessStart(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string // substring of the one report; "" means none
	}{
		{"os/exec", `package p
import "os/exec"
func f() { _ = exec.Command("systemctl", "reboot").Run() }`, `imports "os/exec"`},
		{"os/exec under another name", `package p
import run "os/exec"
func f() { _ = run.Command("reboot") }`, `imports "os/exec"`},
		{"os.StartProcess", `package p
import "os"
func f() { _, _ = os.StartProcess("/usr/bin/systemctl", nil, nil) }`, "uses os.StartProcess"},
		{"syscall.ForkExec", `package p
import "syscall"
func f() { _, _ = syscall.ForkExec("/usr/bin/systemctl", nil, nil) }`, "uses syscall.ForkExec"},
		{"syscall.ForkExec under another name", `package p
import sc "syscall"
func f() { _, _ = sc.ForkExec("/usr/bin/systemctl", nil, nil) }`, "uses syscall.ForkExec"},
		{"syscall.Exec", `package p
import "syscall"
func f() { _ = syscall.Exec("/usr/bin/systemctl", nil, nil) }`, "uses syscall.Exec"},
		{"syscall.StartProcess", `package p
import "syscall"
func f() { _, _, _ = syscall.StartProcess("/usr/bin/systemctl", nil, nil) }`, "uses syscall.StartProcess"},
		{"unix.Exec", `package p
import "golang.org/x/sys/unix"
func f() { _ = unix.Exec("/usr/bin/systemctl", nil, nil) }`, "uses golang.org/x/sys/unix.Exec"},
		{"a method value of os.StartProcess", `package p
import "os"
var start = os.StartProcess`, "uses os.StartProcess"},
		{"unix.SYS_EXECVE", `package p
import "golang.org/x/sys/unix"
func f() { unix.Syscall(unix.SYS_EXECVE, 0, 0, 0) }`, "names SYS_EXECVE"},
		{"syscall.SYS_EXECVEAT", `package p
import "syscall"
func f() { syscall.Syscall6(syscall.SYS_EXECVEAT, 0, 0, 0, 0, 0, 0) }`, "names SYS_EXECVEAT"},
		{"a dot import of syscall", `package p
import . "syscall"
func f() { _, _ = ForkExec("/usr/bin/systemctl", nil, nil) }`, `dot-imports "syscall"`},
		{"clean", `package p
import (
	"database/sql"
	"os"
	"syscall"
)
func f(db *sql.DB) {
	_, _ = db.Exec("select 1")
	_ = os.Getenv("HOME")
	_ = syscall.Getpid()
	exec := "a word, not a package"
	_ = exec
}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, tc.name+".go", tc.src, parser.SkipObjectResolution)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			got := processStarts(fset, file)
			if tc.want == "" {
				if len(got) != 0 {
					t.Errorf("a clean file was reported: %q", got)
				}
				return
			}
			if len(got) == 0 {
				t.Fatalf("not recognised: the guard would let %s through", tc.name)
			}
			found := false
			for _, g := range got {
				if strings.Contains(g, tc.want) {
					found = true
				}
			}
			if !found {
				t.Errorf("reports %q, want one containing %q", got, tc.want)
			}
		})
	}
}
