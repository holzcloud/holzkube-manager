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

	"github.com/holzcloud/holzkube-manager/internal/store"
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

// Restore replaces a data directory's contents with an archive's.
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
//
// The restore happens in two halves, and nothing of the current contents is
// touched until the first one has finished:
//
//  1. The whole archive is unpacked into a staging directory inside the data
//     directory and read to its end, the gzip length and checksum included. A
//     truncated or corrupt archive, an entry that escapes, a symlink: each of
//     these fails here, the staging directory is removed, and the data
//     directory is as it was. A half-restored directory is worse than a
//     refused restore: it is a directory that looks restored.
//  2. The current contents are moved aside and the staged ones moved in, one
//     rename per top-level entry, and what was moved aside is then deleted. The
//     result is the archive and nothing else: a record written after the
//     backup was taken does not survive into the restored directory (it is in
//     the pre-restore backup). A rename that fails puts back what had moved.
//
// Staging inside the data directory rather than next to it is deliberate: the
// data directory is the one place the service is sure to be allowed to write
// (it owns /var/lib/holzkube-manager, not /var/lib), the renames stay on one
// filesystem, and the directory keeps its own inode, owner and mode. The price
// is that the swap is a rename per entry rather than one; a crash in the
// middle of it leaves what was there in a .holzkube-manager-tmp-replaced-*
// directory beside the pre-restore backup.
//
// What a backup never carries stays where it is: backups/ -- where the archive
// being restored and the pre-restore backup usually are --, upgrade-snapshots/
// and the lock file. An archive that tries to put something there is refused.
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

	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return safetyPath, fmt.Errorf("backup: create %s: %w", dir, err)
	}

	staging, err := os.MkdirTemp(dir, store.TempFilePrefix+"restore-")
	if err != nil {
		return safetyPath, fmt.Errorf("backup: create a staging directory in %s: %w", dir, err)
	}
	// Empty after a swap; after a refusal, what was unpacked before it.
	defer os.RemoveAll(staging) //nolint:errcheck // the restore's verdict is unpack's or swapIn's

	if err := unpack(archive, staging); err != nil {
		return safetyPath, fmt.Errorf("%w (nothing in %s has been changed)", err, dir)
	}
	return safetyPath, swapIn(dir, staging)
}

// unpack reads the whole archive into staging, and fails on anything that is
// not a complete, well-formed archive of files and directories.
func unpack(archive, staging string) error {
	f, err := os.Open(archive) //nolint:gosec // the archive is named by the operator on their own host
	if err != nil {
		return fmt.Errorf("backup: open %s: %w", archive, err)
	}
	defer f.Close() //nolint:errcheck // the restore's verdict is the read's

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("backup: %s is not a gzip archive: %w", archive, err)
	}
	defer gz.Close() //nolint:errcheck // as above

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("backup: read %s: %w", archive, err)
		}

		target, err := safeJoin(staging, header.Name)
		if err != nil {
			return err
		}
		if reserved(staging, target) {
			return fmt.Errorf("backup: %s in this archive is where the data directory keeps "+
				"what a backup never carries, and a restore does not replace it", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, dirPerm); err != nil {
				return fmt.Errorf("backup: create %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := writeFile(target, tr, header); err != nil {
				return err
			}
		default:
			// Symlinks, devices, hard links: refused rather than skipped. A
			// symlink in an archive is a path the next write follows, and
			// silently dropping one produces a restored directory that is
			// missing something nobody is told about.
			return fmt.Errorf("backup: %s in this archive is a %q entry, and only "+
				"files and directories are restored", header.Name, entryKind(header.Typeflag))
		}
	}

	// The tar end marker comes before the gzip trailer, so a stream cut after
	// the last entry still reads as a complete tar. Reading the gzip stream to
	// its end is what checks its length and CRC. What may follow the end
	// marker is record padding (tar pads to 10 KiB blocks); more than that is
	// not an archive this product wrote, and reading it unbounded would be a
	// decompression bomb.
	const maxPadding = 64 << 10
	rest, err := io.Copy(io.Discard, io.LimitReader(gz, maxPadding+1))
	if err != nil {
		return fmt.Errorf("backup: read %s: %w", archive, err)
	}
	if rest > maxPadding {
		return fmt.Errorf("backup: %s carries more than %d bytes after the end of its tar stream", archive, maxPadding)
	}
	return syncTree(staging)
}

// reserved reports whether an archive entry lands on something a backup never
// carries: the names writeTree skips.
func reserved(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." {
		return false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if kept(parts[0]) {
		return true
	}
	for _, part := range parts {
		if strings.HasPrefix(part, store.TempFilePrefix) {
			return true
		}
	}
	return false
}

// kept reports whether a top-level entry of the data directory stays where it
// is across a restore: the backups, the etcd snapshots taken before an
// upgrade, the lock, and anything mid-write (the restore's own staging among
// them).
func kept(name string) bool {
	return name == DirName || name == UpgradeSnapshotsDirName || name == store.LockFileName ||
		strings.HasPrefix(name, store.TempFilePrefix)
}

// swapIn replaces the data directory's contents with staging's.
func swapIn(dir, staging string) (err error) {
	current, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("backup: read %s: %w (nothing has been changed)", dir, err)
	}
	incoming, err := os.ReadDir(staging)
	if err != nil {
		return fmt.Errorf("backup: read %s: %w (nothing has been changed)", staging, err)
	}
	aside, err := os.MkdirTemp(dir, store.TempFilePrefix+"replaced-")
	if err != nil {
		return fmt.Errorf("backup: create a directory in %s: %w (nothing has been changed)", dir, err)
	}

	var movedAside, movedIn []string
	inPlace := false
	defer func() {
		if err == nil || inPlace {
			return
		}
		// Put back what was there.
		for _, name := range movedIn {
			_ = os.Rename(filepath.Join(dir, name), filepath.Join(staging, name))
		}
		for _, name := range movedAside {
			_ = os.Rename(filepath.Join(aside, name), filepath.Join(dir, name))
		}
		_ = syncDir(dir)
		_ = os.Remove(aside)
	}()

	for _, e := range current {
		if kept(e.Name()) {
			continue
		}
		if err := os.Rename(filepath.Join(dir, e.Name()), filepath.Join(aside, e.Name())); err != nil {
			return fmt.Errorf("backup: move %s aside: %w (what was there has been put back)", e.Name(), err)
		}
		movedAside = append(movedAside, e.Name())
	}
	for _, e := range incoming {
		if err := os.Rename(filepath.Join(staging, e.Name()), filepath.Join(dir, e.Name())); err != nil {
			return fmt.Errorf("backup: move %s in: %w (what was there has been put back)", e.Name(), err)
		}
		movedIn = append(movedIn, e.Name())
	}
	if err := syncDir(dir); err != nil {
		return err
	}
	inPlace = true

	// The restore is in place, and what was there is in the pre-restore backup.
	if rerr := os.RemoveAll(aside); rerr != nil {
		return fmt.Errorf("backup: the restore is in place, but what it replaced could not be "+
			"removed from %s: %w", aside, rerr)
	}
	return nil
}

// syncTree fsyncs every directory under root, so that the staged entries have
// names on disk before they are renamed into place. writeFile has synced each
// file's contents already.
func syncTree(root string) error {
	return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		return syncDir(p)
	})
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
