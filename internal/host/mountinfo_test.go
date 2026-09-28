package host

import (
	"io/fs"
	"slices"
	"testing"
)

// The mountinfo shapes below are documentation values in the shape measured on
// two namespaces: the production unit's (one /proc, subset=pid in the super
// options) and an unprivileged user namespace that mounts a fresh proc over the
// inherited one (two stacked /proc entries).

const (
	mountSystemd = "22 1 179:2 / / ro,noatime master:1 - ext4 /dev/mmcblk0p2 rw\n" +
		"337 22 0:53 / /proc rw,nosuid,nodev,noexec,relatime shared:384 - proc proc rw,hidepid=invisible,subset=pid\n"

	mountStackedLower = "363 328 0:20 / /proc rw,relatime - proc proc rw\n"
	mountStackedUpper = "573 363 0:46 / /proc rw,relatime - proc proc rw,hidepid=invisible,subset=pid\n"

	// subset=pid only among the per-mount options. The kernel never writes it
	// there; a parser that looks at the wrong field would still find it.
	mountWrongField = "25 22 0:20 / /proc rw,subset=pid shared:12 - proc proc rw\n"

	mountEscaped = "90 22 8:17 /a\\134b /mnt/with\\040space rw - ext4 /dev/sdb1 rw,errors=remount-ro\n"
)

func TestMountinfo(t *testing.T) {
	t.Parallel()

	t.Run("fields around the separator", func(t *testing.T) {
		t.Parallel()
		ms, err := parseMountinfo([]byte(mountSystemd))
		if err != nil {
			t.Fatal(err)
		}
		if len(ms) != 2 {
			t.Fatalf("got %d entries, want 2", len(ms))
		}
		p := ms[1]
		if p.ID != 337 || p.Parent != 22 || p.MajMin != "0:53" || p.Root != "/" || p.MountPoint != "/proc" {
			t.Errorf("proc entry = %+v", p)
		}
		if p.FSType != "proc" || p.Source != "proc" {
			t.Errorf("fstype/source = %q/%q, want proc/proc", p.FSType, p.Source)
		}
		if want := []string{"rw", "hidepid=invisible", "subset=pid"}; !slices.Equal(p.SuperOpts, want) {
			t.Errorf("super options = %q, want %q", p.SuperOpts, want)
		}
	})

	t.Run("no optional fields", func(t *testing.T) {
		t.Parallel()
		ms, err := parseMountinfo([]byte(mountStackedLower))
		if err != nil {
			t.Fatal(err)
		}
		if ms[0].FSType != "proc" || !slices.Equal(ms[0].SuperOpts, []string{"rw"}) {
			t.Errorf("entry = %+v", ms[0])
		}
	})

	t.Run("octal escapes", func(t *testing.T) {
		t.Parallel()
		ms, err := parseMountinfo([]byte(mountEscaped))
		if err != nil {
			t.Fatal(err)
		}
		if ms[0].MountPoint != "/mnt/with space" {
			t.Errorf("mount point = %q, want %q", ms[0].MountPoint, "/mnt/with space")
		}
		if ms[0].Root != `/a\b` {
			t.Errorf("root = %q, want %q", ms[0].Root, `/a\b`)
		}
	})

	t.Run("a line without the separator is an error", func(t *testing.T) {
		t.Parallel()
		if _, err := parseMountinfo([]byte("25 22 0:20 / /proc rw proc proc rw\n")); err == nil {
			t.Error("parsed a line with no \" - \" separator")
		}
	})

	t.Run("the fixture trees", func(t *testing.T) {
		t.Parallel()
		for fixture, want := range map[string]bool{"pi5": false, "procsubset-pid": true} {
			raw, err := fs.ReadFile(fixtureFS(t, fixture), "proc/self/mountinfo")
			if err != nil {
				t.Fatal(err)
			}
			ms, err := parseMountinfo(raw)
			if err != nil {
				t.Fatalf("%s: %v", fixture, err)
			}
			if got := procSubsetPid(ms); got != want {
				t.Errorf("%s: procSubsetPid = %v, want %v", fixture, got, want)
			}
		}
	})
}

func TestProcSubsetPid(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		raw  string
		want bool
	}{
		{"systemd: one /proc with subset=pid", mountSystemd, true},
		{"unshare: fresh proc stacked on the inherited one", mountStackedLower + mountStackedUpper, true},
		{"unshare, lines in reverse order", mountStackedUpper + mountStackedLower, true},
		{"only the lower of two carries it", "363 328 0:20 / /proc rw - proc proc rw,subset=pid\n573 363 0:46 / /proc rw - proc proc rw\n", false},
		{"subset=pid among the per-mount options only", mountWrongField, false},
		{"plain /proc", mountStackedLower, false},
		{"no /proc at all", "22 1 179:2 / / rw - ext4 /dev/mmcblk0p2 rw\n", false},
		{"a non-proc filesystem at /proc", "25 22 0:20 / /proc rw - tmpfs tmpfs rw,subset=pid\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ms, err := parseMountinfo([]byte(tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			if got := procSubsetPid(ms); got != tc.want {
				top, _ := topmostAt(ms, "/proc")
				t.Errorf("procSubsetPid = %v, want %v (topmost /proc = %+v)", got, tc.want, top)
			}
		})
	}
}
