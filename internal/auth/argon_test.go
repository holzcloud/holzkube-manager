package auth

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/argon2id"
)

// weakParams are deliberately far below anything holzkube would ship. They
// stand in for "a hash written by an older, cheaper release" and they keep the
// tests that are not about cost from paying a quarter second per verification.
var weakParams = &argon2id.Params{
	Memory:      8 * 1024,
	Iterations:  1,
	Parallelism: 1,
	SaltLength:  16,
	KeyLength:   32,
}

// useTestParams swaps the process-wide active parameters for cheap ones and
// restores whatever was there before.
//
// It lives in a test file on purpose. Production code has no way to lower the
// cost of a hash: the only writer of the active parameters is the calibration,
// and the calibration cannot return anything below DefaultParams.
func useTestParams(t *testing.T) {
	t.Helper()
	prev := activeParams.Load()
	activeParams.Store(weakParams)
	t.Cleanup(func() { activeParams.Store(prev) })
}

// TestArgonHashCarriesItsParameters is FOUND-02 in one assertion: the encoded
// hash is self-describing, so a later verification needs no external record of
// what produced it.
func TestArgonHashCarriesItsParameters(t *testing.T) {
	useTestParams(t)

	h, err := Hash("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	if !strings.HasPrefix(h, "$argon2id$") {
		t.Fatalf("hash does not identify its algorithm: %q", h)
	}

	got, _, _, err := argon2id.DecodeHash(h)
	if err != nil {
		t.Fatalf("DecodeHash: %v", err)
	}
	want := ActiveParams()
	if got.Memory != want.Memory || got.Iterations != want.Iterations ||
		got.Parallelism != want.Parallelism || got.KeyLength != want.KeyLength {
		t.Fatalf("encoded parameters = m=%d t=%d p=%d k=%d, want m=%d t=%d p=%d k=%d",
			got.Memory, got.Iterations, got.Parallelism, got.KeyLength,
			want.Memory, want.Iterations, want.Parallelism, want.KeyLength)
	}
}

// TestArgonVerifyCostsAtLeastTheTarget measures the thing FOUND-04 actually
// asks for. A claimed cost is not a cost.
func TestArgonVerifyCostsAtLeastTheTarget(t *testing.T) {
	p, measured := CalibrateParams(CalibrationTarget)
	t.Logf("calibrated to m=%d t=%d p=%d, measured %v", p.Memory, p.Iterations, p.Parallelism, measured)

	h, err := argon2id.CreateHash("correct-horse-battery-staple", p)
	if err != nil {
		t.Fatalf("CreateHash: %v", err)
	}

	// Take the fastest of three runs: a single sample can be inflated by a
	// noisy host, and inflating the number would be the one way to pass this
	// test without the property holding.
	fastest := time.Duration(1<<63 - 1)
	for range 3 {
		start := time.Now()
		ok, err := Verify("correct-horse-battery-staple", h)
		elapsed := time.Since(start)
		if err != nil || !ok {
			t.Fatalf("Verify: ok=%v err=%v", ok, err)
		}
		if elapsed < fastest {
			fastest = elapsed
		}
	}

	if fastest < CalibrationTarget {
		t.Fatalf("fastest verification took %v, want at least %v", fastest, CalibrationTarget)
	}
}

// TestArgonCalibrationNeverGoesBelowTheFloor pins the half of calibration that
// matters on a slow host: it may raise the cost, never lower it.
func TestArgonCalibrationNeverGoesBelowTheFloor(t *testing.T) {
	p, _ := CalibrateParams(time.Nanosecond)

	if p.Memory != DefaultParams.Memory {
		t.Errorf("memory = %d, want the floor %d", p.Memory, DefaultParams.Memory)
	}
	if p.Iterations < DefaultParams.Iterations {
		t.Errorf("iterations = %d, below the floor %d", p.Iterations, DefaultParams.Iterations)
	}
	if p.Parallelism != DefaultParams.Parallelism {
		t.Errorf("parallelism = %d, want the floor %d", p.Parallelism, DefaultParams.Parallelism)
	}
}

// TestArgonVerifiesAHashMadeWithOlderParameters is the property that makes
// raising the cost safe: existing passwords keep working.
func TestArgonVerifiesAHashMadeWithOlderParameters(t *testing.T) {
	old, err := argon2id.CreateHash("correct-horse-battery-staple", weakParams)
	if err != nil {
		t.Fatalf("CreateHash: %v", err)
	}

	ok, err := Verify("correct-horse-battery-staple", old)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !ok {
		t.Fatal("a hash written with lower parameters no longer verifies")
	}

	ok, err = Verify("not-the-password", old)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if ok {
		t.Fatal("a wrong password verified against an old hash")
	}
}

// TestArgonNeedsRehashDetectsRaisedParameters proves the rehash decision is
// made from the stored hash rather than guessed.
func TestArgonNeedsRehashDetectsRaisedParameters(t *testing.T) {
	strong := &argon2id.Params{Memory: 64 * 1024, Iterations: 9, Parallelism: 4, SaltLength: 16, KeyLength: 32}
	prev := activeParams.Load()
	activeParams.Store(strong)
	t.Cleanup(func() { activeParams.Store(prev) })

	old, err := argon2id.CreateHash("correct-horse-battery-staple", weakParams)
	if err != nil {
		t.Fatalf("CreateHash: %v", err)
	}
	if !NeedsRehash(old) {
		t.Error("NeedsRehash = false for a hash below the active parameters")
	}

	current, err := argon2id.CreateHash("correct-horse-battery-staple", strong)
	if err != nil {
		t.Fatalf("CreateHash: %v", err)
	}
	if NeedsRehash(current) {
		t.Error("NeedsRehash = true for a hash written with the active parameters")
	}
}

// TestLoginRehashesAnOutdatedHash is what turns "the parameters can be raised"
// from a claim into a mechanism: the upgrade happens on the one occasion the
// cleartext password is available.
func TestLoginRehashesAnOutdatedHash(t *testing.T) {
	svc, st := newTestService(t, 24*time.Hour)
	useTestParams(t)

	older := &argon2id.Params{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 16}
	stored, err := argon2id.CreateHash(testPassword, older)
	if err != nil {
		t.Fatalf("CreateHash: %v", err)
	}
	seedUser(t, st, stored)

	runInSession(t, svc, func(ctx context.Context) {
		if _, err := svc.Login(ctx, testUsername, testPassword); err != nil {
			t.Fatalf("Login: %v", err)
		}
	})

	users, err := st.Users().List(context.Background())
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("got %d users, want 1", len(users))
	}
	if users[0].PasswordHash == stored {
		t.Fatal("the stored hash was not upgraded after a login with weaker parameters")
	}
	got, _, _, err := argon2id.DecodeHash(users[0].PasswordHash)
	if err != nil {
		t.Fatalf("DecodeHash: %v", err)
	}
	if got.KeyLength != ActiveParams().KeyLength || got.Memory != ActiveParams().Memory {
		t.Fatalf("upgraded hash uses m=%d k=%d, want m=%d k=%d",
			got.Memory, got.KeyLength, ActiveParams().Memory, ActiveParams().KeyLength)
	}

	// The upgraded record must still accept the same password.
	ok, err := Verify(testPassword, users[0].PasswordHash)
	if err != nil || !ok {
		t.Fatalf("upgraded hash rejects the unchanged password: ok=%v err=%v", ok, err)
	}
}
