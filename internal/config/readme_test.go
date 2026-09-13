package config

// The README's option table is a claim about this package, so it is checked
// like one.
//
// Three options -- dry-run, allow-prerelease and image-factory -- had been
// added to the option table without a row in README.md, which is how an
// operator comes to believe a capability does not exist. The help output cannot
// drift because it is generated; the README can, because it is written.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// documentedElsewhere names the options the README covers in prose rather than
// in the table, with the reason.
var documentedElsewhere = map[string]string{}

var readmeFlagRow = regexp.MustCompile("(?m)^\\| `--([a-z0-9-]+)` \\| `([A-Z0-9_]+)` \\|")

func TestTheReadmeDocumentsEveryOption(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("read the README: %v", err)
	}

	rows := map[string]string{}
	for _, m := range readmeFlagRow.FindAllStringSubmatch(string(raw), -1) {
		rows[m[1]] = m[2]
	}
	if len(rows) == 0 {
		t.Fatal("no option rows were found in the README, so this test proves nothing")
	}

	for _, o := range optionTable("") {
		if _, prose := documentedElsewhere[o.name]; prose {
			continue
		}
		env, listed := rows[o.name]
		if !listed {
			t.Errorf("--%s is an option and has no row in the README's table; an operator "+
				"reading the README cannot know it exists", o.name)
			continue
		}
		if want := EnvPrefix + o.env; env != want {
			t.Errorf("--%s is documented with environment variable %s, want %s", o.name, env, want)
		}
	}

	table := map[string]bool{}
	for _, o := range optionTable("") {
		table[o.name] = true
	}
	for name := range rows {
		if !table[name] {
			t.Errorf("the README documents --%s, which is not an option; either it was renamed "+
				"or it was removed and its row stayed", name)
		}
	}

	for name, reason := range documentedElsewhere {
		if !table[name] {
			t.Errorf("documentedElsewhere names %q, which is not an option", name)
		}
		if strings.TrimSpace(reason) == "" {
			t.Errorf("%q is exempted from the table with no reason", name)
		}
	}
}
