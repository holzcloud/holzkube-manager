package hostaction

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// What the operator installs by hand has to agree with what the daemon looks
// for, and has to load the way it reads. The helper is five files outside this
// package -- two units, the script, the guide, the release archive's list --
// plus the paths and the install commands in here, and none of them can see
// the others. These tests hold them together (D-17, D-18, D-19, HACT-08):
//
//   - TestUnitsVerify: systemd itself accepts both units, read by its output
//     and not only by its exit code, with a negative control;
//   - TestUnitsAgree: the units' paths and the script's defaults are the
//     paths this package uses, and the service carries the hardening D-18
//     asks for and none that would break a reboot;
//   - TestInstallCommandsMatchTheGuide: the guide's install block is
//     InstallCommands, byte for byte;
//   - TestGuideKeepsTheDaemonsHardening: the guide names the daemon's
//     hardening as unchanged and asks for none of it to go;
//   - TestTheArchiveCarriesTheHelper: the release archive has all of it;
//   - TestTheUpdateScriptDoesNotShipTheHelper: the root script that replaces
//     itself does not start carrying other root code.
//
// Nothing here installs anything: the units are verified as copies in a
// temporary directory, and no command addresses the system manager.

// The shipped files, from this package's directory.
var (
	deployDir         = filepath.Join("..", "..", "..", "deploy")
	shippedPathUnit   = filepath.Join(deployDir, "holzkube-manager-host.path")
	shippedService    = filepath.Join(deployDir, "holzkube-manager-host.service")
	shippedScript     = filepath.Join(deployDir, "holzkube-manager-host.sh")
	shippedGuide      = filepath.Join(deployDir, "HOST-HELPER.md")
	shippedUpdater    = filepath.Join(deployDir, "holzkube-manager-update.sh")
	goreleaserConfig  = filepath.Join("..", "..", "..", ".goreleaser.yaml")
	noSystemdAnalyze  = "HOLZKUBE_MANAGER_NO_SYSTEMD_ANALYZE"
	systemdAnalyzeMax = 2 * time.Minute
)

// unitAssignment is one Key=Value line of a unit file.
type unitAssignment struct {
	section string
	key     string
	value   string
	line    int
}

// unitFile is a unit file as far as these tests need it: its sections in
// order and its assignments. This is not a unit parser -- systemd-analyze is
// that -- only enough to compare names and values. Comments (# and ;) and
// blank lines are skipped; a line that is neither a section header nor an
// assignment fails the test rather than being ignored.
type unitFile struct {
	name        string
	sections    []string
	assignments []unitAssignment
}

func readUnitFile(t *testing.T, path string) unitFile {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	u := unitFile{name: filepath.Base(path)}
	section := ""
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";"):
			continue
		case strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]"):
			section = strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			u.sections = append(u.sections, section)
		default:
			key, value, ok := strings.Cut(line, "=")
			if !ok || section == "" {
				t.Fatalf("%s:%d: %q is neither a section header nor an assignment inside one", path, n, line)
			}
			u.assignments = append(u.assignments, unitAssignment{
				section: section,
				key:     strings.TrimSpace(key),
				value:   strings.TrimSpace(value),
				line:    n,
			})
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return u
}

// values returns every value assigned to key in section, in file order.
func (u unitFile) values(section, key string) []string {
	var out []string
	for _, a := range u.assignments {
		if a.section == section && a.key == key {
			out = append(out, a.value)
		}
	}
	return out
}

// lines returns every assignment of key, in any section.
func (u unitFile) lines(key string) []unitAssignment {
	var out []unitAssignment
	for _, a := range u.assignments {
		if a.key == key {
			out = append(out, a)
		}
	}
	return out
}

func (u unitFile) hasSection(name string) bool {
	for _, s := range u.sections {
		if s == name {
			return true
		}
	}
	return false
}

// wantOnly fails unless key is assigned in section, and every assignment is
// want. A second, later line with another value is what systemd would use, so
// "the first one is right" is not enough.
func wantOnly(t *testing.T, u unitFile, section, key, want string) {
	t.Helper()
	got := u.values(section, key)
	if len(got) == 0 {
		t.Errorf("%s: [%s] has no %s=; want %s=%s", u.name, section, key, key, want)
		return
	}
	for _, v := range got {
		if v != want {
			t.Errorf("%s: [%s] %s=%s, want %s=%s (every assignment: %q)", u.name, section, key, v, key, want, got)
		}
	}
}

// TestUnitsVerify runs systemd-analyze verify over copies of both units.
//
// Its exit code alone says nothing about the hardening: an unknown key
// ("ProtectHom=true") or a bad value is reported as a warning, the line is
// ignored, and verify still exits 0 -- measured three ways on the Pi. A unit
// with a misspelt protection loads without that protection. So any output at
// all fails the test, and a copy with exactly that misspelling must produce
// output: without that control, a verify that stopped reporting would leave
// this green.
//
// ExecStart= is rewritten to an executable in the temporary directory, because
// verify fails with "is not executable" when the command does not exist, and
// the helper is (deliberately) not installed where the tests run.
// TestUnitsAgree holds the shipped ExecStart= to HelperScriptPath instead.
func TestUnitsVerify(t *testing.T) {
	bin, err := exec.LookPath("systemd-analyze")
	if err != nil {
		if runtime.GOOS != "linux" || os.Getenv(noSystemdAnalyze) == "1" {
			t.Skipf("SKIPPED, not verified: systemd-analyze is not available here (%v); the helper's units were not checked", err)
		}
		t.Fatalf("systemd-analyze is not installed, so the helper's units cannot be verified (%v). "+
			"Install systemd, or set %s=1 to skip this knowingly -- the test then reports SKIPPED, not verified.", err, noSystemdAnalyze)
	}

	pathSrc, err := os.ReadFile(shippedPathUnit)
	if err != nil {
		t.Fatalf("read the path unit: %v", err)
	}
	serviceSrc, err := os.ReadFile(shippedService)
	if err != nil {
		t.Fatalf("read the service unit: %v", err)
	}

	verify := func(t *testing.T, service string) (string, int) {
		t.Helper()
		dir := t.TempDir()
		stub := filepath.Join(dir, "helper-stub")
		if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		execStart := regexp.MustCompile(`(?m)^ExecStart=.*$`)
		if n := len(execStart.FindAllString(service, -1)); n != 1 {
			t.Fatalf("the service has %d ExecStart= lines, want exactly 1 to point at the stub", n)
		}
		service = execStart.ReplaceAllLiteralString(service, "ExecStart="+stub)

		pathFile := filepath.Join(dir, filepath.Base(shippedPathUnit))
		serviceFile := filepath.Join(dir, filepath.Base(shippedService))
		if err := os.WriteFile(pathFile, pathSrc, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(serviceFile, []byte(service), 0o644); err != nil {
			t.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), systemdAnalyzeMax)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, "verify", "--man=no", pathFile, serviceFile)
		cmd.Env = append(os.Environ(), "SYSTEMD_COLORS=0", "SYSTEMD_PAGER=", "LC_ALL=C")
		out, err := cmd.CombinedOutput()
		var exitErr *exec.ExitError
		switch {
		case err == nil:
			return string(out), 0
		case errors.As(err, &exitErr):
			return string(out), exitErr.ExitCode()
		default:
			t.Fatalf("run systemd-analyze verify: %v\n%s", err, out)
			return "", -1
		}
	}

	out, rc := verify(t, string(serviceSrc))
	if rc != 0 || out != "" {
		t.Errorf("systemd-analyze verify on the shipped units: exit %d, output:\n%s\n"+
			"Any output fails this gate, not only a non-zero exit: verify reports an unknown key or a bad value "+
			"as a warning, ignores the line and still exits 0, so the unit would load without that protection.",
			rc, out)
	}

	// The negative control. The shipped service must have the line, or there
	// is nothing to misspell and the control would prove nothing.
	misspelt := strings.Replace(string(serviceSrc), "\nProtectHome=true\n", "\nProtectHom=true\n", 1)
	if misspelt == string(serviceSrc) {
		t.Fatalf("the shipped service has no line ProtectHome=true to misspell for the negative control")
	}
	out, rc = verify(t, misspelt)
	if out == "" {
		t.Errorf("negative control: systemd-analyze verify printed nothing (exit %d) for a service with ProtectHom=true. "+
			"If verify no longer reports an unknown key, an empty output no longer means the units are clean, and this gate proves nothing.", rc)
	} else {
		t.Logf("negative control (ProtectHom=true), exit %d: %s", rc, strings.TrimSpace(out))
	}
}

// TestUnitsAgree holds the units, the script's defaults and this package's
// paths to one another, and the service to the hardening D-18 asks for.
func TestUnitsAgree(t *testing.T) {
	t.Parallel()

	path := readUnitFile(t, shippedPathUnit)
	service := readUnitFile(t, shippedService)

	// The file names are the names Detect looks for and the install command
	// copies.
	if path.name != filepath.Base(PathUnitPath) {
		t.Errorf("the shipped path unit is %s, Detect looks for %s", path.name, PathUnitPath)
	}
	if service.name != filepath.Base(ServiceUnitPath) {
		t.Errorf("the shipped service is %s, Detect looks for %s", service.name, ServiceUnitPath)
	}

	// The path unit: one trigger, on the order, starting the service, pulled
	// in by paths.target.
	triggers := []string{"PathExists", "PathExistsGlob", "PathChanged", "PathModified", "DirectoryNotEmpty"}
	var found []unitAssignment
	for _, k := range triggers {
		found = append(found, path.lines(k)...)
	}
	if len(found) != 1 || found[0].key != "PathExists" || found[0].value != ReferenceOrderPath {
		t.Errorf("%s: triggers %+v, want exactly PathExists=%s", path.name, found, ReferenceOrderPath)
	}
	wantOnly(t, path, "Path", "Unit", filepath.Base(ServiceUnitPath))
	wantOnly(t, path, "Install", "WantedBy", "paths.target")
	if filepath.Dir(WantsLinkPath) != filepath.Join(filepath.Dir(PathUnitPath), "paths.target.wants") ||
		filepath.Base(WantsLinkPath) != filepath.Base(PathUnitPath) {
		t.Errorf("WantsLinkPath %s is not where `systemctl enable` links a unit with WantedBy=paths.target", WantsLinkPath)
	}
	// MakeDirectory= would create a missing data directory as root 0755, and
	// the daemon then refuses to start in it.
	if l := path.lines("MakeDirectory"); len(l) > 0 {
		t.Errorf("%s:%d: MakeDirectory= in the path unit", path.name, l[0].line)
	}

	// The service: runs the installed script, once per start.
	wantOnly(t, service, "Service", "ExecStart", HelperScriptPath)
	wantOnly(t, service, "Service", "Type", "oneshot")
	if v := service.values("Service", "TimeoutStartSec"); len(v) == 0 || v[len(v)-1] == "" || v[len(v)-1] == "infinity" || v[len(v)-1] == "0" {
		t.Errorf("%s: TimeoutStartSec=%q; a oneshot has no start limit without one", service.name, v)
	}
	// Its own state, where the daemon reads the result.
	resultDir := filepath.Dir(DefaultResultPath)
	if filepath.Dir(resultDir) != "/var/lib" {
		t.Errorf("DefaultResultPath %s is not under /var/lib, where StateDirectory= puts it", DefaultResultPath)
	}
	wantOnly(t, service, "Service", "StateDirectory", filepath.Base(resultDir))
	// The consuming rm: ProtectSystem=strict makes the daemon's directory
	// read-only without this line.
	orderDir := filepath.Dir(ReferenceOrderPath)
	rw := service.values("Service", "ReadWritePaths")
	hasOrderDir := false
	for _, v := range rw {
		for _, f := range strings.Fields(v) {
			if strings.TrimPrefix(f, "-") == orderDir {
				hasOrderDir = true
			}
		}
	}
	if !hasOrderDir {
		t.Errorf("%s: ReadWritePaths=%q does not name %s; under ProtectSystem=strict the script cannot remove the order", service.name, rw, orderDir)
	}

	// D-18: hardened as far as a reboot allows.
	for _, kv := range [][2]string{
		{"NoNewPrivileges", "true"},
		{"ProtectSystem", "strict"},
		{"ProtectHome", "true"},
		{"PrivateTmp", "true"},
		{"PrivateDevices", "true"},
		{"ProtectKernelTunables", "true"},
		{"ProtectKernelModules", "true"},
		{"ProtectKernelLogs", "true"},
		{"ProtectControlGroups", "true"},
		{"ProtectClock", "true"},
		{"ProtectHostname", "true"},
		{"RestrictAddressFamilies", "AF_UNIX"},
		{"RestrictNamespaces", "true"},
		{"RestrictRealtime", "true"},
		{"RestrictSUIDSGID", "true"},
		{"LockPersonality", "true"},
		{"MemoryDenyWriteExecute", "true"},
		{"SystemCallArchitectures", "native"},
		{"StateDirectoryMode", "0755"},
		{"UMask", "0022"},
	} {
		wantOnly(t, service, "Service", kv[0], kv[1])
	}

	// And nothing that stops it from working. No [Install]: only the path
	// unit starts it. No RemainAfterExit: an active oneshot never runs a
	// second order. No capability or system-call restriction: the reboot goes
	// through logind (polkit or CAP_SYS_BOOT), and entering the daemon's 0700
	// directory takes CAP_DAC_OVERRIDE (Pitfall 7). No Environment: the
	// script's overrides are for tests.
	if service.hasSection("Install") {
		t.Errorf("%s has an [Install] section; only the path unit may start it", service.name)
	}
	for _, key := range []string{"RemainAfterExit", "Environment", "EnvironmentFile", "CapabilityBoundingSet", "AmbientCapabilities", "SystemCallFilter"} {
		if l := service.lines(key); len(l) > 0 {
			t.Errorf("%s:%d: %s=%s must not be in the helper's service", service.name, l[0].line, key, l[0].value)
		}
	}
	for _, key := range []string{"Environment", "EnvironmentFile"} {
		if l := path.lines(key); len(l) > 0 {
			t.Errorf("%s:%d: %s= in the path unit", path.name, l[0].line, key)
		}
	}

	// The script's defaults are the same paths.
	script, err := os.ReadFile(shippedScript)
	if err != nil {
		t.Fatalf("read the helper script: %v", err)
	}
	for _, c := range []struct{ variable, override, want string }{
		{"ORDER", "HOLZKUBE_MANAGER_HOST_ORDER", ReferenceOrderPath},
		{"STATE_DIR", "HOLZKUBE_MANAGER_HOST_STATE_DIR", resultDir},
	} {
		re := regexp.MustCompile(`(?m)^` + c.variable + `=\$\{` + c.override + `:-([^}]*)\}$`)
		m := re.FindAllStringSubmatch(string(script), -1)
		if len(m) != 1 {
			t.Errorf("the script has %d lines %s=${%s:-...}, want exactly 1", len(m), c.variable, c.override)
			continue
		}
		if m[0][1] != c.want {
			t.Errorf("the script's %s defaults to %s, want %s", c.variable, m[0][1], c.want)
		}
	}
	if !strings.Contains(string(script), `"$STATE_DIR/`+filepath.Base(DefaultResultPath)+`"`) {
		t.Errorf("the script does not write $STATE_DIR/%s, the file ReadResult reads", filepath.Base(DefaultResultPath))
	}
}

// guideBlock returns the lines between the guide's install markers, fence
// lines excluded.
func guideBlock(t *testing.T, guide string) string {
	t.Helper()
	const begin, end = "<!-- install-commands:begin -->", "<!-- install-commands:end -->"
	lines := strings.Split(guide, "\n")
	b, e := -1, -1
	for i, l := range lines {
		switch l {
		case begin:
			if b >= 0 {
				t.Fatalf("the guide has %s twice", begin)
			}
			b = i
		case end:
			if e >= 0 {
				t.Fatalf("the guide has %s twice", end)
			}
			e = i
		}
	}
	if b < 0 || e < 0 || e < b {
		t.Fatalf("the guide has no install block between %s and %s (lines %d, %d)", begin, end, b+1, e+1)
	}
	var block []string
	for _, l := range lines[b+1 : e] {
		if strings.HasPrefix(l, "```") {
			continue
		}
		block = append(block, l)
	}
	return strings.Join(block, "\n")
}

// TestInstallCommandsMatchTheGuide: the page shows InstallCommands when the
// helper is missing, and the guide carries the same lines. A difference of one
// character sends the operator somewhere else, so they are compared byte for
// byte.
func TestInstallCommandsMatchTheGuide(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(shippedGuide)
	if err != nil {
		t.Fatalf("read the guide: %v", err)
	}
	got := guideBlock(t, string(data))
	want := strings.Join(InstallCommands, "\n")
	if got != want {
		t.Errorf("the guide's install block differs from InstallCommands (the page's block).\nguide:\n%q\nInstallCommands:\n%q", got, want)
	}

	// Every file the commands install is one that is shipped.
	for _, cmd := range InstallCommands {
		for _, f := range strings.Fields(cmd) {
			if !strings.HasPrefix(f, "deploy/") {
				continue
			}
			if _, err := os.Stat(filepath.Join(deployDir, strings.TrimPrefix(f, "deploy/"))); err != nil {
				t.Errorf("the install commands name %s, which is not in the repository: %v", f, err)
			}
		}
	}
}

// daemonHardening is the reference daemon unit's hardening the guide names as
// unchanged (D-18). Installing the helper asks for none of it to go.
var daemonHardening = []string{
	"NoNewPrivileges=true",
	"CapabilityBoundingSet=",
	"RestrictAddressFamilies=AF_INET AF_INET6",
	"ProcSubset=pid",
	"ProtectProc=invisible",
	"ProtectSystem=strict",
	"StateDirectoryMode=0700",
	"UMask=0077",
}

// weakening are phrases that, in the guide, would ask for the daemon's
// hardening to be loosened. The helper's own unit has AF_UNIX and the guide
// may say so; the daemon's, which has AF_INET, may not gain it.
var weakening = []*regexp.Regexp{
	regexp.MustCompile(`NoNewPrivileges=(false|no|0|off)`),
	regexp.MustCompile(`ProcSubset=all`),
	regexp.MustCompile(`ProtectProc=(default|noaccess|ptraceable)`),
	regexp.MustCompile(`ProtectSystem=(false|no|true|full)\b`),
	regexp.MustCompile(`RestrictAddressFamilies=[^\n` + "`" + `]*(AF_INET[^\n` + "`" + `]*AF_UNIX|AF_UNIX[^\n` + "`" + `]*AF_INET)`),
	regexp.MustCompile(`CapabilityBoundingSet=~?CAP_`),
	regexp.MustCompile(`AmbientCapabilities=`),
	regexp.MustCompile(`systemctl edit( --\S+)* holzkube-manager(\.service)?([\s` + "`" + `]|$)`),
	regexp.MustCompile(`holzkube-manager\.service\.d`),
	regexp.MustCompile(`(?i)sudoers\.d`),
}

// TestGuideKeepsTheDaemonsHardening: the guide says the daemon's unit stays as
// it is, names each hardening line in that section, and nowhere asks for one
// of them to change.
func TestGuideKeepsTheDaemonsHardening(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(shippedGuide)
	if err != nil {
		t.Fatalf("read the guide: %v", err)
	}
	guide := string(data)

	const heading = "## The service's own unit stays as it is"
	_, section, ok := strings.Cut(guide, "\n"+heading+"\n")
	if !ok {
		t.Fatalf("the guide has no section %q", heading)
	}
	if next := strings.Index(section, "\n## "); next >= 0 {
		section = section[:next]
	}
	if !strings.Contains(section, "asks for no change to `holzkube-manager.service`") {
		t.Errorf("the section %q does not say that installing the helper asks for no change to the daemon's unit", heading)
	}
	for _, line := range daemonHardening {
		if !strings.Contains(section, "- `"+line+"`") {
			t.Errorf("the section %q does not name `%s` as unchanged", heading, line)
		}
	}

	for _, re := range weakening {
		if m := re.FindString(guide); m != "" {
			t.Errorf("the guide contains %q, which loosens the daemon's hardening (matched %s)", m, re)
		}
	}
}

// TestTheArchiveCarriesTheHelper: the operator installs the helper from the
// release archive, so the archive has to hold all four files.
func TestTheArchiveCarriesTheHelper(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(goreleaserConfig)
	if err != nil {
		t.Fatalf("read .goreleaser.yaml: %v", err)
	}
	var cfg struct {
		Archives []struct {
			ID    string `yaml:"id"`
			Files []any  `yaml:"files"`
		} `yaml:"archives"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse .goreleaser.yaml: %v", err)
	}
	var files []string
	seen := false
	for _, a := range cfg.Archives {
		if a.ID != "default" {
			continue
		}
		seen = true
		for _, f := range a.Files {
			// A file entry is either a string or a map with src:.
			switch v := f.(type) {
			case string:
				files = append(files, v)
			case map[string]any:
				if s, ok := v["src"].(string); ok {
					files = append(files, s)
				}
			}
		}
	}
	if !seen {
		t.Fatalf(".goreleaser.yaml has no archive with id default")
	}
	in := func(name string) bool {
		for _, f := range files {
			if f == name {
				return true
			}
		}
		return false
	}
	want := []string{
		"deploy/holzkube-manager-host.sh",
		"deploy/holzkube-manager-host.path",
		"deploy/holzkube-manager-host.service",
		"deploy/HOST-HELPER.md",
	}
	for _, cmd := range InstallCommands {
		for _, f := range strings.Fields(cmd) {
			if strings.HasPrefix(f, "deploy/") {
				want = append(want, f)
			}
		}
	}
	for _, name := range want {
		if !in(name) {
			t.Errorf("the default release archive does not carry %s (files: %q)", name, files)
		}
	}
}

// TestTheUpdateScriptDoesNotShipTheHelper: the update script runs as root and
// replaces itself from the archive after a healthy update. If it began to
// install or refresh the helper too, new root code would reach the host
// without the operator deciding it (D-19). It does not so much as name it.
func TestTheUpdateScriptDoesNotShipTheHelper(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(shippedUpdater)
	if err != nil {
		t.Fatalf("read the update script: %v", err)
	}
	for i, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "holzkube-manager-host") || strings.Contains(line, "HOST-HELPER") {
			t.Errorf("%s:%d names the host helper: %s\nThe update script must not install or replace it (D-19).", shippedUpdater, i+1, strings.TrimSpace(line))
		}
	}
}
