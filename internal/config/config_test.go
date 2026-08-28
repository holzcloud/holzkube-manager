package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testHome = "/home/op"

func load(t *testing.T, args []string, env map[string]string) Config {
	t.Helper()
	cfg, err := LoadWith(args, envFrom(env), testHome)
	if err != nil {
		t.Fatalf("LoadWith(%v, %v): %v", args, env, err)
	}
	return cfg
}

// Nothing set anywhere: loopback, 8443, XDG data directory, D-05's five minute
// sudo window and D-07's 24 hour session lifetime.
func TestDefaultsWithoutAnyInput(t *testing.T) {
	cfg := load(t, nil, nil)

	if cfg.Listen != "127.0.0.1:8443" {
		t.Errorf("Listen = %q, want the loopback default", cfg.Listen)
	}
	if want := filepath.Join(testHome, ".local", "share", "holzkube"); cfg.DataDir != want {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, want)
	}
	if cfg.SudoWindow != 5*time.Minute {
		t.Errorf("SudoWindow = %v, want 5m", cfg.SudoWindow)
	}
	if cfg.SessionLifetime != 24*time.Hour {
		t.Errorf("SessionLifetime = %v, want 24h", cfg.SessionLifetime)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v, want info", cfg.LogLevel)
	}
	if cfg.InsecureHTTP {
		t.Error("InsecureHTTP is true by default")
	}
	if cfg.TLSCert != "" || cfg.TLSKey != "" {
		t.Errorf("TLS material defaults to %q/%q, want empty", cfg.TLSCert, cfg.TLSKey)
	}
	for _, o := range optionTable("") {
		if got := cfg.Origin(o.name); got != OriginDefault {
			t.Errorf("origin of %s = %q, want %q", o.name, got, OriginDefault)
		}
	}
}

// D-03's precedence, demonstrated one option at a time rather than in bulk, so
// a regression names the option it broke.
func TestPrecedenceFlagBeatsEnvBeatsDefault(t *testing.T) {
	cases := []struct {
		option  string
		env     string
		envVal  string
		flagArg string
		wantEnv string
		want    string
		get     func(Config) string
	}{
		{
			option: "listen", env: "HOLZKUBE_LISTEN", envVal: "127.0.0.1:9000",
			flagArg: "--listen=127.0.0.1:9100",
			wantEnv: "127.0.0.1:9000", want: "127.0.0.1:9100",
			get: func(c Config) string { return c.Listen },
		},
		{
			option: "sudo-window", env: "HOLZKUBE_SUDO_WINDOW", envVal: "9m",
			flagArg: "--sudo-window=11m",
			wantEnv: "9m0s", want: "11m0s",
			get: func(c Config) string { return c.SudoWindow.String() },
		},
		{
			option: "session-lifetime", env: "HOLZKUBE_SESSION_LIFETIME", envVal: "2h",
			flagArg: "--session-lifetime=3h",
			wantEnv: "2h0m0s", want: "3h0m0s",
			get: func(c Config) string { return c.SessionLifetime.String() },
		},
		{
			option: "log-level", env: "HOLZKUBE_LOG_LEVEL", envVal: "warn",
			flagArg: "--log-level=debug",
			wantEnv: "WARN", want: "DEBUG",
			get: func(c Config) string { return c.LogLevel.String() },
		},
	}

	for _, tc := range cases {
		t.Run(tc.option, func(t *testing.T) {
			env := map[string]string{tc.env: tc.envVal}

			envOnly := load(t, nil, env)
			if got := tc.get(envOnly); got != tc.wantEnv {
				t.Errorf("with only the environment set: %q, want %q", got, tc.wantEnv)
			}
			if got := envOnly.Origin(tc.option); got != OriginEnv {
				t.Errorf("origin = %q, want %q", got, OriginEnv)
			}

			both := load(t, []string{tc.flagArg}, env)
			if got := tc.get(both); got != tc.want {
				t.Errorf("with flag and environment set: %q, want the flag value %q", got, tc.want)
			}
			if got := both.Origin(tc.option); got != OriginFlag {
				t.Errorf("origin = %q, want %q", got, OriginFlag)
			}
		})
	}
}

// The data directory has three layers rather than two, because XDG_DATA_HOME is
// not a holzkube variable and must lose to one (D-02).
func TestDataDirPrecedence(t *testing.T) {
	xdg := map[string]string{"XDG_DATA_HOME": "/xdg"}

	if got := load(t, nil, xdg).DataDir; got != filepath.Join("/xdg", "holzkube") {
		t.Errorf("XDG_DATA_HOME ignored: %q", got)
	}

	withEnv := map[string]string{"XDG_DATA_HOME": "/xdg", "HOLZKUBE_DATA_DIR": "/env"}
	if got := load(t, nil, withEnv).DataDir; got != "/env" {
		t.Errorf("HOLZKUBE_DATA_DIR did not beat XDG_DATA_HOME: %q", got)
	}

	cfg := load(t, []string{"--data-dir=/flag"}, withEnv)
	if cfg.DataDir != "/flag" {
		t.Errorf("--data-dir did not beat the environment: %q", cfg.DataDir)
	}
	if got := cfg.Origin("data-dir"); got != OriginFlag {
		t.Errorf("origin = %q, want %q", got, OriginFlag)
	}
}

// Every option is reachable from a flag and from a HOLZKUBE_ variable. The test
// value table is checked against the option table, so a new option that forgets
// either half fails here rather than in production.
func TestEveryOptionIsSettableByFlagAndByEnvironment(t *testing.T) {
	values := map[string]string{
		"listen":           "127.0.0.1:9443",
		"data-dir":         "/data",
		"tls-cert":         "/tls/cert.pem",
		"tls-key":          "/tls/key.pem",
		"insecure-http":    "true",
		"sudo-window":      "7m0s",
		"session-lifetime": "48h0m0s",
		"log-level":        "debug",
	}

	table := optionTable("")
	if len(values) != len(table) {
		t.Fatalf("the test value table covers %d options, the option table has %d", len(values), len(table))
	}

	var args []string
	env := map[string]string{}
	for _, o := range table {
		v, ok := values[o.name]
		if !ok {
			t.Fatalf("option %q has no test value; add one", o.name)
		}
		if o.env == "" {
			t.Errorf("option %q has no environment variable", o.name)
		}
		if !strings.Contains(o.usage, EnvPrefix+o.env) {
			t.Errorf("usage of %q does not name %s%s", o.name, EnvPrefix, o.env)
		}
		args = append(args, "--"+o.name+"="+v)
		env[EnvPrefix+o.env] = v
	}

	fromEnv := load(t, nil, env)
	fromFlags := load(t, args, nil)

	for _, o := range table {
		want := values[o.name]
		if got := o.display(fromEnv); got != want {
			t.Errorf("%s from the environment = %q, want %q", o.name, got, want)
		}
		if got := fromEnv.Origin(o.name); got != OriginEnv {
			t.Errorf("%s origin = %q, want %q", o.name, got, OriginEnv)
		}
		if got := o.display(fromFlags); got != want {
			t.Errorf("%s from a flag = %q, want %q", o.name, got, want)
		}
		if got := fromFlags.Origin(o.name); got != OriginFlag {
			t.Errorf("%s origin = %q, want %q", o.name, got, OriginFlag)
		}
	}
}

// A value that does not parse aborts the start. Falling back to the default
// would run the server with a configuration the operator did not ask for and
// believes is in force.
func TestUnparsableValueAbortsAndNamesOptionAndOrigin(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		env      map[string]string
		contains []string
	}{
		{
			name:     "duration without a unit from the environment",
			env:      map[string]string{"HOLZKUBE_SUDO_WINDOW": "5"},
			contains: []string{"sudo-window", "HOLZKUBE_SUDO_WINDOW", `"5"`},
		},
		{
			name:     "duration without a unit from a flag",
			args:     []string{"--session-lifetime=24"},
			contains: []string{"session-lifetime", "flag", `"24"`},
		},
		{
			// An operator setting this to mean "always re-ask" used to get a
			// five-minute window, with the startup log reporting the 0 that was
			// not in force.
			name:     "a sudo window of zero",
			env:      map[string]string{"HOLZKUBE_SUDO_WINDOW": "0s"},
			contains: []string{"sudo-window", "HOLZKUBE_SUDO_WINDOW", `"0s"`},
		},
		{
			name:     "a negative sudo window",
			args:     []string{"--sudo-window=-1m"},
			contains: []string{"sudo-window", "flag", `"-1m"`},
		},
		{
			name:     "a sudo window longer than a session",
			args:     []string{"--sudo-window=48h"},
			contains: []string{"sudo-window", "flag", `"48h"`},
		},
		{
			name:     "unknown log level",
			env:      map[string]string{"HOLZKUBE_LOG_LEVEL": "chatty"},
			contains: []string{"log-level", "HOLZKUBE_LOG_LEVEL", `"chatty"`},
		},
		{
			name:     "listen address without a port",
			args:     []string{"--listen=127.0.0.1"},
			contains: []string{"listen", "flag", `"127.0.0.1"`},
		},
		{
			name:     "boolean that is not a boolean",
			env:      map[string]string{"HOLZKUBE_INSECURE_HTTP": "maybe"},
			contains: []string{"insecure-http", "HOLZKUBE_INSECURE_HTTP", `"maybe"`},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadWith(tc.args, envFrom(tc.env), testHome)
			if err == nil {
				t.Fatal("bad value accepted")
			}
			for _, want := range tc.contains {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

// Half a TLS pair is a configuration mistake, not a reason to generate one:
// silently self-signing would leave the operator believing their certificate is
// in force (T-01-40).
func TestHalfATLSPairIsRefused(t *testing.T) {
	_, err := LoadWith([]string{"--tls-cert=/tls/cert.pem"}, envFrom(nil), testHome)
	if err == nil {
		t.Fatal("--tls-cert without --tls-key was accepted")
	}
	if !strings.Contains(err.Error(), "tls-key") {
		t.Errorf("error %q does not name the missing counterpart --tls-key", err)
	}

	_, err = LoadWith([]string{"--tls-key=/tls/key.pem"}, envFrom(nil), testHome)
	if err == nil {
		t.Fatal("--tls-key without --tls-cert was accepted")
	}
	if !strings.Contains(err.Error(), "tls-cert") {
		t.Errorf("error %q does not name the missing counterpart --tls-cert", err)
	}
}

func logRecords(t *testing.T, cfg Config) []map[string]any {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	cfg.LogEffective(logger)

	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line %q is not JSON: %v", line, err)
		}
		out = append(out, rec)
	}
	return out
}

// One line per option, with the effective value and where it came from. A
// misconfiguration is then visible at start rather than in the failure it
// eventually causes (D-03).
func TestLogEffectiveLogsEveryOptionExactlyOnce(t *testing.T) {
	cfg := load(t, []string{"--sudo-window=9m"}, map[string]string{"HOLZKUBE_LISTEN": "127.0.0.1:9443"})

	seen := map[string]int{}
	for _, rec := range logRecords(t, cfg) {
		name, ok := rec["option"].(string)
		if !ok {
			continue
		}
		seen[name]++
		if _, ok := rec["value"]; !ok {
			t.Errorf("option %s logged without a value", name)
		}
		if _, ok := rec["origin"]; !ok {
			t.Errorf("option %s logged without an origin", name)
		}
	}

	for _, o := range optionTable("") {
		switch seen[o.name] {
		case 1:
		case 0:
			t.Errorf("option %s never appears in the startup log", o.name)
		default:
			t.Errorf("option %s appears %d times in the startup log", o.name, seen[o.name])
		}
	}

	for _, rec := range logRecords(t, cfg) {
		if rec["option"] == "listen" {
			if rec["value"] != "127.0.0.1:9443" || rec["origin"] != string(OriginEnv) {
				t.Errorf("listen logged as %v from %v", rec["value"], rec["origin"])
			}
		}
		if rec["option"] == "sudo-window" {
			if rec["value"] != "9m0s" || rec["origin"] != string(OriginFlag) {
				t.Errorf("sudo-window logged as %v from %v", rec["value"], rec["origin"])
			}
		}
	}
}

// No option carries a secret today -- TLS material is referenced by path, and
// the path is what gets logged. The rule is enforced at the renderer so that the
// first option that does carry one cannot leak by omission (T-01-43).
func TestSecretOptionsAreNeverRenderedInCleartext(t *testing.T) {
	cfg := load(t, []string{"--tls-cert=/tls/cert.pem", "--tls-key=/tls/key.pem"}, nil)

	secret := option{
		name:   "token",
		secret: true,
		render: func(Config) string { return "hunter2" },
	}
	if got := secret.display(cfg); strings.Contains(got, "hunter2") {
		t.Fatalf("a secret option rendered as %q", got)
	}

	for _, rec := range logRecords(t, cfg) {
		if rec["option"] == "tls-key" && rec["value"] != "/tls/key.pem" {
			t.Errorf("tls-key logged as %v, want the path", rec["value"])
		}
	}
}

// A wildcard bind is a legitimate choice and is not refused -- but it is never
// silent (PITFALLS.md:497,633, T-01-37).
func TestBindBeyondLoopbackIsWarned(t *testing.T) {
	warnings := func(cfg Config) []string {
		var out []string
		for _, rec := range logRecords(t, cfg) {
			if msg, ok := rec["msg"].(string); ok && rec["level"] == "WARN" {
				out = append(out, msg)
			}
		}
		return out
	}

	if got := warnings(load(t, nil, nil)); len(got) != 0 {
		t.Errorf("the loopback default warned: %v", got)
	}
	if got := warnings(load(t, []string{"--listen=[::1]:8443"}, nil)); len(got) != 0 {
		t.Errorf("an IPv6 loopback bind warned: %v", got)
	}

	for _, listen := range []string{"0.0.0.0:8443", ":8443", "192.168.1.10:8443", "[::]:8443"} {
		got := warnings(load(t, []string{"--listen=" + listen}, nil))
		if len(got) == 0 {
			t.Errorf("binding %s produced no warning", listen)
		}
	}
}

func TestIsLoopback(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:8443": true,
		"127.0.0.53:80":  true,
		"[::1]:8443":     true,
		"localhost:8443": true,
		"LocalHost:8443": true,
		"0.0.0.0:8443":   false,
		":8443":          false,
		"[::]:8443":      false,
		"192.168.1.10:1": false,
		"example.com:80": false,
	}
	for listen, want := range cases {
		got, err := IsLoopback(listen)
		if err != nil {
			t.Fatalf("IsLoopback(%q): %v", listen, err)
		}
		if got != want {
			t.Errorf("IsLoopback(%q) = %v, want %v", listen, got, want)
		}
	}
}

func TestHelpAndVersionAreSentinels(t *testing.T) {
	if _, err := LoadWith([]string{"--help"}, envFrom(nil), testHome); !errors.Is(err, ErrHelp) {
		t.Errorf("--help returned %v, want ErrHelp", err)
	}
	if _, err := LoadWith([]string{"--version"}, envFrom(nil), testHome); !errors.Is(err, ErrVersion) {
		t.Errorf("--version returned %v, want ErrVersion", err)
	}

	var buf bytes.Buffer
	Usage(&buf)
	for _, o := range optionTable("") {
		if !strings.Contains(buf.String(), "-"+o.name) {
			t.Errorf("usage does not mention --%s", o.name)
		}
		if !strings.Contains(buf.String(), EnvPrefix+o.env) {
			t.Errorf("usage does not mention %s%s", EnvPrefix, o.env)
		}
	}
}
