package hostaction

import (
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
	"unicode"
	"unicode/utf8"

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
//     paths this package uses, and the path unit and the service carry
//     exactly the lines listed for them -- the hardening D-18 asks for and
//     none that would break a reboot;
//   - TestTheHelperNamesItsOrders: the script's marker line, its pattern, its
//     case arms and Actions() are the same words;
//   - TestTheCheckUnitRunsOnlyTheCheck: the check unit carries exactly the
//     lines listed for it, so it runs the update script's --check and nothing
//     else, sandboxed, and its worst case ends below the helper's limit;
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
// assignment fails the test rather than being ignored, and so does a file
// systemd would split into other lines than this reader does
// (refuseAmbiguousUnitBytes).
type unitFile struct {
	name        string
	sections    []string
	assignments []unitAssignment
}

// refuseAmbiguousUnitBytes fails unless systemd and readUnitFile see the same
// lines in data (13-REVIEW-2 V-15). systemd 257 also ends a line at a bare
// carriage return or a NUL, drops a UTF-8 byte-order mark before a key, and
// joins a line ending in a backslash with the next one; this reader splits on
// newlines only. So "# a comment\rExecStartPost=..." is a comment here and a
// command to systemd, and every check on the parsed lines would be blind to
// it. Rather than imitate systemd's reader, a unit file of this repository
// carries none of that: valid UTF-8, no byte-order mark, no control character
// but the tab and the newline, no white space but the space and the tab (the
// Unicode kinds this reader trims and systemd keeps), and no line ending in a
// backslash -- comment lines included, so the question of whether systemd
// continues a comment does not arise.
func refuseAmbiguousUnitBytes(t *testing.T, path string, data []byte) {
	t.Helper()
	if !utf8.Valid(data) {
		t.Fatalf("%s is not valid UTF-8; systemd and these tests could read it differently", path)
	}
	for i, line := range strings.Split(string(data), "\n") {
		n := i + 1
		if strings.ContainsRune(line, '\uFEFF') {
			t.Fatalf("%s:%d: a UTF-8 byte-order mark; systemd drops it before a key, so the line is not what it looks like", path, n)
		}
		for _, r := range line {
			if (r < 0x20 && r != '\t') || r == 0x7f {
				t.Fatalf("%s:%d: control character %U; systemd ends a line at a carriage return or a NUL, "+
					"so what follows it could be an assignment this test never reads", path, n, r)
			}
			// strings.TrimSpace and strings.Fields strip Unicode white space
			// (U+0085, U+00A0, U+2028, ...); systemd's is the space, the tab
			// and the line ends only. "TimeoutStopSec=10s\u00a0" is 10s here
			// and a value systemd refuses -- falling back to 90 s -- there
			// (13-REVIEW-2 round 3, I5).
			if unicode.IsSpace(r) && r != ' ' && r != '\t' {
				t.Fatalf("%s:%d: white space %U that systemd does not take for white space, "+
					"while this test strips it; the value would not be the one checked", path, n, r)
			}
		}
		if strings.HasSuffix(strings.TrimRight(line, " \t"), `\`) {
			t.Fatalf("%s:%d: the line ends in a backslash; systemd joins it with the next line, this test does not", path, n)
		}
	}
}

func readUnitFile(t *testing.T, path string) unitFile {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	refuseAmbiguousUnitBytes(t, path, data)
	u := unitFile{name: filepath.Base(path)}
	section := ""
	for i, raw := range strings.Split(string(data), "\n") {
		n := i + 1
		line := strings.TrimSpace(raw)
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
	return u
}

// unitLine is one line a unit may carry: its section, its key, and the value
// it must have -- want exactly, or, when check is set, whatever check accepts
// (check returns why not, or "").
type unitLine struct {
	section, key string
	want         string
	check        func(value string) string
}

// wantExactly holds u to an allow-list: exactly these sections, in this
// order, each once, and exactly these lines, each once and with its value.
// Any other key fails, naming its line -- whatever it is. A list of keys a
// unit must not carry only ever names the ones somebody thought of
// (13-REVIEW-2 V-14, V-19: OnSuccess=, Wants=, BindPaths=, PassEnvironment=,
// StandardOutput=file:, TimeoutSec=, SendSIGKILL= all passed one), and every
// line systemd knows is one more way for a unit to do something else. A key
// assigned twice fails too: systemd uses the later line, or for list keys
// both, or resets the list on an empty one.
func wantExactly(t *testing.T, u unitFile, sections []string, spec []unitLine) {
	t.Helper()
	if !slices.Equal(u.sections, sections) {
		t.Errorf("%s: sections %q, want exactly %q", u.name, u.sections, sections)
	}
	type slot struct{ section, key string }
	allowed := map[slot]unitLine{}
	for _, l := range spec {
		allowed[slot{l.section, l.key}] = l
	}
	seen := map[slot]int{}
	for _, a := range u.assignments {
		s := slot{a.section, a.key}
		l, ok := allowed[s]
		if !ok {
			t.Errorf("%s:%d: [%s] %s=%s is not a line this unit may carry", u.name, a.line, a.section, a.key, a.value)
			continue
		}
		if first, twice := seen[s]; twice {
			t.Errorf("%s:%d: [%s] %s= a second time (first on line %d); systemd would use the later line, or both",
				u.name, a.line, a.section, a.key, first)
			continue
		}
		seen[s] = a.line
		switch {
		case l.check != nil:
			if why := l.check(a.value); why != "" {
				t.Errorf("%s:%d: [%s] %s=%s: %s", u.name, a.line, a.section, a.key, a.value, why)
			}
		case a.value != l.want:
			t.Errorf("%s:%d: [%s] %s=%s, want %s=%s", u.name, a.line, a.section, a.key, a.value, a.key, l.want)
		}
	}
	for _, l := range spec {
		if _, ok := seen[slot{l.section, l.key}]; !ok {
			t.Errorf("%s: [%s] has no %s=", u.name, l.section, l.key)
		}
	}
}

// helperGuideURL is every helper unit's Documentation=.
const helperGuideURL = "https://github.com/holzcloud/holzkube-manager/blob/main/deploy/HOST-HELPER.md"

// unitHeader is the [Unit] section every helper unit carries, and all it may:
// a description and the guide. No dependency, no ordering, no OnSuccess= or
// OnFailure= -- nothing that starts another unit or holds this one back.
func unitHeader() []unitLine {
	return []unitLine{
		{section: "Unit", key: "Description", check: func(v string) string {
			if v == "" {
				return "an empty description"
			}
			return ""
		}},
		{section: "Unit", key: "Documentation", want: helperGuideURL},
	}
}

// hardening is the sandbox both services carry, value for value.
func hardening() []unitLine {
	var out []unitLine
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
		{"RestrictNamespaces", "true"},
		{"RestrictRealtime", "true"},
		{"RestrictSUIDSGID", "true"},
		{"LockPersonality", "true"},
		{"MemoryDenyWriteExecute", "true"},
		{"SystemCallArchitectures", "native"},
		{"StateDirectoryMode", "0755"},
		{"UMask", "0022"},
	} {
		out = append(out, unitLine{section: "Service", key: kv[0], want: kv[1]})
	}
	return out
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
	// The ExecStart= rewrite below finds lines by newline, as systemd would
	// not if a line hid behind a carriage return (13-REVIEW-2 V-15).
	refuseAmbiguousUnitBytes(t, shippedPathUnit, pathSrc)
	refuseAmbiguousUnitBytes(t, shippedService, serviceSrc)
	refuseAmbiguousUnitBytes(t, shippedCheckUnit, checkSrc)

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
// paths to one another, and the path unit and the service to an allow-list
// (wantExactly) that includes the hardening D-18 asks for.
//
// Faults injected and seen red: TimeoutSec=1min after TimeoutStartSec=3min
// and TimeoutStartSec=5min in the service; Wants=, PassEnvironment=,
// CapabilityBoundingSet= and an ExecStartPost= behind a carriage return in
// it; PathChanged= and MakeDirectory= in the path unit.
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

	// The path unit, line for line (13-REVIEW-2 V-14..V-19, as for the check
	// unit): one trigger, on the order, starting the service, pulled in by
	// paths.target. No other trigger -- a second one would start the helper
	// without an order -- and no MakeDirectory=, which would create a missing
	// data directory as root 0755, in which the daemon then refuses to start.
	wantExactly(t, path, []string{"Unit", "Path", "Install"}, append(unitHeader(),
		unitLine{section: "Path", key: "PathExists", want: ReferenceOrderPath},
		unitLine{section: "Path", key: "Unit", want: filepath.Base(ServiceUnitPath)},
		unitLine{section: "Install", key: "WantedBy", want: "paths.target"},
	))
	if filepath.Dir(WantsLinkPath) != filepath.Join(filepath.Dir(PathUnitPath), "paths.target.wants") ||
		filepath.Base(WantsLinkPath) != filepath.Base(PathUnitPath) {
		t.Errorf("WantsLinkPath %s is not where `systemctl enable` links a unit with WantedBy=paths.target", WantsLinkPath)
	}

	// The service, line for line: it runs the installed script, once per
	// start, below the limit the daemon assumes (HelperServiceLimit); writes
	// its own state, where the daemon reads the result, and the daemon's
	// directory, where the consuming rm needs it under ProtectSystem=strict;
	// and carries the hardening D-18 asks for. What it leaves out is left out
	// by not being on the list: no [Install] (only the path unit starts it),
	// no RemainAfterExit= (an active oneshot never runs a second order), no
	// capability or system-call restriction (the reboot goes through logind,
	// by polkit or CAP_SYS_BOOT, and entering the daemon's 0700 directory
	// takes CAP_DAC_OVERRIDE -- Pitfall 7), no Environment= (the script's
	// overrides are for tests), and no TimeoutSec=, which would set the start
	// limit behind TimeoutStartSec='s back (13-REVIEW-2 V-10).
	resultDir := filepath.Dir(DefaultResultPath)
	if filepath.Dir(resultDir) != "/var/lib" {
		t.Errorf("DefaultResultPath %s is not under /var/lib, where StateDirectory= puts it", DefaultResultPath)
	}
	wantExactly(t, service, []string{"Unit", "Service"}, append(append(unitHeader(),
		unitLine{section: "Service", key: "Type", want: "oneshot"},
		unitLine{section: "Service", key: "ExecStart", want: HelperScriptPath},
		unitLine{section: "Service", key: "TimeoutStartSec", check: func(v string) string {
			if d := systemdSpan(t, v); d != HelperServiceLimit {
				return "HelperServiceLimit is " + HelperServiceLimit.String() + "; the routes would hold orders back " +
					"for a different time than systemd lets the helper wait for a check"
			}
			return ""
		}},
		unitLine{section: "Service", key: "StateDirectory", want: filepath.Base(resultDir)},
		unitLine{section: "Service", key: "ReadWritePaths", want: "-" + filepath.Dir(ReferenceOrderPath)},
		unitLine{section: "Service", key: "RestrictAddressFamilies", want: "AF_UNIX"},
	), hardening()...))

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

// maxUnitSpan is the longest span systemdSpan reads. No limit in these units
// comes near it, and a time.Duration holds only some 292 years: systemd
// accepts "2562048h 9223371278s" (584 years), which summed as a Duration
// wraps to about 4 s and would pass every comparison below it (13-REVIEW-2
// round 3, W2). So every part and every partial sum is held under this cap
// before it is added, never after.
const maxUnitSpan = 24 * time.Hour

// systemdSpan parses a systemd time span of the forms the units use -- "90",
// "90s", "2min", "3min", "1h 30min" -- into a duration. Anything else fails
// the test: a span it cannot read is not one it can compare, and neither is
// one above maxUnitSpan.
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
		n, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			t.Fatalf("cannot read %q as a systemd time span: %v", s, err)
		}
		// Compared before the multiplication and before the sum, so neither
		// can wrap: n <= maxUnitSpan/unit keeps n*unit <= maxUnitSpan, and
		// total <= maxUnitSpan - d keeps the sum there too.
		if n > int64(maxUnitSpan/unit) {
			t.Fatalf("the systemd time span %q is longer than %v; a limit that long is no limit this test can reason about", s, maxUnitSpan)
		}
		d := time.Duration(n) * unit
		if total > maxUnitSpan-d {
			t.Fatalf("the systemd time span %q is longer than %v; a limit that long is no limit this test can reason about", s, maxUnitSpan)
		}
		total += d
	}
	return total
}

// checkUnitMargin is how far below the helper's limit the check unit's worst
// case must end. The helper spends some of its own limit before it starts the
// check (claiming the order, recording "started") and after (recording
// "failed"), its limit runs from before the check's, and each of systemd's
// timers on the way may fire a little late. None of that is more than a
// second; this leaves fifteen.
const checkUnitMargin = 15 * time.Second

// checkUnitWorstCase is the longest the helper's blocking `systemctl start` of
// the check unit can take, as systemd 257 runs a oneshot with only the keys
// TestTheCheckUnitRunsOnlyTheCheck allows (13-REVIEW-2 V-09..V-13). When the
// start limit runs out (TimeoutStartFailureMode= is not allowed, so the
// default, terminate), systemd walks stop-sigterm, stop-sigkill, final-sigterm
// and final-sigkill and arms TimeoutStopSec= for each of them (there is no
// ExecStop= or ExecStopPost= in between: they are not allowed either); the
// start job ends only after the last. Processes that SIGKILL ends -- the
// usual case -- cut that short after the second; one stuck in uninterruptible
// sleep (an SD card stalling under curl) goes through all four. Measured on
// the Pi with a transient unit, TimeoutStartSec=3s and TimeoutStopSec=2s:
// 3.2 s for a process SIGTERM ends, 5.5 s for one that ignores SIGTERM, 12.0 s
// for one no signal ends (KillSignal= and FinalKillSignal=SIGCONT, which takes
// the same states) -- start + 4 x stop.
//
// SendSIGKILL=no, FinalKillSignal=, TimeoutAbortSec=, TimeoutSec= and any
// ordering would each change this sum, and none of them is on the list; that
// the list is closed is what makes this the worst case.
func checkUnitWorstCase(start, stop time.Duration) time.Duration {
	return start + 4*stop
}

// TestTheCheckUnitRunsOnlyTheCheck: the check unit runs as root, and the only
// thing it may run is the update script's look -- never the update itself.
// So it is held to an allow-list (wantExactly), line for line, and anything
// not on it fails, naming its line: its one command is UpdateScriptPath
// --check, and no ExecStartPre=, ExecStartPost=, ExecStop= or other Exec line
// runs anything else as root (13-REVIEW-2 WR-04); its [Unit] section starts
// nothing (no OnSuccess=, OnFailure=, Wants=, Requires=, BindsTo=, Upholds=:
// holzkube-manager-update.service is the full update, V-14, V-19) and waits
// for nothing (no After=); it puts nothing over the script or the update.conf
// it sources (no BindPaths=, BindReadOnlyPaths=, RootDirectory=, V-16), takes
// no environment from anywhere (no Environment=, EnvironmentFile=,
// PassEnvironment=, V-17), and writes nowhere but the update status directory
// the daemon reads (no ReadWritePaths=, no StandardOutput=file:, V-18); it may
// reach the network, which the helper's own service may not, but only as far
// as GitHub needs; it runs as root with no capability (no User=, Group=,
// DynamicUser=: the sandbox is reasoned for root with an empty bounding set,
// writing a directory only root owns and reading a token only root may read);
// it carries no [Install] (nothing starts it but the helper) and no
// RemainAfterExit= (a second check would never run); and its worst case ends
// below the helper's limit, with checkUnitMargin to spare, because the helper
// waits for it and must still record failed (WR-03, V-09).
//
// Faults injected and seen red: --check removed from ExecStart=; an
// ExecStartPost= running the update with --force; AF_PACKET added to
// RestrictAddressFamilies=; DynamicUser=true; TimeoutStopSec= absent, 90s
// and 1min; OnSuccess= and Wants=holzkube-manager-update.service;
// BindPaths=; PassEnvironment=; StandardOutput=file:; TimeoutSec=;
// SendSIGKILL=no; FinalKillSignal=; TimeoutStartFailureMode=abort; After=;
// TimeoutStopSec=15s, the shipped value before V-09; an ExecStartPost= behind
// a carriage return or a NUL in a comment; a byte-order mark before one; a
// backslash at the end of the line before ExecStart=; TimeoutStopSec= and
// TimeoutStartSec= of 584 years ("2562048h 9223371278s"), which systemd takes
// and a time.Duration sum wraps to about 4 s, and "23h 2h" (maxUnitSpan).
func TestTheCheckUnitRunsOnlyTheCheck(t *testing.T) {
	t.Parallel()

	check := readUnitFile(t, shippedCheckUnit)
	service := readUnitFile(t, shippedService)

	if check.name != filepath.Base(UpdateCheckUnitPath) {
		t.Errorf("the shipped check unit is %s, the helper starts and the guide installs %s", check.name, UpdateCheckUnitPath)
	}

	statusDir := filepath.Dir(updatestatus.DefaultPath)
	if filepath.Dir(statusDir) != "/var/lib" {
		t.Errorf("updatestatus.DefaultPath %s is not under /var/lib, where StateDirectory= puts it", updatestatus.DefaultPath)
	}
	positiveSpan := func(v string) string {
		if systemdSpan(t, v) <= 0 {
			return "want a limit above 0; 0 means none"
		}
		return ""
	}
	wantExactly(t, check, []string{"Unit", "Service"}, append(append(unitHeader(),
		unitLine{section: "Service", key: "Type", want: "oneshot"},
		unitLine{section: "Service", key: "ExecStart", want: UpdateScriptPath + " --check"},
		unitLine{section: "Service", key: "TimeoutStartSec", check: positiveSpan},
		unitLine{section: "Service", key: "TimeoutStopSec", check: positiveSpan},
		unitLine{section: "Service", key: "StateDirectory", want: filepath.Base(statusDir)},
		unitLine{section: "Service", key: "CapabilityBoundingSet", want: ""},
		// The look asks GitHub for the release list -- AF_INET and AF_INET6
		// for that, AF_UNIX and AF_NETLINK for name resolution -- and nothing
		// more (13-REVIEW-2 WR-04): exactly these four, in any order.
		unitLine{section: "Service", key: "RestrictAddressFamilies", check: func(v string) string {
			got := strings.Fields(v)
			slices.Sort(got)
			if want := []string{"AF_INET", "AF_INET6", "AF_NETLINK", "AF_UNIX"}; !slices.Equal(got, want) {
				return "want exactly " + strings.Join(want, " ") + ": the network GitHub needs and nothing more"
			}
			return ""
		}},
	), hardening()...))

	// Below the helper's limit. The allow-list above has made both keys appear
	// exactly once, and ruled out every other key that changes the sum.
	span := func(u unitFile, key string) time.Duration {
		var v []string
		for _, a := range u.assignments {
			switch {
			case a.section != "Service":
			case a.key == "TimeoutSec":
				t.Fatalf("%s:%d: TimeoutSec=%s sets both limits; this test reads TimeoutStartSec= and TimeoutStopSec= only",
					u.name, a.line, a.value)
			case a.key == key:
				v = append(v, a.value)
			}
		}
		if len(v) != 1 {
			t.Fatalf("%s: %d %s= lines, want exactly one", u.name, len(v), key)
		}
		return systemdSpan(t, v[0])
	}
	// The daemon's idea of the helper's limit is the unit's: the routes hold
	// every order back while a check is younger than it (CheckRunning).
	helper := span(service, "TimeoutStartSec")
	if helper != HelperServiceLimit {
		t.Errorf("%s: TimeoutStartSec is %v, HelperServiceLimit is %v; the routes would hold orders back "+
			"for a different time than systemd lets the helper wait for a check", service.name, helper, HelperServiceLimit)
	}
	start, stop := span(check, "TimeoutStartSec"), span(check, "TimeoutStopSec")
	if worst := checkUnitWorstCase(start, stop); worst+checkUnitMargin > helper {
		t.Errorf("%s: TimeoutStartSec %v + 4 x TimeoutStopSec %v = %v, systemd's worst case for the start the helper waits for; "+
			"want it at least %v below the helper service's %v, or systemd ends the helper before it can record failed",
			check.name, start, stop, worst, checkUnitMargin, helper)
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
