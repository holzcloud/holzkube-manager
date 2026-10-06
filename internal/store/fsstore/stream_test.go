package fsstore_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/store/fsstore"
)

func TestWriteStreamAtomicPlacesTheWholeFileAt0600(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "sub", "a.snapshot")
	n, err := fsstore.WriteStreamAtomic(path, func(w io.Writer) (int64, error) {
		k, err := io.WriteString(w, "hello etcd")
		return int64(k), err
	})
	if err != nil || n != 10 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 || fi.Size() != 10 {
		t.Fatalf("mode %v size %d", fi.Mode().Perm(), fi.Size())
	}
}

// A stream that fails leaves neither the file nor the temporary: what is under
// the name is only ever a whole file.
func TestWriteStreamAtomicLeavesNothingWhenTheStreamFails(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	boom := errors.New("reset")
	_, err := fsstore.WriteStreamAtomic(filepath.Join(dir, "a.snapshot"), func(w io.Writer) (int64, error) {
		_, _ = io.WriteString(w, "half")
		return 4, boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("left behind: %v", entries)
	}
}

func TestListDirOfAMissingDirectoryIsEmptyAndRemoveFileToleratesGone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	got, err := fsstore.ListDir(filepath.Join(dir, "nope"))
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v err %v", got, err)
	}
	if err := fsstore.RemoveFile(filepath.Join(dir, "nope")); err != nil {
		t.Fatalf("removing a file that is gone: %v", err)
	}
}
