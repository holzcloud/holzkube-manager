---
status: complete
phase: 01-foundation-skeleton
source: [01-VERIFICATION.md]
started: 2026-08-28T07:10:00Z
updated: 2026-08-28T17:36:02Z
---

## Current Test

[testing complete]

## Tests

### 1. Render the UI in a real browser

expected: Open `https://127.0.0.1:8443`, complete setup, log in, click every sidebar entry, toggle the theme, and change the password. The shell, sidebar, header, toasts and audit table render correctly and legibly in both themes; the sudo dialog appears on the password change and the underlying form is not lost.
why_human: All 60 frontend tests run under jsdom, which asserts structure and behaviour but renders nothing. Visual appearance, layout, contrast and focus handling cannot be verified programmatically.
result: pass
reported: "weiter"
scope: |
  Passed on the four criteria that are observable in this phase (a, b, c, e), all
  confirmed by automated browser verification. Criterion (d) — the sudo dialog on the
  password change — is NOT covered by this pass: it is unobservable because the phase
  ships no password-change UI, and remains open as gap G-01-1 pending a scope decision.
  The 14 confirmed visual findings remain open as G-01-2 .. G-01-5.
automated_coverage: |
  Driven in headless Chromium (Playwright 1.62, ignoreHTTPSErrors) against a cold
  holzkubed on a throwaway data dir. 20 full-page screenshots, both themes, reviewed
  by five independent visual agents with an adversarial refutation pass (3 of 17 raw
  findings refuted).

  (a) shell / sidebar / header / toasts / audit table render correctly — CONFIRMED.
      Setup -> login -> all 8 sidebar routes -> audit table (5 columns, outcome badges)
      -> session-expiry toast -> error page. No console errors, no pageerror, no
      clipping, no overflow. Audit columns align to the pixel between header and body.
  (b) both dark and light themes — CONFIRMED. Toggle flips `html.dark` and writes
      `holzkube.theme`; the choice survives a reload; all 8 routes captured in both.
  (c) placeholder pages read as intentional — CONFIRMED. Each of the six carries the
      area title, "Coming in phase N." matching its sidebar badge, a description of the
      future screen, and an explicit "Nothing here is built yet." paragraph.
  (d) the sudo dialog appears ON THE PASSWORD CHANGE — NOT OBSERVABLE. See gap G-01-1.
  (e) the underlying form is not lost — CONFIRMED, via an injected 428 on a different
      route (see gap G-01-1 for why). Audit filters (From / To / Action) were identical
      before the dialog, while it was open, after a wrong password, after Cancel, and
      after Confirm; Confirm replayed the original request and the filtered table
      rendered 8 matching rows.

  14 confirmed visual findings, all minor or cosmetic, none blocking legibility.
  Recorded as gaps G-01-2 .. G-01-5 (grouped); full list in the UAT session notes.

### 2. Compare the certificate fingerprint in a browser dialog

expected: Start holzkubed, read the `sha256_fingerprint` line from the startup log, open the URL in a browser and compare the fingerprint shown in the certificate dialog character by character. The two strings are identical, in the same colon-separated upper-case hex format, so the comparison needs no conversion.
why_human: Known gap D6. The verifier confirmed the fingerprint is byte-identical to `openssl x509 -noout -fingerprint -sha256`, which is the format browsers use, but no browser was opened. The whole value of the format is that a human can compare it in a dialog.
result: pass
confirmed: |
  Operator compared the fingerprint in the browser's own certificate dialog against the
  sha256_fingerprint line from the startup log on 2026-08-28 and confirmed it matches.
  This is the last piece of known gap D6, and the one that could only ever be closed by a
  person: the two strings are identical, in the same colon-separated upper-case hex, so the
  comparison needed no conversion — which is the whole reason the log prints that format.
automated_coverage: |
  Re-confirmed on this host against a freshly generated certificate. The logged
  `sha256_fingerprint` is byte-identical to BOTH `openssl x509 -noout -fingerprint
  -sha256` on cert.pem AND the certificate served over the live TLS socket
  (`openssl s_client`). Format is colon-separated upper-case hex, 32 octets.
  Leaf checked: CN=holzkube, ECDSA P-256, NotAfter 2036-08-25,
  SAN = DNS:localhost, DNS:build-host.home, IP:127.0.0.1, IP:::1.

  What is left is exactly D6's residue: a browser's certificate dialog is a native OS
  window and cannot be screenshotted or read by an automated browser. Only a human can
  open it and confirm the string is both present and findable.
deferred_this_session: |
  2026-08-28 — operator had no browser to hand. Test left pending, UAT status partial.
  The value to compare, from this host's certificate:
  F2:5C:27:25:6A:30:E9:56:B5:D6:D4:B3:A2:C1:EB:B0:F8:C4:7B:67:09:F1:6C:BC:6A:CE:01:66:83:63:E4:C3
  Note this is regenerated per data directory — re-read the startup log on the instance
  actually being checked rather than reusing this string.

### 3. Run the three build/lint/release gates with the real tools

expected: Install go-task, golangci-lint v2.13.1 and goreleaser v2.18.0 (commands in `deferred-items.md` item 4), then run `task build`, `golangci-lint run` and `goreleaser release --snapshot --clean`. `task build` produces `bin/holzkubed` with the frontend built first; golangci-lint reports zero issues under the v2 config with gosec enabled; the snapshot archives contain the binary, README.md and docs/*.
why_human: None of the three tools is on this host — 01-06 installed them into a scratch GOBIN, ran them, and did not leave them installed. The verifier reproduced the build chain manually (`npm run build` → byte-identical bundle → `go build` → working UI) and asserted the Taskfile and goreleaser ordering edges structurally, but the three gates themselves were never executed here.
result: pass
source: automated
evidence: |
  Tools installed into a throwaway GOBIN (the host PATH was deliberately left
  unchanged, exactly as 01-06 did): task v3.53.1, golangci-lint v2.13.1
  (go1.26.7), goreleaser v2.18.0. All three gates run from the repository root.

  1. `task build` — EXIT 0. Ordering confirmed from the task output itself:
     `npm --prefix web ci` -> `npm --prefix web run build` (tsc --noEmit && vite build,
     emitting into internal/httpapi/dist) -> `go build -ldflags ... -o bin/holzkubed`.
     Frontend is built before the Go compile. Binary produced; `--version` prints
     `holzkubed cd70b39-dirty`, which is deferred-items.md item 3, not a defect.
  2. `golangci-lint run` — EXIT 0, output `0 issues.` Config is `version: "2"`
     (.golangci.yml:3) with gosec in the enabled set (.golangci.yml:21).
  3. `goreleaser release --snapshot --clean` — "release succeeded after 9s".
     Three archives built (darwin_arm64, linux_amd64, linux_arm64). Contents of each
     verified with `tar -tzf`: `README.md`, `docs/api-contract.md`, `holzkubed`.
     docs/ contains exactly one file, so `docs/*` is fully covered.

  All three expected outcomes met. No human judgement is required for this item — it
  needed only the three absent tools.

## Summary

total: 3
passed: 3
issues: 0
pending: 0
skipped: 0
blocked: 0
open_gaps: 0
resolved_gaps: 5

## Gaps

- gap_id: G-01-1
  truth: "The sudo dialog appears when the operator changes their password, and the underlying form is not lost"
  status: resolved
  resolved_by: scope-decision
  resolved_at: 2026-08-28
  resolution: |
    Operator decision: move the criterion rather than build the screen in phase 1. The clause
    was removed from 01-VERIFICATION.md human item 1 (both the frontmatter and body forms, each
    carrying a scope_amended note) and added to ROADMAP.md Phase 10 as success criterion 5, the
    phase that builds the Settings screen owning the password change. No fix plan is needed
    against phase 1.
  reason: "Phase 1 ships no password-change UI. `api.changePassword` exists in web/src/api.ts:320 but has no caller anywhere in web/src; /settings is a phase-10 ComingSoon placeholder and the Header holds only the identity label, the theme toggle and Sign out. POST /api/v1/account/password is the only Destructive route, so the SudoDialog — which opens only on a 428 whose code starts `sudo.` — is unreachable by clicking. The criterion cannot be observed as written."
  severity: major
  test: 1
  artifacts:
    - path: "web/src/api.ts:320"
      issue: "changePassword is defined but never called from any component"
    - path: "web/src/routes/placeholders.tsx"
      issue: "/settings is a generated ComingSoon placeholder; there is no /account route"
    - path: "web/src/components/SudoDialog.tsx:30"
      issue: "mounted globally in __root.tsx but no rendered control can trigger it"
  missing: []
  note: |
    The MECHANISM was verified in a real browser by injecting a 428 sudo.required on
    GET /api/v1/audit: dialog opens in both themes, the audit filter form is byte-identical
    before / during / after, a wrong password keeps the dialog open with an error, Cancel
    restores the form untouched, and Confirm replays the original request successfully.
    Only the password-change entry point is missing. The dialog description reads the
    generic fallback ("This destructive action ...") under injection; against the real
    endpoint ACTION_LABELS would render "Change the operator password".

- gap_id: G-01-2
  truth: "Interactive controls are distinguishable from their background"
  status: resolved
  resolved_by: commit 5bf359e
  resolved_at: 2026-08-28
  reason: "Light theme: text input borders are #E5E5E5 on a #FFFFFF card = 1.26:1, against the 3:1 WCAG 1.4.11 threshold for component boundaries; the input interior is the same white as the card, so the hairline is the only cue. The card's own border is 1.25:1 against the page. Verified by pixel decode, not by eye."
  severity: minor
  test: 1
  artifacts:
    - path: "web/src/components/ui/input.tsx"
      issue: "border token too light against card/background in light theme"
  missing:
    - "Darken the input border token in light theme to reach 3:1"

- gap_id: G-01-3
  truth: "Keyboard focus is clearly visible (the criterion names focus handling explicitly)"
  status: resolved
  resolved_by: commit 5bf359e
  resolved_at: 2026-08-28
  reason: "The focus indicator is a 3px neutral-grey halo at #D0D0D0 on white = ~1.54:1, under the 3:1 required of a focus indicator, and it is not an accent colour. At 100% zoom it reads as a slightly thicker border rather than a position marker, so tabbing through the setup form is hard to follow."
  severity: minor
  test: 1
  artifacts:
    - path: "web/src/components/ui/input.tsx"
      issue: "focus ring uses a neutral grey with insufficient contrast"
  missing:
    - "Give the focus ring an accent colour at >=3:1 against both the field and the card"

- gap_id: G-01-4
  truth: "The audit table's outcome badges are legible in both themes"
  status: resolved
  resolved_by: commit 5bf359e
  resolved_at: 2026-08-28
  reason: "The `error` badge is #E7000B on #FCE5E6 = 3.97:1 in light theme, short of the 4.5:1 needed at ~12px bold. Its siblings are far stronger (success 16.4:1, attempt 19.8:1), so the one badge an operator most needs to catch is the faintest. Dark theme is fine at 5.31:1."
  severity: minor
  test: 1
  artifacts:
    - path: "web/src/routes/audit.tsx"
      issue: "OutcomeBadge destructive variant below AA in light theme only"
  missing:
    - "Darken the destructive badge foreground in light theme to >=4.5:1"

- gap_id: G-01-5
  truth: "Chrome and copy read as deliberate"
  status: resolved
  resolved_by: commit 5bf359e
  resolved_at: 2026-08-28
  reason: |
    Four confirmed cosmetic/minor findings, grouped:
    (1) An unknown URL renders "Something went wrong / holzkube could not complete that
        request. The details are in the server log." — indistinguishable from a real 500,
        and it sends an operator who merely mistyped a URL to a log with nothing in it.
        ErrorPage deliberately ignores error.message, so the router's own
        "This page does not exist." never reaches the screen.
    (2) A 1px header separator at x=1275 runs y=0..19 in a 56px header whose content is
        centred at y~27 — it reads as a clipped tick rather than a divider. Both themes,
        every authenticated page.
    (3) The active nav row's phase badge loses its chip: badge fill and active-row
        highlight are the same token (#F5F5F5 light, #262626 dark), so the current page's
        badge is bare text while every other row keeps its pill. Related: the active pill
        itself is 1.045:1 against the sidebar in light theme, with no secondary cue.
    (4) The audit filter row does not share a baseline — the Action select sits 4px above
        From / To / Apply filters. And the expiry toast's headline repeats the login
        card's first sentence verbatim while its only new text is backend-shaped phrasing
        about rejected credentials; its close chip is centred on the panel's top-left
        corner, two thirds outside the panel.
  severity: cosmetic
  test: 1
  missing:
    - "Decide which of these are worth fixing in phase 1 versus deferring"

## Gap disposition (operator decision, 2026-08-28)

- G-01-1 — resolved by scope decision; criterion moved to ROADMAP Phase 10, criterion 5.
- G-01-2 .. G-01-5 — **fixed in phase 1** (commit 5bf359e). Operator chose the full polish
  pass over the accessibility-only subset. All 14 findings were closed and each fix was
  re-measured by decoding the rendered PNG; the re-audit confirmed 13 of 14 on the first
  pass and the remaining three partials (card outline, focus outline, toast chip inset)
  were closed in the same commit. Post-fix numbers: input border 3.23:1, focused border
  7.46:1, card outline 1.57:1, error badge 4.65:1, filter row all four controls y=174..205,
  header separator centred at 27.5 in a 56px header, active nav carrying pill + weight +
  left bar. Gates re-run green: 64 frontend tests, biome, task build, golangci-lint
  0 issues, go test ./... all pass, goreleaser snapshot.

  Two pre-existing items were surfaced by the re-audit and deliberately NOT fixed here,
  because they are outside the 14 and outside phase 1's criterion: the native
  datetime-local placeholder renders at full text strength so empty date filters read as
  filled, and the dark-theme table row hover highlight resolves to the card surface
  (1.00:1, no feedback). Both belong in a later UI pass.

## Resolved (previously recorded as open blockers)

The three code-review blockers recorded here on 2026-08-28 were re-checked against the
current source, not against the review documents. All three are fixed, with regression
tests. CR-02 was additionally confirmed live against the running server.

- id: CR-01
  status: resolved
  detail: "`GET /api/v1/system/status` is still `RequiresSession:false` (setup_required gates the wizard), but the live re-verify is now behind `d.Auth.IsAuthenticated(r.Context())` and memoised by `CachedVerify` with a 30s floor (internal/httpapi/handlers/system.go:47-61; internal/audit/audit.go:76,336-353). An anonymous caller gets the startup snapshot and never takes the mutex. `VerifyContext` threads the context and checks cancellation every 256 records (internal/audit/chain.go:61,71-93)."
- id: CR-02
  status: resolved
  detail: "A `Public()` reducer strips the directory at both construction and serialization (internal/httpapi/router.go:66-71; cmd/holzkubed/main.go:145-149; handlers/system.go:74). Confirmed live: an anonymous GET returns `\"file\": \"audit-2026-08-28.jsonl\"` and no absolute path. docs/api-contract.md:313 now documents the bare filename and states the invariant at :320-325. Regression: TestSystemStatusKeepsAChainBreakVisible."
- id: CR-03
  status: resolved
  detail: "`run` now resolves the whole chain with `plan` before taking the backup, refuses on a gap, and only writes VERSION when `at == target` (internal/store/migrate/migrate.go:76-127,131-151). `readVersion` rejects `v < 1` (migrate.go:187-190). Regression: TestMigrateRefusesVersionZero and the gap test at migrate_test.go:300-329."

## UAT session notes

Environment: holzkubed built from source at commit cd70b39-dirty (npm run build →
go build), started cold on 127.0.0.1:8443 with a throwaway data directory. The data
directory was created by the server itself (0700); pre-creating it 0755 is correctly
refused with a repair hint naming the exact chmod. Cold start clean: argon2id calibrated
(65536 KiB, 21 iterations, parallelism 4), TLS certificate generated, audit chain
verified, no errors. Test account `operator` exists only in that throwaway directory.

Audit records write an attempt/success pair per mutation (seq 1 attempt, seq 2 success,
same request_id) — this is by design, not a double submit. The /audit table shows the
Outcome column and distinguishes them; the dashboard's 3-column "Recent activity" preview
omits Outcome, so four `auth.login` rows there look identical when they are in fact
attempt / error / attempt / success. Not filed as a gap — the full table one click away
carries the column — but worth a decision.
