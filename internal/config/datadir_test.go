package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func envFrom(m map[string]string) Lookup {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

// D-02: $XDG_DATA_HOME/holzkube, falling back to ~/.local/share/holzkube, with
// an explicit override winning over both. The override is also the Docker
// volume path, so it has to beat an environment that a base image may set.
func TestResolvePrecedence(t *testing.T) {
	home := "/home/op"

	cases := []struct {
		name     string
		env      map[string]string
		override string
		want     string
	}{
		{
			name: "falls back to the home directory",
			env:  map[string]string{},
			want: filepath.Join(home, ".local", "share", "holzkube"),
		},
		{
			name: "XDG_DATA_HOME moves it",
			env:  map[string]string{"XDG_DATA_HOME": "/xdg"},
			want: filepath.Join("/xdg", "holzkube"),
		},
		{
			name: "an empty XDG_DATA_HOME is treated as unset",
			env:  map[string]string{"XDG_DATA_HOME": ""},
			want: filepath.Join(home, ".local", "share", "holzkube"),
		},
		{
			name:     "the override beats XDG_DATA_HOME",
			env:      map[string]string{"XDG_DATA_HOME": "/xdg"},
			override: "/volume/holzkube",
			want:     "/volume/holzkube",
		},
		{
			name:     "the override beats the home fallback",
			env:      map[string]string{},
			override: "/volume/holzkube",
			want:     "/volume/holzkube",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve(envFrom(tc.env), home, tc.override)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got != tc.want {
				t.Fatalf("Resolve = %q, want %q", got, tc.want)
			}
		})
	}
}

// Without XDG_DATA_HOME, without a home directory and without an override there
// is nothing to guess. Guessing here would put cluster PKI somewhere the
// operator never looks.
func TestResolveWithoutAnyBaseFails(t *testing.T) {
	_, err := Resolve(envFrom(nil), "", "")
	if err == nil {
		t.Fatal("Resolve with no base returned no error")
	}
	for _, want := range []string{"XDG_DATA_HOME", "data-dir"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestEnsureDirCreatesWith0700(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on windows")
	}
	base := t.TempDir()
	dir := filepath.Join(base, "nested", "holzkube")

	if err := EnsureDir(dir); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != DirPerm {
		t.Fatalf("mode = %04o, want %04o", got, DirPerm)
	}
}

// An existing directory is left exactly as it is: the permission guard belongs
// to the store (plan 02), which refuses to start on a wrong mode. Quietly
// chmod-ing it here would repair the very condition the guard exists to report.
func TestEnsureDirLeavesAnExistingDirectoryAlone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on windows")
	}
	dir := filepath.Join(t.TempDir(), "holzkube")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := EnsureDir(dir); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("mode = %04o, want it untouched at 0755", got)
	}
}

func TestEnsureDirRefusesAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "holzkube")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	err := EnsureDir(path)
	if err == nil {
		t.Fatal("EnsureDir on a regular file returned no error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name the path", err)
	}
}
