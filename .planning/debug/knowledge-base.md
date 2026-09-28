# GSD Debug Knowledge Base

Resolved debug sessions. Used by `gsd-debugger` to surface known-pattern hypotheses at the start of new investigations.

---

## installer-requestion-arm64 — a hijacking test fake counts net/http's invisible retries on reused connections under go1.27
- **Date:** 2026-09-28
- **Error patterns:** asked 3 times want 2, asked 2 times want 1, request count too high, fails on the Pi but green in CI, arm64-only failure, hijack, silent repository, setRepoUnreachable, setRepoSilentForNext, vacuous green with fault reinstated, go1.27, toolchain mismatch
- **Root cause(s):** environment: the Pi tests with go1.27.1 (go.mod's `toolchain go1.26.7` never downgrades) while CI/releases use go1.26.7, and go1.27's Transport drains a small unread body on Close and pools the connection; code (test harness): fakeFactory simulated silence with Hijack()+Close() on a keep-alive server, and net/http transparently retries an idempotent GET whose reused connection closed before the first byte — so one question became two wire requests, and a one-shot silence was consumed by the attempt while the retry was answered (lost-update guard vacuous). Not arm64; product logic correct.
- **Fix:** fakeFactory.serve sets `Connection: close` on every response, so silence always lands on a fresh connection that is never retried; new guard test TestFakeSilenceIsNeverRetriedByTheClient. No assertion or product code changed.
- **Files changed:** internal/imagefactory/fake_test.go
- **Why not caught:** CI (test gate) was pinned to go1.26.7, where the fresh-connection assumption held by accident; no gate existed for the fake's own contract, so the vacuous concurrency guard would not have been caught by anything.
- **Recurrence guard:** internal/imagefactory/fake_test.go:TestFakeSilenceIsNeverRetriedByTheClient (red on go1.26.7 and go1.27.1 when the fake allows reuse) + the `Connection: close` invariant in fakeFactory.serve. Pattern: when a count is too high only on one machine, compare `go version` before blaming the architecture.
---

