package planning_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/talos"
)

// The confirmed deadline-class policy, held against the code that implements it.
//
// 02-CONTEXT.md says of that section: "This section is closed. A later plan that
// wants to change a deadline value or a class membership is changing a confirmed
// decision, and must amend [three files] in the same commit." It was changed
// twice without the table moving.
//
//   - EtcdRecover left the mutation class for a new upload class. Recorded as
//     ledger entry 98, which named the un-amended table as its own open half, so
//     this one was at least written down.
//   - A watch class was added for COSIWatch, with its reasoning in the phase
//     summary that added it and nowhere else. No ledger entry, no amendment. It
//     was found only because closing entry 98 meant reading the table beside the
//     code.
//
// A rule that is only a sentence in a document is a rule that holds until
// somebody is busy. This is that sentence as a test.
const deadlinePolicyPath = "../../.planning/phases/02-transport-seam-talossim-image-factory/02-CONTEXT.md"

// classRow matches the bolded class name that opens each row of the policy
// table: "| **Fast read** | 10 s | ...". The name is taken from the document
// rather than searched for, so a row nobody expected is still read.
var classRow = regexp.MustCompile(`(?m)^\| \*\*([A-Za-z ]+)\*\*`)

// knownClasses walks the DeadlineClass enum rather than listing its members, so
// a class added to the code and to nothing else is still seen here. The walk
// ends where String() stops recognising the value and falls back to its
// "DeadlineClass(7)" spelling.
func knownClasses(t *testing.T) []talos.DeadlineClass {
	t.Helper()

	var classes []talos.DeadlineClass
	for i := 1; i < 64; i++ {
		c := talos.DeadlineClass(i)
		if strings.Contains(c.String(), "DeadlineClass(") {
			break
		}
		classes = append(classes, c)
	}
	if len(classes) == 0 {
		t.Fatal("no deadline classes found; this guard is reading the wrong thing")
	}
	return classes
}

// TestEveryDeadlineClassHasARowInTheConfirmedPolicy.
//
// Be exact about what this does and does not check, because the useful version
// of a guard is the one whose limits are written down.
//
// It checks that every class the code defines appears in the confirmed table and
// that every row in that table is a class the code still defines. The first
// direction is precisely the drift that happened twice.
//
// It does NOT check per-method membership. The table spells its members as prose
// -- "ServiceStart/Stop/Restart", "EtcdDowngrade*", "COSI Get/List" -- which no
// exact comparison can read, and an approximate comparison here would be worse
// than none: it would report an agreement it had not established. Closing that
// gap means rewriting the table into a machine-readable list first, and that is
// an amendment to a confirmed decision rather than a test change.
//
// One class is deliberately absent from the method table and present here:
// ClassProbe. Version is a fast read at ten seconds and the D-05 liveness check
// at five, and it is the same RPC either way, so the caller that means the
// liveness check says so on the context rather than the table carrying the
// method twice. conn.classOf says this too. A guard keyed on the method table
// would therefore have reported the probe class as abandoned, which is how the
// first version of this test failed -- correctly, against the wrong question.
func TestEveryDeadlineClassHasARowInTheConfirmedPolicy(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(deadlinePolicyPath)
	if err != nil {
		t.Fatalf("read the confirmed policy: %v", err)
	}

	documented := map[string]bool{}
	for _, m := range classRow.FindAllStringSubmatch(string(raw), -1) {
		// The document writes "Fast read" and DeadlineClass.String() writes
		// "fast read". Compared with spaces removed and case folded, so the
		// document stays prose and the code stays readable.
		documented[key(m[1])] = true
	}
	if len(documented) == 0 {
		t.Fatal("no class rows found in the policy table; this guard is reading the wrong thing")
	}

	defined := map[string]bool{}
	for _, class := range knownClasses(t) {
		defined[key(class.String())] = true
		if !documented[key(class.String())] {
			t.Errorf("the code defines the %s class and the confirmed policy has no row for it; "+
				"amending that table in the same commit is what the section requires", class)
		}
	}

	for name := range documented {
		if !defined[name] {
			t.Errorf("the confirmed policy has a row for a %q class the code does not define; "+
				"a class only the document believes in is a decision the code walked away from "+
				"without saying so", name)
		}
	}
}

// TestEveryClassAMethodIsAssignedIsAClassTheCodeDefines is the small sanity half:
// the method table cannot hand out a class that is not one of the enum's own.
func TestEveryClassAMethodIsAssignedIsAClassTheCodeDefines(t *testing.T) {
	t.Parallel()

	defined := map[string]bool{}
	for _, class := range knownClasses(t) {
		defined[key(class.String())] = true
	}
	for method, class := range talos.DeadlineClasses() {
		if !defined[key(class.String())] {
			t.Errorf("%s is assigned %v, which is not a class this package defines", method, class)
		}
	}
}

// TestEveryClassNamesItsDeadlineTheSameWayTheCodeDoes keeps the two unbounded
// answers honest.
//
// "No total deadline" is the claim a reader takes away from the table, and it is
// the one that would be most expensive to get wrong in either direction: a class
// documented as unbounded that carries a total deadline kills long work that is
// succeeding, and one documented as bounded that does not can hang forever.
func TestEveryClassNamesItsDeadlineTheSameWayTheCodeDoes(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(deadlinePolicyPath)
	if err != nil {
		t.Fatalf("read the confirmed policy: %v", err)
	}
	lines := strings.Split(string(raw), "\n")

	for _, class := range knownClasses(t) {
		name := class.String()
		row := rowFor(lines, name)
		if row == "" {
			t.Errorf("no row for class %s", name)
			continue
		}
		saysUnbounded := strings.Contains(row, "no total deadline")
		if saysUnbounded != class.Unbounded() {
			t.Errorf("class %s: the policy says unbounded=%t, the code says %t",
				name, saysUnbounded, class.Unbounded())
		}
	}
}

// rowFor finds the table row whose bolded class name matches, with spaces
// removed and case folded so "Fast read" finds ClassFastRead.
func rowFor(lines []string, name string) string {
	for _, line := range lines {
		m := classRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if key(m[1]) == key(name) {
			return line
		}
	}
	return ""
}

// key folds a class name to the one spelling both sides can be compared in.
func key(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, " ", ""))
}
