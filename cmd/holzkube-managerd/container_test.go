package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The container's security properties, asserted against the files themselves
// (OPS-04).
//
// This is a static check and it says so: no Docker daemon runs here, so
// nothing below proves the image builds or that the process can write its
// volume. What it does prove is that the two properties that make this
// container safe to run are actually written down -- and those are the two an
// edit in a hurry removes.
//
// It reads a Dockerfile from Go, which is the direction this repository's
// other cross-language guards already go: budget_drift_test.go reads a .tsx
// file for the same reason. The alternative is a comment asking people to keep
// two files in step, which is not a mechanism.

func repoFile(t *testing.T, name string) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("..", "..", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(raw)
}

// TestTheContainerRunsAsANonRootUser.
//
// holzkube-manager holds cluster PKI. A container that ran as root and wrote a
// world-readable volume would put every secret this product exists to protect
// one `docker cp` away from anybody on the host.
func TestTheContainerRunsAsANonRootUser(t *testing.T) {
	t.Parallel()

	dockerfile := repoFile(t, "Dockerfile")

	user := regexp.MustCompile(`(?m)^USER\s+(\S+)`).FindStringSubmatch(dockerfile)
	if user == nil {
		t.Fatal("the Dockerfile declares no USER, so the container runs as root")
	}
	if user[1] == "root" || strings.HasPrefix(user[1], "0:") || user[1] == "0" {
		t.Fatalf("the Dockerfile runs as %q", user[1])
	}

	// The same uid in the Compose file, because the volume's ownership has to
	// match it and a mismatch is the single most common way this setup fails.
	compose := repoFile(t, "compose.yaml")
	uid, _, _ := strings.Cut(user[1], ":")
	if !strings.Contains(compose, uid) {
		t.Errorf("the Dockerfile runs as uid %s and compose.yaml does not mention it; a volume "+
			"prepared for the wrong uid is the usual way this setup fails", uid)
	}
}

// TestTheContainerHasNoShell.
//
// A scratch image has no shell, no package manager and no system trust store,
// and it needs none of them: the binary is static, it embeds its own web
// assets, and the Talos endpoints it reaches are verified against the cluster
// PKI it holds rather than against a system CA bundle. Every one of those
// would be a way in this product has no use for.
func TestTheContainerHasNoShell(t *testing.T) {
	t.Parallel()

	dockerfile := repoFile(t, "Dockerfile")

	final := dockerfile[strings.LastIndex(dockerfile, "\nFROM "):]
	if !strings.Contains(final, "FROM scratch") {
		t.Errorf("the final stage is not scratch:\n%s", firstLine(final))
	}

	// CGO off, or the binary needs a dynamic loader that scratch does not have.
	if !strings.Contains(dockerfile, "CGO_ENABLED=0") {
		t.Error("the build does not set CGO_ENABLED=0; a dynamically linked binary cannot run " +
			"in a scratch image")
	}
}

// TestTheDataDirectoryIsAVolume.
//
// A `docker run` without `-v` is otherwise an installation that silently loses
// its cluster PKI on the next `docker rm`.
func TestTheDataDirectoryIsAVolume(t *testing.T) {
	t.Parallel()

	dockerfile := repoFile(t, "Dockerfile")

	if !strings.Contains(dockerfile, "VOLUME") {
		t.Error("the Dockerfile declares no VOLUME for the data directory")
	}
	if !strings.Contains(dockerfile, "HOLZKUBE_MANAGER_DATA_DIR") {
		t.Error("the Dockerfile does not point the data directory at the volume")
	}
}

// TestTheComposeFileDoesNotPublishOnEveryInterface.
//
// The dashboard shows every node's state and the API can wipe a machine.
// Publishing that on a LAN address is a deliberate act, and a default that did
// it for the operator is a default that decides for them.
func TestTheComposeFileDoesNotPublishOnEveryInterface(t *testing.T) {
	t.Parallel()

	compose := repoFile(t, "compose.yaml")

	for _, line := range strings.Split(compose, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- \"") || !strings.Contains(trimmed, ":") {
			continue
		}
		// A published port with no host address binds every interface.
		if regexp.MustCompile(`^- "\d+:\d+"`).MatchString(trimmed) {
			t.Errorf("compose.yaml publishes %s on every interface", trimmed)
		}
	}

	if !strings.Contains(compose, "127.0.0.1:") {
		t.Error("compose.yaml does not bind the published port to loopback by default")
	}
}

// TestTheContainerDropsWhatItDoesNotNeed.
//
// The process opens one listening socket and writes one directory. A
// capability it never uses is a capability an exploit inherits.
func TestTheContainerDropsWhatItDoesNotNeed(t *testing.T) {
	t.Parallel()

	compose := repoFile(t, "compose.yaml")

	for _, want := range []string{"cap_drop", "no-new-privileges", "read_only"} {
		if !strings.Contains(compose, want) {
			t.Errorf("compose.yaml does not set %s", want)
		}
	}
}

// TestTheBuildContextExcludesTheGitDirectory.
//
// A .git directory in a build context is every secret anybody ever committed
// and then removed.
func TestTheBuildContextExcludesTheGitDirectory(t *testing.T) {
	t.Parallel()

	ignore := repoFile(t, ".dockerignore")

	for _, want := range []string{".git", "node_modules", "*.pem", "*.key"} {
		if !strings.Contains(ignore, want) {
			t.Errorf(".dockerignore does not exclude %s", want)
		}
	}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
