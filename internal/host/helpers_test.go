package host

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeSys answers the four syscalls from its fields. A nil error field means
// the call succeeds with the value beside it.
type fakeSys struct {
	uname     Uname
	unameErr  error
	boot      time.Duration
	bootErr   error
	loads     Loads
	loadsErr  error
	statfs    map[string]FSStats
	statfsErr map[string]error
}

func (f fakeSys) Uname() (Uname, error) {
	if f.unameErr != nil {
		return Uname{}, f.unameErr
	}
	return f.uname, nil
}

func (f fakeSys) BootTime() (time.Duration, error) {
	if f.bootErr != nil {
		return 0, f.bootErr
	}
	return f.boot, nil
}

func (f fakeSys) Loads() (Loads, error) {
	if f.loadsErr != nil {
		return Loads{}, f.loadsErr
	}
	return f.loads, nil
}

func (f fakeSys) Statfs(path string) (FSStats, error) {
	if err := f.statfsErr[path]; err != nil {
		return FSStats{}, err
	}
	s, ok := f.statfs[path]
	if !ok {
		return FSStats{}, fs.ErrNotExist
	}
	return s, nil
}

// fixtureFS opens testdata/<name> as the root a Collector reads.
//
// Through os.OpenRoot rather than os.DirFS, deliberately: a fixture symlink that
// escapes the tree then fails instead of quietly reading this machine's real
// /sys -- which would make a fixture test pass or fail by where it ran.
func fixtureFS(t *testing.T, name string) fs.FS {
	t.Helper()
	root, err := os.OpenRoot(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("open fixture %s: %v", name, err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root.FS()
}

// marshalView is the View as a browser receives it, decoded generically so a
// test sees the keys and not the Go types.
func marshalView(t *testing.T, v View) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal view: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode view: %v (%s)", err, raw)
	}
	return out
}
