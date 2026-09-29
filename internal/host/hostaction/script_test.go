package hostaction

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The helper script is the only code of this product that runs as root, and it
// is tested by running it, not by reading it. Every case runs a copy of
// deploy/holzkube-manager-host.sh in a temporary directory, as root only inside
// `unshare --user --map-root-user`, with its three overrides pointing the order,
// its state directory and systemctl into that directory. Whatever it records is
// then parsed with the daemon's own ReadResult, so one test holds both halves:
// the file root writes is the file the host page accepts.
//
// Nothing is installed and the host's own paths are never addressed: a copy
// that no longer reads the overrides is refused before it runs.

// scriptOverrides are the environment names through which the copy is pointed
// away from the host's order, its state directory and its real systemctl.
var scriptOverrides = []string{
	"HOLZKUBE_MANAGER_HOST_ORDER",
	"HOLZKUBE_MANAGER_HOST_STATE_DIR",
	"HOLZKUBE_MANAGER_SYSTEMCTL",
}

// stubLogVar names the file the stand-in systemctl appends to.
const stubLogVar = "HKM_STUB_SYSTEMCTL_LOG"

// orderStillThere is the stand-in's second line when the order file (or a
// link by its name) still exists at the moment systemctl is called: the script
// must consume the order before it acts, or a reboot finds it again at boot.
const orderStillThere = "order-still-there"

// The stand-in systemctl: its whole argv on one line, then whether the order
// is still there. It reads the order's path from the environment the script
// inherited, exactly as the real systemctl would see it.
const systemctlStandIn = `#!/bin/sh
printf '%s\n' "$*" >> "$` + stubLogVar + `"
if [ -e "$HOLZKUBE_MANAGER_HOST_ORDER" ] || [ -L "$HOLZKUBE_MANAGER_HOST_ORDER" ]; then
  printf '%s\n' '` + orderStillThere + `' >> "$` + stubLogVar + `"
fi
`

// scriptTimeout bounds every run. A FIFO opened without O_NONBLOCK blocks
// forever; the test fails by this, never by hanging CI.
const scriptTimeout = 5 * time.Second

// hostScriptEnv is one prepared run of the helper script.
type hostScriptEnv struct {
	t        *testing.T
	dir      string
	script   string
	dataDir  string
	order    string
	stateDir string
	stub     string
	stubLog  string
	// extraEnv is added to the run's environment (the claim-race hooks).
	extraEnv []string
}

func requireHostScriptTools(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skipf("the helper script targets Linux hosts (GNU dd, stat, systemd); this is %s", runtime.GOOS)
	}
	for _, tool := range []string{"bash", "dd", "stat", "mktemp"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed here; the helper script needs it", tool)
		}
	}
}

// newHostScriptEnv copies the script into a temporary directory with a data
// directory (0700, the daemon's), a state directory (0755, the helper's) and a
// stand-in systemctl. failing makes the stand-in exit 1 after it has logged.
func newHostScriptEnv(t *testing.T, failing bool) *hostScriptEnv {
	t.Helper()
	requireHostScriptTools(t)

	src, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "holzkube-manager-host.sh"))
	if err != nil {
		t.Fatalf("read the helper script: %v", err)
	}
	// A copy that ignored these would read the host's own order, write into
	// the host's own state directory and, as root, call the real systemctl --
	// reboot included. Refuse to run it at all rather than trust the
	// namespace to stop it.
	for _, name := range scriptOverrides {
		if !strings.Contains(string(src), name) {
			t.Fatalf("the helper script does not read %s; running it here would address this host", name)
		}
	}

	dir := t.TempDir()
	e := &hostScriptEnv{
		t:        t,
		dir:      dir,
		script:   filepath.Join(dir, "holzkube-manager-host.sh"),
		dataDir:  filepath.Join(dir, "data"),
		stateDir: filepath.Join(dir, "state"),
		stub:     filepath.Join(dir, "systemctl"),
		stubLog:  filepath.Join(dir, "systemctl.log"),
	}
	e.order = filepath.Join(e.dataDir, OrderFileName)
	e.write(e.script, src, 0o755)
	if err := os.Mkdir(e.dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(e.stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Mkdir is subject to the umask; the helper refuses a group- or
	// other-writable state directory, so set the mode it will check.
	if err := os.Chmod(e.stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stub := systemctlStandIn
	if failing {
		stub += "exit 1\n"
	}
	e.write(e.stub, []byte(stub), 0o755)
	return e
}

func (e *hostScriptEnv) write(path string, content []byte, mode os.FileMode) {
	e.t.Helper()
	if err := os.WriteFile(path, content, mode); err != nil {
		e.t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		e.t.Fatal(err)
	}
}

// writeOrder puts content at the order's path, as the daemon would (0600).
func (e *hostScriptEnv) writeOrder(content string) {
	e.t.Helper()
	e.write(e.order, []byte(content), 0o600)
}

// setAge makes the order's mtime d in the past (a negative d: in the future).
func (e *hostScriptEnv) setAge(d time.Duration) {
	e.t.Helper()
	at := time.Now().Add(-d)
	if err := os.Chtimes(e.order, at, at); err != nil {
		e.t.Fatal(err)
	}
}

// env is the whole environment of a run: nothing from the test process leaks
// in, so neither the host's paths nor its systemctl can reach the script.
func (e *hostScriptEnv) env() []string {
	return append([]string{
		"PATH=/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=" + e.dir,
		// The script's work directory (mktemp -d) goes here too, so a run
		// the timeout kills leaves nothing behind in the host's /tmp.
		"TMPDIR=" + e.dir,
		"LC_ALL=C",
		"HOLZKUBE_MANAGER_HOST_ORDER=" + e.order,
		"HOLZKUBE_MANAGER_HOST_STATE_DIR=" + e.stateDir,
		"HOLZKUBE_MANAGER_SYSTEMCTL=" + e.stub,
		stubLogVar + "=" + e.stubLog,
	}, e.extraEnv...)
}

// run executes the copy -- as root inside a user namespace when asRoot -- and
// returns its exit code, read from the process itself, and its combined
// output. A run that outlives scriptTimeout is killed with its whole process
// group (a dd blocked on a FIFO included) and fails the test.
func (e *hostScriptEnv) run(asRoot bool) (int, string) {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), scriptTimeout)
	defer cancel()

	var cmd *exec.Cmd
	if asRoot {
		cmd = exec.CommandContext(ctx, "unshare", "--user", "--map-root-user", "bash", e.script)
	} else {
		cmd = exec.CommandContext(ctx, "bash", e.script)
	}
	cmd.Env = e.env()
	cmd.Dir = e.dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second

	out, err := cmd.CombinedOutput()
	e.t.Logf("helper script (root=%v):\n%s", asRoot, out)
	if ctx.Err() != nil {
		e.t.Fatalf("the helper script did not finish within %v -- something blocked it", scriptTimeout)
	}
	if err == nil {
		return 0, string(out)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		e.t.Fatalf("run the helper script: %v", err)
	}
	return exitErr.ExitCode(), string(out)
}

// calls is what the stand-in systemctl logged, "" when it was never called.
func (e *hostScriptEnv) calls() string {
	e.t.Helper()
	raw, err := os.ReadFile(e.stubLog)
	if errors.Is(err, fs.ErrNotExist) {
		return ""
	}
	if err != nil {
		e.t.Fatal(err)
	}
	return string(raw)
}

// wantCalled asserts the stand-in was called exactly once with argv, and that
// the order was already gone when it was.
func (e *hostScriptEnv) wantCalled(argv string) {
	e.t.Helper()
	got := e.calls()
	if strings.Contains(got, orderStillThere) {
		e.t.Errorf("systemctl was called while the order was still there -- the script must consume it first:\n%s", got)
	}
	if got != argv+"\n" {
		e.t.Errorf("systemctl called with %q, want exactly %q", got, argv+"\n")
	}
}

func (e *hostScriptEnv) wantNotCalled() {
	e.t.Helper()
	if got := e.calls(); got != "" {
		e.t.Errorf("systemctl was called, want never:\n%s", got)
	}
}

// wantOrder asserts whether something is left at the order's path.
func (e *hostScriptEnv) wantOrder(present bool) {
	e.t.Helper()
	_, err := os.Lstat(e.order)
	switch {
	case present && err != nil:
		e.t.Errorf("the order is gone, want it left in place: %v", err)
	case !present && err == nil:
		e.t.Errorf("the order is still there, want it consumed")
	case !present && !errors.Is(err, fs.ErrNotExist):
		e.t.Errorf("lstat the order: %v", err)
	}
	if !present {
		e.wantNoHelperClaim()
	}
}

// helperClaimGlob matches the name the script renames the order to before it
// reads it (WR-01).
const helperClaimGlob = ".holzkube-manager-tmp-helper-claim-*"

// wantNoHelperClaim asserts the script removed the order under its claim name
// too: nothing it took is left in the daemon's directory.
func (e *hostScriptEnv) wantNoHelperClaim() {
	e.t.Helper()
	left, _ := filepath.Glob(filepath.Join(e.dataDir, helperClaimGlob))
	if len(left) != 0 {
		e.t.Errorf("the script left its claim in the data directory: %v", left)
	}
}

// wantResult parses `last` with the daemon's reader and compares it.
func (e *hostScriptEnv) wantResult(id string, action Action, outcome Outcome) {
	e.t.Helper()
	path := filepath.Join(e.stateDir, "last")
	info, err := os.Lstat(path)
	if err != nil {
		e.t.Fatalf("the script recorded nothing: %v", err)
	}
	if info.Mode().Perm() != 0o644 || !info.Mode().IsRegular() {
		e.t.Errorf("last mode = %v, want a regular file 0644", info.Mode())
	}
	r, err := ReadResult(os.DirFS(e.stateDir), "/last")
	if err != nil {
		e.t.Fatalf("the daemon's reader refuses what the script wrote: %v", err)
	}
	if r.ID != id || r.Action != action || r.Outcome != outcome {
		e.t.Errorf("last = {%q %q %q}, want {%q %q %q}", r.ID, r.Action, r.Outcome, id, action, outcome)
	}
	if d := time.Since(r.At); d < -time.Minute || d > time.Minute {
		e.t.Errorf("last records %v, not the time of this run", r.At)
	}
	e.wantNoTemporaries()
}

func (e *hostScriptEnv) wantNoResult() {
	e.t.Helper()
	if _, err := ReadResult(os.DirFS(e.stateDir), "/last"); !errors.Is(err, ErrNoResult) {
		e.t.Errorf("ReadResult after this run: %v, want ErrNoResult -- this run must record nothing", err)
	}
	e.wantNoTemporaries()
}

func (e *hostScriptEnv) wantNoTemporaries() {
	e.t.Helper()
	left, _ := filepath.Glob(filepath.Join(e.stateDir, ".last.*"))
	if len(left) != 0 {
		e.t.Errorf("temporary files left in the state directory: %v", left)
	}
}

// requireHostNamespace skips, naming what unshare said, when an unprivileged
// user namespace cannot be made here. On the Pi it was measured working, so
// there the root matrix runs.
func requireHostNamespace(t *testing.T) {
	t.Helper()
	requireHostScriptTools(t)
	if _, err := exec.LookPath("unshare"); err != nil {
		t.Skip("unshare (util-linux) is not installed")
	}
	out, err := exec.Command("unshare", "--user", "--map-root-user", "id", "-u").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "0" {
		t.Skipf("no unprivileged user namespace here: %v: %s", err, out)
	}
}

// Fixed ids: the script records what it read, and the test compares.
const (
	idReboot   = "0123456789abcdef"
	idPoweroff = "fedcba9876543210"
	idRestart  = "00112233445566ff"
	idUpdate   = "a1b2c3d4e5f60718"
)

func TestHostScript(t *testing.T) {
	t.Parallel()
	requireHostScriptTools(t)

	t.Run("syntax", func(t *testing.T) {
		t.Parallel()
		e := newHostScriptEnv(t, false)
		if out, err := exec.Command("bash", "-n", e.script).CombinedOutput(); err != nil {
			t.Fatalf("bash -n: %v\n%s", err, out)
		}
	})

	// Without root the script does nothing at all: not even consume the
	// order, which then waits for the root run the path unit makes.
	t.Run("refused without root", func(t *testing.T) {
		t.Parallel()
		if os.Geteuid() == 0 {
			t.Skip("the test runs as root; the non-root refusal cannot be observed")
		}
		e := newHostScriptEnv(t, false)
		e.writeOrder("reboot " + idReboot + "\n")
		rc, _ := e.run(false)
		if rc != 1 {
			t.Errorf("exit = %d, want 1", rc)
		}
		e.wantNotCalled()
		e.wantNoResult()
		e.wantOrder(true)
	})
}

func TestHostScriptAsRoot(t *testing.T) {
	t.Parallel()
	requireHostNamespace(t)

	// The four orders, each to its one fixed command (D-06).
	for _, tc := range []struct {
		action Action
		id     string
		argv   string
	}{
		{Reboot, idReboot, "reboot"},
		{Poweroff, idPoweroff, "poweroff"},
		{RestartService, idRestart, "restart holzkube-manager.service"},
		{Update, idUpdate, "start --no-block holzkube-manager-update.service"},
	} {
		t.Run("valid "+string(tc.action), func(t *testing.T) {
			t.Parallel()
			e := newHostScriptEnv(t, false)
			e.writeOrder(string(tc.action) + " " + tc.id + "\n")
			rc, _ := e.run(true)
			if rc != 0 {
				t.Errorf("exit = %d, want 0", rc)
			}
			e.wantCalled(tc.argv)
			e.wantOrder(false)
			e.wantResult(tc.id, tc.action, OutcomeStarted)
		})
	}

	t.Run("systemctl fails", func(t *testing.T) {
		t.Parallel()
		e := newHostScriptEnv(t, true)
		e.writeOrder("update " + idUpdate + "\n")
		rc, _ := e.run(true)
		if rc != 1 {
			t.Errorf("exit = %d, want 1", rc)
		}
		e.wantCalled("start --no-block holzkube-manager-update.service")
		e.wantOrder(false)
		e.wantResult(idUpdate, Update, OutcomeFailed)
	})

	t.Run("no order", func(t *testing.T) {
		t.Parallel()
		e := newHostScriptEnv(t, false)
		rc, _ := e.run(true)
		if rc != 0 {
			t.Errorf("exit = %d, want 0", rc)
		}
		e.wantNotCalled()
		e.wantNoResult()
	})

	// Every shape but the one: refused, consumed, recorded as `- - rejected`,
	// and journalled with the reason and the byte count -- never with the
	// order's bytes. Each payload carries a marker the output must not contain.
	for _, tc := range []struct {
		name    string
		payload string
		marker  string
		reason  string
		bytes   string
	}{
		{"foreign word", "halt " + idReboot + "\n", "halt", "unbekannte Form", "22"},
		{"a path", "../etc/passwd\n", "passwd", "unbekannte Form", "14"},
		{"shell metacharacters", "reboot; rm -rf /tmp/hkmZq7\n", "hkmZq7", "unbekannte Form", "27"},
		{"command substitution", "reboot $(echo hkmSubst)\n", "hkmSubst", "unbekannte Form", "24"},
		{"backtick command", "reboot `echo hkmTick`\n", "hkmTick", "unbekannte Form", "22"},
		{"reboot without an id", "reboot\n", "reboot", "unbekannte Form", "7"},
		{"15-hex id", "reboot 0123456789abcde\n", "0123456789abcde", "unbekannte Form", "23"},
		{"17-hex id", "reboot 0123456789abcdef0\n", "0123456789abcdef0", "unbekannte Form", "25"},
		{"upper-case hex id", "reboot 0123456789ABCDEF\n", "0123456789ABCDEF", "unbekannte Form", "24"},
		{"no trailing newline", "reboot " + idReboot, idReboot, "keine vollstaendige Zeile", "23"},
		{"CRLF", "reboot " + idReboot + "\r\n", idReboot, "unbekannte Form", "25"},
		{"the line twice", "reboot " + idReboot + "\nreboot " + idReboot + "\n", idReboot, "nicht genau eine Zeile", "48"},
		{"a NUL byte", "reboot 01234567\x0089abcdef\n", "89abcdef", "nicht genau eine Zeile", "25"},
		{"100 bytes", strings.Repeat("hkmLong", 14) + "x\n", "hkmLong", "zu lang", "mehr als 64"},
		{"empty file", "", "", "keine vollstaendige Zeile", "0"},
		{"a leading space", " reboot " + idReboot + "\n", idReboot, "unbekannte Form", "25"},
	} {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			t.Parallel()
			e := newHostScriptEnv(t, false)
			e.writeOrder(tc.payload)
			rc, out := e.run(true)
			if rc != 2 {
				t.Errorf("exit = %d, want 2", rc)
			}
			e.wantNotCalled()
			e.wantOrder(false)
			e.wantResult("", "", OutcomeRejected)
			if want := "Auftrag verworfen: " + tc.reason + " (" + tc.bytes + " Byte)"; !strings.Contains(out, want) {
				t.Errorf("output lacks %q", want)
			}
			if tc.marker != "" && strings.Contains(out, tc.marker) {
				t.Errorf("output quotes the order (%q): the journal must carry only the reason and the length", tc.marker)
			}
		})
	}

	// F10: the symlink's target must hold a VALID order. A target the pattern
	// refuses anyway would be rejected even if the script followed the link,
	// and removing the defence would then inject nothing.
	t.Run("rejects a symlink to a valid order", func(t *testing.T) {
		t.Parallel()
		e := newHostScriptEnv(t, false)
		target := filepath.Join(e.dir, "elsewhere")
		e.write(target, []byte("reboot "+idReboot+"\n"), 0o600)
		if err := os.Symlink(target, e.order); err != nil {
			t.Fatal(err)
		}
		rc, out := e.run(true)
		if rc != 2 {
			t.Errorf("exit = %d, want 2", rc)
		}
		e.wantNotCalled()
		e.wantOrder(false)
		e.wantResult("", "", OutcomeRejected)
		if !strings.Contains(out, "Auftrag verworfen: symlink (0 Byte)") {
			t.Errorf("output lacks the symlink reason")
		}
		if strings.Contains(out, idReboot) {
			t.Errorf("output quotes the link's target")
		}
		// rm removes the link, never the file it points at.
		if got, err := os.ReadFile(target); err != nil || string(got) != "reboot "+idReboot+"\n" {
			t.Errorf("the link's target was touched: %q, %v", got, err)
		}
	})

	// F11: a FIFO with no writer. Opened without O_NONBLOCK, dd blocks
	// forever; this case fails by the run's timeout, never by hanging.
	t.Run("rejects a FIFO", func(t *testing.T) {
		t.Parallel()
		e := newHostScriptEnv(t, false)
		if err := syscall.Mkfifo(e.order, 0o600); err != nil {
			t.Fatal(err)
		}
		rc, out := e.run(true)
		if rc != 2 {
			t.Errorf("exit = %d, want 2", rc)
		}
		e.wantNotCalled()
		e.wantOrder(false)
		e.wantResult("", "", OutcomeRejected)
		if !strings.Contains(out, "Auftrag verworfen: not-regular (0 Byte)") {
			t.Errorf("output lacks the not-regular reason")
		}
	})

	// R5: a well-formed order outside the window is a leftover (a power cut
	// inside the pick-up window, a restored backup), not a wish. It was
	// valid, so its id and action are recorded.
	for _, tc := range []struct {
		name   string
		age    time.Duration
		reason string
	}{
		{"stale", 10 * time.Minute, "veraltet"},
		{"future", -10 * time.Minute, "aus der Zukunft"},
	} {
		t.Run("rejects a "+tc.name+" order", func(t *testing.T) {
			t.Parallel()
			e := newHostScriptEnv(t, false)
			e.writeOrder("reboot " + idReboot + "\n")
			e.setAge(tc.age)
			rc, out := e.run(true)
			if rc != 2 {
				t.Errorf("exit = %d, want 2", rc)
			}
			e.wantNotCalled()
			e.wantOrder(false)
			e.wantResult(idReboot, Reboot, OutcomeRejected)
			if want := "Auftrag verworfen: " + tc.reason + " (24 Byte)"; !strings.Contains(out, want) {
				t.Errorf("output lacks %q", want)
			}
		})
	}

	// Only a compromised daemon can put a directory there. The script neither
	// takes nor removes it, records failed and does not act; root never runs
	// rm -r in a directory another user owns. systemd's start limit then stops
	// the path unit (RESEARCH Pitfall 4, measured), and the daemon's 10-s
	// withdrawal and HOST-HELPER.md name the recovery.
	t.Run("a directory named host-order fails", func(t *testing.T) {
		t.Parallel()
		e := newHostScriptEnv(t, false)
		if err := os.Mkdir(e.order, 0o700); err != nil {
			t.Fatal(err)
		}
		rc, _ := e.run(true)
		if rc != 1 {
			t.Errorf("exit = %d, want 1", rc)
		}
		e.wantNotCalled()
		e.wantResult("", "", OutcomeFailed)
		if info, err := os.Lstat(e.order); err != nil || !info.IsDir() {
			t.Errorf("the directory should stay: %v", err)
		}
	})

	// The state directory is checked before the order is read: root writes
	// nowhere a link points, and an order it will not record stays for the
	// run that can.
	t.Run("a symlinked state directory is refused", func(t *testing.T) {
		t.Parallel()
		e := newHostScriptEnv(t, false)
		elsewhere := filepath.Join(e.dir, "elsewhere")
		if err := os.Mkdir(elsewhere, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(e.stateDir); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(elsewhere, e.stateDir); err != nil {
			t.Fatal(err)
		}
		e.writeOrder("reboot " + idReboot + "\n")
		rc, _ := e.run(true)
		if rc != 1 {
			t.Errorf("exit = %d, want 1", rc)
		}
		e.wantNotCalled()
		e.wantOrder(true)
		entries, err := os.ReadDir(elsewhere)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Errorf("the script wrote through the link: %v", entries)
		}
	})
}

// raceHooks is sourced by bash before the script (BASH_ENV) and wraps one
// command in a function -- a function wins over any PATH, the script's fixed
// one included. Just before the first call of that command it plays the
// daemon's withdrawal (fsstore.Claim: a rename of the order to its own claim
// name) and, when HKM_RACE_NEXT is set, the next placement: a new order under
// the order's name. It records whether the daemon's rename took the order.
const raceHooks = `__hkm_daemon() {
  [ -e "$HKM_RACE_DONE" ] && return 0
  : > "$HKM_RACE_DONE"
  if command mv -T -- "$HOLZKUBE_MANAGER_HOST_ORDER" "$HKM_RACE_TAKEN" 2>/dev/null; then
    printf won > "$HKM_RACE_DONE"
  fi
  if [ -n "${HKM_RACE_NEXT:-}" ]; then
    printf '%s\n' "$HKM_RACE_NEXT" > "$HKM_RACE_TAKEN.next"
    command mv -T -- "$HKM_RACE_TAKEN.next" "$HOLZKUBE_MANAGER_HOST_ORDER"
  fi
  return 0
}
`

// TestHostScriptClaimRace is WR-01: the daemon withdraws an order by renaming
// it, and whoever renames first must own it. Before the fix the script read the
// order and then ran "rm -f", which succeeds on a name that is already gone --
// so an order the daemon had withdrawn (and reported "nothing was done") was
// carried out anyway, and a newer order that had meanwhile been placed under
// the name was deleted without a trace.
//
// The daemon's rename is played just before each command the script runs on
// the order, in turn. Whatever the interleaving, exactly one side owns A: the
// daemon (A is never carried out) or the script (A is carried out). And B, the
// newer order, is never lost: it is carried out, or it still waits.
//
// Fault injected and seen red: the old read-then-"rm -f" consumption. With the
// daemon's rename landing between dd and rm, the script rebooted anyway, and
// deleted B.
func TestHostScriptClaimRace(t *testing.T) {
	t.Parallel()
	requireHostNamespace(t)

	const (
		idA   = idReboot
		idB   = idRestart
		argvA = "reboot"
		argvB = "restart holzkube-manager.service"
	)
	for _, before := range []string{"stat", "mv", "dd", "rm"} {
		for _, withB := range []bool{false, true} {
			name := "the daemon claims before " + before
			if withB {
				name += ", then a newer order is placed"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				e := newHostScriptEnv(t, false)
				hooks := filepath.Join(e.dir, "race-hooks.sh")
				done := filepath.Join(e.dir, "race-done")
				taken := filepath.Join(e.dataDir, ".holzkube-manager-tmp-claim-withdrawn-"+idA)
				e.write(hooks, []byte(raceHooks+before+`() { __hkm_daemon; command `+before+` "$@"; }`+"\n"), 0o644)
				e.extraEnv = []string{"BASH_ENV=" + hooks, "HKM_RACE_DONE=" + done, "HKM_RACE_TAKEN=" + taken}
				if withB {
					e.extraEnv = append(e.extraEnv, "HKM_RACE_NEXT=restart-service "+idB)
				}
				e.writeOrder("reboot " + idA + "\n")

				rc, _ := e.run(true)

				marker, err := os.ReadFile(done)
				if err != nil {
					t.Fatalf("the hook never ran before %s -- this case measured nothing: %v", before, err)
				}
				daemonWon := string(marker) == "won"
				var calls []string
				for _, l := range strings.Split(strings.TrimSpace(e.calls()), "\n") {
					if l != "" && l != orderStillThere {
						calls = append(calls, l)
					}
				}
				ranA := false
				ranB := false
				for _, c := range calls {
					switch c {
					case argvA:
						ranA = true
					case argvB:
						ranB = true
					default:
						t.Errorf("systemctl called with %q", c)
					}
				}
				if len(calls) > 1 {
					t.Errorf("one run carried out %d orders: %q", len(calls), calls)
				}
				if daemonWon && ranA {
					t.Errorf("the daemon withdrew A before %s, and the script carried A out anyway (exit %d)", before, rc)
				}
				if !daemonWon && !ranA {
					t.Errorf("the script took A before %s, and did not carry it out (exit %d)", before, rc)
				}
				if withB {
					got, err := os.ReadFile(e.order)
					bWaits := err == nil && string(got) == "restart-service "+idB+"\n"
					if ranB == bWaits {
						t.Errorf("B carried out = %v, B still waiting = %v: want exactly one -- a newer order must never be lost", ranB, bWaits)
					}
				} else if _, err := os.Lstat(e.order); !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("something is left under the order's name: %v", err)
				}
				e.wantNoHelperClaim()
				if !daemonWon && rc != 0 {
					t.Errorf("exit = %d, want 0", rc)
				}
			})
		}
	}
}
