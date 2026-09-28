---
status: resolved
trigger: "TestInstallerImageReQuestionsAProvisionalAnswer fails deterministically on the arm64 Pi"
created: 2026-09-28
updated: 2026-09-28
---

# Debug: installer-requestion-arm64

## Symptoms

- **Expected:** `go test -count=3 -run TestInstallerImageReQuestionsAProvisionalAnswer ./internal/imagefactory/` passes, as it did in CI (GitHub Actions, amd64) on commit 3e772fc.
- **Actual:** it fails every time on the operator's Raspberry Pi 5 (linux/arm64).
- **Error:** `installer_test.go:541: the never-ruled-out candidate was asked 3 times across two calls, want 2 -- a stale provisional entry must be re-questioned`
- **Timeline:** seen 2026-09-28; fails on unchanged main 3e772fc too, so not caused by later edits. Unknown when it started on arm64 (CI is amd64 only, and since 2026-09-28 CI no longer runs on push).
- **Reproduction:** `export PATH=$HOME/.local/go/bin:$PATH; go test -count=3 -run TestInstallerImageReQuestionsAProvisionalAnswer ./internal/imagefactory/` (0.04 s, deterministic).

## Current Focus

- bug_class: Bohrbug in cause (toolchain-deterministic), timing-dependent in magnitude (async drain)
- hypothesis: CONFIRMED; fix applied, self-verified, and independently verified by the session manager (see Human Verification)
- next_action: none in this session -- session manager commits internal/imagefactory/fake_test.go and this archived file; option B (below, Open Follow-ups) waits on the operator

reasoning_checkpoint:
  hypothesis: "The fake's hijack-silence knobs assume every manifest request arrives on a fresh TCP connection. go1.27's Transport drains an unread <=256KiB body on Close and pools the connection (go1.26 closed it), so a later manifest request can ride a reused conn; the fake hijacks+closes it, net/http's shouldRetryRequest transparently retries the idempotent GET on another conn, and the fake sees two wire requests (setRepoUnreachable: count inflated -> ReQuestions RED, cache subtest flaky) or answers the retry normally (setRepoSilentForNext: silence consumed -> forced interleaving defeated -> lost-update guard vacuous)."
  confirming_evidence:
    - "Same arm64 host: go1.26.7 10/10 PASS, go1.27.1 9/10 FAIL"
    - "httptrace: go1.27.1 call2 GotConn reused=true wasIdle=true then a fresh GotConn; go1.26.7 single fresh GotConn"
    - "transport.go go1.27.1: tryDrain/maybeDrainBody then tryPutIdleConn; shouldRetryRequest true for replayable GET on reused conn"
    - "Lost-update fault reinstated: go1.26.7 10/10 RED, go1.27.1 1/10 RED"
  falsification_test: "With Connection: close on every fake response, go1.27.1 must still show reused=true or a count of 3 -- if so, reuse is not the channel and the hypothesis is wrong."
  fix_rationale: "The defect is in the test harness's unstated assumption (one logical request = one wire request, silence is observed as silence), not in the product: the product asks once and its budget covers the retry (same callCtx). Making the fake refuse connection reuse restores the assumption on every toolchain at the one place it lives, and keeps every assertion unchanged."
  blind_spots: "amd64+go1.27 not executed (no emulation here); inferred from arch-free transport code and the arch-held-constant differential. internal/httpapi/handlers has its own hijacking fake -- checked separately."
  candidate_causes:
    - "environment: local toolchain go1.27.1 vs CI go1.26.7 (GOTOOLCHAIN=auto never downgrades) -- the trigger"
    - "code (test harness): fake silence via Hijack on a keep-alive server; counts wire requests -- the latent defect"
    - "code (product): probeStatus closes without draining / re-question asks twice -- eliminated"
  and_gate: "yes -- needs BOTH go>=1.27's pooled-after-early-close connections AND the fake's hijack-on-keep-alive silence. Either alone is harmless: go1.26 + fake passes; go1.27 + a fake that closes every connection passes."

## Evidence

- timestamp: 2026-09-28
  checked: reproduction on the Pi (go1.27.1 linux/arm64), -count=3 -v
  found: fails 3/3 runs; counts observed 3, 4, 3 (not a fixed number)
  implication: the failure is deterministic but its magnitude varies run to run -> an extra, timing-dependent request source, not a logic error in the resolver's candidate list (which would give a fixed count)

- timestamp: 2026-09-28
  checked: installer.go resolveInstallerRepo / requestionInstallerRepo, probe.go probeStatus, client.go New
  found: re-question calls resolveInstallerRepo(entry.unresolved...) = only "metal-installer" -> one logical GET. probeStatus closes resp.Body without reading it. New() builds &http.Client{CheckRedirect:...} with nil Transport -> http.DefaultTransport (keep-alive on).
  implication: application logic asks metal-installer exactly once per call; any extra count must come from the transport layer

- timestamp: 2026-09-28
  checked: fake_test.go setRepoUnreachable / serveManifest
  found: an unreachable repo is answered by Hijack()+Close() with no HTTP response; record() counts every arriving request, including transport-level retries
  implication: the fake counts wire requests, not logical questions

- timestamp: 2026-09-28
  checked: ~/.local/go/src/net/http/transport.go (go1.27.1)
  found: readLoop: on early Close, `tryDrain := !bodyEOF && resp.ContentLength <= maxPostCloseReadBytes` (256 KiB) -> maybeDrainBody (<=50ms, async after Close returns) -> tryPutIdleConn. shouldRetryRequest returns true for a replayable GET on a *reused* conn that fails with transportReadFromServerError / errServerClosedIdle. No GODEBUG knob controls the drain.
  implication: under go1.27 the 200 conn from the legacy candidate goes back to the pool; a hijack on it triggers an invisible Transport retry

- timestamp: 2026-09-28
  checked: go.mod, .github/workflows/ci.yml
  found: go.mod `go 1.26.6` / `toolchain go1.26.7`; CI uses setup-go with go-version-file: go.mod -> go1.26.7. Pi: go1.27.1 (GOTOOLCHAIN=auto does not downgrade).
  implication: CI and the Pi differ in toolchain as well as architecture -- a confound the symptom report attributed to arm64

- timestamp: 2026-09-28
  checked: differential on the SAME arm64 host, only the toolchain varied (GOTOOLCHAIN=go1.26.7 downloaded)
  found: go1.26.7: 10/10 PASS. go1.27.1: 9/10 FAIL (counts 3 or 4; one run passed)
  implication: H1 confirmed at the toolchain level. Architecture held constant, toolchain varied -> the failure is toolchain-dependent, not arm64-dependent. Magnitude/occasional pass is timing (async drain + dial/idle race)

- timestamp: 2026-09-28
  checked: scratch test with httptrace GotConn on both calls (zz_scratch_trace_test.go, to be deleted)
  found: go1.27.1: call2 GotConn reused=true wasIdle=true on the legacy candidate's call-1 conn, then a second GotConn on a fresh conn; fake counts metal=3. go1.26.7: call2 a single fresh conn; metal=2.
  implication: mechanism observed directly: one logical re-question = two wire requests under go1.27 (Transport retry of an idempotent GET after the server closed a reused conn)

- timestamp: 2026-09-28
  checked: full package -count=3 under both toolchains
  found: go1.26.7 exit 0. go1.27.1 exit 1: ReQuestions 3/3 fail AND TestInstallerNameCheckDoesNotReadAFallbackAsAnObservation/the_same_answer_served_from_the_cache 1/3 fail ("asked 2 times, want 1") -- in a COLD first call the legacy candidate's drained conn can be handed to the still-waiting preferred request, which is then retried
  implication: same root cause, second test, flaky rather than consistent. Any fake knob that hijacks (setRepoUnreachable, setRepoSilentForNext) is exposed. Concern: setRepoSilentForNext is consumed by the first attempt, so the Transport's retry gets the normal answer -> the forced interleaving in TestInstallerImageNeverRevertsAProvenNameUnderConcurrentResolution may be silently defeated (vacuous green) under go1.27

- timestamp: 2026-09-28
  checked: reinstated the lost-update fault (storeInstallerRepo's proven-guard disabled) and ran TestInstallerImageNeverRevertsAProvenNameUnderConcurrentResolution -count=10 per toolchain; installer.go restored from git afterwards
  found: go1.26.7: 10/10 RED (guard works). go1.27.1: 1/10 RED -- 9/10 vacuously GREEN with the fault in
  implication: the concern is real and it is the worse half of this bug: under go1.27 the setRepoSilentForNext silence is consumed by the attempt on the reused conn, the Transport's retry is answered 200, the forced interleaving never happens, and the concurrency guard stops guarding. Same root cause, silent instead of loud.

<!-- The next two entries were first appended under Eliminated by mistake; moved here verbatim at archive time. -->

- timestamp: 2026-09-28
  checked: falsification test -- the scratch httptrace run with the fix in place, go1.27.1
  found: call2 single GotConn reused=false; fake metal=2 installer=1
  implication: hypothesis survived its falsification test; reuse was the channel

- timestamp: 2026-09-28
  checked: internal/httpapi/handlers (has its own hijacking fake, steady-state knob only, no consumable silence) under go1.27.1 -count=3, before the fix
  found: ok, exit 0 (253s)
  implication: not affected by a failing assertion; its hijack knob is steady-state so a retry is silenced too. Not fault-injected in this session.

- timestamp: 2026-09-28
  checked: independent verification by the session manager on the Pi (aarch64), recorded under Human Verification below
  found: fix green 10/10 on go1.27.1; with only the Connection: close line removed, both target tests red 5/5 on go1.27.1 and the new guard red 5/5 on go1.26.7; full package green -count=3 on both toolchains; lint clean
  implication: the fix is what makes the difference, on both toolchains, and the guard goes red on the pinned toolchain too -- not only on the one that exposed the bug

- timestamp: 2026-09-28
  checked: archive-time existence check of the recorded guard (go1.27.1 linux/arm64): grep for the guard and the header line, then `go test -count=3 -run 'TestFakeSilenceIsNeverRetriedByTheClient|TestInstallerImageReQuestionsAProvisionalAnswer' ./internal/imagefactory/`
  found: guard at internal/imagefactory/fake_test.go:308, `w.Header().Set("Connection", "close")` at :438; ok, exit=0
  implication: the recurrence guard recorded below exists and passes as of archiving

## Eliminated

- hypothesis: the failure is arm64-specific (word size, memory model, scheduler)
  evidence: same arm64 host passes 10/10 with go1.26.7 and fails 9/10 with go1.27.1; transport.go drain/retry code has no arch-specific path
  timestamp: 2026-09-28

- hypothesis: the product re-question asks the preferred candidate more than once (logic regression in installer.go)
  evidence: resolveInstallerRepo is called with entry.unresolved (one name), one goroutine, one probeStatus; httptrace shows one logical request that the Transport retried internally
  timestamp: 2026-09-28

## Resolution

root_cause: "Toolchain-dependent, not arm64-dependent (AND of two conditions). (1) environment: the Pi builds with go1.27.1 -- go.mod's `toolchain go1.26.7` never downgrades -- while CI/releases use go1.26.7; go1.27's net/http Transport drains an unread body <=256 KiB on Close and pools the connection, where go1.26 closed it. (2) test harness: fakeFactory's silence knobs hijack+close on a keep-alive server, assuming each request arrives on a fresh connection. Under go1.27 a manifest request can ride a pooled connection; net/http transparently retries an idempotent GET whose reused connection closed before the first byte, so setRepoUnreachable counts two wire requests per question (ReQuestions RED, cache subtest flaky) and setRepoSilentForNext's silence is consumed by the first attempt while the retry is answered normally (lost-update guard vacuous: 1/10 RED with its fault in). Product logic is correct: one logical question, retry inside the same ManifestTimeout context."
fix: "fakeFactory.serve answers every request with `Connection: close`, so no connection to the fake is ever reused and a hijacked silence always lands on a fresh connection that net/http does not retry. New deterministic guard TestFakeSilenceIsNeverRetriedByTheClient (reads a 200 body to EOF so the connection is pooled on every toolchain, then asserts one silent request reaches the fake once). No assertion changed. No product code changed."
oracle_type: specified (exact wire-request counts; fake's documented knob contract)
files_changed:
  - internal/imagefactory/fake_test.go
verification:
  target_test: { result: pass, detail: "ReQuestions + NameCheck + self-test -count=20 green on go1.27.1 and go1.26.7" }
  mutation_check: { result: pass, reason_if_skipped: "no Stryker for Go; manual mutation instead", mutant_killed: "Connection: close removed -> self-test 10/10 RED on go1.26.7 AND go1.27.1; ReQuestions 8/10 RED on go1.27.1" }
  no_op_deletion: { result: pass, deletion_justified_by_rca: n/a, detail: "76 insertions, 0 deletions" }
  adjacent_tests: { result: pass, suites_run: ["go test -count=3 ./internal/imagefactory/ go1.27.1 exit 0", "same on go1.26.7 exit 0", "golangci-lint v2.13.1 ./internal/imagefactory/... 0 issues exit 0"] }
  revert_and_reconfirm: { result: pass, bug_returned_on_revert: true, fixed_on_reapply: true }
  product_faults_reinstated_with_fix: "freeze (no re-question) 10/10 RED; full-list re-question 10/10 RED; lost-update guard removed 10/10 RED -- each on go1.26.7 AND go1.27.1 (lost-update was 1/10 on go1.27.1 before the fix)"
  guardrail_verdict: accepted
  environment: "all of the above ran on the operator's Pi, linux/arm64 (aarch64). amd64 was not executed in this session; CI (amd64, go1.26.7) was not dispatched."

## Human Verification

**Who:** the session manager, independently, on the operator's Pi (aarch64). **Not the operator in person** -- the operator has standing instructions for full autonomy, and the session manager ran the checks itself rather than relaying them. Recorded as an independent verification, not as operator sign-off.

**Results (as reported by the session manager, exit codes read from the commands):**

- With the fix: `TestInstallerImageReQuestionsAProvisionalAnswer|TestFakeSilenceIsNeverRetriedByTheClient` `-count=10` on go1.27.1 -> exit=0.
- Fault reinstated (only the `w.Header().Set("Connection", "close")` line removed):
  - both tests failed 5/5 on go1.27.1 (exit=1);
  - the new guard failed 5/5 on `GOTOOLCHAIN=go1.26.7` (exit=1);
  - fix restored byte-identical afterwards.
- Full package `go test -count=3 ./internal/imagefactory/` -> exit=0 on go1.27.1 and on go1.26.7.
- golangci-lint v2.13.1 on `./internal/imagefactory/...` -> 0 issues, exit=0.

**Disposition:** option A -- fix confirmed as-is. The session manager commits `internal/imagefactory/fake_test.go`. Option B was left open for the operator and not applied (see Open Follow-ups).

## Prevention

### Branching 5-Whys (blameless)

**Branch 1 -- environment (the trigger).**
- Why did the test fail on the Pi and not in CI? The Pi ran go1.27.1; CI ran go1.26.7.
- Why were they different? go.mod's `toolchain go1.26.7` is a minimum: `GOTOOLCHAIN=auto` upgrades to it but never downgrades from a newer local Go, so the Pi silently tests with whatever `~/.local/go` holds.
- Why did nobody notice? Nothing records or compares the toolchain a local check ran on; a report says "passed on the Pi" without saying "on go1.27.1". The symptom was consequently read as arm64-specific, because architecture was the difference everyone knew about.
- Actionable condition: the toolchain is part of a check's environment and must be named with it (and see Open Follow-ups, option B).

**Branch 2 -- code, test harness (the latent defect).**
- Why did go1.27 break the fake? The silence knobs used Hijack()+Close() on a keep-alive server, and a hijack on a reused connection is not silence to net/http: the Transport retries an idempotent GET on a fresh connection.
- Why did the harness rely on fresh connections? It held by accident -- `probeStatus` closes an unread manifest body, and up to go1.26 that closed the connection -- and the assumption was never written down or tested.
- Why was it never tested? The fake had no self-test of its own contract; its correctness was inferred from the product tests passing, which is exactly what fails silently when the fake is wrong.
- Actionable condition: the fake's contract ("one silent request reaches the fake once") is pinned by its own test, and the fake enforces the condition (`Connection: close`) instead of inheriting it.

**AND-gate:** both branches were needed. go1.26 + the old fake passes; go1.27 + a fake that closes every connection passes.

### Why wasn't this caught?

- **The test gate (CI)** was pinned to go1.26.7 via `go-version-file: go.mod`, where the harness's assumption happens to hold, so it could not see the go1.27 behaviour. Since 2026-09-28 CI also no longer runs on push.
- **No gate existed for the fake's own contract.** The worse half of the bug -- `TestInstallerImageNeverRevertsAProvenNameUnderConcurrentResolution` going vacuously green (9/10) with the lost-update fault reinstated on go1.27.1 -- would not have been caught by any gate at all; it only turned up because this session reinstated the fault. The loud failure (ReQuestions) is what led to the silent one.

### Recurrence guard

- **Regression test:** `internal/imagefactory/fake_test.go:TestFakeSilenceIsNeverRetriedByTheClient` -- reads a 200 body to EOF so the connection is pooled on *every* toolchain, then asserts one silent request reaches the fake exactly once. Goes red on go1.26.7 as well as go1.27.1 when the fake allows connection reuse (session manager: 5/5 red on go1.26.7 with the fix line removed), so the pinned CI toolchain now guards it too.
- **Harness invariant:** `w.Header().Set("Connection", "close")` at the top of `fakeFactory.serve` (fake_test.go:438), with a comment naming the retry mechanism and the guard test.
- **Knowledge-base pattern:** entry `installer-requestion-arm64` in `.planning/debug/knowledge-base.md` -- "a test fake that hijacks to simulate silence counts net/http retries on reused connections; check the toolchain before blaming the architecture."
- Verified at archive time: the guard exists (fake_test.go:308) and passes on go1.27.1 (exit=0).

## Open Follow-ups

**Operator decision, not applied -- how Pi-local checks pick their Go toolchain:**

(Labels here are separate from the checkpoint's "option A = fix confirmed as-is"; only "option B" carries over.)

- **Keep (status quo): keep go1.27.1 as the Pi's local toolchain.** Costs: Pi-local results and CI/release results come from different toolchains, so a green Pi check does not certify the shipped binary's toolchain, and every Pi report has to name the Go version. Buys: an early warning for the next Go release -- this bug is exactly what it catches, on the machine that runs production.
- **Option B: pin Pi-local checks to go1.26.7** (e.g. `GOTOOLCHAIN=go1.26.7` in the Pi's environment or in the task runner). Costs: loses the early warning; go1.27-only defects surface only when go.mod's toolchain is raised. Buys: Pi-local checks run the same toolchain as CI and the released binaries, so "green on the Pi" means what the release will be built with.
- **Both:** the gate runs on go1.26.7, plus a separate, non-blocking go1.27 run. Costs: twice the test time on the Pi. Buys: parity and the early warning, kept apart.

Recommendation from this session: Keep, or Both. The fix no longer depends on the toolchain, and the go1.27 run found both a loud failure and a silent vacuous guard that go1.26.7 could not have shown. B is the choice if Pi-local results must match the release toolchain exactly. Left to the operator; the session manager did not apply B.

**Minor, not required:** amd64 + go1.27 was never executed (no emulation from the Pi); the conclusion rests on arch-free transport code and the same-host toolchain differential. `internal/httpapi/handlers`' own hijacking fake passed on go1.27.1 but was not fault-injected.

**MemPalace indexing skipped:** MemPalace is not installed on this host; `.planning/debug/knowledge-base.md` is the durable record.
