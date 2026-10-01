package updatestatus

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The update script is tested by running it, not by reading it. Every case
// runs a copy of deploy/holzkube-manager-update.sh in a temporary directory
// against stub curl, systemctl, journalctl and sleep, with every path it
// touches pointed into that directory through its HOLZKUBE_MANAGER_*
// overrides, and then parses whatever status file it left with the daemon's
// own Read. One test therefore holds both halves: the writer writes what the
// reader accepts, and the reader's refusal would fail the writer's test.
//
// A copy, never the repository file: after a healthy update the script
// replaces readlink -f "$0" with the copy from the archive (Pitfall 10).

const (
	fakeInstalled = "0.1.0"
	fakeRelease   = "0.2.0"
	testHealthURL = "https://health.invalid/api/v1/system/status"
)

// scriptEnv is one prepared run: the script copy, its stubs and fixtures, and
// the paths the script is pointed at.
type scriptEnv struct {
	t         *testing.T
	dir       string
	script    string
	stubs     string
	bin       string
	previous  string
	statusDir string
	extra     []string
	// flock is the real flock(1), which the stub of that name hands every
	// call to; empty when this host has none, and then there is no stub.
	flock string
}

func requireScriptTools(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skipf("the update script targets Linux hosts (GNU install, sha256sum); this is %s", runtime.GOOS)
	}
	for _, tool := range []string{"bash", "python3", "tar", "sha256sum"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed here; the update script needs it", tool)
		}
	}
}

// newScriptEnv prepares a run with the installed binary at version installed.
// checksumOK false writes a wrong sha256 into checksums.txt.
func newScriptEnv(t *testing.T, installed string, checksumOK bool) *scriptEnv {
	t.Helper()
	requireScriptTools(t)

	dir := t.TempDir()
	e := &scriptEnv{
		t:         t,
		dir:       dir,
		script:    filepath.Join(dir, "holzkube-manager-update.sh"),
		stubs:     filepath.Join(dir, "stubs"),
		bin:       filepath.Join(dir, "usr-local-bin", "holzkube-managerd"),
		previous:  filepath.Join(dir, "usr-local-lib", "holzkube-managerd.previous"),
		statusDir: filepath.Join(dir, "status"),
	}

	src, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "holzkube-manager-update.sh"))
	if err != nil {
		t.Fatalf("read the update script: %v", err)
	}
	// A script without these overrides would read the host's configuration
	// and binary, and as root would try to replace the real one. Refuse to
	// run it at all rather than trust the sandbox to stop it.
	for _, override := range []string{
		"HOLZKUBE_MANAGER_UPDATE_CONF", "HOLZKUBE_MANAGER_BIN", "HOLZKUBE_MANAGER_PREVIOUS",
		"HOLZKUBE_MANAGER_TOKEN_FILE", "HOLZKUBE_MANAGER_HEALTH_URL", "HOLZKUBE_MANAGER_UPDATE_STATUS_DIR",
	} {
		if !strings.Contains(string(src), override) {
			t.Fatalf("the update script does not honour %s; running it here would touch the host's own paths", override)
		}
	}
	e.write(e.script, string(src), 0o755)

	e.write(e.bin, fakeDaemon(installed), 0o755)
	if err := os.Mkdir(e.statusDir, 0o755); err != nil {
		t.Fatal(err)
	}

	e.setRelease(fakeDaemon(fakeRelease), checksumOK)
	e.write(filepath.Join(dir, "fixtures", "releases.json"), `[{"tag_name":"v`+fakeRelease+`","draft":false,"prerelease":true,"assets":[`+
		`{"id":11,"name":"holzkube-manager_`+fakeRelease+`_linux_arm64.tar.gz"},`+
		`{"id":12,"name":"holzkube-manager_`+fakeRelease+`_linux_amd64.tar.gz"},`+
		`{"id":13,"name":"checksums.txt"}]}]`, 0o644)

	e.write(filepath.Join(e.stubs, "curl"), curlStub, 0o755)
	e.write(filepath.Join(e.stubs, "systemctl"), systemctlStub, 0o755)
	e.write(filepath.Join(e.stubs, "journalctl"), "#!/usr/bin/env bash\nexit 0\n", 0o755)
	e.write(filepath.Join(e.stubs, "sleep"),
		"#!/usr/bin/env bash\nif [[ -n ${HKM_STUB_SLEEP_BLOCK:-} ]]; then touch \"$HKM_STUB_SLEEP_BLOCK\"; exec /bin/sleep 60; fi\nexit 0\n", 0o755)
	if flockPath, err := exec.LookPath("flock"); err == nil {
		e.flock = flockPath
		e.write(filepath.Join(e.stubs, "flock"), flockStub, 0o755)
	}
	return e
}

// setRelease puts daemon into the release archive the curl stub serves. The
// release is listed with both architectures, so the test passes on the Pi and
// on an amd64 runner alike. checksumOK false writes a wrong sha256.
func (e *scriptEnv) setRelease(daemon string, checksumOK bool) {
	e.t.Helper()
	archive := releaseArchive(e.t, daemon)
	e.write(filepath.Join(e.dir, "fixtures", "archive.tar.gz"), string(archive), 0o644)
	sum := sha256.Sum256(archive)
	hexSum := hex.EncodeToString(sum[:])
	if !checksumOK {
		hexSum = strings.Repeat("0", 64)
	}
	e.write(filepath.Join(e.dir, "fixtures", "checksums.txt"),
		hexSum+"  holzkube-manager_"+fakeRelease+"_linux_arm64.tar.gz\n"+
			hexSum+"  holzkube-manager_"+fakeRelease+"_linux_amd64.tar.gz\n", 0o644)
}

func (e *scriptEnv) write(path, content string, mode os.FileMode) {
	e.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		e.t.Fatal(err)
	}
}

// env is the whole environment of a run: nothing from the test process
// leaks in, so the host's own REPO, TMP or token never reach the script.
func (e *scriptEnv) env() []string {
	return append([]string{
		"PATH=" + e.stubs + ":/usr/local/bin:/usr/bin:/bin",
		"HOME=" + e.dir,
		"LC_ALL=C",
		"HKM_STUB_FIXTURES=" + filepath.Join(e.dir, "fixtures"),
		"HKM_STUB_HEALTH_URL=" + testHealthURL,
		"HOLZKUBE_MANAGER_UPDATE_CONF=" + filepath.Join(e.dir, "etc", "update.conf"),
		"HOLZKUBE_MANAGER_BIN=" + e.bin,
		"HOLZKUBE_MANAGER_PREVIOUS=" + e.previous,
		"HOLZKUBE_MANAGER_TOKEN_FILE=" + filepath.Join(e.dir, "etc", "github-token"),
		"HOLZKUBE_MANAGER_HEALTH_URL=" + testHealthURL,
		"HOLZKUBE_MANAGER_UPDATE_STATUS_DIR=" + e.statusDir,
		"HKM_STUB_FLOCK_REAL=" + e.flock,
	}, e.extra...)
}

// run executes the script copy and returns its exit code, read from the
// process itself.
func (e *scriptEnv) run(asRoot bool, args ...string) int {
	e.t.Helper()
	var cmd *exec.Cmd
	if asRoot {
		cmd = exec.Command("unshare", append([]string{"--user", "--map-root-user", "bash", e.script}, args...)...)
	} else {
		cmd = exec.Command("bash", append([]string{e.script}, args...)...)
	}
	cmd.Env = e.env()
	cmd.Dir = e.dir
	out, err := cmd.CombinedOutput()
	e.t.Logf("update script %v (root=%v):\n%s", args, asRoot, out)
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		e.t.Fatalf("run the update script: %v", err)
	}
	return exitErr.ExitCode()
}

// runSignalled starts the script, waits until a stub has created ready, and
// sends sig to the whole process group -- as systemctl stop does to the unit's
// cgroup and a terminal does on Ctrl-C. It returns the exit code, -1 when the
// script died of the signal itself.
func (e *scriptEnv) runSignalled(asRoot bool, ready string, sig syscall.Signal, args ...string) int {
	e.t.Helper()
	var cmd *exec.Cmd
	if asRoot {
		cmd = exec.Command("unshare", append([]string{"--user", "--map-root-user", "bash", e.script}, args...)...)
	} else {
		cmd = exec.Command("bash", append([]string{e.script}, args...)...)
	}
	cmd.Env = e.env()
	cmd.Dir = e.dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// A blocking stub that outlived the signal would hold the output pipe
	// for its whole sleep; Wait gives up on the pipe after this instead.
	cmd.WaitDelay = 5 * time.Second
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		e.t.Fatalf("start the update script: %v", err)
	}
	pgid := cmd.Process.Pid
	defer func() { _ = syscall.Kill(-pgid, syscall.SIGKILL) }()

	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
			_ = cmd.Wait()
			e.t.Fatalf("the script never reached the blocking stub:\n%s", out.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := syscall.Kill(-pgid, sig); err != nil {
		e.t.Fatalf("send %v: %v", sig, err)
	}
	err := cmd.Wait()
	e.t.Logf("update script %v (root=%v, %v):\n%s", args, asRoot, sig, out.String())
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		e.t.Fatalf("run the update script: %v", err)
	}
	return exitErr.ExitCode()
}

// status parses what the script recorded, with the daemon's reader.
func (e *scriptEnv) status() Status {
	e.t.Helper()
	path := filepath.Join(e.statusDir, "status.json")
	info, err := os.Stat(path)
	if err != nil {
		e.t.Fatalf("the script recorded no status: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		e.t.Errorf("status.json mode = %o, want 0644", info.Mode().Perm())
	}
	s, err := Read(os.DirFS(e.statusDir), "/status.json")
	if err != nil {
		e.t.Fatalf("the daemon's reader refuses what the script wrote: %v", err)
	}
	if s.CheckedAt.IsZero() {
		e.t.Errorf("checked_at is zero")
	}
	leftovers, _ := filepath.Glob(filepath.Join(e.statusDir, ".status.*"))
	if len(leftovers) != 0 {
		e.t.Errorf("temporary files left in the status directory: %v", leftovers)
	}
	return s
}

func (e *scriptEnv) wantStatus(outcome string, installed, latest *string) {
	e.t.Helper()
	s := e.status()
	if s.Outcome != outcome {
		e.t.Errorf("outcome = %q, want %q", s.Outcome, outcome)
	}
	if !samePtr(s.Installed, installed) {
		e.t.Errorf("installed = %s, want %s", show(s.Installed), show(installed))
	}
	if !samePtr(s.Latest, latest) {
		e.t.Errorf("latest = %s, want %s", show(s.Latest), show(latest))
	}
}

func (e *scriptEnv) wantNoStatus(dir string) {
	e.t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		e.t.Fatalf("read %s: %v", dir, err)
	}
	for _, entry := range entries {
		if entry.Name() == "status.json" || strings.HasPrefix(entry.Name(), ".status.") {
			e.t.Errorf("%s holds %s; this run must record nothing", dir, entry.Name())
		}
	}
}

// recordCalls points the curl and systemctl stubs at logs in the run's
// directory, so a test can ask afterwards what the script asked for.
func (e *scriptEnv) recordCalls() {
	e.t.Helper()
	e.extra = append(e.extra,
		"HKM_STUB_URLS="+filepath.Join(e.dir, "curl-urls"),
		"HKM_STUB_SYSTEMCTL_LOG="+filepath.Join(e.dir, "systemctl-calls"))
}

// wantInstalledNothing asserts what --check promises (D-19): the installed
// binary answers the version it had, no copy of it was kept as the previous
// one, the release list is the only URL asked for -- no asset, no checksum
// file, no health check -- and systemctl was never called. It needs
// recordCalls before the run.
func (e *scriptEnv) wantInstalledNothing(installed string) {
	e.t.Helper()
	if got := e.binVersion(); got != "holzkube-managerd "+installed {
		e.t.Errorf("installed binary answers %q, want %q: the check replaced it", got, "holzkube-managerd "+installed)
	}
	if _, err := os.Lstat(e.previous); !errors.Is(err, os.ErrNotExist) {
		e.t.Errorf("%s exists (%v): the check kept a previous binary, as only an install does", e.previous, err)
	}
	urls, err := os.ReadFile(filepath.Join(e.dir, "curl-urls"))
	if err != nil {
		e.t.Fatalf("the curl stub recorded no URL -- the check never asked for the release list, or recordCalls was not set: %v", err)
	}
	for _, u := range strings.Fields(string(urls)) {
		if !strings.HasSuffix(u, "/releases?per_page=20") {
			e.t.Errorf("the check asked for %s; it may ask for the release list and nothing else", u)
		}
	}
	if calls, err := os.ReadFile(filepath.Join(e.dir, "systemctl-calls")); err == nil {
		e.t.Errorf("the check called systemctl:\n%s", calls)
	} else if !errors.Is(err, os.ErrNotExist) {
		e.t.Fatal(err)
	}
}

// binVersion is what the installed binary answers now.
func (e *scriptEnv) binVersion() string {
	e.t.Helper()
	out, err := exec.Command(e.bin, "--version").Output()
	if err != nil {
		e.t.Fatalf("%s --version: %v", e.bin, err)
	}
	return strings.TrimSpace(string(out))
}

func fakeDaemon(version string) string {
	return "#!/bin/sh\necho \"holzkube-managerd " + version + "\"\n"
}

func releaseArchive(t *testing.T, daemon string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "holzkube-managerd", Mode: 0o755, Size: int64(len(daemon)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(daemon)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// curlStub answers the four kinds of request the script makes: the release
// list, an asset download (-o), and the health check. HKM_STUB_LIST_FAIL and
// HKM_STUB_HEALTHY steer it; HKM_STUB_PWD, when set, names a file each call
// appends the directory it was run in to, and HKM_STUB_URLS one each call
// appends the URL it was asked for to. HKM_STUB_LIST_BLOCK names a file the
// release-list request creates before it blocks, so a test can tell when to
// send its signal.
//
// HKM_STUB_LIST_GATE and HKM_STUB_HEALTH_GATE hold a run at the release list
// or at the health check until the test opens the gate: the request creates
// GATE.ready, waits until the file GATE exists (at most 30 s, then it fails),
// and then answers as usual. Two runs started with different gates can so be
// stopped exactly where they would overlap.
const curlStub = `#!/usr/bin/env bash
[[ -n ${HKM_STUB_PWD:-} ]] && pwd >> "$HKM_STUB_PWD"
gate() {
  touch "$1.ready"
  for _ in $(seq 1 600); do [[ -e $1 ]] && return 0; /bin/sleep 0.05; done
  echo "curl stub: gate $1 never opened" >&2; exit 28
}
out="" url=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o) out=$2; shift 2 ;;
    -H|--max-time) shift 2 ;;
    http*) url=$1; shift ;;
    *) shift ;;
  esac
done
[[ -n ${HKM_STUB_URLS:-} ]] && printf '%s\n' "$url" >> "$HKM_STUB_URLS"
case "$url" in
  */releases\?per_page=20)
    if [[ -n ${HKM_STUB_LIST_BLOCK:-} ]]; then touch "$HKM_STUB_LIST_BLOCK"; exec /bin/sleep 60; fi
    [[ -n ${HKM_STUB_LIST_GATE:-} ]] && gate "$HKM_STUB_LIST_GATE"
    [[ ${HKM_STUB_LIST_FAIL:-0} == 1 ]] && { echo '{"message":"stub: unavailable"}'; exit 22; }
    cat "$HKM_STUB_FIXTURES/releases.json" ;;
  */releases/assets/11|*/releases/assets/12) cp "$HKM_STUB_FIXTURES/archive.tar.gz" "$out" ;;
  */releases/assets/13) cp "$HKM_STUB_FIXTURES/checksums.txt" "$out" ;;
  "$HKM_STUB_HEALTH_URL")
    [[ -n ${HKM_STUB_HEALTH_GATE:-} ]] && gate "$HKM_STUB_HEALTH_GATE"
    [[ ${HKM_STUB_HEALTHY:-1} == 1 ]] || exit 7
    echo '{"audit_chain":"intact"}' ;;
  *) echo "curl stub: unexpected URL $url" >&2; exit 2 ;;
esac
`

// flockStub is flock(1) itself, run through a stub so a test can tell when a
// run has asked for the lock: HKM_STUB_FLOCK_LOG, when set, names a file each
// call appends its argv to before it hands over. exec keeps the descriptor the
// script locks. HKM_STUB_FLOCK_FAIL, when set, is the exit code every call
// ends with instead, as flock(1) ends on an error of its own rather than a
// held lock (71 is its EX_OSERR).
const flockStub = `#!/usr/bin/env bash
[[ -n ${HKM_STUB_FLOCK_LOG:-} ]] && printf '%s\n' "$*" >> "$HKM_STUB_FLOCK_LOG"
[[ -n ${HKM_STUB_FLOCK_FAIL:-} ]] && exit "$HKM_STUB_FLOCK_FAIL"
exec "$HKM_STUB_FLOCK_REAL" "$@"
`

// systemctlStub answers restart and is-active; HKM_STUB_SYSTEMCTL_LOG, when
// set, names a file each call appends its argv to.
const systemctlStub = `#!/usr/bin/env bash
[[ -n ${HKM_STUB_SYSTEMCTL_LOG:-} ]] && printf '%s\n' "$*" >> "$HKM_STUB_SYSTEMCTL_LOG"
case "$1" in
  restart) exit 0 ;;
  is-active) [[ ${HKM_STUB_HEALTHY:-1} == 1 ]] && exit 0; exit 3 ;;
  *) exit 0 ;;
esac
`

func TestUpdateScript(t *testing.T) {
	t.Parallel()
	requireScriptTools(t)

	t.Run("syntax", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		if out, err := exec.Command("bash", "-n", e.script).CombinedOutput(); err != nil {
			t.Fatalf("bash -n: %v\n%s", err, out)
		}
	})

	t.Run("check current", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeRelease, true)
		if rc := e.run(false, "--check"); rc != 0 {
			t.Fatalf("exit = %d, want 0", rc)
		}
		e.wantStatus(OutcomeCurrent, ptr(fakeRelease), ptr(fakeRelease))
	})

	t.Run("check available", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		e.recordCalls()
		if rc := e.run(false, "--check"); rc != 0 {
			t.Fatalf("exit = %d, want 0", rc)
		}
		e.wantStatus(OutcomeAvailable, ptr(fakeInstalled), ptr(fakeRelease))
		e.wantInstalledNothing(fakeInstalled)
	})

	t.Run("check with the release list unreadable", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		e.extra = append(e.extra, "HKM_STUB_LIST_FAIL=1")
		if rc := e.run(false, "--check"); rc != 1 {
			t.Fatalf("exit = %d, want 1", rc)
		}
		e.wantStatus(OutcomeFailed, ptr(fakeInstalled), nil)
	})

	t.Run("check with the status directory not writable", func(t *testing.T) {
		t.Parallel()
		if os.Geteuid() == 0 {
			t.Skip("running as root: a 0555 directory is still writable, so this case cannot be set up")
		}
		e := newScriptEnv(t, fakeInstalled, true)
		if err := os.Chmod(e.statusDir, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(e.statusDir, 0o755) })
		if rc := e.run(false, "--check"); rc != 0 {
			t.Fatalf("exit = %d, want 0 -- the same as with a writable directory", rc)
		}
		e.wantNoStatus(e.statusDir)
	})

	t.Run("check with python3 failing on the status write", func(t *testing.T) {
		t.Parallel()
		python, err := exec.LookPath("python3")
		if err != nil {
			t.Skip("python3 missing")
		}
		e := newScriptEnv(t, fakeInstalled, true)
		// python3 stays itself for the release list (python3 -c) and fails
		// for the status write (python3 - with the program on stdin). The
		// options before the program are skipped, whichever they are.
		e.write(filepath.Join(e.stubs, "python3"),
			"#!/usr/bin/env bash\nfor a in \"$@\"; do case $a in -) exit 1 ;; -c) break ;; esac; done\nexec "+python+" \"$@\"\n", 0o755)
		if rc := e.run(false, "--check"); rc != 0 {
			t.Fatalf("exit = %d, want 0 -- recording the status must never change the exit code", rc)
		}
		e.wantNoStatus(e.statusDir)
	})

	// CR-01: the script runs as root, and an operator runs it from wherever
	// the shell happens to be. python3 - and python3 -c put the current
	// directory first on sys.path and read PYTHONPATH, so a json.py or
	// datetime.py another user left there would run as root. Both places get
	// one here, each leaving a mark when imported; the script must import
	// neither, and nothing it runs may run in the caller's directory.
	t.Run("python3 imports nothing from the caller's directory or PYTHONPATH", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		marker := filepath.Join(e.dir, "hijacked")
		pylib := filepath.Join(e.dir, "pylib")
		for where, dir := range map[string]string{"cwd": e.dir, "PYTHONPATH": pylib} {
			for _, mod := range []string{"json", "datetime"} {
				e.write(filepath.Join(dir, mod+".py"),
					"open("+pyQuote(marker)+", 'a').write("+pyQuote(where+" "+mod+"\n")+")\n", 0o644)
			}
		}
		pwdLog := filepath.Join(e.dir, "curl-pwd")
		e.extra = append(e.extra, "PYTHONPATH="+pylib, "HKM_STUB_PWD="+pwdLog)
		rc := e.run(false, "--check")
		if got, err := os.ReadFile(marker); err == nil {
			t.Errorf("the script imported planted modules:\n%s", got)
		}
		if rc != 0 {
			t.Fatalf("exit = %d, want 0", rc)
		}
		e.wantStatus(OutcomeAvailable, ptr(fakeInstalled), ptr(fakeRelease))
		pwds, err := os.ReadFile(pwdLog)
		if err != nil {
			t.Fatalf("curl stub recorded no directory: %v", err)
		}
		for _, d := range strings.Fields(string(pwds)) {
			if d != "/" {
				t.Errorf("curl ran in %s, want / -- not the caller's directory", d)
			}
		}
	})

	// WR-04: a run ended by a signal is a failed run, and says so. Before the
	// signal traps, the EXIT trap saw $? = 0, recorded nothing, and the page
	// kept showing the previous run as if it were the latest.
	for _, tc := range []struct {
		sig  syscall.Signal
		code int
	}{{syscall.SIGTERM, 143}, {syscall.SIGINT, 130}, {syscall.SIGHUP, 129}} {
		t.Run("check ended by "+tc.sig.String()+" records failed", func(t *testing.T) {
			t.Parallel()
			e := newScriptEnv(t, fakeInstalled, true)
			ready := filepath.Join(e.dir, "list-requested")
			e.extra = append(e.extra, "HKM_STUB_LIST_BLOCK="+ready)
			rc := e.runSignalled(false, ready, tc.sig, "--check")
			e.wantStatus(OutcomeFailed, ptr(fakeInstalled), nil)
			if rc != tc.code {
				t.Errorf("exit = %d, want %d", rc, tc.code)
			}
		})
	}

	for _, tc := range []struct {
		name string
		args []string
		want int
	}{
		{"help", []string{"--help"}, 0},
		{"rollback refused without root", []string{"--rollback"}, 1},
		{"update refused without root", nil, 1},
		{"unknown option", []string{"--bogus"}, 1},
	} {
		t.Run(tc.name+" records nothing", func(t *testing.T) {
			t.Parallel()
			e := newScriptEnv(t, fakeInstalled, true)
			if rc := e.run(false, tc.args...); rc != tc.want {
				t.Fatalf("exit = %d, want %d", rc, tc.want)
			}
			e.wantNoStatus(e.statusDir)
		})
	}
}

// requireUserNamespace skips when an unprivileged user namespace cannot be
// made here, naming what unshare said. On the Pi it was measured working, so
// there this test runs.
func requireUserNamespace(t *testing.T) {
	t.Helper()
	requireScriptTools(t)
	if _, err := exec.LookPath("unshare"); err != nil {
		t.Skip("unshare (util-linux) is not installed")
	}
	out, err := exec.Command("unshare", "--user", "--map-root-user", "id", "-u").CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "0" {
		t.Skipf("no unprivileged user namespace here: %v: %s", err, out)
	}
}

func TestUpdateScriptAsRoot(t *testing.T) {
	t.Parallel()
	requireUserNamespace(t)

	t.Run("healthy update", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		// Absent: the script creates it, root-owned 0755.
		if err := os.Remove(e.statusDir); err != nil {
			t.Fatal(err)
		}
		if rc := e.run(true); rc != 0 {
			t.Fatalf("exit = %d, want 0", rc)
		}
		info, err := os.Stat(e.statusDir)
		if err != nil {
			t.Fatalf("status directory not created: %v", err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Errorf("status directory mode = %o, want 0755", info.Mode().Perm())
		}
		e.wantStatus(OutcomeUpdated, ptr(fakeRelease), ptr(fakeRelease))
		if got := e.binVersion(); got != "holzkube-managerd "+fakeRelease {
			t.Errorf("installed binary answers %q, want the release's", got)
		}
	})

	// The check unit runs --check as root (holzkube-manager-update-check.service),
	// where nothing but the script itself would stop an install: with a newer
	// release there, the check installs nothing, as root.
	//
	// Fault injected and seen red: the exit 0 that ends the check removed.
	t.Run("check installs nothing, as root", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		e.recordCalls()
		if rc := e.run(true, "--check"); rc != 0 {
			t.Fatalf("exit = %d, want 0", rc)
		}
		e.wantStatus(OutcomeAvailable, ptr(fakeInstalled), ptr(fakeRelease))
		e.wantInstalledNothing(fakeInstalled)
	})

	t.Run("already current", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeRelease, true)
		if rc := e.run(true); rc != 0 {
			t.Fatalf("exit = %d, want 0", rc)
		}
		e.wantStatus(OutcomeCurrent, ptr(fakeRelease), ptr(fakeRelease))
	})

	t.Run("unhealthy after restart rolls back", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		e.extra = append(e.extra, "HKM_STUB_HEALTHY=0")
		if rc := e.run(true); rc != 1 {
			t.Fatalf("exit = %d, want 1", rc)
		}
		e.wantStatus(OutcomeRolledBack, ptr(fakeInstalled), ptr(fakeRelease))
		if got := e.binVersion(); got != "holzkube-managerd "+fakeInstalled {
			t.Errorf("installed binary answers %q, want the previous one back", got)
		}
	})

	// WR-05: what is recorded as installed is what is on disk when the run
	// ends, and a run that cannot say what that is has not "updated". Each of
	// these used to record a version from before the run, or a null the
	// daemon's own reader refuses for anything but failed.
	t.Run("killed in the health loop records the binary it left installed", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		ready := filepath.Join(e.dir, "health-wait")
		e.extra = append(e.extra, "HKM_STUB_HEALTHY=0", "HKM_STUB_SLEEP_BLOCK="+ready)
		rc := e.runSignalled(true, ready, syscall.SIGTERM)
		// The new binary is in place and nothing rolled it back.
		e.wantStatus(OutcomeFailed, ptr(fakeRelease), ptr(fakeRelease))
		if got := e.binVersion(); got != "holzkube-managerd "+fakeRelease {
			t.Errorf("installed binary answers %q; the case assumes the kill came after the install", got)
		}
		if rc != 143 {
			t.Errorf("exit = %d, want 143", rc)
		}
	})

	t.Run("an update whose binary cannot say its version is failed", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		// Answers from the download directory, where the script checks it
		// before installing, and not once installed.
		e.setRelease("#!/bin/sh\ncase \"$0\" in */usr-local-bin/*) exit 1 ;; esac\n"+
			"echo \"holzkube-managerd "+fakeRelease+"\"\n", true)
		rc := e.run(true)
		e.wantStatus(OutcomeFailed, nil, ptr(fakeRelease))
		// The exit code stays the update's, whose health check passed; only
		// the record is downgraded.
		if rc != 0 {
			t.Errorf("exit = %d, want 0", rc)
		}
	})

	t.Run("rollback with no binary before the run records the one rolled back to", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		if err := os.Remove(e.bin); err != nil {
			t.Fatal(err)
		}
		e.write(e.previous, fakeDaemon("0.0.9"), 0o755)
		e.extra = append(e.extra, "HKM_STUB_HEALTHY=0")
		if rc := e.run(true); rc != 1 {
			t.Fatalf("exit = %d, want 1", rc)
		}
		e.wantStatus(OutcomeRolledBack, ptr("0.0.9"), ptr(fakeRelease))
	})

	t.Run("checksum mismatch", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, false)
		if rc := e.run(true); rc != 1 {
			t.Fatalf("exit = %d, want 1", rc)
		}
		e.wantStatus(OutcomeFailed, ptr(fakeInstalled), ptr(fakeRelease))
		if got := e.binVersion(); got != "holzkube-managerd "+fakeInstalled {
			t.Errorf("installed binary answers %q, want it untouched", got)
		}
	})

	t.Run("rollback records nothing", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		e.write(e.previous, fakeDaemon("0.0.9"), 0o755)
		if rc := e.run(true, "--rollback"); rc != 0 {
			t.Fatalf("exit = %d, want 0", rc)
		}
		e.wantNoStatus(e.statusDir)
	})

	t.Run("status directory is a symlink", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		target := filepath.Join(e.dir, "elsewhere")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(e.statusDir); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, e.statusDir); err != nil {
			t.Fatal(err)
		}
		if rc := e.run(true); rc != 0 {
			t.Fatalf("exit = %d, want 0 -- the refusal to record must not fail the update", rc)
		}
		e.wantNoStatus(target)
	})

	// WR-06: owned by root is not enough. A root-owned directory others can
	// write to lets them swap mktemp's file for a symlink before root writes
	// it -- with the sticky bit too, since the swapped file would be theirs.
	// Inside the user namespace the test's own directory is root's.
	for _, mode := range []os.FileMode{0o777, 0o775, 0o757, 0o777 | os.ModeSticky} {
		t.Run("status directory root-owned but mode "+mode.String(), func(t *testing.T) {
			t.Parallel()
			e := newScriptEnv(t, fakeInstalled, true)
			if err := os.Chmod(e.statusDir, mode); err != nil {
				t.Fatal(err)
			}
			if rc := e.run(true, "--check"); rc != 0 {
				t.Fatalf("exit = %d, want 0 -- the refusal to record must not fail the run", rc)
			}
			e.wantNoStatus(e.statusDir)
		})
	}

	// /tmp belongs to the host's root, which a user namespace maps to the
	// overflow uid: from inside, it is a directory root does not own.
	t.Run("status directory owned by another uid", func(t *testing.T) {
		const foreign = "/tmp"
		planted := filepath.Join(foreign, "status.json")
		if _, err := os.Lstat(planted); err == nil {
			t.Skipf("%s already exists; this case needs it absent to tell whether the script wrote it", planted)
		}
		before, _ := filepath.Glob(filepath.Join(foreign, ".status.*"))

		e := newScriptEnv(t, fakeInstalled, true)
		e.statusDir = foreign
		rc := e.run(true, "--check")

		if _, err := os.Lstat(planted); err == nil {
			_ = os.Remove(planted)
			t.Errorf("the script wrote %s into a directory root does not own", planted)
		}
		after, _ := filepath.Glob(filepath.Join(foreign, ".status.*"))
		if len(after) > len(before) {
			t.Errorf("the script left temporary files in %s: %v", foreign, after)
		}
		if rc != 0 {
			t.Fatalf("exit = %d, want 0", rc)
		}
	})
}

// background is a run of the script the test does not wait for at once, so
// that a second run can be started beside it.
type background struct {
	t    *testing.T
	name string
	cmd  *exec.Cmd
	out  bytes.Buffer
	done chan struct{}
	rc   int
}

// start runs the script copy with the run's environment plus extra, in a
// process group of its own, and returns at once. Whatever is still running
// when the test ends is killed, group and all.
func (e *scriptEnv) start(name string, asRoot bool, extra []string, args ...string) *background {
	e.t.Helper()
	var cmd *exec.Cmd
	if asRoot {
		cmd = exec.Command("unshare", append([]string{"--user", "--map-root-user", "bash", e.script}, args...)...)
	} else {
		cmd = exec.Command("bash", append([]string{e.script}, args...)...)
	}
	cmd.Env = append(e.env(), extra...)
	cmd.Dir = e.dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 5 * time.Second
	b := &background{t: e.t, name: name, cmd: cmd, done: make(chan struct{})}
	cmd.Stdout, cmd.Stderr = &b.out, &b.out
	if err := cmd.Start(); err != nil {
		e.t.Fatalf("start the %s: %v", name, err)
	}
	pgid := cmd.Process.Pid
	go func() {
		defer close(b.done)
		err := cmd.Wait()
		var exitErr *exec.ExitError
		switch {
		case err == nil:
			b.rc = 0
		case errors.As(err, &exitErr):
			b.rc = exitErr.ExitCode()
		default:
			b.rc = -2
		}
	}()
	e.t.Cleanup(func() {
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		<-b.done
	})
	return b
}

func (b *background) exited() bool {
	select {
	case <-b.done:
		return true
	default:
		return false
	}
}

// wait returns the run's exit code, read from the process itself, and fails
// the test if it has not ended within 30 s.
func (b *background) wait() int {
	b.t.Helper()
	select {
	case <-b.done:
	case <-time.After(30 * time.Second):
		b.t.Fatalf("the %s has not ended after 30 s", b.name)
	}
	b.t.Logf("%s (exit %d):\n%s", b.name, b.rc, b.out.String())
	return b.rc
}

// waitUntil polls cond for up to 20 s and fails the test with what when it
// never holds.
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("waited 20 s for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// holdLock takes the lock the script takes, from the test process, and keeps
// it until the test ends: a run beside it then finds it held.
func (e *scriptEnv) holdLock() {
	e.t.Helper()
	f, err := os.OpenFile(filepath.Join(e.statusDir, ".lock"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _ = f.Close() })
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		e.t.Fatalf("take the lock: %v", err)
	}
}

// 13-REVIEW-2 IN-02: a check (holzkube-manager-update-check.service) and the
// hourly update (holzkube-manager-update.service) run the same script, and
// nothing ordered them. Every run that may record takes one lock in the
// status directory first, waits a bounded time for it, and gives up without
// recording when it stays held. These cases run two copies of the script
// against one set of stand-ins, as root, and stop one where the other would
// overlap it.
func TestUpdateScriptRunsOneAtATime(t *testing.T) {
	t.Parallel()
	requireUserNamespace(t)
	if _, err := exec.LookPath("flock"); err != nil {
		t.Skip("flock (util-linux) is not installed; the script runs unlocked then")
	}

	// The race the review describes: a check that looked before the update
	// installed, and recorded after it, overwrote "updated" with "available"
	// -- for a release that was by then installed.
	t.Run("an update waits for a running check", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		gate := filepath.Join(e.dir, "list-gate")
		check := e.start("check", true, []string{"HKM_STUB_LIST_GATE=" + gate}, "--check")
		waitUntil(t, "the check to ask for the release list", func() bool { return exists(gate+".ready") || check.exited() })

		asked := filepath.Join(e.dir, "update-asked-for-the-lock")
		update := e.start("update", true, []string{"HKM_STUB_FLOCK_LOG=" + asked})
		waitUntil(t, "the update to ask for the lock or end", func() bool { return exists(asked) || update.exited() })
		if update.exited() {
			t.Errorf("the update ended while a check was looking; it must wait for it")
		}
		e.write(gate, "", 0o644)

		if rc := check.wait(); rc != 0 {
			t.Errorf("check exit = %d, want 0", rc)
		}
		if rc := update.wait(); rc != 0 {
			t.Errorf("update exit = %d, want 0", rc)
		}
		e.wantStatus(OutcomeUpdated, ptr(fakeRelease), ptr(fakeRelease))
		if got := e.binVersion(); got != "holzkube-managerd "+fakeRelease {
			t.Errorf("installed binary answers %q, want the release's", got)
		}
		info, err := os.Lstat(filepath.Join(e.statusDir, ".lock"))
		if err != nil {
			t.Fatalf("no lock file: %v", err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			t.Errorf("lock file mode = %v, want a regular file 0600: the daemon's user could otherwise hold it", info.Mode())
		}
	})

	// The other order: the update has installed and waits for the service to
	// come up healthy; a check started then waits until it has recorded.
	t.Run("a check waits for a running update", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		gate := filepath.Join(e.dir, "health-gate")
		update := e.start("update", true, []string{"HKM_STUB_HEALTH_GATE=" + gate})
		waitUntil(t, "the update to reach its health check", func() bool { return exists(gate+".ready") || update.exited() })

		asked := filepath.Join(e.dir, "check-asked-for-the-lock")
		check := e.start("check", true, []string{"HKM_STUB_FLOCK_LOG=" + asked}, "--check")
		waitUntil(t, "the check to ask for the lock or end", func() bool { return exists(asked) || check.exited() })
		if check.exited() {
			t.Errorf("the check ended while an update was installing; it must wait for it")
		}
		e.write(gate, "", 0o644)

		if rc := update.wait(); rc != 0 {
			t.Errorf("update exit = %d, want 0", rc)
		}
		if rc := check.wait(); rc != 0 {
			t.Errorf("check exit = %d, want 0", rc)
		}
		// The check looked after the install, so it found it current.
		e.wantStatus(OutcomeCurrent, ptr(fakeRelease), ptr(fakeRelease))
	})

	// Held beyond the wait: the run gives up, says why, and records nothing --
	// the holder records, and a record written beside it is the race.
	for _, tc := range []struct {
		name, article string
		args          []string
	}{{"check", "a", []string{"--check"}}, {"update", "an", nil}} {
		t.Run(tc.article+" "+tc.name+" gives up on a held lock and records nothing", func(t *testing.T) {
			t.Parallel()
			e := newScriptEnv(t, fakeInstalled, true)
			e.recordCalls()
			before := `{"checked_at": "2026-09-28T09:00:12Z", "installed": "0.1.0", "latest": "0.1.0", "outcome": "current"}` + "\n"
			e.write(filepath.Join(e.statusDir, "status.json"), before, 0o644)
			e.holdLock()
			run := e.start(tc.name, true, []string{"HOLZKUBE_MANAGER_UPDATE_LOCK_WAIT=1"}, tc.args...)
			if rc := run.wait(); rc != 1 {
				t.Errorf("exit = %d, want 1", rc)
			}
			if out := run.out.String(); !strings.Contains(out, "ein anderer Lauf") {
				t.Errorf("the run does not say another run holds the lock:\n%s", out)
			}
			if got, err := os.ReadFile(filepath.Join(e.statusDir, "status.json")); err != nil || string(got) != before {
				t.Errorf("status.json = %q (%v), want it untouched: %q", got, err, before)
			}
			e.wantInstalledNothingAtAll(fakeInstalled)
		})
	}

	// flock(1) failing for a reason of its own is not a held lock. The run
	// goes ahead unlocked, as it did before there was a lock, rather than wait
	// and give up every hour: the hourly update must not stop on the lock.
	t.Run("an update whose flock fails goes ahead unlocked", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		run := e.start("update", true, []string{"HKM_STUB_FLOCK_FAIL=71", "HOLZKUBE_MANAGER_UPDATE_LOCK_WAIT=1"})
		if rc := run.wait(); rc != 0 {
			t.Fatalf("exit = %d, want 0", rc)
		}
		if out := run.out.String(); !strings.Contains(out, "ohne Sperre") {
			t.Errorf("the run does not say it went ahead unlocked:\n%s", out)
		}
		e.wantStatus(OutcomeUpdated, ptr(fakeRelease), ptr(fakeRelease))
	})

	// A lock is the kernel's, on an open file: it ends with the last process
	// holding it. A run killed outright -- SIGKILL to the whole group, as
	// systemd's last step does -- leaves the file behind and no lock, and the
	// next run goes ahead at once.
	t.Run("a run killed while holding the lock does not hold the next", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		ready := filepath.Join(e.dir, "list-requested")
		e.extra = append(e.extra, "HKM_STUB_LIST_BLOCK="+ready)
		_ = e.runSignalled(true, ready, syscall.SIGKILL, "--check")
		if !exists(filepath.Join(e.statusDir, ".lock")) {
			t.Fatalf("the killed run left no lock file; this case needs one to show it does not hold the next run")
		}
		e.extra = []string{"HOLZKUBE_MANAGER_UPDATE_LOCK_WAIT=5"}
		if rc := e.run(true, "--check"); rc != 0 {
			t.Fatalf("exit = %d, want 0: a dead run's lock held this one", rc)
		}
		e.wantStatus(OutcomeAvailable, ptr(fakeInstalled), ptr(fakeRelease))
	})
}

// wantInstalledNothingAtAll is wantInstalledNothing for a run that gave up
// before it looked: not even the release list was asked for.
func (e *scriptEnv) wantInstalledNothingAtAll(installed string) {
	e.t.Helper()
	if got := e.binVersion(); got != "holzkube-managerd "+installed {
		e.t.Errorf("installed binary answers %q, want %q", got, "holzkube-managerd "+installed)
	}
	if _, err := os.Lstat(e.previous); !errors.Is(err, os.ErrNotExist) {
		e.t.Errorf("%s exists (%v): the run kept a previous binary", e.previous, err)
	}
	if urls, err := os.ReadFile(filepath.Join(e.dir, "curl-urls")); err == nil {
		e.t.Errorf("the run asked curl for:\n%s", urls)
	}
	if calls, err := os.ReadFile(filepath.Join(e.dir, "systemctl-calls")); err == nil {
		e.t.Errorf("the run called systemctl:\n%s", calls)
	}
}

// pyQuote is s as a Python string literal.
func pyQuote(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`, "\n", `\n`).Replace(s) + "'"
}
