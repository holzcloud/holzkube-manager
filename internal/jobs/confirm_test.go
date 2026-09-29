package jobs_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/holzcloud/holzkube-manager/internal/jobs"
)

// The property the confirmation token exists for is not "a token was
// presented". It is that a confirmation for one action cannot authorise
// another — because the thing an operator read in the dialog and the thing the
// server does have to be the same action.

func newConfirmer(t *testing.T) *jobs.Confirmer {
	t.Helper()

	c, err := jobs.NewConfirmer()
	if err != nil {
		t.Fatalf("NewConfirmer: %v", err)
	}
	return c
}

func TestAConfirmationMatchesExactlyWhatWasConfirmed(t *testing.T) {
	t.Parallel()

	c := newConfirmer(t)
	intent := jobs.Intent{
		Action:  "node.reset",
		Machine: "m1",
		Params:  map[string]string{"wipe_mode": "user-disks", "graceful": "true", "reboot": "true"},
	}

	token, _ := c.Issue(intent)
	if err := c.Check(token, intent); err != nil {
		t.Fatalf("the confirmation does not verify against its own intent: %v", err)
	}
}

// TestAConfirmationCannotBeWidened is the one that matters.
//
// An operator reads "wipe the user disks" and confirms. If that token also
// authorised "wipe everything", the dialog would be decoration.
func TestAConfirmationCannotBeWidened(t *testing.T) {
	t.Parallel()

	c := newConfirmer(t)
	confirmed := jobs.Intent{
		Action:  "node.reset",
		Machine: "m1",
		Params:  map[string]string{"wipe_mode": "user-disks", "graceful": "true", "reboot": "true"},
	}
	token, _ := c.Issue(confirmed)

	widened := []struct {
		name   string
		intent jobs.Intent
	}{
		{
			name: "a bigger wipe scope",
			intent: jobs.Intent{Action: "node.reset", Machine: "m1", Params: map[string]string{
				"wipe_mode": "all", "graceful": "true", "reboot": "true",
			}},
		},
		{
			name: "ungraceful, which risks etcd quorum",
			intent: jobs.Intent{Action: "node.reset", Machine: "m1", Params: map[string]string{
				"wipe_mode": "user-disks", "graceful": "false", "reboot": "true",
			}},
		},
		{
			name: "halting instead of rebooting",
			intent: jobs.Intent{Action: "node.reset", Machine: "m1", Params: map[string]string{
				"wipe_mode": "user-disks", "graceful": "true", "reboot": "false",
			}},
		},
		{
			name: "a different machine",
			intent: jobs.Intent{Action: "node.reset", Machine: "m2", Params: map[string]string{
				"wipe_mode": "user-disks", "graceful": "true", "reboot": "true",
			}},
		},
		{
			name: "a different action entirely",
			intent: jobs.Intent{Action: "node.shutdown", Machine: "m1", Params: map[string]string{
				"wipe_mode": "user-disks", "graceful": "true", "reboot": "true",
			}},
		},
		{
			name: "a parameter dropped",
			intent: jobs.Intent{Action: "node.reset", Machine: "m1", Params: map[string]string{
				"wipe_mode": "user-disks", "graceful": "true",
			}},
		},
	}

	for _, row := range widened {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			if err := c.Check(token, row.intent); !errors.Is(err, jobs.ErrConfirmationInvalid) {
				t.Fatalf("a confirmation for one action authorised %s (err = %v)", row.name, err)
			}
		})
	}
}

// TestAForgedTokenIsRefused covers the other direction: a token this instance
// never issued.
func TestAForgedTokenIsRefused(t *testing.T) {
	t.Parallel()

	c := newConfirmer(t)
	intent := jobs.Intent{Action: "node.reboot", Machine: "m1"}

	for _, token := range []string{
		"",
		"not-a-token",
		"9999999999.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	} {
		if err := c.Check(token, intent); !errors.Is(err, jobs.ErrConfirmationInvalid) {
			t.Errorf("Check(%q) = %v, want ErrConfirmationInvalid", token, err)
		}
	}

	// Another instance's key must not verify here. A confirmation is a
	// statement made by *this* process to *this* operator.
	other := newConfirmer(t)
	token, _ := other.Issue(intent)
	if err := c.Check(token, intent); !errors.Is(err, jobs.ErrConfirmationInvalid) {
		t.Errorf("a token from another instance verified: %v", err)
	}
}

// TestParameterEncodingCannotBeRearranged pins the quoting in the canonical
// form. Without it, two different parameter sets could encode identically and
// one confirmation would cover both.
func TestParameterEncodingCannotBeRearranged(t *testing.T) {
	t.Parallel()

	c := newConfirmer(t)

	a := jobs.Intent{Action: "node.reset", Machine: "m1", Params: map[string]string{
		"a": "x|b=y",
	}}
	b := jobs.Intent{Action: "node.reset", Machine: "m1", Params: map[string]string{
		"a": "x", "b": "y",
	}}

	token, _ := c.Issue(a)
	if err := c.Check(token, b); !errors.Is(err, jobs.ErrConfirmationInvalid) {
		t.Fatal("two different parameter sets encoded to the same confirmation")
	}
}

// TestResetOptionsHaveNoSafeDefault is JOB-07's other half, at the level where
// it is enforced.
//
// talosctl reset defaults to --wipe-mode=all --reboot=false: the most
// destructive scope, and the machine left powered off. A Go zero value would
// reproduce exactly that for a caller who omitted a field, so nothing here has
// a zero value that means anything.
func TestResetOptionsHaveNoSafeDefault(t *testing.T) {
	t.Parallel()

	rows := []struct {
		name   string
		params map[string]string
	}{
		{name: "no wipe mode", params: map[string]string{"graceful": "true", "reboot": "true"}},
		{name: "no graceful flag", params: map[string]string{"wipe_mode": "all", "reboot": "true"}},
		{name: "no reboot flag", params: map[string]string{"wipe_mode": "all", "graceful": "true"}},
		{name: "an invented wipe mode", params: map[string]string{
			"wipe_mode": "everything-please", "graceful": "true", "reboot": "true",
		}},
		{name: "user-disks naming no disks", params: map[string]string{
			"wipe_mode": "user-disks", "graceful": "true", "reboot": "true",
		}},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			if _, err := jobs.ResetOptionsFromParams(row.params); err == nil {
				t.Fatal("accepted; every omission here has a dangerous false value")
			}
		})
	}

	ok, err := jobs.ResetOptionsFromParams(map[string]string{
		"wipe_mode": "user-disks", "graceful": "true", "reboot": "true", "user_disks": "sdb,sdc",
	})
	if err != nil {
		t.Fatalf("a complete parameter set was refused: %v", err)
	}
	if ok.Mode != "user-disks" || !ok.Graceful || !ok.Reboot || len(ok.UserDisksToWipe) != 2 {
		t.Fatalf("parsed options are %+v", ok)
	}
}

// TestCheckOnceOpensOneRequest is WR-05: a host token stands for one typed
// hostname and must open one order, not every order for ten minutes.
//
// Fault injected and seen red: CheckOnce not recording the spent token.
func TestCheckOnceOpensOneRequest(t *testing.T) {
	t.Parallel()

	c := newConfirmer(t)
	intent := jobs.Intent{Action: "host.reboot", Machine: "@host", Params: map[string]string{"session": "s1"}}
	token, _ := c.IssueOnce(intent)

	// A refusal spends nothing: the wrong intent first, then the right one.
	other := jobs.Intent{Action: "host.reboot", Machine: "@host", Params: map[string]string{"session": "s2"}}
	if err := c.CheckOnce(token, other); !errors.Is(err, jobs.ErrConfirmationInvalid) {
		t.Fatalf("CheckOnce for another session = %v, want ErrConfirmationInvalid", err)
	}
	if err := c.CheckOnce(token, intent); err != nil {
		t.Fatalf("first CheckOnce = %v, want nil", err)
	}
	err := c.CheckOnce(token, intent)
	if !errors.Is(err, jobs.ErrConfirmationUsed) {
		t.Fatalf("second CheckOnce = %v, want ErrConfirmationUsed", err)
	}
	if !errors.Is(err, jobs.ErrConfirmationInvalid) {
		t.Errorf("ErrConfirmationUsed does not wrap ErrConfirmationInvalid: the routes would answer 500 rather than 403")
	}

	// Two confirmations of one intent in one second are two tokens: each
	// opens its own request. (Issue's tokens would be identical.)
	a, _ := c.IssueOnce(intent)
	b, _ := c.IssueOnce(intent)
	if a == b {
		t.Fatalf("two single-use tokens for one intent are identical: %q", a)
	}
	if err := c.CheckOnce(a, intent); err != nil {
		t.Errorf("CheckOnce(a) = %v, want nil", err)
	}
	if err := c.CheckOnce(b, intent); err != nil {
		t.Errorf("CheckOnce(b) after a was spent = %v, want nil", err)
	}

	// The two kinds do not cross: a single-use token never passes Check, and
	// a token from Issue never passes CheckOnce.
	if err := c.Check(token, intent); !errors.Is(err, jobs.ErrConfirmationInvalid) {
		t.Errorf("Check of a single-use token = %v, want ErrConfirmationInvalid", err)
	}
	plain, _ := c.Issue(intent)
	if err := c.CheckOnce(plain, intent); !errors.Is(err, jobs.ErrConfirmationInvalid) {
		t.Errorf("CheckOnce of a token from Issue = %v, want ErrConfirmationInvalid", err)
	}

	// Issue and Check are untouched: the node routes keep their tokens good
	// until they expire.
	if err := c.Check(plain, intent); err != nil {
		t.Errorf("first Check = %v", err)
	}
	if err := c.Check(plain, intent); err != nil {
		t.Errorf("second Check = %v, want nil: Check must stay stateless", err)
	}
}

// TestCheckOnceUnderRace: many requests with one token, one success.
func TestCheckOnceUnderRace(t *testing.T) {
	t.Parallel()

	c := newConfirmer(t)
	intent := jobs.Intent{Action: "host.update", Machine: "@host"}
	token, _ := c.IssueOnce(intent)

	const n = 16
	var wg sync.WaitGroup
	results := make(chan error, n)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- c.CheckOnce(token, intent)
		}()
	}
	wg.Wait()
	close(results)
	ok := 0
	for err := range results {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, jobs.ErrConfirmationUsed):
		default:
			t.Errorf("CheckOnce: %v", err)
		}
	}
	if ok != 1 {
		t.Errorf("%d of %d racing requests got through with one token, want exactly 1", ok, n)
	}
}
