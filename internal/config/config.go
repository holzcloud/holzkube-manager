// Package config resolves holzkube's runtime configuration.
//
// There is deliberately no configuration file, no search path and no schema
// (D-03): flags and HOLZKUBE_* environment variables only, with precedence
// flag > env > default. Nothing to parse means nothing to migrate, and
// Docker/Compose speaks environment variables natively.
package config

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// EnvPrefix is prepended to every environment variable holzkube reads.
const EnvPrefix = "HOLZKUBE_"

// Config is the resolved configuration for one process.
type Config struct {
	Listen          string
	DataDir         string
	TLSCert         string
	TLSKey          string
	InsecureHTTP    bool
	SudoWindow      time.Duration
	SessionLifetime time.Duration
}

// Load resolves the configuration from args (without the program name) and the
// process environment.
func Load(args []string) (Config, error) {
	dataDir, err := DefaultDataDir()
	if err != nil {
		return Config{}, err
	}

	// Seeding each flag's default from the environment is what makes the
	// precedence flag > env > default fall out for free: an unset flag keeps
	// the environment value, a set flag overrides it.
	cfg := Config{
		Listen:          envString("LISTEN", "127.0.0.1:8443"),
		DataDir:         envString("DATA_DIR", dataDir),
		TLSCert:         envString("TLS_CERT", ""),
		TLSKey:          envString("TLS_KEY", ""),
		InsecureHTTP:    envBool("INSECURE_HTTP", false),
		SudoWindow:      envDuration("SUDO_WINDOW", 5*time.Minute),
		SessionLifetime: envDuration("SESSION_LIFETIME", 24*time.Hour),
	}

	fs := flag.NewFlagSet("holzkubed", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.Listen, "listen", cfg.Listen, "address to listen on (env "+EnvPrefix+"LISTEN)")
	fs.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "state directory (env "+EnvPrefix+"DATA_DIR)")
	fs.StringVar(&cfg.TLSCert, "tls-cert", cfg.TLSCert, "PEM certificate; generated on first run when unset")
	fs.StringVar(&cfg.TLSKey, "tls-key", cfg.TLSKey, "PEM private key for --tls-cert")
	fs.BoolVar(&cfg.InsecureHTTP, "insecure-http", cfg.InsecureHTTP, "serve plain HTTP (loopback only)")
	fs.DurationVar(&cfg.SudoWindow, "sudo-window", cfg.SudoWindow, "how long a re-authentication authorises destructive actions")
	fs.DurationVar(&cfg.SessionLifetime, "session-lifetime", cfg.SessionLifetime, "absolute session lifetime")

	if err := fs.Parse(args); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	if cfg.DataDir == "" {
		return Config{}, fmt.Errorf("config: data directory must not be empty")
	}
	if (cfg.TLSCert == "") != (cfg.TLSKey == "") {
		return Config{}, fmt.Errorf("config: --tls-cert and --tls-key must be given together")
	}
	return cfg, nil
}

// DefaultDataDir follows the XDG base directory specification (D-02):
// $XDG_DATA_HOME/holzkube, falling back to ~/.local/share/holzkube.
func DefaultDataDir() (string, error) {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "holzkube"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config: resolve home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "holzkube"), nil
}

func envString(name, fallback string) string {
	if v, ok := os.LookupEnv(EnvPrefix + name); ok {
		return v
	}
	return fallback
}

func envBool(name string, fallback bool) bool {
	v, ok := os.LookupEnv(EnvPrefix + name)
	if !ok {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(name string, fallback time.Duration) time.Duration {
	v, ok := os.LookupEnv(EnvPrefix + name)
	if !ok {
		return fallback
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return parsed
}
