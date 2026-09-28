package host

import (
	"bufio"
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// pathMountinfo is this process's own view of its mounts. It stays readable
// under ProcSubset=pid, because it lives under /proc/self -- which is what
// makes it the proof that the hardening is in force (D-03).
const pathMountinfo = "/proc/self/mountinfo"

// mountEntry is one line of /proc/self/mountinfo, per proc(5):
//
//	ID PARENT MAJ:MIN ROOT MOUNTPOINT MOUNTOPTS [OPTIONAL...] - FSTYPE SOURCE SUPEROPTS
//
// SuperOpts are the filesystem's own options, the field after the source. They
// are where proc's subset=pid is written, not in the per-mount options before
// the separator (measured on the production unit's namespace).
type mountEntry struct {
	ID, Parent int
	MajMin     string
	Root       string
	MountPoint string
	FSType     string
	Source     string
	SuperOpts  []string
}

// parseMountinfo reads every line of a mountinfo file.
//
// A line that does not have the shape proc(5) describes is an error for the
// whole file, not a skipped line: a mount table read with holes in it would
// answer "which filesystem is this" wrongly rather than not at all.
func parseMountinfo(raw []byte) ([]mountEntry, error) {
	out := make([]mountEntry, 0, bytes.Count(raw, []byte("\n"))+1)
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, 4096), maxMountinfo)
	n := 0
	for sc.Scan() {
		n++
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		e, err := parseMountLine(line)
		if err != nil {
			return nil, fmt.Errorf("mountinfo line %d: %w", n, err)
		}
		out = append(out, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func parseMountLine(line string) (mountEntry, error) {
	fields := strings.Fields(line)
	// The optional fields vary in number (shared:N, master:N, ...), so the
	// lone "-" is the only fixed landmark after the sixth field.
	sep := slices.Index(fields, "-")
	if sep < 6 || len(fields) < sep+4 {
		return mountEntry{}, fmt.Errorf("not the shape proc(5) describes: %q", line)
	}

	id, err := strconv.Atoi(fields[0])
	if err != nil {
		return mountEntry{}, fmt.Errorf("mount ID %q: %w", fields[0], err)
	}
	parent, err := strconv.Atoi(fields[1])
	if err != nil {
		return mountEntry{}, fmt.Errorf("parent ID %q: %w", fields[1], err)
	}

	return mountEntry{
		ID:         id,
		Parent:     parent,
		MajMin:     fields[2],
		Root:       unescapeMount(fields[3]),
		MountPoint: unescapeMount(fields[4]),
		FSType:     fields[sep+1],
		Source:     unescapeMount(fields[sep+2]),
		SuperOpts:  strings.Split(fields[sep+3], ","),
	}, nil
}

// mountEscapes are the four characters the kernel writes as octal escapes in
// a mountinfo path (fs/proc_namespace.c, mangle()).
var mountEscapes = strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)

func unescapeMount(s string) string {
	return mountEscapes.Replace(s)
}

// topmostAt is the mount a path at mountPoint actually resolves through.
//
// There can be more than one entry at the same point. In a user namespace a
// fresh proc mounted over /proc sits on top of the inherited one, and both are
// listed (measured: the new one parented on the old). The topmost is the entry
// no other entry at that point is parented on; if the parent links do not
// decide it, the last such line, because the kernel lists a mount after the
// one it was mounted over.
func topmostAt(ms []mountEntry, mountPoint string) (mountEntry, bool) {
	var at []mountEntry
	for _, m := range ms {
		if m.MountPoint == mountPoint {
			at = append(at, m)
		}
	}
	if len(at) == 0 {
		return mountEntry{}, false
	}

	for _, cand := range at {
		covered := false
		for _, other := range at {
			if other.ID != cand.ID && other.Parent == cand.ID {
				covered = true
				break
			}
		}
		if !covered {
			return cand, true
		}
	}
	return at[len(at)-1], true
}

// procSubsetPid reports whether the /proc this process sees is mounted with
// subset=pid -- what systemd's ProcSubset=pid does. It is the proof D-03 asks
// for: a missing /proc/stat counts as the hardening only when this says so.
func procSubsetPid(ms []mountEntry) bool {
	m, ok := topmostAt(ms, "/proc")
	if !ok || m.FSType != "proc" {
		return false
	}
	return slices.Contains(m.SuperOpts, "subset=pid")
}
