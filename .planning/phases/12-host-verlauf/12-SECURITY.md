---
phase: 12
slug: host-verlauf
status: verified
threats_total: 20
threats_closed: 20
threats_open: 0
asvs_level: 1
block_on: high
audited: 2026-09-29
---

# Phase 12 — Security

Retroactive audit of the threat registers in 12-01..12-08-PLAN.md against the code at
0bd5144, after the review fixes (12-REVIEW-FIX.md). Ran on the operator's Pi (aarch64),
Go 1.27.1, Node 22. The production service was only read (`systemctl show`); it and its
data directory were not touched. Fault injections ran through `go test -overlay` from
scratch copies; the working tree was not edited.

## Trust Boundaries

| Boundary | Description |
|---|---|
| browser → GET /api/v1/host/history | an authenticated reader asks for the host's recorded day; `range` is caller input |
| sampler → host sysfs, statfs, uname | the daemon reads kernel interfaces on a 15-s timer, for nobody in particular |
| daemon → /sys/class/thermal | driver-written trip points become limits that decide a state |
| wall link → GET /api/v1/clusters/{id}/wall | a URL on a television opens exactly one route; what it carries is public to the corridor |
| sampler snapshot → wall | a stale snapshot must not read as a current state |
| server JSON → browser rendering | limits, severity and state shown must be the server's, not a browser copy |
| reading → chart | an unreadable value drawn as a line at 0 is a false statement |
| sidebar → GET /api/v1/host | a background poll from every open tab |
| rendering / verification session → public repository, production host | measured values must not reach the tree; the live service must not be disturbed |

## Threat Register

| ID | Category | Component | Severity | Disposition | Status | Evidence |
|---|---|---|---|---|---|---|
| T-12-01 | EoP | GET /api/v1/host/history | medium | mitigate | CLOSED | handlers/history.go:48-54 RequiresSession + MinRole reader, no WallLink; TestHostHistoryAPI: no session → 401, reader → 200, nothing audited; TestOnlyTheWallAcceptsAWallLink passes, red with WallLink on the route |
| T-12-02 | Tampering | `?range=` | low | mitigate | CLOSED | history.go:60-68 historyRange → history.ParseRange (internal/history/history.go:84-93: "", 1h, 6h, 24h only); TestHostHistoryAPI range=7d → 400 Validation naming field `range` |
| T-12-03 | DoS | Collector.Sample every 15 s | low | mitigate | CLOSED | sample.go:28-48 calls readLive only, never readService's dir walk or update status; every file through readBounded; sampler.go one ticker, passes serial; WR-03: sampleHost waits at most HostBudget 5 s, Sample refuses a second read while one hangs (ErrSampleBusy) |
| T-12-04 | Info Disc. | host history for readers | low | accept | CLOSED | Accepted Risks Log |
| T-12-05 | DoS | trip-point walk | low | mitigate | CLOSED | sensors.go:26-31 (64 zones, 16 trips), loop bounded by maxTripPoints, each file via readTrimmed → readBounded 64 KiB; TestTripPoints "at most sixteen" red with the cap at 17 |
| T-12-06 | Tampering | Assess and the host's limits | medium | mitigate | CLOSED | active trips never a line (TestTripPoints red when counted); health.go Unread → unknown (CR-01), no capacity → unknown (WR-01), warn > unknown > ok; TestUnreadableIsNeverHealthy and TestSensorsThatFailAreNotNone red with Unread ignored; integer `>=` over used + available, same denominator as host.tsx / Meter.tsx. See Residual 1 |
| T-12-07 | Info Disc. | Health.Unreadable sentences | low | accept | CLOSED | Accepted Risks Log |
| T-12-08 | Info Disc. | wall answer `host` field | medium | mitigate | CLOSED | wall_host.go name/state/reason only; unknown → fixed "not readable"; warning taken from Health.Public (`json:"-"`, role, never path; WR-02); TestTheWallsHost "unknown never shows its sentences" and "a filesystem's path never reaches the wall" (red with Warnings read) |
| T-12-09 | EoP | wall link → /api/v1/host, /api/v1/host/history | medium | mitigate | CLOSED | only the wall route sets WallLink; walllinkapi_test.go lists both host routes; TestOnlyTheWallAcceptsAWallLink and TestAWallLinkOpensTheWallAndNothingElse pass, both red with WallLink on the history route |
| T-12-10 | Spoofing | stalled sampler's last "ok" | medium | mitigate | CLOSED | wall_host.go older than 3 × FineStep → unknown; TestTheWallsHost stale rows red with the check skipped. See Residual 2 |
| T-12-11 | DoS | wall refresh every 10 s per screen | low | mitigate | CLOSED | wall_host.go reads d.Host.Latest() (under c.mu), no host read; TestTheWallDoesNotReadTheHost passes |
| T-12-12 | Tampering | temperatureSchema / Sensors | low | mitigate | CLOSED | api.ts `warn_c`/`danger_c` z.number() with no default; no browser limit rule left in web/src; Sensors.tsx severityOf(celsius, warn_c, danger_c); Sensors.test.tsx "the limits are the server's" incl. "refuses a temperature without its lines" pass |
| T-12-13 | Tampering | host.tsx live pick and chart slots | medium | mitigate | CLOSED | host.tsx writes readable keys only, never a 0; "Not readable — no history" instead of an empty axis; WR-04 breaks curves at 1.5 × step. See Residual 3 |
| T-12-14 | Spoofing | Sidebar mark after a failed poll | medium | mitigate | CLOSED | Sidebar.tsx host.error → "did not answer" ring, checked before data; Sidebar.test.tsx passes |
| T-12-15 | DoS | sidebar polling from every tab | low | mitigate | CLOSED | Sidebar.tsx `retry: false`, 30-s interval, none on /host; the dir walk behind the answer is cached 60 s (T-11-14) |
| T-12-16 | Info Disc. | sidebar query on the wall screen | low | accept | CLOSED | Accepted Risks Log |
| T-12-17 | Info Disc. | demo.json, host.png, wall.png, layout audit | medium | mitigate | CLOSED | fixture host is manager-01.homelab.example; host history synthesized from the fixture; screenshots carry documentation values only; `go test ./internal/publicrepo` exit 0 at HEAD |
| T-12-18 | Info Disc. | SUMMARY and docs after measuring the host | medium | mitigate | CLOSED | 12-08-SUMMARY records field names, states and counts only; docs use documentation values; publicrepo passes at HEAD |
| T-12-19 | DoS | production holzkube-manager.service | high | mitigate | CLOSED | 12-08-SUMMARY: ActiveEnterTimestamp the same before and after; read again at audit: unchanged, NRestarts=0; no dev daemon running |
| T-12-SC | Tampering | dependencies | low | accept | CLOSED | Accepted Risks Log |

## Accepted Risks Log

| ID | Risk | Rationale | Premise checked |
|---|---|---|---|
| T-12-04 | Readers see the host's recorded day | Same values a reader already sees live on /host (T-11-02, T-11-18, T-11-19) | host.HistoryValues emits only cpu, core:n, memory, rx, tx, temp:, fan: — the live section's kinds |
| T-12-07 | Unreadable sentences show paths and error text on /host | Same reason messages /host readers already see (T-11-13); the wall never carries them | wall_host.go maps unknown to "not readable"; CR-01's added sentences carry only chip/label driver strings |
| T-12-16 | Sidebar query on a wall-link screen | The Sidebar renders only in the AppShell under the authenticated layout | the wall route hangs off the root route, not the AppShell; /api/v1/host refuses a wall link anyway (T-12-09) |
| T-12-SC | Supply chain | No package added | go.mod, go.sum, web/package.json and web/package-lock.json unchanged across phase 12 |

## Residual Observations (non-blocking)

1. On the zone-fallback path (a machine whose CPU has no hwmon chip), a thermal zone whose `type` file cannot be read is skipped without a trace, and Assess can return ok. CR-01's shape on a path its fix did not cover; the Pi 5 is not affected. Carried into phase 13 as a small fix.
2. IN-01 deferred: the wall's age is measured from a `now` taken before ForTheWall; the limit is fixed at 3 × FineStep.
3. IN-04 not fixed: identical chip/label pairs collide in the history map (node path too).
4. IN-06: a tab outside /host still polls /api/v1/host every 30 s and advances the page's rate baseline (accuracy, not load).
5. IN-02 / IN-03 unchanged (twin limits keyed by chip name; 90 % red on the meter but never "critical" server-side).
6. After CR-01, an input that fails permanently keeps the host "Not readable" while it fails (D-11 vs D-12, flagged for a human check).
7. Summaries 12-06..12-08 have no `## Threat Flags` section.

## Security Audit Trail

| Date | Auditor | Result | Notes |
|---|---|---|---|
| 2026-09-29 | gsd-security-auditor | SECURED, 20/20 closed, threats_open 0 | ASVS L1, block_on high; after 12-REVIEW-FIX; Pi aarch64; guards seen red through go test -overlay |
