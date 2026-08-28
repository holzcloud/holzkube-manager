package audit

import "context"

// Chain computation and verification.
//
// The formula is fixed and was settled by a human decision at the plan 01 gate:
//
//	hash_n = sha256(hash_{n-1} || canonical_json(record_n without its hash field))
//
// It is implemented in record.go and is deliberately not restated here. D-16
// keeps every rotated file forever, so the canonical form is a permanent
// obligation: changing it would invalidate the entire archive retroactively.
// This file adds only the two things the format needs to be usable across
// files -- an anchor at the start, and verification over a sequence of files.

// genesisDomain is the domain-separation string behind Genesis. It names the
// project and the format version, so a chain from a different tool or a future
// incompatible format cannot accidentally share an anchor.
const genesisDomain = "holzkube/audit/genesis/v1"

// Genesis is the prev_hash of the very first record of a data directory.
//
// It is sha256(genesisDomain), not the empty string. An empty prev_hash is
// indistinguishable from a field that was stripped, and "the first record has
// no predecessor" is a claim the format should state rather than imply. A
// defined anchor also means the first record's hash is bound to this format,
// so a chain lifted wholesale into another context does not verify by default.
const Genesis = "666cff38f20b5876c113a32abf0153c1d4b518c855df7ae2c0551ce9044745cf"

// ComputeHash returns the hash a record must carry, given the hash of the
// record before it.
//
// The previous hash is both an input to the digest and a field of the record,
// so a rewritten link is detectable from either direction.
func ComputeHash(prev string, r Record) (string, error) {
	r.PrevHash = prev
	return r.ComputeHash()
}

// Verify recomputes the chain over the given files, in the order given, and
// reports the first record that does not add up.
//
// Files may be plain .jsonl or compressed .jsonl.gz; the caller does not have
// to care which. ok is false with file and line naming the first break, where
// line is 1-based within that file. err is reserved for a file that cannot be
// read or decoded at all -- a broken chain is a finding, not an error.
//
// Verify opens every file read-only and writes nothing, ever. A break that is
// silently recomputed is worse than a break, because it destroys the only
// evidence that the archive was touched. There is deliberately no counterpart
// to this function that fixes what it finds: the finding stays until the
// affected file has been dealt with by hand (D-15).
func Verify(paths []string) (ok bool, file string, line int, err error) {
	return VerifyContext(context.Background(), paths)
}

// cancelCheckEvery is how many records pass between cancellation checks. A
// check per record would cost more than the hash it guards; a check per file
// would let a single large day file run to completion after the caller left.
const cancelCheckEvery = 256

// VerifyContext is Verify, abandoned when ctx is done.
//
// Verification re-reads and re-hashes every record in the window, so it is the
// one read in this package whose cost grows with the archive. A caller that
// has gone away — a disconnected HTTP client, a cancelled startup — should not
// keep paying for it, and on the request path it holds the append mutex while
// it runs, so the work it abandons is work the next audit write does not queue
// behind.
func VerifyContext(ctx context.Context, paths []string) (ok bool, file string, line int, err error) {
	prev := ""
	seeded := false

	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return false, "", 0, err
		}
		recs, readErr := readFile(p)
		if readErr != nil {
			return false, p, 0, readErr
		}
		for i, rec := range recs {
			at := i + 1
			if i%cancelCheckEvery == 0 {
				if err := ctx.Err(); err != nil {
					return false, "", 0, err
				}
			}

			switch {
			case !seeded:
				// The oldest record in the window has no predecessor inside
				// it. Only the very first record of the archive can be checked
				// against the anchor; for anything later the file that would
				// prove its prev_hash lies outside the window, so it is taken
				// as the seed. Startup verification looks at two files, not at
				// the whole archive, and must not report that boundary as a
				// break.
				if rec.Seq == 1 && rec.PrevHash != Genesis {
					return false, p, at, nil
				}
				seeded = true
			case rec.PrevHash != prev:
				return false, p, at, nil
			}

			want, hashErr := ComputeHash(rec.PrevHash, rec)
			if hashErr != nil {
				return false, p, at, hashErr
			}
			if want != rec.Hash {
				return false, p, at, nil
			}
			prev = rec.Hash
		}
	}
	return true, "", 0, nil
}
