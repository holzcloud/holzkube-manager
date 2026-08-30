package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Lookup reads one environment variable. It is a parameter rather than a direct
// call to os.LookupEnv so that resolution is testable without mutating the
// process environment, which is global state shared by every parallel test.
type Lookup func(name string) (string, bool)

const (
	// DirPerm is the mode of the data directory (D-02, FOUND-10). The directory
	// holds session material today and cluster PKI from phase 2 onward, which is
	// equivalent to root on every managed node.
	DirPerm = 0o700

	// parentPerm applies to intermediate directories such as ~/.local/share,
	// which are shared with other applications and are not ours to restrict.
	parentPerm = 0o755

	appDir = "holzkube-manager"
)

// Resolve returns the data directory (D-02): an explicit override wins, then
// $XDG_DATA_HOME/holzkube-manager, then ~/.local/share/holzkube-manager.
//
// The override is what --data-dir and HOLZKUBE_MANAGER_DATA_DIR fill in, and it is also
// the container's mounted volume path -- which is why it has to beat an
// XDG_DATA_HOME that some base image happened to set.
func Resolve(env Lookup, home, override string) (string, error) {
	if env == nil {
		env = func(string) (string, bool) { return "", false }
	}
	if override != "" {
		return filepath.Clean(override), nil
	}
	if xdg, ok := env("XDG_DATA_HOME"); ok && xdg != "" {
		return filepath.Join(xdg, appDir), nil
	}
	if home != "" {
		return filepath.Join(home, ".local", "share", appDir), nil
	}
	return "", fmt.Errorf(
		"config: cannot determine the data directory: --data-dir and %sDATA_DIR are unset, "+
			"XDG_DATA_HOME is unset, and the home directory could not be determined", EnvPrefix)
}

// EnsureDir creates the data directory with DirPerm if it does not exist.
//
// An existing directory is left exactly as it is, including its mode: the
// permission guard lives in the store and refuses to start on a world-readable
// data directory. Quietly chmod-ing it here would repair the condition the guard
// exists to report, and the operator would never learn that their backup tarball
// or their Docker volume had the wrong ownership.
func EnsureDir(path string) error {
	switch info, err := os.Stat(path); {
	case err == nil:
		if !info.IsDir() {
			return fmt.Errorf("config: data directory %s exists and is not a directory", path)
		}
		return nil
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("config: inspect data directory %s: %w", path, err)
	}

	if parent := filepath.Dir(path); parent != path {
		if err := os.MkdirAll(parent, parentPerm); err != nil {
			return fmt.Errorf("config: create %s: %w", parent, err)
		}
	}
	if err := os.Mkdir(path, DirPerm); err != nil && !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("config: create data directory %s: %w", path, err)
	}
	return nil
}
