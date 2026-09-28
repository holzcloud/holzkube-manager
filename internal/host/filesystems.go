package host

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// Roles a filesystem row can carry. One row can carry both.
const (
	roleRoot    = "root"
	roleDataDir = "data directory"
)

// Filesystem is one row of the Filesystems card (HMON-02).
//
// Its keys are a superset of inventory.HardwareFilesystem's (D-13), so the node
// page and the host page speak about a filesystem in the same words; the usage
// is a Reading because statfs(2) can fail for one row while the other is fine.
type Filesystem struct {
	Mount  string `json:"mount"`
	Device string `json:"device"`
	FSType string `json:"fstype"`
	// Roles say why this filesystem is on the page: "root", "data directory",
	// or both when they are one filesystem. Never null.
	Roles []string         `json:"roles"`
	Usage Reading[FSUsage] `json:"usage"`
}

// FSUsage is statfs(2) in bytes, computed exactly as df(1) computes it.
//
// Used is (blocks - bfree) and Available is bavail: the blocks an ext4
// filesystem reserves for root are in neither, so used + available is less than
// size, and df's Use% is used / (used + available) -- the figure the page draws.
type FSUsage struct {
	SizeBytes      uint64 `json:"size_bytes"`
	UsedBytes      uint64 `json:"used_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
}

// filesystems is exactly two paths' filesystems: "/" and the one holding
// dataDir -- and one row, not two, when they are the same filesystem (D-07).
//
// "The same filesystem" is decided by device number, never by mount point.
// The production unit (ProtectSystem=strict with StateDirectory=) makes systemd
// bind-mount the data directory over itself, so the daemon's own mountinfo has
// a line for /var/lib/holzkube-manager next to the one for / -- both on 179:2,
// the same ext4 partition (measured on the Pi). A lookup that compared mount
// points would find two different ones and draw two identical bars for one
// disk. The merged row is labelled with the root entry, because "/" is the
// name an operator knows that partition by.
//
// Without a mount table (it could not be read or parsed) nothing can prove two
// paths share a filesystem, so each row is labelled with its own path and
// neither device nor type is claimed.
func filesystems(ms []mountEntry, dataDir string, sys Sys) []Filesystem {
	out := make([]Filesystem, 0, 2)

	root, haveRoot := topmostAt(ms, "/")
	if !haveRoot {
		out = append(out, Filesystem{Mount: "/", Roles: roles(roleRoot), Usage: fsUsage(sys, "/")})
		if dataDir != "" {
			out = append(out, Filesystem{Mount: dataDir, Roles: roles(roleDataDir), Usage: fsUsage(sys, dataDir)})
		}
		return out
	}

	if dataDir == "" {
		return append(out, rowFor(root, "/", sys, roleRoot))
	}

	data, haveData := mountOf(ms, dataDir)
	if !haveData || data.MajMin == root.MajMin {
		return append(out, rowFor(root, "/", sys, roleRoot, roleDataDir))
	}
	return append(out, rowFor(root, "/", sys, roleRoot), rowFor(data, dataDir, sys, roleDataDir))
}

// rowFor is the row for mount entry m, its usage read by statfs on statPath.
func rowFor(m mountEntry, statPath string, sys Sys, rs ...string) Filesystem {
	return Filesystem{
		Mount:  m.MountPoint,
		Device: m.Source,
		// The type's name from mountinfo, never statfs's magic number: 0xef53
		// is ext2, ext3 and ext4 alike.
		FSType: m.FSType,
		Roles:  roles(rs...),
		Usage:  fsUsage(sys, statPath),
	}
}

func roles(rs ...string) []string {
	out := make([]string, 0, len(rs))
	return append(out, rs...)
}

// mountOf is the mount p resolves through: the topmost entry at the longest
// mount point that is a prefix of p at a "/" boundary. /var/lib/holzkube is
// not a prefix of /var/lib/holzkube-manager.
func mountOf(ms []mountEntry, p string) (mountEntry, bool) {
	p = path.Clean(p)
	best := ""
	found := false
	for _, m := range ms {
		mp := m.MountPoint
		if !underMount(p, mp) {
			continue
		}
		if !found || len(mp) > len(best) {
			best, found = mp, true
		}
	}
	if !found {
		return mountEntry{}, false
	}
	return topmostAt(ms, best)
}

func underMount(p, mountPoint string) bool {
	if mountPoint == "/" {
		return strings.HasPrefix(p, "/")
	}
	return p == mountPoint || strings.HasPrefix(p, mountPoint+"/")
}

// fsUsage is statfs(2) on p in df's arithmetic.
func fsUsage(sys Sys, p string) Reading[FSUsage] {
	st, err := sys.Statfs(p)
	if err != nil {
		return Hidden[FSUsage](reasonFor("statfs("+p+")", err))
	}
	used := uint64(0)
	if st.Blocks > st.Free {
		used = (st.Blocks - st.Free) * st.FrameSize
	}
	return Read(FSUsage{
		SizeBytes:      st.Blocks * st.FrameSize,
		UsedBytes:      used,
		AvailableBytes: st.Avail * st.FrameSize,
	})
}

// DirSize is the data directory's size on disk and when it was measured.
// MeasuredAt is part of the answer because the size is cached: the page says
// how old the number is rather than passing it off as the current one.
type DirSize struct {
	Bytes      uint64    `json:"bytes"`
	MeasuredAt time.Time `json:"measured_at"`
}

// dirSizeTTL is how long one walk of the data directory answers for, and
// dirSizeDeadline how long one walk may take (D-07). The page polls every 3 s;
// walking the SD card that often would be the daemon's heaviest disk load.
const (
	dirSizeTTL      = 60 * time.Second
	dirSizeDeadline = 5 * time.Second
)

// errWalkTooSlow is what a walk that hit dirSizeDeadline says.
var errWalkTooSlow = errors.New("the size walk took longer than 5 s")

// dirSizer measures the data directory the way du -s -B1 does: allocated
// blocks, every entry including the directories, each inode once (hard links
// are one file), symlinks not followed.
//
// It walks at most once per dirSizeTTL, and one walk at a time: requests that
// arrive while a walk runs wait for that walk instead of starting their own.
type dirSizer struct {
	fsys fs.FS
	dir  string

	group singleflight.Group

	mu sync.Mutex
	// tried is whether any walk has finished; attempted is when the last one
	// did, good or not -- a failing walk is not retried on every poll either.
	tried     bool
	attempted time.Time
	// last is the newest good measurement, nil until there is one.
	last *DirSize
	// lastErr is why the newest walk failed, nil when it did not.
	lastErr *Reason
}

// size is the data directory's size as of the newest walk no older than
// dirSizeTTL at now.
func (s *dirSizer) size(ctx context.Context, now time.Time) Reading[DirSize] {
	if s.dir == "" {
		return Hidden[DirSize](Reason{Code: CodeReadFailed, Message: "No data directory is configured."})
	}

	s.mu.Lock()
	fresh := s.tried && now.Sub(s.attempted) < dirSizeTTL && !now.Before(s.attempted)
	s.mu.Unlock()

	if !fresh {
		// The walk is not the request's: a browser tab that goes away must not
		// cancel the walk every other waiting request shares. The deadline
		// bounds it instead.
		walkCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), dirSizeDeadline)
		defer cancel()
		_, _, _ = s.group.Do("walk", func() (any, error) {
			n, err := walkSize(walkCtx, s.fsys, s.dir)
			s.record(now, n, err)
			return nil, nil
		})
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last != nil {
		// Also after a failed walk: the good value states its age through
		// measured_at, which is truer than hiding a size that was measured.
		return Read(*s.last)
	}
	if s.lastErr != nil {
		return Hidden[DirSize](*s.lastErr)
	}
	return Hidden[DirSize](Reason{Code: CodeReadFailed, Message: "The data directory has not been measured yet."})
}

func (s *dirSizer) record(now time.Time, n uint64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tried, s.attempted = true, now
	if err != nil {
		r := reasonFor(s.dir, err)
		s.lastErr = &r
		return
	}
	s.last, s.lastErr = &DirSize{Bytes: n, MeasuredAt: now.UTC()}, nil
}

// inode identifies one file for the hard-link count.
type inode struct{ dev, ino uint64 }

// walkSize sums st_blocks x 512 over everything under dir, each inode once.
func walkSize(ctx context.Context, fsys fs.FS, dir string) (uint64, error) {
	name := fsPath(path.Clean(dir))
	if name == "" {
		name = "."
	}
	seen := make(map[inode]struct{})
	var total uint64
	err := fs.WalkDir(fsys, name, func(p string, d fs.DirEntry, err error) error {
		// An entry listed and gone before it was stat'ed or read has vanished,
		// and du skips it the same way: the store, the sessions and the audit
		// log all write a temporary file and rename it, so this happens in
		// normal operation, and one such file must not cost the whole
		// measurement. The data directory itself missing is still an error.
		if err != nil {
			if p != name && errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		info, err := d.Info()
		if p != name && errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		blocks, dev, ino, ok := statBlocks(info)
		if !ok {
			return errUnsupported
		}
		key := inode{dev: dev, ino: ino}
		if _, dup := seen[key]; dup {
			return nil
		}
		seen[key] = struct{}{}
		if blocks > 0 {
			total += uint64(blocks) * 512 //nolint:gosec // checked positive on the line above
		}
		return nil
	})
	if errors.Is(err, context.DeadlineExceeded) {
		return 0, errWalkTooSlow
	}
	if err != nil {
		return 0, err
	}
	return total, nil
}
