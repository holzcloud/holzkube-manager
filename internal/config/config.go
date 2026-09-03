// Package config resolves holzkube-manager's runtime configuration.
//
// There is deliberately no configuration file, no search path and no schema
// (D-03): flags and HOLZKUBE_MANAGER_* environment variables only, with precedence
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
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// NormalizeHost reduces a Host header or a configured address to a comparable
// host: no port, no brackets, lowercase.
//
// It lives here, and middleware.AllowHosts calls it, so that the set of hosts
// this instance answers to and the set on which the password is refused are
// compared by the same rule. Two implementations of "is this the same host"
// that disagree would mean a name the allowlist admits and the SSO policy does
// not recognise -- which fails open, on the one host where that is worst.
func NormalizeHost(host string) string {
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(strings.Trim(host, "[]"))
}

// splitHosts parses a comma-separated host list into normalised, de-duplicated
// entries. An entry carrying a scheme or a path is refused rather than
// normalised: it means a URL was pasted where a host belongs, and quietly
// keeping the part that happens to parse would admit a host the operator did
// not name.
func splitHosts(raw string) ([]string, error) {
	var hosts []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.ContainsAny(part, "/\\ \t") {
			return nil, fmt.Errorf("%q is not a bare host; give a name or an address, without scheme or path", part)
		}
		h := NormalizeHost(part)
		if h == "" {
			return nil, fmt.Errorf("%q is not a host", part)
		}
		if !slices.Contains(hosts, h) {
			hosts = append(hosts, h)
		}
	}
	return hosts, nil
}

// EnvPrefix is prepended to every environment variable holzkube-manager reads.
const EnvPrefix = "HOLZKUBE_MANAGER_"

// redacted replaces the value of an option marked secret.
const redacted = "<redacted>"

// maxSudoWindow bounds --sudo-window. The window is the only mechanism that
// limits what a stolen session cookie can do (T-01-25), and one that outlives
// the session it sits in has stopped being a window.
const maxSudoWindow = 24 * time.Hour

// Origin records where an effective value came from. It is logged next to the
// value, because "the option is wrong" and "the option is being read from a
// place I forgot about" are different problems with the same symptom.
type Origin string

const (
	// OriginDefault means nothing set the option; the built-in default applies.
	OriginDefault Origin = "default"
	// OriginEnv means a HOLZKUBE_MANAGER_ environment variable set the option.
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
	DryRun          bool
	SudoWindow      time.Duration
	SessionLifetime time.Duration
	LogLevel        slog.Level

	// AllowedHosts names further Host header values this instance answers to,
	// beyond the bind address and the loopback names the composition root
	// derives on its own. A reverse proxy publishing holzkube-manager under a
	// public name is the reason it exists: that name reaches the process in the
	// Host header and in nothing else, so there is no way to infer it.
	AllowedHosts []string

	// SSOOnlyHosts is the subset of the answered hosts on which the local
	// password is not accepted -- neither to log in nor to open the sudo
	// window. Only the identity provider can authenticate there.
	//
	// The split exists because the two ways in are not equally exposed. A
	// break-glass account that works from the LAN is what gets the operator
	// back in when the identity provider is down; the same account reachable
	// from the internet is a password on the public net guarding cluster PKI.
	SSOOnlyHosts []string

	OIDCIssuer       string
	OIDCClientID     string
	OIDCClientSecret string

	// oidcClientSecretFile is resolved into OIDCClientSecret by Load and is
	// never a second source of truth afterwards.
	oidcClientSecretFile string

	origins map[string]Origin
}

// OIDCEnabled reports whether the identity provider is configured. All three
// values are required together; Load refuses a partial set.
func (c Config) OIDCEnabled() bool {
	return c.OIDCIssuer != "" && c.OIDCClientID != "" && c.OIDCClientSecret != ""
}

// IsSSOOnly reports whether host may not use the local password. The comparison
// is on the same normalised form middleware.AllowHosts uses, so a configured
// name and an incoming Host header agree on ports, brackets and case.
func (c Config) IsSSOOnly(host string) bool {
	return slices.Contains(c.SSOOnlyHosts, NormalizeHost(host))
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

// optionTable is the single definition of holzkube-manager's configuration surface.
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
			// The bind address and the loopback names are derived, not
			// configured. This is for the names that only ever arrive in a Host
			// header -- a reverse proxy's public hostname -- which nothing in
			// the process can otherwise discover.
			name: "allowed-hosts", env: "ALLOWED_HOSTS", def: "",
			usage: "further Host values this instance answers to, comma separated (env " + EnvPrefix + "ALLOWED_HOSTS)",
			apply: func(c *Config, raw string) error {
				hosts, err := splitHosts(raw)
				if err != nil {
					return err
				}
				c.AllowedHosts = hosts
				return nil
			},
			render: func(c Config) string { return strings.Join(c.AllowedHosts, ",") },
		},
		{
			name: "sso-only-hosts", env: "SSO_ONLY_HOSTS", def: "",
			usage: "hosts on which only the identity provider may authenticate, comma separated (env " + EnvPrefix + "SSO_ONLY_HOSTS)",
			apply: func(c *Config, raw string) error {
				hosts, err := splitHosts(raw)
				if err != nil {
					return err
				}
				c.SSOOnlyHosts = hosts
				return nil
			},
			render: func(c Config) string { return strings.Join(c.SSOOnlyHosts, ",") },
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
			// FOUND-12 / D-03. The refusal itself is not here: it is a gRPC
			// client interceptor on internal/talos's single shared connect
			// path, so that "no mutation reaches a node" is enforced at the
			// last layer before the wire rather than at the HTTP boundary,
			// which would not see anything the jobs engine drives. This entry
			// is only how the operator says so.
			name: "dry-run", env: "DRY_RUN", def: "false", boolean: true,
			usage: "refuse every mutating node call at the transport (env " + EnvPrefix + "DRY_RUN)",
			apply: func(c *Config, raw string) error {
				v, err := strconv.ParseBool(raw)
				if err != nil {
					return errors.New("not a boolean")
				}
				c.DryRun = v
				return nil
			},
			render: func(c Config) string { return strconv.FormatBool(c.DryRun) },
		},
		{
			name: "sudo-window", env: "SUDO_WINDOW", def: "5m0s",
			usage: "how long a re-authentication authorises destructive actions (env " + EnvPrefix + "SUDO_WINDOW)",
			apply: func(c *Config, raw string) error {
				d, err := time.ParseDuration(raw)
				if err != nil {
					return errors.New("not a duration; durations need a unit, for example 5m")
				}
				// Validated here rather than left to IsSudoOpen, which
				// substitutes the default for any value <= 0. That substitution
				// is right inside the gate -- a zero window would make every
				// destructive route unreachable -- but as configuration
				// handling it meant an operator setting 0 to mean "always
				// re-ask" silently got five minutes, while LogEffective
				// faithfully printed the value that was not in force. The
				// startup log D-03 exists to make misconfiguration visible, so
				// the refusal belongs where the operator can still act on it.
				if d <= 0 {
					return errors.New("must be positive; there is no value that disables the sudo gate")
				}
				if d > maxSudoWindow {
					return errors.New("must not exceed 24h; a window longer than a session is not a window")
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
			name: "oidc-issuer", env: "OIDC_ISSUER", def: "",
			usage: "OpenID Connect issuer URL; discovery is read from it (env " + EnvPrefix + "OIDC_ISSUER)",
			apply: func(c *Config, raw string) error {
				if raw != "" {
					u, err := url.Parse(raw)
					if err != nil || u.Scheme == "" || u.Host == "" {
						return errors.New("not an absolute URL, for example https://authentik.example.com/application/o/holzkube-manager/")
					}
					// The issuer is compared byte for byte against the iss
					// claim of every token (OIDC Core 3.1.3.7). A trailing
					// slash the operator added or dropped would otherwise fail
					// verification with a message about the token rather than
					// about this setting.
					if u.Scheme != "https" && !isLoopbackURL(u) {
						return errors.New("must be https; an issuer reached over plaintext can be substituted in transit")
					}
				}
				c.OIDCIssuer = raw
				return nil
			},
			render: func(c Config) string { return c.OIDCIssuer },
		},
		{
			name: "oidc-client-id", env: "OIDC_CLIENT_ID", def: "",
			usage: "OpenID Connect client ID (env " + EnvPrefix + "OIDC_CLIENT_ID)",
			apply: func(c *Config, raw string) error {
				c.OIDCClientID = raw
				return nil
			},
			render: func(c Config) string { return c.OIDCClientID },
		},
		{
			// secret: the value is redacted in the startup log and in help.
			name: "oidc-client-secret", env: "OIDC_CLIENT_SECRET", def: "", secret: true,
			usage: "OpenID Connect client secret; prefer the -file form (env " + EnvPrefix + "OIDC_CLIENT_SECRET)",
			apply: func(c *Config, raw string) error {
				c.OIDCClientSecret = raw
				return nil
			},
			render: func(c Config) string { return c.OIDCClientSecret },
		},
		{
			// The reason this exists rather than only the value form: a systemd
			// unit file is world-readable, and an Environment= line in one puts
			// the client secret in front of every account on the host. systemd
			// LoadCredential= and Docker secrets both present a file instead,
			// and this is how holzkube-manager reads it.
			name: "oidc-client-secret-file", env: "OIDC_CLIENT_SECRET_FILE", def: "",
			usage: "file holding the client secret, read at start (env " + EnvPrefix + "OIDC_CLIENT_SECRET_FILE)",
			apply: func(c *Config, raw string) error {
				c.oidcClientSecretFile = raw
				return nil
			},
			render: func(c Config) string { return c.oidcClientSecretFile },
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
		// Not fatal on its own: --data-dir or HOLZKUBE_MANAGER_DATA_DIR may supply the
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
	fs := flag.NewFlagSet("holzkube-managerd", flag.ContinueOnError)
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
		return Config{}, fmt.Errorf("config: unexpected argument %q; holzkube-managerd takes options only", fs.Arg(0))
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
				"holzkube-manager does not fall back to a generated certificate once one was configured",
			given, missing)
	}

	if err := cfg.resolveOIDC(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// resolveOIDC reads the client secret file if one was named and refuses every
// half-configured identity provider.
//
// Each refusal here replaces a failure that would otherwise surface much later
// and much less legibly: a partial client configuration fails at the first
// login attempt, and an SSO-only host with no provider configured fails by
// having no way in at all -- discovered by the operator locked out of it.
func (c *Config) resolveOIDC() error {
	if c.oidcClientSecretFile != "" {
		if c.OIDCClientSecret != "" {
			return errors.New("config: --oidc-client-secret and --oidc-client-secret-file were both given; " +
				"the secret has to come from exactly one place, or the log says one thing and the process uses another")
		}
		b, err := os.ReadFile(c.oidcClientSecretFile)
		if err != nil {
			return fmt.Errorf("config: --oidc-client-secret-file: %w", err)
		}
		// A trailing newline is what every editor and `echo` adds, and a secret
		// that differs from the provider's by one byte fails at the token
		// endpoint with an error that names neither the byte nor the file.
		secret := strings.TrimRight(string(b), "\r\n")
		if secret == "" {
			return fmt.Errorf("config: --oidc-client-secret-file: %s is empty", c.oidcClientSecretFile)
		}
		c.OIDCClientSecret = secret
	}

	set := map[string]bool{
		"oidc-issuer":        c.OIDCIssuer != "",
		"oidc-client-id":     c.OIDCClientID != "",
		"oidc-client-secret": c.OIDCClientSecret != "",
	}
	var missing []string
	var any bool
	for _, name := range []string{"oidc-issuer", "oidc-client-id", "oidc-client-secret"} {
		if set[name] {
			any = true
		} else {
			missing = append(missing, "--"+name)
		}
	}
	if any && len(missing) > 0 {
		return fmt.Errorf("config: the identity provider is half configured; %s %s missing",
			strings.Join(missing, " and "), plural(len(missing), "is", "are"))
	}

	if len(c.SSOOnlyHosts) > 0 && !c.OIDCEnabled() {
		return fmt.Errorf(
			"config: --sso-only-hosts names %s but no identity provider is configured; "+
				"that host would refuse the password and have nothing to offer instead",
			strings.Join(c.SSOOnlyHosts, ", "))
	}
	return nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// isLoopbackURL reports whether u names the loopback interface, which is the
// one case where a plaintext issuer is not a substitutable one.
func isLoopbackURL(u *url.URL) bool {
	host := u.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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
		logger.Warn("listening beyond loopback: holzkube-manager is reachable from every device on this network",
			slog.String("listen", c.Listen))
	}

	// Dry-run gets a warning of its own, above the option line, and the line
	// states the consequence rather than the setting. "dry-run: true" is a
	// label; an operator who reads it still has to know what the mode does, and
	// the whole failure this mode exists to prevent is an operator who is
	// mistaken about which way round it is (T-02-44).
	if c.DryRun {
		logger.Warn("dry-run is on: every mutating call is refused at the transport and no mutation will reach any node")
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
	fmt.Fprint(w, `holzkube-managerd serves the holzkube-manager web UI over HTTPS.

Usage: holzkube-managerd [options]

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
