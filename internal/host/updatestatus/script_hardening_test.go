package updatestatus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// installStub is install(1) that can be made to fail for the temporary file of
// an atomic install ("*.new"): HKM_STUB_INSTALL_FAIL_NEW=1. Everything else is
// the real install.
const installStub = `#!/usr/bin/env bash
dst="${@: -1}"
if [[ ${HKM_STUB_INSTALL_FAIL_NEW:-0} == 1 && $dst == *.new ]]; then
  echo "install stub: cannot write $dst" >&2; exit 1
fi
exec /usr/bin/install "$@"
`

// mvStub logs "src dst" of each call to HKM_STUB_MV_LOG and hands over to mv.
const mvStub = `#!/usr/bin/env bash
[[ -n ${HKM_STUB_MV_LOG:-} ]] && printf '%s\n' "${*: -2}" >> "$HKM_STUB_MV_LOG"
exec /bin/mv "$@"
`

// TestUpdateScriptHardening holds the behaviours of the 2026-10 audit: what
// the script installs (only a newer stable release), what it refuses (no
// checksum file, a version that failed before), how it replaces files
// (atomically, with a syntax check and a copy of the old script) and what it
// counts as healthy (several answers in a row, no restart in between).
func TestUpdateScriptHardening(t *testing.T) {
	t.Parallel()
	requireUserNamespace(t)

	downloaded := func(e *scriptEnv) bool { return e.countCalls("curl-urls", "/releases/assets/") > 0 }

	t.Run("picks the highest version, not the first of the list", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		e.setRelease(fakeDaemon("0.3.0"), true)
		e.setReleases("v0.2.5", "v0.3.0", "v0.2.0")
		if rc := e.run(true); rc != 0 {
			t.Fatalf("exit = %d, want 0", rc)
		}
		e.wantStatus(OutcomeUpdated, ptr("0.3.0"), ptr("0.3.0"))
	})

	t.Run("skips release candidates, betas and drafts", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		e.setRelease(fakeDaemon("0.3.0"), true)
		e.setReleases("v0.9.0-rc.1", "v0.8.0-beta.1", "draft:v0.7.0", "v0.3.0")
		if rc := e.run(true); rc != 0 {
			t.Fatalf("exit = %d, want 0", rc)
		}
		e.wantStatus(OutcomeUpdated, ptr("0.3.0"), ptr("0.3.0"))
	})

	t.Run("only a prerelease tag listed is not installable", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		e.setReleases("v0.9.0-rc.1")
		e.recordCalls()
		if rc := e.run(true); rc != 1 {
			t.Fatalf("exit = %d, want 1", rc)
		}
		e.wantStatus(OutcomeFailed, ptr(fakeInstalled), nil)
		if downloaded(e) {
			t.Errorf("the run downloaded an asset")
		}
	})

	t.Run("a release that is not newer installs nothing", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, "0.5.0", true)
		e.recordCalls()
		if rc := e.run(true); rc != 0 {
			t.Fatalf("exit = %d, want 0", rc)
		}
		e.wantStatus(OutcomeCurrent, ptr("0.5.0"), ptr(fakeRelease))
		if got := e.binVersion(); got != "holzkube-managerd 0.5.0" {
			t.Errorf("installed binary answers %q: the run downgraded", got)
		}
		if downloaded(e) {
			t.Errorf("the run downloaded an asset for a downgrade")
		}
		// --force reinstalls the same version; it does not allow going back.
		if rc := e.run(true, "--force"); rc != 0 {
			t.Fatalf("--force exit = %d, want 0", rc)
		}
		if got := e.binVersion(); got != "holzkube-managerd 0.5.0" {
			t.Errorf("--force downgraded to %q", got)
		}
		// --check says "current", not "available".
		if rc := e.run(true, "--check"); rc != 0 {
			t.Fatalf("--check exit = %d, want 0", rc)
		}
		e.wantStatus(OutcomeCurrent, ptr("0.5.0"), ptr(fakeRelease))
	})

	t.Run("--allow-downgrade goes back", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, "0.5.0", true)
		if rc := e.run(true, "--allow-downgrade"); rc != 0 {
			t.Fatalf("exit = %d, want 0", rc)
		}
		e.wantStatus(OutcomeUpdated, ptr(fakeRelease), ptr(fakeRelease))
	})

	t.Run("a release without checksums.txt is refused", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		e.write(filepath.Join(e.dir, "fixtures", "releases.json"), `[{"tag_name":"v`+fakeRelease+`","draft":false,"assets":[`+
			`{"id":11,"name":"holzkube-manager_`+fakeRelease+`_linux_arm64.tar.gz"},`+
			`{"id":12,"name":"holzkube-manager_`+fakeRelease+`_linux_amd64.tar.gz"}]}]`, 0o644)
		e.recordCalls()
		out, rc := e.runOutput(true)
		if rc != 1 {
			t.Fatalf("exit = %d, want 1", rc)
		}
		if !strings.Contains(out, "checksums.txt") {
			t.Errorf("the refusal does not name checksums.txt:\n%s", out)
		}
		e.wantStatus(OutcomeFailed, ptr(fakeInstalled), ptr(fakeRelease))
		if got := e.binVersion(); got != "holzkube-managerd "+fakeInstalled {
			t.Errorf("installed binary answers %q: installed without a checksum", got)
		}
		if downloaded(e) {
			t.Errorf("the run downloaded the archive before it knew it could not check it")
		}
	})

	t.Run("a version that failed its health check is not installed again", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		e.extra = append(e.extra, "HKM_STUB_HEALTHY=0")
		if rc := e.run(true); rc != 1 {
			t.Fatalf("exit = %d, want 1", rc)
		}
		e.wantStatus(OutcomeRolledBack, ptr(fakeInstalled), ptr(fakeRelease))
		if got := e.badVersion(); got != fakeRelease {
			t.Fatalf("bad-version = %q, want %q", got, fakeRelease)
		}

		// The next (healthy) hour: same release, skipped, nothing downloaded.
		e.extra = nil
		e.recordCalls()
		if rc := e.run(true); rc != 0 {
			t.Fatalf("second run exit = %d, want 0", rc)
		}
		e.wantStatus(OutcomeCurrent, ptr(fakeInstalled), ptr(fakeRelease))
		if downloaded(e) {
			t.Errorf("the run downloaded the version that failed before")
		}
		if got := e.binVersion(); got != "holzkube-managerd "+fakeInstalled {
			t.Errorf("installed binary answers %q", got)
		}

		// --force asks for it by name, installs it and forgets the mark.
		if rc := e.run(true, "--force"); rc != 0 {
			t.Fatalf("--force exit = %d, want 0", rc)
		}
		e.wantStatus(OutcomeUpdated, ptr(fakeRelease), ptr(fakeRelease))
		if got := e.badVersion(); got != "" {
			t.Errorf("bad-version = %q after a healthy update, want none", got)
		}
	})

	t.Run("a newer tag is installed despite an older bad version", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		e.extra = append(e.extra, "HKM_STUB_HEALTHY=0")
		if rc := e.run(true); rc != 1 {
			t.Fatalf("exit = %d, want 1", rc)
		}
		e.extra = nil
		e.setRelease(fakeDaemon("0.3.0"), true)
		e.setReleases("v0.3.0")
		if rc := e.run(true); rc != 0 {
			t.Fatalf("exit = %d, want 0", rc)
		}
		e.wantStatus(OutcomeUpdated, ptr("0.3.0"), ptr("0.3.0"))
	})

	t.Run("--check does not offer a version known to be bad", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		e.write(filepath.Join(e.statusDir, "bad-version"), fakeRelease+"\n", 0o644)
		if rc := e.run(true, "--check"); rc != 0 {
			t.Fatalf("exit = %d, want 0", rc)
		}
		e.wantStatus(OutcomeCurrent, ptr(fakeInstalled), ptr(fakeRelease))
	})

	t.Run("a broken script in the archive is not installed", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		old, err := os.ReadFile(e.script)
		if err != nil {
			t.Fatal(err)
		}
		e.setReleaseWith(fakeDaemon(fakeRelease), "#!/usr/bin/env bash\nif then fi (\n", true)
		out, rc := e.runOutput(true)
		if rc != 0 {
			t.Fatalf("exit = %d, want 0: a bad script must not fail a healthy update", rc)
		}
		e.wantStatus(OutcomeUpdated, ptr(fakeRelease), ptr(fakeRelease))
		if got, _ := os.ReadFile(e.script); string(got) != string(old) {
			t.Errorf("the script was replaced by one that does not parse")
		}
		if !strings.Contains(out, "WARNUNG") {
			t.Errorf("no warning that the new script was refused:\n%s", out)
		}
		if exists(e.script + ".previous") {
			t.Errorf("%s.previous exists for a replacement that did not happen", e.script)
		}
	})

	t.Run("the replaced script is kept as .previous", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		old, err := os.ReadFile(e.script)
		if err != nil {
			t.Fatal(err)
		}
		newer := string(old) + "# newer\n"
		e.setReleaseWith(fakeDaemon(fakeRelease), newer, true)
		if rc := e.run(true); rc != 0 {
			t.Fatalf("exit = %d, want 0", rc)
		}
		if got, _ := os.ReadFile(e.script); string(got) != newer {
			t.Errorf("the script was not replaced")
		}
		if got, _ := os.ReadFile(e.script + ".previous"); string(got) != string(old) {
			t.Errorf("%s.previous does not hold the old script", e.script)
		}
		if exists(e.script + ".new") {
			t.Errorf("%s.new left behind", e.script)
		}
	})

	t.Run("binaries are laid down through a temporary file and mv", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		mvLog := filepath.Join(e.dir, "mv-log")
		e.write(filepath.Join(e.stubs, "mv"), mvStub, 0o755)
		e.extra = append(e.extra, "HKM_STUB_MV_LOG="+mvLog)
		if rc := e.run(true); rc != 0 {
			t.Fatalf("exit = %d, want 0", rc)
		}
		data, _ := os.ReadFile(mvLog)
		for _, want := range []string{e.bin + ".new " + e.bin, e.previous + ".new " + e.previous} {
			if !strings.Contains(string(data), want) {
				t.Errorf("no mv %s; mv calls:\n%s", want, data)
			}
		}
		for _, p := range []string{e.bin + ".new", e.previous + ".new"} {
			if exists(p) {
				t.Errorf("%s left behind", p)
			}
		}
	})

	t.Run("a failed write of the new binary leaves the old one whole", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		e.write(filepath.Join(e.stubs, "install"), installStub, 0o755)
		e.extra = append(e.extra, "HKM_STUB_INSTALL_FAIL_NEW=1")
		if rc := e.run(true); rc != 1 {
			t.Fatalf("exit = %d, want 1", rc)
		}
		if got := e.binVersion(); got != "holzkube-managerd "+fakeInstalled {
			t.Errorf("installed binary answers %q after a failed install", got)
		}
		if exists(e.bin + ".new") {
			t.Errorf("%s.new left behind", e.bin)
		}
	})

	t.Run("a healthy update asks several times, not once", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		e.recordCalls()
		if rc := e.run(true); rc != 0 {
			t.Fatalf("exit = %d, want 0", rc)
		}
		e.wantStatus(OutcomeUpdated, ptr(fakeRelease), ptr(fakeRelease))
		// One answer before the install (the host check), three after.
		if n := e.countCalls("curl-urls", testHealthURL); n != 4 {
			t.Errorf("the health URL was asked %d times, want 4 (1 before, 3 in a row after the restart)", n)
		}
	})

	t.Run("answers that are never healthy twice in a row roll back", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		e.extra = append(e.extra, "HKM_STUB_HEALTH_ALTERNATE=1")
		if rc := e.run(true); rc != 1 {
			t.Fatalf("exit = %d, want 1", rc)
		}
		e.wantStatus(OutcomeRolledBack, ptr(fakeInstalled), ptr(fakeRelease))
	})

	t.Run("a service that restarts while it is being watched rolls back", func(t *testing.T) {
		t.Parallel()
		e := newScriptEnv(t, fakeInstalled, true)
		e.extra = append(e.extra, "HKM_STUB_PID_CHANGE=1")
		if rc := e.run(true); rc != 1 {
			t.Fatalf("exit = %d, want 1", rc)
		}
		e.wantStatus(OutcomeRolledBack, ptr(fakeInstalled), ptr(fakeRelease))
		if got := e.binVersion(); got != "holzkube-managerd "+fakeInstalled {
			t.Errorf("installed binary answers %q, want the previous one back", got)
		}
	})
}
