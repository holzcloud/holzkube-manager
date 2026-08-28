package fsstore

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPermissionGuardAcceptsACleanDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "users"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "users", "a.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := Guard(dir); err != nil {
		t.Fatalf("Guard on a 0700 directory of 0600 files: %v", err)
	}
}

func TestPermissionGuardRejectsAWideOpenDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	err := Guard(dir)
	if err == nil {
		t.Fatal("Guard accepted a 0755 data directory; it holds the cluster PKI")
	}
	msg := err.Error()
	if !strings.Contains(msg, dir) {
		t.Fatalf("error %q does not name the directory %q", msg, dir)
	}
	if !strings.Contains(msg, "0755") {
		t.Fatalf("error %q does not report the mode found", msg)
	}
	if !strings.Contains(msg, "0700") {
		t.Fatalf("error %q does not report the mode required", msg)
	}
}

func TestPermissionGuardNamesAWideOpenFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "users"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	leaky := filepath.Join(dir, "users", "leaky.json")
	if err := os.WriteFile(leaky, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	err := Guard(dir)
	if err == nil {
		t.Fatal("Guard accepted a 0644 state file")
	}
	if !strings.Contains(err.Error(), leaky) {
		t.Fatalf("error %q does not name the offending file %q", err, leaky)
	}
	if !strings.Contains(err.Error(), "0600") {
		t.Fatalf("error %q does not report the mode required for a file", err)
	}
}

func TestPermissionGuardReportsEveryViolationAtOnce(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	for _, sub := range []string{"users", "sessions"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	bad := []string{
		filepath.Join(dir, "users", "one.json"),
		filepath.Join(dir, "sessions", "two.json"),
		filepath.Join(dir, "settings.json"),
	}
	for _, p := range bad {
		if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}

	err := Guard(dir)
	if err == nil {
		t.Fatal("Guard accepted three 0644 state files")
	}
	for _, p := range bad {
		if !strings.Contains(err.Error(), p) {
			t.Fatalf("error does not name %q; the operator should repair once, not start seven times:\n%v", p, err)
		}
	}
}

func TestPermissionGuardCoversBackupsAndTheLockFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "backups"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	tarball := filepath.Join(dir, "backups", "pre-migration-0-to-1-x.tar.gz")
	if err := os.WriteFile(tarball, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	lock := filepath.Join(dir, "holzkube.lock")
	if err := os.WriteFile(lock, []byte("1\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	err := Guard(dir)
	if err == nil {
		t.Fatal("Guard ignored backups/ and the lock file; a backup is a verbatim copy of every secret")
	}
	if !strings.Contains(err.Error(), tarball) {
		t.Fatalf("error %q does not name the world-readable backup", err)
	}
	if !strings.Contains(err.Error(), lock) {
		t.Fatalf("error %q does not name the world-readable lock file", err)
	}
}

func TestOpenRefusesAWideOpenDataDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	s, err := Open(dir)
	if err == nil {
		_ = s.Close()
		t.Fatal("Open accepted a 0755 data directory")
	}
	if !strings.Contains(err.Error(), "0700") || !strings.Contains(err.Error(), dir) {
		t.Fatalf("error %q must name the path and the required mode", err)
	}

	// The refused start must release its process lock, or repairing the
	// permissions would still not let the operator in.
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("Open after repairing the permissions: %v", err)
	}
	_ = s2.Close()
}

// TestNoDirectFileAccessOutsideFsstore is the architecture guard.
//
// The store interface is entity-shaped and never path-shaped, and that
// property is only worth anything if nothing above it reaches around to the
// filesystem. A grep in a review catches that on the day someone looks; this
// test catches it on the day it is written.
//
// Exempt, and why:
//
//   - internal/store/... — the persistence layer itself. fsstore owns state
//     paths by definition, lock.go owns the flock file, and migrate/ plus
//     migrate/backup own VERSION and the tarball. They are the seam, not
//     callers of it. store.go is checked separately below and is not exempt.
//   - internal/tlsx — owns the certificate and key files. TLS material is not
//     a store record; it is read by crypto/tls before any store exists.
//   - internal/audit — the audit log is, by design, an append-only JSONL file
//     next to the store rather than a store entity. Routing an append-only
//     hash chain through a rev-CAS record store would mean rewriting the whole
//     log on every line.
//   - _test.go files — tests legitimately plant fixtures and inspect results.
func TestNoDirectFileAccessOutsideFsstore(t *testing.T) {
	root := filepath.Join("..", "..", "..")

	exemptDirs := []string{
		filepath.Join("internal", "store"),
		filepath.Join("internal", "tlsx"),
		filepath.Join("internal", "audit"),
	}

	// os functions that read, write, enumerate or move a file. MkdirAll and
	// Stat are absent on purpose: creating the data directory and asking
	// whether a path exists are not accesses to a state record, and the
	// composition root legitimately does the former before handing the
	// directory to the store.
	forbidden := map[string]bool{
		"ReadFile": true, "WriteFile": true, "OpenFile": true, "Open": true,
		"Create": true, "CreateTemp": true, "ReadDir": true, "Remove": true,
		"RemoveAll": true, "Rename": true, "Truncate": true, "Link": true,
		"Symlink": true, "Chmod": true, "Chown": true, "Readlink": true,
	}

	var violations []string
	scan := func(path string, exempt bool) {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
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
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if pkg.Name == "ioutil" || (pkg.Name == "os" && forbidden[sel.Sel.Name]) {
				if !exempt {
					violations = append(violations, fmtViolation(fset, path, call.Pos(), pkg.Name, sel.Sel.Name))
				}
			}
			return true
		})
	}

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
			for _, ex := range exemptDirs {
				if strings.HasPrefix(rel, ex+string(filepath.Separator)) {
					return nil
				}
			}
			scan(p, false)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", top, err)
		}
	}

	// The exemption above covers internal/store as a subtree, so assert
	// separately that the seam itself stays clean: store.go declares the
	// interface and must not touch a file.
	scan(filepath.Join(root, "internal", "store", "store.go"), false)

	if len(violations) > 0 {
		t.Fatalf("direct filesystem access outside the store implementation:\n  %s\n\n"+
			"Records are reached through store.Users()/Settings()/Sessions(), never through a path.",
			strings.Join(violations, "\n  "))
	}
}

func fmtViolation(fset *token.FileSet, path string, pos token.Pos, pkg, fn string) string {
	p := fset.Position(pos)
	return path + ":" + itoa(p.Line) + ": " + pkg + "." + fn
}
