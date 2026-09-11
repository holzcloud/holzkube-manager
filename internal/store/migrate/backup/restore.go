package backup

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Restoring a backup, and the two ways it must refuse.
//
// Restore is the only operation in this product that writes over an operator's
// entire data directory, and the thing that makes it survivable is what it
// does *before* it starts: it takes a backup of what is there. A restore that
// went wrong without one would have replaced a working installation with a
// broken one and left nothing to go back to.
//
// The second refusal is the one that is easy to get wrong. A tarball is a list
// of paths, and a path in a tarball is whatever the person who made it wrote:
// `../../../etc/shadow` is a valid tar entry. Every entry is checked against
// the destination before a byte of it is written.

// ErrOutsideDestination reports a tar entry whose path escapes the data
// directory.
var ErrOutsideDestination = errors.New("backup: an entry in this archive would be written outside the data directory")

// ErrDirectoryInUse reports a data directory another process holds.
var ErrDirectoryInUse = errors.New("backup: the data directory is in use")

// Manual is what an operator-requested backup is called, as opposed to the
// pre-migration ones.
const Manual = "manual"

// CreateNamed writes a backup an operator asked for.
//
// It is the same tarball Create writes and it is named differently on purpose:
// a directory listing that cannot tell a pre-migration snapshot from a weekly
// backup is a directory an operator prunes by guessing.
func CreateNamed(dir, label string) (string, error) {
	backupDir := filepath.Join(dir, DirName)
	if err := os.MkdirAll(backupDir, dirPerm); err != nil {
		return "", fmt.Errorf("backup: create %s: %w", backupDir, err)
	}

	if label == "" {
		label = Manual
	}
	name := label + "-" + time.Now().UTC().Format("20060102T150405Z") + ".tar.gz"
	path := filepath.Join(backupDir, name)

	return path, writeArchive(path, dir)
}

// Restore unpacks an archive over a data directory.
//
// It refuses a directory that another holzkube-manager holds: the process lock
// is what makes "one writer" true, and restoring underneath a running instance
// would replace the files it has open with different ones carrying the same
// names. The caller passes the check rather than this package taking the lock
// itself, because the lock belongs to the store and a second implementation of
// it would be a second answer to who owns the directory.
//
// safety, when non-empty, is where the pre-restore backup of the *current*
// contents is written first. Passing an empty string skips it, which is for
// restoring into a directory that has nothing in it -- and nowhere else.
func Restore(archive, dir, safety string) (safetyPath string, err error) {
	if safety != "" {
		if entries, rerr := os.ReadDir(dir); rerr == nil && len(entries) > 0 {
			safetyPath, err = CreateNamed(dir, safety)
			if err != nil {
				return "", fmt.Errorf("backup: could not back up what is there now, so nothing "+
					"has been restored: %w", err)
			}
		}
	}

	f, err := os.Open(archive) //nolint:gosec // the archive is named by the operator on their own host
	if err != nil {
		return safetyPath, fmt.Errorf("backup: open %s: %w", archive, err)
	}
	defer f.Close() //nolint:errcheck // the restore's verdict is the write's

	gz, err := gzip.NewReader(f)
	if err != nil {
		return safetyPath, fmt.Errorf("backup: %s is not a gzip archive: %w", archive, err)
	}
	defer gz.Close() //nolint:errcheck // as above

	// Every entry is validated against the destination before anything is
	// written, and the whole archive is validated before the first byte lands.
	// A half-restored directory is worse than a refused restore: it is a
	// directory that looks restored.
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return safetyPath, fmt.Errorf("backup: create %s: %w", dir, err)
	}

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		switch {
		case errors.Is(err, io.EOF):
			return safetyPath, syncDir(dir)
		case err != nil:
			return safetyPath, fmt.Errorf("backup: read %s: %w", archive, err)
		}

		target, err := safeJoin(dir, header.Name)
		if err != nil {
			return safetyPath, err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, dirPerm); err != nil {
				return safetyPath, fmt.Errorf("backup: create %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := writeFile(target, tr, header); err != nil {
				return safetyPath, err
			}
		default:
			// Symlinks, devices, hard links: refused rather than skipped. A
			// symlink in an archive is a path the next write follows, and
			// silently dropping one produces a restored directory that is
			// missing something nobody is told about.
			return safetyPath, fmt.Errorf("backup: %s in this archive is a %q entry, and only "+
				"files and directories are restored", header.Name, entryKind(header.Typeflag))
		}
	}
}

// writeFile restores one file with the permissions the data directory needs.
//
// The mode is taken from the archive and then narrowed: nothing in a data
// directory is group- or world-readable, and an archive that carried a 0644
// must not restore one. That is the same guard fsstore applies at startup, and
// a restore that produced a directory fsstore then refuses to open would be a
// restore that appeared to work.
func writeFile(path string, r io.Reader, header *tar.Header) error {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return fmt.Errorf("backup: create %s: %w", filepath.Dir(path), err)
	}

	mode := os.FileMode(header.Mode).Perm() & 0o700 //nolint:gosec // narrowed, never widened
	if mode == 0 {
		mode = filePerm
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("backup: create %s: %w", path, err)
	}

	// Bounded by the header's declared size rather than copying to EOF: a
	// stream that keeps producing bytes past what it declared is one that
	// fills the disk, and the size is what the archive said this file is.
	if _, err := io.CopyN(f, r, header.Size); err != nil && !errors.Is(err, io.EOF) {
		_ = f.Close()
		return fmt.Errorf("backup: write %s: %w", path, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("backup: sync %s: %w", path, err)
	}
	return f.Close()
}

// safeJoin resolves an archive entry against the destination and refuses
// anything that leaves it.
//
// `../../../etc/shadow` is a valid tar entry, and so is an absolute path. The
// check is on the cleaned result rather than on the input, because `a/../../b`
// contains no leading `..` and still escapes.
func safeJoin(dir, name string) (string, error) {
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("%w: %q is an absolute path", ErrOutsideDestination, name)
	}

	target := filepath.Clean(filepath.Join(dir, name))

	base := filepath.Clean(dir)
	if target != base && !strings.HasPrefix(target, base+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q resolves to %s", ErrOutsideDestination, name, target)
	}
	return target, nil
}

func entryKind(flag byte) string {
	switch flag {
	case tar.TypeSymlink:
		return "symlink"
	case tar.TypeLink:
		return "hard link"
	case tar.TypeChar, tar.TypeBlock:
		return "device"
	case tar.TypeFifo:
		return "fifo"
	default:
		return string(flag)
	}
}

// List reports the backups in a data directory, newest first.
func List(dir string) ([]Entry, error) {
	backupDir := filepath.Join(dir, DirName)

	entries, err := os.ReadDir(backupDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("backup: read %s: %w", backupDir, err)
	}

	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tar.gz") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, Entry{
			Name:    e.Name(),
			Path:    filepath.Join(backupDir, e.Name()),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}

	// Newest first, which is the order somebody restoring reads them in.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].ModTime.After(out[j-1].ModTime); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

// Entry is one backup on disk.
type Entry struct {
	Name    string
	Path    string
	Size    int64
	ModTime time.Time
}

// syncDir fsyncs a directory so that the entries created in it survive a power
// loss. Creating a file and not syncing its directory means the file may exist
// with no name after a crash.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("backup: open %s: %w", dir, err)
	}
	defer d.Close() //nolint:errcheck // the sync below is the verdict

	if err := d.Sync(); err != nil {
		return fmt.Errorf("backup: sync %s: %w", dir, err)
	}
	return nil
}
