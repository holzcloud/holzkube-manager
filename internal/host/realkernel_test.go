package host

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// subsetChildEnv marks the re-executed test binary that runs inside the new
// namespaces. Only TestRealKernelProcSubset looks at it.
const subsetChildEnv = "HOLZKUBE_HOST_SUBSET_CHILD"

// subsetMount mounts a fresh proc over /proc the way systemd's ProcSubset=pid
// and ProtectProc=invisible do, then runs the command it is given.
const subsetMount = `mount -t proc -o subset=pid,hidepid=invisible proc /proc && exec "$@"`

// TestRealKernelProcSubset is the hardening against the real kernel rather
// than a fixture: the test binary re-executes itself in an unprivileged user,
// mount and PID namespace with /proc mounted subset=pid, and there the
// production Collector -- os.DirFS("/") and the real syscalls -- has to report
// CPU usage and memory as hidden by the hardening and load as read from
// sysinfo(2).
//
// systemd cannot be used for this: ProcSubset= is only honoured for system
// services, and `systemd-run --user -p ProcSubset=pid` runs with every file
// still visible (measured). The kernel option is the same one either way.
func TestRealKernelProcSubset(t *testing.T) {
	if os.Getenv(subsetChildEnv) == "1" {
		realKernelChild(t)
		return
	}

	if runtime.GOOS != "linux" {
		t.Skipf("proc's subset=pid is a Linux mount option; this is %s", runtime.GOOS)
	}
	if _, err := exec.LookPath("unshare"); err != nil {
		t.Skipf("no unshare(1) on PATH: %v", err)
	}

	ns := []string{"--user", "--map-root-user", "--mount", "--pid", "--fork", "--", "sh", "-c", subsetMount, "sh"}

	// Can this machine make the namespace at all? A kernel or sandbox that
	// refuses unprivileged user namespaces is a reason to skip; anything that
	// goes wrong after that is a failure.
	probe := exec.Command("unshare", append(ns, "true")...) //nolint:gosec // fixed arguments
	if out, err := probe.CombinedOutput(); err != nil {
		t.Skipf("unshare cannot create a user namespace with proc mounted subset=pid here: %v\n%s", err, out)
	}

	args := append(ns, os.Args[0], "-test.run=^TestRealKernelProcSubset$", "-test.count=1", "-test.v")
	cmd := exec.Command("unshare", args...) //nolint:gosec // the test binary itself
	cmd.Env = append(os.Environ(), subsetChildEnv+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the child in the subset=pid namespace failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "--- PASS: TestRealKernelProcSubset") {
		t.Fatalf("the child did not report its own pass:\n%s", out)
	}
	t.Logf("child output:\n%s", out)
}

func realKernelChild(t *testing.T) {
	if _, err := os.Stat("/proc/stat"); err == nil {
		t.Fatal("/proc/stat is visible: this is not a subset=pid namespace, so the test would prove nothing")
	}

	c := New(Config{FS: os.DirFS("/"), Sys: OS(), Now: time.Now})
	live := c.Read(context.Background()).Live

	assertHidden(t, "usage", live.CPU.Usage.Readable, live.CPU.Usage.Value == nil, live.CPU.Usage.Reason, CodeProcSubset)
	assertHidden(t, "per_core", live.CPU.PerCore.Readable, live.CPU.PerCore.Value == nil, live.CPU.PerCore.Reason, CodeProcSubset)
	assertHidden(t, "memory", live.Memory.Readable, live.Memory.Value == nil, live.Memory.Reason, CodeProcSubset)

	load := mustRead(t, "load", live.CPU.Load)
	if load.Source != loadSourceSysinfo {
		t.Errorf("load source = %q, want sysinfo: /proc/loadavg is hidden here", load.Source)
	}
	t.Logf("load from sysinfo(2): %.2f %.2f %.2f", load.Load1, load.Load5, load.Load15)
}
