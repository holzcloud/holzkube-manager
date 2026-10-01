package hostaction

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/holzcloud/holzkube-manager/internal/host/updatestatus"
)

// What the operator installs by hand has to agree with what the daemon looks
// for, and has to load the way it reads. The helper is six files outside this
// package -- three units, the script, the guide, the release archive's list --
// plus the paths and the install commands in here, and none of them can see
// the others. These tests hold them together (D-17, D-18, D-19, HACT-08):
//
//   - TestUnitsVerify: systemd itself accepts all three units, read by its
//     output and not only by its exit code, with a negative control;
//   - TestUnitsAgree: the units' paths and the script's defaults are the
//     paths this package uses, and the service carries the hardening D-18
//     asks for and none that would break a reboot;
//   - TestTheHelperNamesItsOrders: the script's marker line, its pattern, its
//     case arms and Actions() are the same words;
//   - TestTheCheckUnitRunsOnlyTheCheck: the check unit runs the update
//     script's --check and nothing else, sandboxed, below the helper's limit;
//   - TestInstallCommandsMatchTheGuide: the guide's install block is
//     InstallCommands, byte for byte;
//   - TestGuideKeepsTheDaemonsHardening: the guide names the daemon's
//     hardening as unchanged and asks for none of it to go;
//   - TestTheArchiveCarriesTheHelper: the release archive has all of it;
//   - TestTheUpdateScriptDoesNotShipTheHelper: the root script that replaces
//     itself does not start carrying other root code;
//   - TestTheFixtureShowsTheRealInstallCommands: the fixture the README's
//     picture of /host is rendered from shows these commands and paths.
//
// Nothing here installs anything: the units are verified as copies in a
// temporary directory, and no command addresses the system manager.

// The shipped files, from this package's directory.
var (
	deployDir         = filepath.Join("..", "..", "..", "deploy")
	shippedPathUnit   = filepath.Join(deployDir, "holzkube-manager-host.path")
	shippedService    = filepath.Join(deployDir, "holzkube-manager-host.service")
	shippedCheckUnit  = filepath.Join(deployDir, "holzkube-manager-update-check.service")
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

// TestUnitsVerify runs systemd-analyze verify over copies of the three units:
// the path unit, the helper's service and the check unit it starts.
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
// the helper is (deliberately) not installed where the tests run -- in the
// check unit to the stub followed by --check, as the shipped line has it.
// TestUnitsAgree holds the shipped ExecStart= to HelperScriptPath instead, and
// TestTheCheckUnitRunsOnlyTheCheck the check unit's to UpdateScriptPath.
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
	checkSrc, err := os.ReadFile(shippedCheckUnit)
	if err != nil {
		t.Fatalf("read the check unit: %v", err)
	}

	verify := func(t *testing.T, service string) (string, int) {
		t.Helper()
		dir := t.TempDir()
		stub := filepath.Join(dir, "helper-stub")
		if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		execStart := regexp.MustCompile(`(?m)^ExecStart=.*$`)
		pointAtStub := func(name, unit, args string) string {
			if n := len(execStart.FindAllString(unit, -1)); n != 1 {
				t.Fatalf("%s has %d ExecStart= lines, want exactly 1 to point at the stub", name, n)
			}
			return execStart.ReplaceAllLiteralString(unit, "ExecStart="+stub+args)
		}
		service = pointAtStub("the service", service, "")
		check := pointAtStub("the check unit", string(checkSrc), " --check")

		pathFile := filepath.Join(dir, filepath.Base(shippedPathUnit))
		serviceFile := filepath.Join(dir, filepath.Base(shippedService))
		checkFile := filepath.Join(dir, filepath.Base(shippedCheckUnit))
		for file, content := range map[string][]byte{
			pathFile:    pathSrc,
			serviceFile: []byte(service),
			checkFile:   []byte(check),
		} {
			if err := os.WriteFile(file, content, 0o644); err != nil {
				t.Fatal(err)
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), systemdAnalyzeMax)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, "verify", "--man=no", pathFile, serviceFile, checkFile)
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

// TestTheHelperNamesItsOrders: the helper script names the orders it carries
// out on one marker line, which the daemon reads from the installed script to
// learn what that helper knows (D-12). The line is only worth reading if it is
// the truth about the code below it, so the words on it, the alternation of
// the one pattern that admits an order, the labels of the case arms before
// the catch-all, and Actions() are the same words, each once.
//
// Faults injected and seen red: check-update dropped from the marker line;
// halt added to the pattern alone.
func TestTheHelperNamesItsOrders(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(shippedScript)
	if err != nil {
		t.Fatalf("read the helper script: %v", err)
	}
	lines := strings.Split(string(data), "\n")

	// The words of one list, refusing a word that appears twice.
	words := func(what string, list []string) map[string]bool {
		t.Helper()
		set := map[string]bool{}
		for _, w := range list {
			if set[w] {
				t.Errorf("%s names %q twice", what, w)
			}
			set[w] = true
		}
		return set
	}

	var marker []string
	for i, l := range lines {
		if strings.HasPrefix(l, HelperOrdersMarker) {
			if marker != nil {
				t.Errorf("line %d: a second marker line %q; the daemon reads exactly one", i+1, l)
				continue
			}
			marker = strings.Split(strings.TrimPrefix(l, HelperOrdersMarker), " ")
		}
	}
	if marker == nil {
		t.Fatalf("the helper script has no line beginning %q", HelperOrdersMarker)
	}

	pattern := regexp.MustCompile(`=~ \^\(([^)]*)\)`)
	var alternation []string
	for i, l := range lines {
		if m := pattern.FindStringSubmatch(l); m != nil {
			if alternation != nil {
				t.Fatalf("line %d: a second pattern line; the script admits orders through one", i+1)
			}
			alternation = strings.Split(m[1], "|")
		}
	}
	if alternation == nil {
		t.Fatalf("the helper script has no pattern line `=~ ^(...)`")
	}

	var arms []string
	armLabel := regexp.MustCompile(`^\s*([^\s#()]+)\)`)
	inCase, closed := false, false
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		switch {
		case trimmed == "case $action in":
			inCase = true
		case !inCase || strings.HasPrefix(trimmed, "#"):
		case trimmed == "esac":
			inCase = false
		default:
			m := armLabel.FindStringSubmatch(l)
			if m == nil {
				continue
			}
			if m[1] == "*" {
				closed = true
				inCase = false
				continue
			}
			arms = append(arms, m[1])
		}
	}
	if !closed {
		t.Fatalf("the helper's case over $action has no catch-all arm *) -- or none was found")
	}

	var actions []string
	for _, a := range Actions() {
		actions = append(actions, string(a))
	}

	want := words("Actions()", actions)
	for _, got := range []struct {
		what string
		set  map[string]bool
	}{
		{"the marker line", words("the marker line", marker)},
		{"the pattern", words("the pattern", alternation)},
		{"the case arms", words("the case arms", arms)},
	} {
		if !mapsEqual(got.set, want) {
			t.Errorf("%s names %v, Actions() is %v: the helper must say exactly what it carries out",
				got.what, sortedKeys(got.set), sortedKeys(want))
		}
	}
}

func mapsEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// systemdSpan parses a systemd time span of the forms the units use -- "90",
// "90s", "2min", "3min", "1h 30min" -- into a duration. Anything else fails
// the test: a span it cannot read is not one it can compare.
func systemdSpan(t *testing.T, s string) time.Duration {
	t.Helper()
	units := map[string]time.Duration{
		"":    time.Second,
		"s":   time.Second,
		"sec": time.Second,
		"m":   time.Minute,
		"min": time.Minute,
		"h":   time.Hour,
		"hr":  time.Hour,
	}
	part := regexp.MustCompile(`^([0-9]+)\s*([a-z]*)$`)
	var total time.Duration
	fields := regexp.MustCompile(`[0-9]+\s*[a-z]*`).FindAllString(s, -1)
	if len(fields) == 0 || strings.Join(fields, "") != strings.Join(strings.Fields(s), "") {
		t.Fatalf("cannot read %q as a systemd time span", s)
	}
	for _, f := range fields {
		m := part.FindStringSubmatch(strings.TrimSpace(f))
		unit, ok := units[m[2]]
		if !ok {
			t.Fatalf("cannot read %q as a systemd time span (unit %q)", s, m[2])
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("cannot read %q as a systemd time span: %v", s, err)
		}
		total += time.Duration(n) * unit
	}
	return total
}

// TestTheCheckUnitRunsOnlyTheCheck: the check unit runs as root, and the only
// thing it may run is the update script's look -- never the update itself.
// So its one command is UpdateScriptPath --check; it is a oneshot whose
// start and stop limits together end below the helper's own limit (the helper waits for it and must still record
// failed); it writes only the update status directory the daemon reads; it
// may reach the network, which the helper's own service may not; and it
// carries no [Install] (nothing starts it but the helper), no RemainAfterExit
// (a second check would never run), no environment and no capability.
//
// Faults injected and seen red: --check removed from ExecStart=; an
// ExecStartPost= running the update with --force; AF_PACKET added to
// RestrictAddressFamilies=; DynamicUser=true; TimeoutStopSec= absent, 90s
// and 1min.
func TestTheCheckUnitRunsOnlyTheCheck(t *testing.T) {
	t.Parallel()

	check := readUnitFile(t, shippedCheckUnit)
	service := readUnitFile(t, shippedService)

	if check.name != filepath.Base(UpdateCheckUnitPath) {
		t.Errorf("the shipped check unit is %s, the helper starts and the guide installs %s", check.name, UpdateCheckUnitPath)
	}

	wantOnly(t, check, "Service", "ExecStart", UpdateScriptPath+" --check")
	// And no other command, in any section (13-REVIEW-2 WR-04): ExecStartPre,
	// ExecStartPost, ExecCondition, ExecStop, ExecStopPost and ExecReload all
	// run as root too, and one line of them -- the update with --force -- would
	// install a release on every check.
	if n := len(check.lines("ExecStart")); n != 1 {
		t.Errorf("%s has %d ExecStart= lines, want exactly 1", check.name, n)
	}
	for _, a := range check.assignments {
		if strings.HasPrefix(a.key, "Exec") && a.key != "ExecStart" {
			t.Errorf("%s:%d: [%s] %s=%s; the check unit runs its one ExecStart= and nothing else",
				check.name, a.line, a.section, a.key, a.value)
		}
	}
	wantOnly(t, check, "Service", "Type", "oneshot")
	statusDir := filepath.Dir(updatestatus.DefaultPath)
	if filepath.Dir(statusDir) != "/var/lib" {
		t.Errorf("updatestatus.DefaultPath %s is not under /var/lib, where StateDirectory= puts it", updatestatus.DefaultPath)
	}
	wantOnly(t, check, "Service", "StateDirectory", filepath.Base(statusDir))
	for _, kv := range [][2]string{
		{"NoNewPrivileges", "true"},
		{"CapabilityBoundingSet", ""},
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
		{"RestrictNamespaces", "true"},
		{"RestrictRealtime", "true"},
		{"RestrictSUIDSGID", "true"},
		{"LockPersonality", "true"},
		{"MemoryDenyWriteExecute", "true"},
		{"SystemCallArchitectures", "native"},
		{"StateDirectoryMode", "0755"},
		{"UMask", "0022"},
	} {
		wantOnly(t, check, "Service", kv[0], kv[1])
	}

	// The network: the look asks GitHub for the release list -- AF_INET and
	// AF_INET6 for that, AF_UNIX and AF_NETLINK for name resolution -- and
	// nothing more (13-REVIEW-2 WR-04): exactly these four, in any order.
	families := check.lines("RestrictAddressFamilies")
	if len(families) != 1 {
		t.Errorf("%s: %d RestrictAddressFamilies= lines, want exactly one", check.name, len(families))
	} else {
		got := strings.Fields(families[0].value)
		slices.Sort(got)
		if want := []string{"AF_INET", "AF_INET6", "AF_NETLINK", "AF_UNIX"}; !slices.Equal(got, want) {
			t.Errorf("%s: RestrictAddressFamilies=%s, want exactly %s: the network GitHub needs and nothing more",
				check.name, families[0].value, strings.Join(want, " "))
		}
	}

	// Below the helper's limit: the helper waits for this unit.
	lastSpan := func(u unitFile) time.Duration {
		v := u.values("Service", "TimeoutStartSec")
		if len(v) == 0 {
			t.Fatalf("%s has no TimeoutStartSec=; a oneshot has no start limit without one", u.name)
		}
		return systemdSpan(t, v[len(v)-1])
	}
	// The daemon's idea of the helper's limit is the unit's: the routes hold
	// every order back while a check is younger than it (CheckRunning).
	if h := lastSpan(service); h != HelperServiceLimit {
		t.Errorf("%s: TimeoutStartSec is %v, HelperServiceLimit is %v; the routes would hold orders back "+
			"for a different time than systemd lets the helper wait for a check", service.name, h, HelperServiceLimit)
	}
	// The worst case is the start limit plus the stop limit: when the start
	// times out, systemd sends SIGTERM and waits TimeoutStopSec (default
	// 90 s) before SIGKILL, and the helper's blocking systemctl start returns
	// only once the unit has stopped (13-REVIEW-2 WR-03). Both must be set,
	// and their sum below the helper's limit, or systemd ends the helper
	// before it can record failed.
	stopSpan := func(u unitFile) time.Duration {
		v := u.values("Service", "TimeoutStopSec")
		if len(v) == 0 {
			t.Fatalf("%s has no TimeoutStopSec=; the default (90 s) on top of TimeoutStartSec= is not bounded here", u.name)
		}
		return systemdSpan(t, v[len(v)-1])
	}
	if c, stop, h := lastSpan(check), stopSpan(check), lastSpan(service); c <= 0 || stop <= 0 || c+stop >= h {
		t.Errorf("%s: TimeoutStartSec %v + TimeoutStopSec %v = %v, want both above 0 and the sum below the helper service's %v -- "+
			"the helper waits for the check and must still record failed when systemd ends it", check.name, c, stop, c+stop, h)
	}

	if check.hasSection("Install") {
		t.Errorf("%s has an [Install] section; only the helper may start it", check.name)
	}
	// User=, Group= and DynamicUser= are left out too: the sandbox above is
	// reasoned for root with an empty bounding set, writing a directory only
	// root owns, and reading a token only root may read.
	for _, key := range []string{"RemainAfterExit", "Environment", "EnvironmentFile", "AmbientCapabilities", "ReadWritePaths",
		"User", "Group", "DynamicUser", "SupplementaryGroups"} {
		if l := check.lines(key); len(l) > 0 {
			t.Errorf("%s:%d: %s=%s must not be in the check unit", check.name, l[0].line, key, l[0].value)
		}
	}

	// And the helper starts exactly this unit for a check-update order.
	script, err := os.ReadFile(shippedScript)
	if err != nil {
		t.Fatalf("read the helper script: %v", err)
	}
	arm := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(string(CheckUpdate)) + `\)\s*cmd=\(([^)]*)\)\s*;;\s*$`)
	m := arm.FindAllStringSubmatch(string(script), -1)
	if len(m) != 1 {
		t.Fatalf("the helper script has %d case arms for %s, want exactly 1", len(m), CheckUpdate)
	}
	if want := "start " + filepath.Base(UpdateCheckUnitPath); m[0][1] != want {
		t.Errorf("the helper's %s arm runs systemctl %s, want exactly systemctl %s", CheckUpdate, m[0][1], want)
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

// demoFixture is the file the README's pictures and the layout audit are
// rendered from.
var demoFixture = filepath.Join("..", "..", "..", "web", "fixtures", "demo.json")

// TestTheFixtureShowsTheRealInstallCommands: the README's picture of /host is
// rendered from web/fixtures/demo.json, and the fixture shows the helper as
// the operator's machine has it until it is installed -- not available, the
// script and the unit files missing, the commands that install it. Were the fixture to keep
// a copy of commands this package no longer has, the picture would show an
// operator lines the guide does not have. So the fixture's missing list is
// what Detect reports on a machine with nothing installed, and its commands
// are InstallCommands, byte for byte.
func TestTheFixtureShowsTheRealInstallCommands(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(demoFixture)
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	var fixtures map[string]json.RawMessage
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatalf("decode the fixture: %v", err)
	}
	raw, ok := fixtures["/api/v1/host"]
	if !ok {
		t.Fatalf("the fixture has no /api/v1/host")
	}
	var host struct {
		Actions *struct {
			Order           json.RawMessage `json:"order"`
			Available       *bool           `json:"available"`
			Missing         []Missing       `json:"missing"`
			InstallCommands []string        `json:"install_commands"`
			Result          struct {
				Readable bool `json:"readable"`
				Reason   *struct {
					Code string `json:"code"`
				} `json:"reason"`
			} `json:"result"`
		} `json:"actions"`
	}
	if err := json.Unmarshal(raw, &host); err != nil {
		t.Fatalf("decode the fixture's /api/v1/host: %v", err)
	}
	a := host.Actions
	if a == nil {
		t.Fatalf("the fixture's /api/v1/host has no actions: the page would parse the default and show no helper notice")
	}

	if got, want := strings.Join(a.InstallCommands, "\n"), strings.Join(InstallCommands, "\n"); got != want {
		t.Errorf("the fixture's install_commands differ from InstallCommands.\nfixture:\n%q\nInstallCommands:\n%q", got, want)
	}

	// Nothing installed: what Detect reports on an empty machine, in its order.
	want := Detect(fstest.MapFS{})
	if exact := []Missing{
		{Item: MissingScript, Path: HelperScriptPath},
		{Item: MissingPathUnit, Path: PathUnitPath},
	}; !slices.Equal(want, exact) {
		t.Fatalf("Detect on an empty machine reports %v, want %v", want, exact)
	}
	if !slices.Equal(a.Missing, want) {
		t.Errorf("the fixture's missing list is %v, want what Detect reports with nothing installed: %v", a.Missing, want)
	}
	// The sentence G-13-2 found false: a path unit that is missing is not
	// "installed but not enabled".
	var pathUnit, notEnabled bool
	for _, m := range a.Missing {
		pathUnit = pathUnit || m.Item == MissingPathUnit
		notEnabled = notEnabled || m.Item == MissingNotEnabled
	}
	if pathUnit && notEnabled {
		t.Errorf("the fixture's missing list %v names path-unit and not-enabled together", a.Missing)
	}
	for _, m := range a.Missing {
		switch m.Path {
		case HelperScriptPath, PathUnitPath, ServiceUnitPath, WantsLinkPath:
		default:
			t.Errorf("the fixture names %s, which is none of the helper's paths", m.Path)
		}
	}

	switch {
	case a.Available == nil:
		t.Errorf("the fixture's actions has no available, want false: the helper is not installed")
	case *a.Available:
		t.Errorf("the fixture's actions.available is true, want false: the helper is not installed")
	}
	if o := strings.TrimSpace(string(a.Order)); o != "null" {
		t.Errorf("the fixture's actions.order is %s, want null: nothing was placed", o)
	}
	if a.Result.Readable || a.Result.Reason == nil || a.Result.Reason.Code != "host-action.no-result" {
		t.Errorf("the fixture's actions.result is not the no-result reading the server sends when the helper recorded nothing")
	}
}
