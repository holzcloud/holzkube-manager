package host

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
)

// The Device card's sources (HOST-01, D-04).
//
// Every one of them survives the unit's ProcSubset=pid: the device tree and DMI
// are under /sys, os-release is under /etc, and the CPU list is under /sys.
// The one /proc read in this file is PID 1's environment for the container
// check, and that is allowed to fail -- under ProtectProc=invisible it always
// does, and on the host there is nothing to find in it anyway.

const (
	pathDeviceTreeModel = "/sys/firmware/devicetree/base/model"
	pathDMIVendor       = "/sys/class/dmi/id/sys_vendor"
	pathDMIProduct      = "/sys/class/dmi/id/product_name"
	pathCPUOnline       = "/sys/devices/system/cpu/online"
	pathOSRelease       = "/etc/os-release"
	pathOSReleaseLib    = "/usr/lib/os-release"
	pathPID1Environ     = "/proc/1/environ"
	pathDockerEnv       = "/.dockerenv"
	pathContainerEnv    = "/run/.containerenv"
)

// osReleasePaths are tried in order, as os-release(5) says. On the Pi the
// first is a symlink to the second; fs.FS follows it.
var osReleasePaths = []string{pathOSRelease, pathOSReleaseLib}

// readModel is what the board is: the device tree's model on an ARM board,
// DMI's vendor and product on a PC.
func readModel(fsys fs.FS) Reading[string] {
	raw, err := readBounded(fsys, fsPath(pathDeviceTreeModel), maxSmallFile)
	if err == nil {
		// Device-tree strings end in NUL (measured on the Pi 5).
		if model := strings.TrimRight(string(raw), "\x00\n "); model != "" {
			return Read(model)
		}
		err = errors.New("the file is empty")
	}
	dtErr := err

	// DMI, shown as the firmware wrote it. Boards whose vendor filled in
	// "To Be Filled By O.E.M." are shown saying exactly that: it is what the
	// machine says about itself.
	var parts []string
	for _, p := range []string{pathDMIVendor, pathDMIProduct} {
		raw, err := readBounded(fsys, fsPath(p), maxSmallFile)
		if err != nil {
			continue
		}
		if s := strings.TrimSpace(string(raw)); s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) > 0 {
		return Read(strings.Join(parts, " "))
	}

	return Hidden[string](reasonFor(pathDeviceTreeModel, dtErr))
}

// readOSRelease is the operating system's name, per os-release(5).
func readOSRelease(fsys fs.FS) Reading[string] {
	var firstErr error
	for _, p := range osReleasePaths {
		raw, err := readBounded(fsys, fsPath(p), maxSmallFile)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		return Read(osName(parseOSRelease(raw)))
	}
	return Hidden[string](reasonFor(osReleasePaths[0], firstErr))
}

// osName picks the name to show: PRETTY_NAME, else NAME and VERSION, else
// "Linux" -- the default os-release(5) itself gives for NAME.
func osName(fields map[string]string) string {
	if s := fields["PRETTY_NAME"]; s != "" {
		return s
	}
	if name := fields["NAME"]; name != "" {
		if version := fields["VERSION"]; version != "" {
			return name + " " + version
		}
		return name
	}
	return "Linux"
}

// parseOSRelease reads KEY=value lines. A value may be wrapped in double or
// single quotes, and \", \\, \$, \` and \' stand for the character itself --
// how os-release(5) describes the shell-compatible format, and how Python's
// platform.freedesktop_os_release reads it.
func parseOSRelease(raw []byte) map[string]string {
	out := map[string]string{}
	for line := range strings.SplitSeq(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			continue
		}
		if n := len(value); n >= 2 && (value[0] == '"' || value[0] == '\'') && value[n-1] == value[0] {
			value = value[1 : n-1]
		}
		out[key] = unescapeOSRelease(value)
	}
	return out
}

func unescapeOSRelease(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && strings.IndexByte("\"\\$`'", s[i+1]) >= 0 {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// readCores is the number of online processors.
func readCores(fsys fs.FS) Reading[int] {
	raw, err := readBounded(fsys, fsPath(pathCPUOnline), maxSmallFile)
	if err != nil {
		return Hidden[int](reasonFor(pathCPUOnline, err))
	}
	n, err := countCPUList(string(raw))
	if err != nil {
		return Hidden[int](reasonFor(pathCPUOnline, err))
	}
	return Read(n)
}

// countCPUList counts a kernel CPU list: "0-3" is 4, "0-3,5,7-8" is 7.
func countCPUList(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("the CPU list is empty")
	}
	total := 0
	for part := range strings.SplitSeq(s, ",") {
		lo, hi, isRange := strings.Cut(part, "-")
		first, err := strconv.Atoi(lo)
		if err != nil || first < 0 {
			return 0, fmt.Errorf("%q is not a CPU list", s)
		}
		last := first
		if isRange {
			last, err = strconv.Atoi(hi)
			if err != nil || last < first {
				return 0, fmt.Errorf("%q is not a CPU list", s)
			}
		}
		total += last - first + 1
	}
	return total, nil
}

// detectContainer reports whether this process runs in a container (D-17).
//
// Three markers, any one enough: Docker's /.dockerenv, Podman's
// .containerenv under /run, and container= in PID 1's environment (systemd-nspawn,
// LXC, Podman). A marker that cannot be read is no evidence either way, and
// "no evidence" is false.
//
// Only the boolean leaves this function. PID 1's environment can carry
// anything its author put there, and none of it is stored or returned (ASVS
// V7, T-11-03).
func detectContainer(fsys fs.FS) bool {
	for _, marker := range []string{pathDockerEnv, pathContainerEnv} {
		if _, err := fs.Stat(fsys, fsPath(marker)); err == nil {
			return true
		}
	}
	environ, err := readBounded(fsys, fsPath(pathPID1Environ), maxSmallFile)
	if err != nil {
		return false
	}
	for entry := range bytes.SplitSeq(environ, []byte{0}) {
		if bytes.HasPrefix(entry, []byte("container=")) {
			return true
		}
	}
	return false
}
