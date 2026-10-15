package configdrift_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// knownSenders is every place outside internal/talos that sends a machine
// configuration to a node, with how many times it does, and why it is allowed.
//
// The drift watch hears an apply because the client carries an observer
// (talos.ClusterClient.SetApplyObserver) and inventory.Connect hangs it on every
// client it gives out (D-10). That holds for a caller only as long as the
// caller gets its client from Connect. A new place that sends a configuration
// must be a conscious addition here, with its reason, so that whoever adds it
// has been told to ask where its client comes from; the alternative is a
// configuration the watch never hears of and calls drift at the next reading.
//
// The count is per file: a second call in a file that is known is a new call
// site and fails just the same.
var knownSenders = map[string]struct {
	calls  int
	reason string
}{
	filepath.Join("internal", "machineconfig", "service.go"): {1,
		"the configuration service, the route config.apply: its client comes from the Connect function the " +
			"daemon wires to inventory.Connect"},
	filepath.Join("internal", "upgrade", "job.go"): {1,
		"the Kubernetes upgrade job (applyImage): its client comes from Deps.Connect, inventory.Connect in the daemon"},
	filepath.Join("internal", "rotateca", "job.go"): {1,
		"the CA rotation job (rewrite): its client comes from Deps.Connect, inventory.Connect in the daemon"},
	filepath.Join("internal", "provision", "job.go"): {1,
		"provisioning writes through a MaintenanceClient into a node that has no configuration and so no baseline; " +
			"the job makes the watch forget the machine's old baseline right before (Deps.ForgetConfig)"},
}

// sendingMethods are the two methods of talos.ClusterClient that send a
// configuration, and the one of MaintenanceClient.
var sendingMethods = map[string]bool{
	"ApplyConfigurationWithMode": true,
	"ApplyConfiguration":         true,
}

// TestOnlyKnownCallersSendAConfigurationToAManagedNode walks the production
// sources under internal and cmd, outside internal/talos and the simulator, and finds every
// selector that names a sending method, called or taken as a value. The set is
// the one above, with the counts above, and nothing else.
func TestOnlyKnownCallersSendAConfigurationToAManagedNode(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")
	// internal/talos is where the client is and the observer is called from;
	// internal/talossim is the simulated node, the RECEIVER of an apply: its
	// ApplyConfiguration is the server side of the RPC and a protobuf message
	// type of the same name, and it reaches no managed node.
	exempt := []string{filepath.Join("internal", "talos"), filepath.Join("internal", "talossim")}

	found := map[string]int{}
	scanned := 0
	for _, top := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			for _, e := range exempt {
				if rel == e || strings.HasPrefix(rel, e+string(filepath.Separator)) {
					return nil
				}
			}

			scanned++
			file, err := parser.ParseFile(token.NewFileSet(), p, nil, parser.SkipObjectResolution)
			if err != nil {
				t.Errorf("parse %s: %v", rel, err)
				return nil
			}
			ast.Inspect(file, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok && sendingMethods[sel.Sel.Name] {
					found[rel]++
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", top, err)
		}
	}

	// A guard that scans nothing passes for the wrong reason.
	if scanned < 15 {
		t.Fatalf("scanned only %d source files; the guard is not reaching the tree (root %q)", scanned, root)
	}
	t.Logf("scanned %d production source files outside %v", scanned, exempt)

	var problems []string
	files := make([]string, 0, len(found))
	for f := range found {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		known, ok := knownSenders[f]
		switch {
		case !ok:
			problems = append(problems, f+": sends a configuration to a node ("+strconv.Itoa(found[f])+" place(s)) and is not a known caller")
		case found[f] != known.calls:
			problems = append(problems, f+": "+strconv.Itoa(found[f])+" place(s) send a configuration, the known number is "+strconv.Itoa(known.calls))
		}
	}
	for f, known := range knownSenders {
		if found[f] == 0 {
			problems = append(problems, f+": is listed as a caller ("+known.reason+") and sends nothing any more; take it off the list")
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf("the callers that send a configuration to a node are not the known ones:\n  %s\n\n"+
			"A new caller must get its client from inventory.Connect, which hangs the observer of the "+
			"configuration drift watch on it (D-10), and must be entered in knownSenders with its reason. "+
			"Otherwise what it sends is a configuration the watch never hears of, and the next reading calls it drift.",
			strings.Join(problems, "\n  "))
	}
}
