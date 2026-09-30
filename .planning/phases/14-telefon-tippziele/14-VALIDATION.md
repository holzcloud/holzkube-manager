---
phase: "14"
slug: "telefon-tippziele"
# status lifecycle: draft (seeded by plan-phase) → validated (set by validate-phase §6)
# audit-milestone §5.5 distinguishes NOT-VALIDATED (draft) from PARTIAL (validated + nyquist_compliant: false) (#2117)
status: draft
nyquist_compliant: false
wave_0_complete: false
created: "2026-09-30"
---

# Phase 14 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> Source: 14-RESEARCH.md § Validation Architecture.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | vitest 4.1.11 (projects `jsdom`, `browser`) + the Playwright audit `web/scripts/layout-audit.mjs` |
| **Config file** | `web/vite.config.ts` (`test.projects`) |
| **Quick run command** | `npm --prefix web exec -- vitest run --project jsdom src/layoutRoutes.test.ts src/fixtures.test.ts` |
| **Full suite command** | `./bin/task ci` |
| **Layout gate** | `./bin/task test:layout; echo "exit=$?"` (never piped) |
| **Estimated runtime** | quick ~20 s; layout gate a few minutes on the Pi; full CI ~20 min |

---

## Sampling Rate

- **After every task commit:** the quick run command, plus `./bin/task test:layout` whenever a class or the audit changed
- **After every plan wave:** `./bin/task test:layout`
- **Before `/gsd-verify-work`:** `./bin/task ci` exit 0, read from the command; the three red checks (a) Button `max-md:h-11` removed, (b) `icon-sm` `max-md:size-11` removed, (c) one route deleted from `layout-routes.json`, each followed by a restore (`cmp` against `git show HEAD:`) and a green run
- **Max feedback latency:** ~20 s for the quick run

---

## Per-Task Verification Map

Filled in by the planner per task; the requirement-level map is:

| Requirement | Behavior | Test Type | Automated Command | What makes it red | File Exists | Status |
|-------------|----------|-----------|-------------------|-------------------|-------------|--------|
| MOB-01 | every control ≥ 44 × 44 at 390 on every route | audit (e2e) | `./bin/task test:layout` | red check (a) → `SMALL 390px <route>`, exit ≠ 0 | ✅ extend | ⬜ pending |
| MOB-01 | 1280 unchanged | one-off measurement | `LAYOUT_DUMP=… npm --prefix web run test:layout` before and after, compared by key | any differing box → count ≠ 0 in SUMMARY | ❌ W0 | ⬜ pending |
| MOB-01 | toast dismiss ≥ 44 at 390 | browser component | `npm --prefix web run test:browser -- <toast test>` | remove `max-md:size-11!` → 24 < 44 | ❌ W0 | ⬜ pending |
| MOB-02 | opened states measured | audit (e2e) | `./bin/task test:layout` | red check (b) → `SMALL 390px <route> · <opener>`; missing trigger → `OPENER`; zero measured → `EMPTY` | ❌ W0 | ⬜ pending |
| MOB-02 | route in router missing from audit | unit (jsdom) | `npm --prefix web run test:layout` (guard first) | red check (c) → "… is in the router but not in …", browser not started | ❌ W0 | ⬜ pending |
| MOB-02 | audit executes nothing | audit (e2e) | same | a 2xx on a non-GET action path → `EXECUTED`, exit ≠ 0 | ❌ W0 | ⬜ pending |
| MOB-02 | helper-installed host variant is a valid answer | unit (jsdom) | `vitest run --project jsdom src/fixtures.test.ts` | variant not parseable by `hostSchema` | ❌ W0 | ⬜ pending |
| MOB-03 | `/host` + host action dialog at 390 | audit (e2e) | `./bin/task test:layout` | dialog close X at 28 px → `SMALL 390px /host · Host action dialog` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

### Per task (planner, 2026-09-30)

| Task | Requirement | Automated command | Red before green |
|------|-------------|-------------------|------------------|
| 14-01 T1 (tracer) | MOB-02 | `npm --prefix web exec -- vitest run --project jsdom src/layoutRoutes.test.ts`; `./bin/task test:layout` | T3 |
| 14-01 T2 | MOB-02 | `./bin/task test:layout` (REQUEST 428 line, Sudo dialog ok at 390 and 1280) | T3 |
| 14-01 T3 | MOB-02 | the same, against the guard's four messages and OPENER / EMPTY / EXECUTED injections; `cmp` restore | itself |
| 14-02 T1 | MOB-03 | `npm --prefix web exec -- vitest run --project jsdom src/components/HostActions.test.tsx` | step 1 |
| 14-02 T2 | MOB-03 | the same plus `src/routes/host.test.tsx`; `npm --prefix web run test:browser -- src/routes/host.browser.test.tsx` | step 1 |
| 14-03 T1 | MOB-02, MOB-03 | `npm --prefix web exec -- vitest run --project jsdom src/fixtures.test.ts`; `go test ./internal/publicrepo/` | step 1 |
| 14-03 T2 | MOB-02, MOB-03 | first extended `./bin/task test:layout`, expected red, no OPENER / EMPTY / EXECUTED | expected red is the finding |
| 14-04 T1 | MOB-01 | `node web/scripts/layout-dump-compare.mjs` on crafted pairs | the moved-box case |
| 14-04 T2 | MOB-01 | the comparer on two dumps of one tree (noise floor 0) | — |
| 14-05 T1 | MOB-01 | jsdom project, browser project, lint, typecheck, built-CSS greps | — |
| 14-05 T2 | MOB-01 | `npm --prefix web run test:browser -- src/components/ui/sonner.browser.test.tsx`; audit log without primitive findings | toast at 24 px |
| 14-06 T1 | MOB-01 | Sidebar/WhatsNew tests; audit log without Navigation / What's new findings or CUT OFF | — |
| 14-06 T2 | MOB-01, MOB-03 | `./bin/task test:layout` exit 0 | — |
| 14-07 T1 | MOB-01, MOB-02 | red checks (a) (b) (c) via `./bin/task test:layout`; `cmp` restores; D-08 comparer | itself |
| 14-07 T2 | all | `go test ./internal/publicrepo/` | — |
| 14-07 T3 | all | `./bin/task ci`; helper absent; D-11 human-check | — |

---

## Wave 0 Requirements

- [ ] `web/src/routeTree.ts` — route tree moved out of `App.tsx`
- [ ] `web/scripts/layout-routes.json` — the audited route list
- [ ] `web/src/layoutRoutes.test.ts` — route guard (router leaves vs JSON)
- [ ] `web/package.json` `test:layout` chains the guard before the audit
- [ ] `layout-audit.mjs` — JSON routes, scoped measure, disabled measured, OPENERS, animation settle, request monitor, `LAYOUT_DUMP`
- [ ] helper-installed host fixture variant + its `fixtures.test.ts` case
- [ ] toast backstop browser test

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| A thumb reaches the typed host confirmation on a real phone | MOB-03 | Needs a real phone, the daemon's TLS and an installed helper (buttons stay disabled without it) | Recorded in the SUMMARY as performed / not performed, never "passed" by default (D-11) |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
