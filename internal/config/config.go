// Package config resolves holzkube's runtime configuration.
//
// There is deliberately no configuration file, no search path and no schema
// (D-03): flags and HOLZKUBE_* environment variables only, with precedence
// flag > environment > default. Nothing to parse means nothing to migrate, and
// Docker/Compose speaks environment variables natively.
//
// Every option lives in one table. The flag registration, the environment
// lookup, the help output and the startup log are all generated from it, so a
// new switch cannot be added and then be missing from the help or from the log
// -- the two places an operator looks when a value is not what they expected.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// EnvPrefix is prepended to every environment variable holzkube reads.
const EnvPrefix = "HOLZKUBE_"

// redacted replaces the value of an option marked secret.
const redacted = "<redacted>"

// Origin records where an effective value came from. It is logged next to the
// value, because "the option is wrong" and "the option is being read from a
// place I forgot about" are different problems with the same symptom.
type Origin string

const (
	// OriginDefault means nothing set the option; the built-in default applies.
	OriginDefault Origin = "default"
	// OriginEnv means a HOLZKUBE_ environment variable set the option.
	OriginEnv Origin = "environment"
	// OriginFlag means a command line flag set the option.
	OriginFlag Origin = "flag"
)

var (
	// ErrHelp is returned when the operator asked for the help output.
	ErrHelp = flag.ErrHelp
	// ErrVersion is returned when the operator asked for the version. The
	// version string itself belongs to the binary, not to this package.
	ErrVersion = errors.New("config: version requested")
)

// Config is the resolved configuration for one process.
type Config struct {
	Listen          string
	DataDir         string
	TLSCert         string
	TLSKey          string
	InsecureHTTP    bool
	SudoWindow      time.Duration
	SessionLifetime time.Duration
	LogLevel        slog.Level

	origins map[string]Origin
}

// Origin reports where the effective value of an option came from.
func (c Config) Origin(name string) Origin {
	if o, ok := c.origins[name]; ok {
		return o
	}
	return OriginDefault
}

// option describes one switch. Everything the process knows about an option is
// here: how to parse it, how to render it, and whether its value may be logged.
type option struct {
	name    string // flag name, without dashes
	env     string // environment variable name, without EnvPrefix
	def     string // default, in the same textual form a flag would take
	usage   string
	boolean bool // the flag may be given without a value
	secret  bool // the value is never rendered in cleartext

	apply  func(*Config, string) error
	render func(Config) string
}

// display is the value as it may appear in a log or in help output.
func (o option) display(c Config) string {
	if o.secret {
		return redacted
	}
	if v := o.render(c); v != "" {
		return v
	}
	return "(unset)"
}

// optionTable is the single definition of holzkube's configuration surface.
//
// defaultDataDir is the XDG-resolved path used as the default of --data-dir; it
// is irrelevant to callers that only need names and renderers, which may pass an
// empty string.
func optionTable(defaultDataDir string) []option {
	return []option{
		{
			name: "listen", env: "LISTEN", def: "127.0.0.1:8443",
			usage: "address to serve on (env " + EnvPrefix + "LISTEN)",
			apply: func(c *Config, raw string) error {
				if _, _, err := net.SplitHostPort(raw); err != nil {
					return fmt.Errorf("not a host:port address: %w", err)
				}
				c.Listen = raw
				return nil
			},
			render: func(c Config) string { return c.Listen },
		},
		{
			name: "data-dir", env: "DATA_DIR", def: defaultDataDir,
			usage: "state directory, also the container volume path (env " + EnvPrefix + "DATA_DIR)",
			apply: func(c *Config, raw string) error {
				c.DataDir = raw
				return nil
			},
			render: func(c Config) string { return c.DataDir },
		},
		{
			name: "tls-cert", env: "TLS_CERT", def: "",
			usage: "PEM certificate; one is generated on first run when unset (env " + EnvPrefix + "TLS_CERT)",
			apply: func(c *Config, raw string) error {
				c.TLSCert = raw
				return nil
			},
			render: func(c Config) string { return c.TLSCert },
		},
		{
			name: "tls-key", env: "TLS_KEY", def: "",
			// The path is logged; the file's contents never are (T-01-43).
			usage: "PEM private key belonging to --tls-cert (env " + EnvPrefix + "TLS_KEY)",
			apply: func(c *Config, raw string) error {
				c.TLSKey = raw
				return nil
			},
			render: func(c Config) string { return c.TLSKey },
		},
		{
			name: "insecure-http", env: "INSECURE_HTTP", def: "false", boolean: true,
			usage: "serve plain HTTP; refused unless --listen is loopback (env " + EnvPrefix + "INSECURE_HTTP)",
			apply: func(c *Config, raw string) error {
				v, err := strconv.ParseBool(raw)
				if err != nil {
					return errors.New("not a boolean")
				}
				c.InsecureHTTP = v
				return nil
			},
			render: func(c Config) string { return strconv.FormatBool(c.InsecureHTTP) },
		},
		{
			name: "sudo-window", env: "SUDO_WINDOW", def: "5m0s",
			usage: "how long a re-authentication authorises destructive actions (env " + EnvPrefix + "SUDO_WINDOW)",
			apply: func(c *Config, raw string) error {
				d, err := time.ParseDuration(raw)
				if err != nil {
					return errors.New("not a duration; durations need a unit, for example 5m")
				}
				c.SudoWindow = d
				return nil
			},
			render: func(c Config) string { return c.SudoWindow.String() },
		},
		{
			name: "session-lifetime", env: "SESSION_LIFETIME", def: "24h0m0s",
			usage: "absolute session lifetime (env " + EnvPrefix + "SESSION_LIFETIME)",
			apply: func(c *Config, raw string) error {
				d, err := time.ParseDuration(raw)
				if err != nil {
					return errors.New("not a duration; durations need a unit, for example 24h")
				}
				c.SessionLifetime = d
				return nil
			},
			render: func(c Config) string { return c.SessionLifetime.String() },
		},
		{
			name: "log-level", env: "LOG_LEVEL", def: "info",
			usage: "debug, info, warn or error (env " + EnvPrefix + "LOG_LEVEL)",
			apply: func(c *Config, raw string) error {
				var l slog.Level
				if err := l.UnmarshalText([]byte(raw)); err != nil {
					return errors.New("not a log level; use debug, info, warn or error")
				}
				c.LogLevel = l
				return nil
			},
			render: func(c Config) string { return strings.ToLower(c.LogLevel.String()) },
		},
	}
}

// Load resolves the configuration from args (without the program name) and the
// process environment.
func Load(args []string) (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		// Not fatal on its own: --data-dir or HOLZKUBE_DATA_DIR may supply the
		// path. Resolve reports it only if nothing else does.
		home = ""
	}
	return LoadWith(args, os.LookupEnv, home)
}

// LoadWith is Load with the environment and the home directory injected.
func LoadWith(args []string, env Lookup, home string) (Config, error) {
	if env == nil {
		env = func(string) (string, bool) { return "", false }
	}

	defaultDataDir, dataDirErr := Resolve(env, home, "")
	table := optionTable(defaultDataDir)

	cfg := Config{origins: make(map[string]Origin, len(table))}
	for _, o := range table {
		if err := o.apply(&cfg, o.def); err != nil {
			return Config{}, fmt.Errorf("config: built-in default for --%s is invalid: %w", o.name, err)
		}
		cfg.origins[o.name] = OriginDefault
	}

	for _, o := range table {
		raw, ok := env(EnvPrefix + o.env)
		if !ok {
			continue
		}
		if err := o.apply(&cfg, raw); err != nil {
			return Config{}, fmt.Errorf("config: --%s from environment %s%s: invalid value %q: %w",
				o.name, EnvPrefix, o.env, raw, err)
		}
		cfg.origins[o.name] = OriginEnv
	}

	// Flags are parsed into raw strings and applied through the same parser the
	// environment uses, so that a bad value produces the same message with the
	// origin swapped instead of the flag package's own wording.
	fs := flag.NewFlagSet("holzkubed", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	raws := make([]*rawValue, len(table))
	for i, o := range table {
		raws[i] = &rawValue{text: o.def, boolean: o.boolean}
		fs.Var(raws[i], o.name, o.usage)
	}
	// --version is a query, not an option: it has no environment variable and no
	// effective value, so it is deliberately outside the table.
	showVersion := fs.Bool("version", false, "print the version and exit")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return Config{}, ErrHelp
		}
		return Config{}, fmt.Errorf("config: %w", err)
	}
	if *showVersion {
		return Config{}, ErrVersion
	}
	if fs.NArg() > 0 {
		return Config{}, fmt.Errorf("config: unexpected argument %q; holzkubed takes options only", fs.Arg(0))
	}

	for i, o := range table {
		if !raws[i].set {
			continue
		}
		if err := o.apply(&cfg, raws[i].text); err != nil {
			return Config{}, fmt.Errorf("config: --%s from flag: invalid value %q: %w", o.name, raws[i].text, err)
		}
		cfg.origins[o.name] = OriginFlag
	}

	if cfg.DataDir == "" {
		if dataDirErr != nil {
			return Config{}, dataDirErr
		}
		return Config{}, errors.New("config: --data-dir must not be empty")
	}
	if (cfg.TLSCert == "") != (cfg.TLSKey == "") {
		given, missing := "tls-cert", "tls-key"
		if cfg.TLSCert == "" {
			given, missing = "tls-key", "tls-cert"
		}
		return Config{}, fmt.Errorf(
			"config: --%s was given without --%s: supply both or neither; "+
				"holzkube does not fall back to a generated certificate once one was configured",
			given, missing)
	}
	return cfg, nil
}

// LogEffective writes one line per option with its effective value and where
// that value came from (D-03).
func (c Config) LogEffective(logger *slog.Logger) {
	// Only names and renderers are used here, so the default data directory is
	// irrelevant.
	for _, o := range optionTable("") {
		logger.Info("configuration",
			slog.String("option", o.name),
			slog.String("value", o.display(c)),
			slog.String("origin", string(c.Origin(o.name))))
	}

	// A wildcard bind is a legitimate choice and is not refused. It is also not
	// silent: a management tool on a flat home network is reachable from every
	// IoT device on it, and this data directory is equivalent to root on every
	// managed node (PITFALLS.md:497,633).
	if loopback, err := IsLoopback(c.Listen); err != nil || !loopback {
		logger.Warn("listening beyond loopback: holzkube is reachable from every device on this network",
			slog.String("listen", c.Listen))
	}
}

// IsLoopback reports whether a host:port bind address refers to the loopback
// interface. A host name other than localhost is reported as not loopback: it
// would take a DNS lookup to know better, and the callers of this function make
// security decisions, where an unresolvable answer must be the careful one.
func IsLoopback(listen string) (bool, error) {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return false, fmt.Errorf("config: %q is not a host:port address: %w", listen, err)
	}
	if host == "" {
		return false, nil // every interface
	}
	if strings.EqualFold(host, "localhost") {
		return true, nil
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		return ip.IsLoopback(), nil
	}
	return false, nil
}

// Usage writes the help output, generated from the same table as the flags.
func Usage(w io.Writer) {
	fmt.Fprint(w, `holzkubed serves the holzkube web UI over HTTPS.

Usage: holzkubed [options]

Options come from flags and `+EnvPrefix+`* environment variables only; there is no
configuration file. Precedence is flag > environment > default.

`)
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	defaultDataDir, _ := Resolve(os.LookupEnv, home, "")

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, o := range optionTable(defaultDataDir) {
		def := o.def
		if def == "" {
			def = "(unset)"
		}
		fmt.Fprintf(tw, "  --%s\tdefault %s\t%s\n", o.name, def, o.usage)
	}
	fmt.Fprintf(tw, "  --%s\t\t%s\n", "version", "print the version and exit")
	fmt.Fprintf(tw, "  --%s\t\t%s\n", "help", "print this help and exit")
	_ = tw.Flush()
}

// rawValue captures a flag's text without interpreting it, so that the option
// table stays the only place that knows how a value is parsed.
type rawValue struct {
	text    string
	set     bool
	boolean bool
}

func (v *rawValue) String() string {
	if v == nil {
		return ""
	}
	return v.text
}

func (v *rawValue) Set(s string) error {
	v.text = s
	v.set = true
	return nil
}

// IsBoolFlag lets the flag package accept --insecure-http without a value.
func (v *rawValue) IsBoolFlag() bool { return v.boolean }
