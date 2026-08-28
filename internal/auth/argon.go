package auth

import (
	"errors"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alexedwards/argon2id"
)

// CalibrationTarget is the minimum wall-clock cost of a single password
// verification (FOUND-04, PITFALLS.md line 638).
//
// The number is the defence. A password hash that verifies in a millisecond
// turns an online guessing attack into something close to an offline one; a
// quarter second makes the same attack take longer than the attacker has, and
// costs the one operator a quarter second once a day.
const CalibrationTarget = 250 * time.Millisecond

// DefaultParams is the floor, not the target (ARCHITECTURE § Sessions and
// transport). Calibration may raise the iteration count above these values on
// a fast host; it may never go below them on a slow one, because "the host was
// slow" is not a reason to ship a weaker hash.
var DefaultParams = &argon2id.Params{
	Memory:      64 * 1024, // KiB
	Iterations:  3,
	Parallelism: 4,
	SaltLength:  16,
	KeyLength:   32,
}

const (
	// maxCalibrationIterations caps the search. Reaching it means the host is
	// slower than the target assumes; the result is logged as a warning rather
	// than silently accepted, because an unmet cost target that nobody
	// mentions is indistinguishable from a met one.
	maxCalibrationIterations = 512

	// maxCalibrationRounds bounds the measurements. The ratio step converges in
	// one or two; the rest is insurance against a pathological host.
	maxCalibrationRounds = 6

	// calibrationMargin overshoots the target deliberately. Hitting it exactly
	// would leave every later verification one scheduling hiccup below the
	// requirement, and being 20% over the security target costs nothing.
	calibrationMargin = 1.2
)

// activeParams holds the parameters new hashes are written with. It is nil
// until the first call to ActiveParams, which calibrates.
//
// Nothing in this package can lower it: CalibrateParams starts from
// DefaultParams and only ever increases the iteration count.
var (
	activeParams   atomic.Pointer[argon2id.Params]
	calibrationMu  sync.Mutex
	errEmptySecret = errors.New("auth: empty password")
)

// CalibrateParams finds parameters whose verification cost reaches target on
// this host, and reports the cost it actually measured.
//
// Only the iteration count moves. Memory is what makes argon2id expensive to
// attack with custom hardware, so lowering it to buy iterations would trade the
// property worth having for the one that is easy to measure.
func CalibrateParams(target time.Duration) (*argon2id.Params, time.Duration) {
	p := *DefaultParams
	measured := measureHash(&p)

	if target <= 0 {
		return &p, measured
	}

	for range maxCalibrationRounds {
		if measured >= target {
			return &p, measured
		}
		if p.Iterations >= maxCalibrationIterations {
			return &p, measured
		}

		scaled := float64(p.Iterations) * calibrationMargin * float64(target) / float64(measured)
		next := uint32(math.Ceil(scaled))
		if next <= p.Iterations {
			next = p.Iterations + 1
		}
		if next > maxCalibrationIterations {
			next = maxCalibrationIterations
		}
		p.Iterations = next
		measured = measureHash(&p)
	}
	return &p, measured
}

// measureHash times hash derivation with the given parameters. Deriving and
// verifying do the same work, so this is the verification cost.
//
// It reports the fastest of several runs rather than one sample. A cold cache
// or a busy core inflates the first measurement, and an inflated measurement
// makes calibration stop early -- shipping parameters that look expensive
// during calibration and are cheap for the rest of the process's life. Erring
// towards the fastest observed run errs towards more iterations, which is the
// direction a mistake here should go.
func measureHash(p *argon2id.Params) time.Duration {
	const samples = 3

	fastest := time.Duration(math.MaxInt64)
	for range samples {
		start := time.Now()
		if _, err := argon2id.CreateHash("holzkube-calibration-probe", p); err != nil {
			// Unusable parameters must not read as "instant": reporting zero
			// would make the caller raise the cost, which is the safe
			// direction to fail in.
			return 0
		}
		if elapsed := time.Since(start); elapsed < fastest {
			fastest = elapsed
		}
	}
	return fastest
}

// ActiveParams returns the calibrated parameters, calibrating on first use.
//
// The result is logged once, because the operator has no other way to find out
// what their hardware bought them -- and, on a host too slow to reach the
// target, no other way to find out that it did not.
func ActiveParams() *argon2id.Params {
	if p := activeParams.Load(); p != nil {
		return p
	}

	calibrationMu.Lock()
	defer calibrationMu.Unlock()
	if p := activeParams.Load(); p != nil {
		return p
	}

	p, measured := CalibrateParams(CalibrationTarget)
	if measured < CalibrationTarget {
		slog.Warn("argon2id calibration could not reach the target cost on this host",
			slog.Duration("target", CalibrationTarget),
			slog.Duration("measured", measured),
			slog.Uint64("memory_kib", uint64(p.Memory)),
			slog.Uint64("iterations", uint64(p.Iterations)),
			slog.Uint64("parallelism", uint64(p.Parallelism)))
	} else {
		slog.Info("argon2id calibrated",
			slog.Duration("target", CalibrationTarget),
			slog.Duration("measured", measured),
			slog.Uint64("memory_kib", uint64(p.Memory)),
			slog.Uint64("iterations", uint64(p.Iterations)),
			slog.Uint64("parallelism", uint64(p.Parallelism)))
	}
	activeParams.Store(p)
	return p
}

// Hash derives an encoded argon2id hash with the active parameters.
//
// The encoded form is PHC: it carries the algorithm, the version, the memory,
// iteration and parallelism costs and the salt alongside the digest. That is
// the whole of FOUND-02's "parameters stored with the hash" -- nothing is
// written beside it, because a second copy is a second thing to keep in sync.
func Hash(password string) (string, error) {
	if password == "" {
		return "", errEmptySecret
	}
	return argon2id.CreateHash(password, ActiveParams())
}

// Verify checks a password against an encoded hash, using the parameters that
// hash was written with rather than the current ones. This is what lets the
// cost be raised without invalidating a single existing password.
func Verify(password, encodedHash string) (bool, error) {
	return argon2id.ComparePasswordAndHash(password, encodedHash)
}

// NeedsRehash reports whether an encoded hash was written with parameters
// weaker than the active ones.
//
// It only ever answers yes for an upgrade. A hash created on a faster machine
// -- or before the operator moved the data directory to a slower one -- is left
// alone, because rewriting it would lower the cost of the credential.
func NeedsRehash(encodedHash string) bool {
	stored, _, _, err := argon2id.DecodeHash(encodedHash)
	if err != nil {
		// An undecodable hash cannot be compared, and rewriting it would
		// destroy the only copy of the credential. Leave it; Verify will fail
		// loudly on the next login.
		return false
	}
	want := ActiveParams()
	return stored.Memory < want.Memory ||
		stored.Iterations < want.Iterations ||
		stored.Parallelism < want.Parallelism ||
		stored.KeyLength < want.KeyLength ||
		stored.SaltLength < want.SaltLength
}
