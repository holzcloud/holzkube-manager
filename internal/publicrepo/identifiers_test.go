// Package publicrepo holds no code. Its test keeps the identifiers of the one
// real installation out of a repository anybody can read.
//
// The repository went public on 2026-09-03 after a commit took the host's LAN
// address, its name and its public name out of the tree. Two weeks later they
// were all back -- in tests, in the ledger, in CLAUDE.md -- because a session
// working against the real cluster wrote down what it saw, and nothing stopped
// it. On 2026-09-26 they came out again, from the history too, and this test
// is what keeps them out.
//
// The forbidden values are listed as SHA-256 hashes, never as text: a list of
// them in plain text would itself be the leak, and a failure message here is
// printed into a public CI log, so it names the file, the line and the hash
// and never the value. The hashes of short values (an IPv4 address) can be
// reversed by trying them all; what the hash buys is that the value is not in
// the tree for a search engine or a casual reader, not that it is secret.
//
// To add one: sha256 of the value in lower case, e.g.
//
//	printf '%s' 'host.example.org' | sha256sum
package publicrepo

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// forbidden are the hashes of the installation's host names, public name,
// addresses, mail addresses, one MAC and the home directories that turned up
// in paths.
var forbidden = map[string]bool{
	"6f00653d74f8b04d975a502c2fd194bb89c20475e141fd24fc214a8f98b56022": true,
	"c45b6f8506266b58afd6b6a2421b82292d5afd5a17f0582af51dc5a258491e16": true,
	"5c3f49ad4047e860f36902c367e7369adede7f9624638150fd53090985b9cb75": true,
	"832d8e61d63506301b5c9866bf0c13ada114311dac8a429eb81dd6dcc1c16112": true,
	"f914f9591cc76287eba6d693580e6c9db0367f38861003ff3aac325258d7a3e8": true,
	"e4176a883e1489aebe884251d01d2059e4f3e2b0eea17aa67e81b321575cce4c": true,
	"1592c3952e233b3a08f6d0e45954cf44248b0e35feb984622f1128a5785c393e": true,
	"1f06968bec4ba6f122172d9ae1ecd0adade2a34b9b7042cc6fffd186a5e9df7c": true,
	"ac5651b2d6b17937d2fe506f7fe8de83ae6919aa9b356bf7da6f8f3c456a6f11": true,
	"94a291e5e946398d0cb4a0bb3a6e3376c4b095f97022f30a906237923cca059f": true,
	"f4b498c3e2341152c7f1cce9994028c1d8460473b3ad8bd6623138342edeb04a": true,
	"dfe0bb9af542f87fb2c6ff9097d65da309ec319f9e95edb1cff26ebf08b27fc5": true,
	"d2ea9f9397b6d90809580075e62c220c2c763d34875748fe9f717899eab5979a": true,
	// The host names' common prefix on its own: a comment quoting how a name
	// broke across lines carries the prefix and never the whole name.
	"584c01d0704b9d8911e5ebf08cdb20114b4d02781f5799b947882ea0f6004d39": true,
}

// The shapes a value is cut out of a line in. A dotted name is also tried
// with each leading label removed, so a subdomain of a forbidden name is
// caught too.
var (
	mailShape   = regexp.MustCompile(`[a-z0-9._%+-]+@[a-z0-9-]+(?:\.[a-z0-9-]+)+`)
	dottedShape = regexp.MustCompile(`[a-z0-9-]+(?:\.[a-z0-9-]+)+`)
	wordShape   = regexp.MustCompile(`[a-z0-9]+(?:-[a-z0-9]+)*`)
	macShape    = regexp.MustCompile(`[0-9a-f]{2}(?::[0-9a-f]{2}){5}`)
	homeShape   = regexp.MustCompile(`/(?:users|home)/[a-z0-9_-]+`)
)

// candidates is every value on a line that could be one of the forbidden ones.
// Backslashes go first: a test that matches a name with a regular expression
// writes its dots as \. and would otherwise carry the name past every shape.
func candidates(line string) []string {
	line = strings.ToLower(strings.ReplaceAll(line, `\`, ""))
	var out []string
	for _, re := range []*regexp.Regexp{mailShape, wordShape, macShape, homeShape} {
		out = append(out, re.FindAllString(line, -1)...)
	}
	for _, name := range dottedShape.FindAllString(line, -1) {
		for {
			out = append(out, name)
			dot := strings.IndexByte(name, '.')
			if dot < 0 {
				break
			}
			name = name[dot+1:]
		}
	}
	return out
}

func hashOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestTheShapesFindWhatTheyAreFor(t *testing.T) {
	line := `see https://a.manager.example.org:8443, mail me@example.org, 02:00:00:00:00:01 at 10.0.0.30/24 on srv-a-01 in /Users/op/x, /as you@example\.net/, /10\.0\.0\.31/`
	want := []string{"manager.example.org", "me@example.org", "02:00:00:00:00:01", "10.0.0.30", "srv-a-01", "/users/op", "you@example.net", "10.0.0.31"}
	got := map[string]bool{}
	for _, c := range candidates(line) {
		got[c] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("candidates(%q) does not include %q", line, w)
		}
	}
}

func TestNoIdentifierOfTheRealInstallationIsInTheTree(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	listed, err := exec.Command("git", "-C", root, "ls-files", "-z").Output()
	if err != nil {
		t.Fatalf("git ls-files: %v", err)
	}
	checked := 0
	for _, name := range strings.Split(string(listed), "\x00") {
		if name == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			// Listed by git and gone from the disk: deleted in the working
			// tree and not yet committed. Nothing there to leak.
			continue
		}
		if bytes.IndexByte(data, 0) >= 0 {
			continue
		}
		checked++
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		for n := 1; scanner.Scan(); n++ {
			for _, c := range candidates(scanner.Text()) {
				if h := hashOf(c); forbidden[h] {
					t.Errorf("%s:%d carries an identifier of the real installation (sha256 %s); "+
						"replace it with a documentation value such as 192.168.1.10, "+
						"homeserver or example.com", name, n, h[:12])
				}
			}
		}
		if err := scanner.Err(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if checked < 100 {
		t.Fatalf("checked only %d files; the walk is not seeing the repository", checked)
	}
}
