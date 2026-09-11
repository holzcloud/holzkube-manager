---
schema_version: 1
open_count: 63
waived_count: 0
fixed_count: 16
total_count: 79
last_updated: 2026-09-11T16:30:00.000Z
---

# Broken Windows Ledger

> Cross-phase defect register. With `workflow.windows_enforce` enabled, `/gsd-ship` blocks while `open_count > 0`.
> Waive with `gsd-tools windows waive <id> "<reason>"` (reason required).
> Mark fixed with `gsd-tools windows fixed <id>`.

| id | phase | kind | file | line | description | status | reason | recorded_at | resolved_at |
|----|-------|------|------|------|-------------|--------|--------|-------------|-------------|
| 1 | 02 | stub | internal/talos/dial_direct.go |  | directDialer.Probe leaves Identity.Version empty: the version is not in the TLS certificate and Dialer.Probe carries no Creds to make an authenticated RPC | open |  | 2026-08-28T19:58:41.457Z |  |
| 2 | 02 | stub | internal/talossim/machine.go |  | talossim implements 2 of 54 MachineService RPCs; the rest inherit Unimplemented. Scoped to plan 02-08 by the plan's own scope_decision | open |  | 2026-08-28T19:58:41.580Z |  |
| 3 | 02 | stub | internal/talossim/machine.go |  | ApplyConfiguration counts an applied config but does not parse it: applying a config that sets a hostname does not change what Hostname reports. Server.SetHostname/SetVersion give a scenario the same effect explicitly. | open |  | 2026-08-29T05:01:33.912Z |  |
| 4 | 02 | stub | internal/talossim/stream.go |  | Events emits an identical MachineStatusEvent{Stage: RUNNING} payload per message; the event stream is not driven by the node's actual state transitions. Correlating events with Bootstrap/Reboot/Reset belongs to plan 02-03's scenario engine. | open |  | 2026-08-29T05:01:34.036Z |  |
| 5 | 02 | unrun-verify | internal/imagefactory/live_test.go |  | TestLiveFactory ist der einzige Drift-Waechter gegen factory.talos.dev, ist opt-in und wird von nichts geplant; factory.talos.dev hat in dieser Sitzung nachweislich gedrosselt, ein Retry fehlt. | open |  | 2026-08-29T05:30:01.609Z |  |
| 6 | 02 | unrun-verify | docs/api-contract.md |  | golangci-lint run could not be executed on this host (binary not installed); go vet and gofmt are clean. Plan 02-06 task acceptance criterion 'golangci-lint run exits 0' is unverified. | fixed | Not installed on the subagent PATH, but present at ~/go/bin/golangci-lint (installed during plan 02-01 at the version CI pins). Orchestrator ran it at the wave-3 gate with that path exported: 0 issues. | 2026-08-29T06:53:45.252Z | 2026-08-29T09:05:00.000Z |
| 7 | 02 | deviation | internal/talossim/scenario_conn.go |  | ip_changes_on_reboot: rebind() and Reboot() carried comments claiming established connections survive the reboot so the Reboot reply is delivered, while closeListener severs them. Plan 02-05 corrected the comments rather than the behaviour (severing is what makes the address change observable to an already-connected client), but the simulator is now harder to satisfy than hardware for this one RPC: talosctl reboot does return a reply. Closing it properly means delivering the reply and severing after a short grace, which needs a scenario-owned goroutine. | open |  | 2026-08-29T07:54:43.845Z |  |
| 8 | 02 | todo | internal/imagefactory/client.go | 103 | [WR-01] WithHTTPClient silently discards the configured timeout. Code review 02-REVIEW.md, scoped out of the phase-2 fix pass. | fixed |  | 2026-08-29T12:40:00.000Z | 2026-09-03T19:57:57.830Z |
| 9 | 02 | todo | internal/imagefactory/client.go | 312 | [WR-05] One additive upstream field takes the whole Images screen down — DisallowUnknownFields on the catalog read turns an upstream addition into a 502. Verifier confirmed still present and still touching a phase-2 success criterion. | open |  | 2026-08-29T12:40:00.000Z |  |
| 10 | 02 | todo | web/src/routes/images.tsx | 455 | [WR-06] A cleared META key silently becomes slot 0 via Number(''); out-of-range keys surface as a raw decoder error. Verifier confirmed still present and still touching a phase-2 success criterion. | open |  | 2026-08-29T12:40:00.000Z |  |
| 11 | 02 | todo | internal/talossim/talossim.go | 183 | [WR-07] talossim.New leaks both listeners when seeding fails. Test infrastructure, not production code. | open |  | 2026-08-29T12:40:00.000Z |  |
| 12 | 02 | todo | internal/talos/client.go | 357 | [CR-04] The stream error path never classifies: RecvMsg calls s.cancel() then classify(s.Context(), ...), and classify returns the error untouched when ctx.Err() is context.Canceled — so KindUnreachable/KindRejected are unreachable on streams. Found by the fixer while writing the WR-02 tests, confirmed independently by the verifier. Lands on Phase 5, which consumes streams. | open |  | 2026-08-29T12:40:00.000Z |  |
| 13 | 02 | todo | internal/httpapi/handlers/schematics.go | 66 | [IN-01] schematicInput.Cluster is accepted, unvalidated and never sent. | open |  | 2026-08-29T12:40:00.000Z |  |
| 14 | 02 | todo | web/src/routes/images.tsx | 927 | [IN-02] CopyButton never returns to its resting label. | open |  | 2026-08-29T12:40:00.000Z |  |
| 15 | 02 | todo | internal/imagefactory/client.go | 85 | [IN-03] The installer repository cache never expires. | fixed |  | 2026-08-29T12:40:00.000Z | 2026-08-29T21:31:38.594Z |
| 16 | 02 | todo | internal/httpapi/handlers/schematics.go | 205 | [IN-04] Raw JSON decoder errors reach the client. | open |  | 2026-08-29T12:40:00.000Z |  |
| 17 | 02 | todo | internal/imagefactory/imagefactory.go | 37 | [IN-05] DefaultBaseURL is documented as overridable but nothing overrides it. | open |  | 2026-08-29T12:40:00.000Z |  |
| 18 | 02 | todo | internal/httpapi/handlers/schematics.go | 156 | [IN-06] NewestStable's reason is discarded entirely. | open |  | 2026-08-29T12:40:00.000Z |  |
| 19 | 02 | unrun-verify | Taskfile.yml |  | Plan 02-11 could not run 'task lint:go' (golangci-lint run): golangci-lint is not installed on this host. gofmt -l and go vet are clean. Same gap as the 02-06 entry. | fixed |  | 2026-08-29T17:10:58.006Z | 2026-08-29T17:16:43.845Z |
| 20 | 02 | deviation | internal/imagefactory/installer.go |  | A cold resolveInstallerRepo still walks two candidates serially at DefaultTimeout=30s each, so GET /schematics/{id}/assets keeps a 2x30=60.000s worst case against writeTimeout=60s -- the composition G-02-2 measured as status=502 duration=1m0.002907792s, unchanged by plan 02-12. The bounded re-question that plan adds asks only the never-ruled-out candidate, costs at most 1x30s and cannot reach that ceiling. Bounding the cold path needs the per-route deadline 02-DECISION-probe-budget.md owns; G-02-2 is deferred to cluster A, and cmd/holzkubed/budget_test.go declares the route as known-over-budget. | fixed |  | 2026-08-29T17:27:33.353Z | 2026-09-03T19:57:57.966Z |
| 21 | 02 | deviation | internal/model/model.go |  | model.Schematic.Arch is additive and unversioned, so every schematic stored before plan 02-13 carries an empty architecture and renders its verdict unqualified forever; those are precisely the records created while the G-02-8 arch leak was live, and the architecture a past probe used is not recoverable from the record, so there is nothing to backfill -- new records are qualified, old ones are readable only by deleting and re-authoring. | open |  | 2026-08-29T17:51:17.158Z |  |
| 22 | 02 | todo | internal/imagefactory/installer.go | 244 | [R2-WR-02] No single-flight: every concurrent request on a cold or stale installer-repo key issues its own registry resolution. Round-2 code review, scoped out by the user. Also the only fix that would make two concurrent callers agree on one reference in the mirror ordering storeInstallerRepo does not close (see its doc comment). | open |  | 2026-08-29T21:31:26.855Z |  |
| 23 | 02 | todo | internal/httpapi/handlers/schematics_test.go | 1195 | [R2-WR-06] The provisional-warning branch of GET /assets has no handler-level test: the warning is asserted in imagefactory and rendered in the web suite, but nothing pins that it survives the handler. Round-2 code review, scoped out by the user. | fixed |  | 2026-08-29T21:31:27.059Z | 2026-08-30T09:59:55.342Z |
| 24 | 02 | todo | internal/imagefactory/installer.go | 360 | [R2-IN-01] The raw transport error is echoed verbatim into the operator-facing fallback warning detail, which is long-lived because it rides every cached answer. Via probe.go:104. Round-2 code review, scoped out by the user. | open |  | 2026-08-29T21:31:27.326Z |  |
| 25 | 02 | todo | docs/api-contract.md | 646 | [R2-IN-02] The SecureBoot assets example omits the warnings field that the same section calls mandatory. Round-2 code review, scoped out by the user. | open |  | 2026-08-29T21:31:27.604Z |  |
| 26 | 02 | todo | docs/api-contract.md | 679 | [R2-IN-03] The contract describes the provisional installer outcome more narrowly than the code produces it. Round-2 code review, scoped out by the user. | open |  | 2026-08-29T21:31:27.863Z |  |
| 27 | 02 | todo | web/src/api.ts | 283 | [R2-IN-04] WARNING_INSTALLER_REPO_FALLBACK_UNVERIFIED is exported and pinned by the Go drift guard but unused by application code. Round-2 code review, scoped out by the user. | open |  | 2026-08-29T21:31:28.120Z |  |
| 28 | 02 | todo | web/src/routes/images.tsx | 100 | [R2-IN-05] Two small dead or silent branches in the images route, also at images.tsx:579. Round-2 code review, scoped out by the user. | open |  | 2026-08-29T21:31:28.399Z |  |
| 29 | 02 | todo | internal/httpapi/handlers/schematics.go | 66 | [R2-RESIDUAL] A lone surrogate submitted through the API rather than the browser is still silently rewritten to U+FFFD by encoding/json before the schematic id is computed, so a non-browser caller gets an id over a character it never sent. Commit ec10e08 refused it at the input, which makes the utf8.ValidString branch unreachable from the HTTP route but leaves API clients exposed. T-02-67. The honest fix is a raw-bytes check in schematicInput.validate before decoding; widening the canonical serialiser would move the precomputed id FACT-06 rests on. | fixed |  | 2026-08-29T21:31:28.635Z | 2026-08-30T09:59:55.616Z |
| 30 | 02 | unrun-verify | internal/imagefactory/live_test.go |  | The SecureBoot installer matrix is a partial observation and the ledger never said so. 02-09-PLAN.md:447 required conjunctively that a throttled live run be recorded AND that this entry be appended; it throttled (first run: metal-installer and metal-installer-secureboot at v1.12.0 both exceeded the 60s client timeout without an HTTP response, the SecureBoot pairing subtest failed the same way after 235s) and the entry was never filed, so a ship gate reading the register learned none of the following. (1) The matrix is two versions wide: v1.13.9, the pin, and v1.12.0, MinSupportedVersion. (2) talos.MaxSupportedVersion is v1.14, a range bound and not a concrete tag, so it has never been probed and cannot be probed as one; live_test.go:499-517 says so in place. (3) No v1.13.x below the pin has ever been probed, and the Factory's newest stable inside v1.12..v1.14 is the pin itself, so no concrete version exists in that window to probe today. (4) The fake's v1.9.0 SecureBoot cell remains an assumption labelled as one. Filed by plan 02-21 closing G-02-21; WINDOWS entry 5 (the throttle itself) stays open independently. See also the version-range entry handed over by plan 02-16. | open |  | 2026-08-30T09:59:13.016Z |  |
| 31 | 02 | deviation | internal/model/model.go |  | SUPERSEDES THE RECOVERABILITY CLAUSE OF ENTRY 21, which stays open for the window itself (the tool has append/fixed/waive and no amend, so a correction has to be a new entry). Entry 21 says flatly that 'the architecture a past probe used is not recoverable from the record'. That is true of a record whose probe succeeded or never answered, and FALSE of one it refused: a refusal stores the architecture verbatim in probe_reason, in the shape '<id> at <version>/<arch> answered HTTP <status>' produced by imagefactory/probe.go and now pinned by handlers.TestRefusalReasonNamesTheArchitectureItAskedAbout, so it is machine-parseable out of that sentence. The claim is narrowed and not inverted: recovering an architecture that way means parsing prose written for an operator to read, which is weaker and more fragile than a field and one rewording away from being wrong, and nothing does or should. The window entry 21 records is unchanged -- pre-02-13 records still render unqualified verdicts, and there is still nothing to backfill, because a refused record has no usable verdict to qualify. Filed by plan 02-21 closing G-02-22. | open |  | 2026-08-30T09:59:13.303Z |  |
| 32 | 02 | unmet-truth | docs/api-contract.md |  | [from 02-14] The refusal set stated in docs/api-contract.md is a floor, not a measurement. Six regions are refused by extrapolation from measured ends rather than by observation: twenty-six of the thirty-two C1 codepoints, the interior of the range above U+FFFF, U+FDD1-U+FDEF, the leading and trailing positions for all but three groups, and the surrogates. The contract states the set as if every member had been observed refused by the Factory. | open |  | 2026-08-30T09:59:13.572Z |  |
| 33 | 02 | stub | web/src/routes/images.tsx |  | [from 02-14, coverage item D7] images.tsx under-refuses relative to the server: hasControlCharacter and the doc comment above it claim to transcribe the server's representable set, but the guard stops only runes below U+0020, U+007F and lone surrogates. An operator typing an emoji, a BOM or U+2028 into a kernel argument learns of it from the server's 400 instead of from the row while they are still looking at it. Behaviour is safe (the 400 is the documented backstop). | fixed |  | 2026-08-30T09:59:13.819Z | 2026-08-30T09:59:55.875Z |
| 34 | 02 | unmet-truth | Taskfile.yml |  | [from 02-14 item 3, SUPERSEDED-AS-WRITTEN, and from 02-16] golangci-lint is NOT absent from this host, and two plans recorded that it was. It is installed at ~/go/bin/golangci-lint (2.13.1, the version CI pins) and is merely off the default PATH, so a bare 'golangci-lint run' or 'task lint:go' fails with command-not-found and reads as a missing tool. That misreading has now produced two unrun-verify entries (6 and 19, both since marked fixed) and one SUMMARY claiming a permanent tooling gap, which invites the next plan to skip the Go lint gate for a reason that is not true. What is filed here is the PATH trap, not an absent tool: 02-14's entry as drafted would have re-filed the falsehood. The repo lints clean at 0 issues with that path exported. Open until the lint task resolves the binary rather than depending on the caller's PATH. | open |  | 2026-08-30T09:59:14.049Z |  |
| 35 | 02 | unrun-verify | internal/imagefactory/canonical_live_test.go |  | [from 02-14] TestLiveCanonical is opt-in and runs against a public service; nothing in CI executes it. A clean run is 16 POSTs to factory.talos.dev. The class it guards is invisible to the offline suite because the fake accepts documents the real Factory refuses, so the offline suite passing says nothing about canonicalisation drift. | open |  | 2026-08-30T09:59:14.271Z |  |
| 36 | 02 | skipped-test | internal/talossim/scenario_test.go | 311 | [from 02-14] TestScenarioGoSilent flaked once under -race and passed on re-run. Not investigated; it was out of 02-14's scope. It asserts that the client's own deadline is what fires, which is a timing assertion, so a flake there is either a real race or a machine-load artefact and the entry does not claim to know which. | open |  | 2026-08-30T09:59:14.503Z |  |
| 37 | 02 | deviation | internal/imagefactory/canonical.go |  | [from 02-14] Schematics stored before plan 02-14 are not re-validated against the codepoints that plan started refusing. Any record whose kernel_args or meta carried one holds an id the Factory did not assign. There is no re-validation pass and no migration, nothing enumerates such records, and the Factory will not enumerate schematics either. Same shape as WINDOWS entry 21 and as the name/cluster residual handed over by plan 02-20 -- different fields, same unmigratable population. | open |  | 2026-08-30T09:59:14.817Z |  |
| 38 | 02 | lint-warning | internal/imagefactory/canonical_live_test.go | 627 | [from 02-16] QF1012 staticcheck: WriteString(fmt.Sprintf(...)) should be fmt.Fprintf(...). Pre-existing from plan 02-14, unchanged since bc9be70. golangci-lint run does not exit 0 until it is fixed, and plan 02-16 was prohibited from editing the file. | fixed |  | 2026-08-30T09:59:15.074Z | 2026-08-30T09:59:56.138Z |
| 39 | 02 | deviation | internal/imagefactory/warnings.go |  | [from 02-16] warnings.go was edited outside plan 02-16's declared files_modified, to declare installer.secureboot-repo-fallback-unverified. Unavoidable for the option that plan chose; recorded so the file set a later reader reconstructs from the plan frontmatter is known to be incomplete. | open |  | 2026-08-30T09:59:15.347Z |  |
| 40 | 02 | todo | web/src/api.ts |  | [from 02-16] The installer.secureboot-repo-fallback-unverified warning code had no TypeScript mirror constant and no row in the contract's warning table when 02-16 introduced it. Handed to plan 02-19 task 4. The contract calls the warning taxonomy closed while it had gained a fourth member unannounced. | fixed |  | 2026-08-30T09:59:15.606Z | 2026-08-30T09:59:56.462Z |
| 41 | 02 | unmet-truth | internal/imagefactory/live_test.go |  | [from 02-16] The installer matrix has still never probed any v1.13.x below the pin, and v1.14.x does not exist yet. installerCandidates' comment now says this explicitly rather than claiming the range is settled, and live_test.go's third row will probe v1.14.x automatically once upstream ships it -- so the gap closes itself on that side and does not on the other. Distinct from the entry plan 02-21 filed for G-02-21: that one records that 02-09's acceptance criterion went unfiled, this one records the version range itself and the self-updating row. | open |  | 2026-08-30T09:59:15.890Z |  |
| 42 | 02 | unmet-truth | web/src/routes/images.browser.test.tsx |  | [from 02-17] Only one browser engine is measured. UAX #14's hyphen rule is a specification and every engine implements it, but the fix is a CSS rule and the measurement is one engine's: Chromium, via playwright 1.62.1, headless, at a 1200x900 viewport. Firefox and WebKit are unmeasured. Severity low -- white-space: nowrap is not a corner of CSS where engines disagree -- but the claim 'occupies one line box' is true of Chromium and inferred elsewhere. | open |  | 2026-08-30T09:59:16.242Z |  |
| 43 | 02 | todo | internal/imagefactory/guard_drift_test.go | 138 | [from 02-17, residual 2] INSTALLER_REPOSITORY_NAMES was not pinned to installerCandidates: a fifth Go candidate would leave the web sweep measuring four stale strings and still passing -- a smaller instance of exactly the defect 02-17 closed. Marked open until plan 02-20 landed the guard. | fixed |  | 2026-08-30T09:59:16.507Z | 2026-08-30T09:59:56.712Z |
| 44 | 02 | unrun-verify | .github/workflows/ci.yml | 71 | [from 02-17] The CI browser-install step has never run. Its form and ordering are verified by reading the file and its local equivalent was exercised, but no CI run has happened against this commit, and 'playwright install --with-deps chromium' on ubuntu-latest is untested. The '--' argument-forwarding bug 02-17 fixed in that same line is precisely the class of failure only a real run confirms is gone. | open |  | 2026-08-30T09:59:16.753Z |  |
| 45 | 02 | deviation | internal/httpapi/problem.go |  | [from 02-18] Wire-format change to a field the contract calls stable: every problem type moved from https://holzkube.dev/problems/<suffix> to urn:holzkube-manager:problem:<suffix>. A client matching on type rather than on code breaks. The project has no external clients today, which is why this is a note and not a migration -- the note is the difference between having decided that and not having noticed it. Matches T-02-90 (Repudiation, disposition accept). | open |  | 2026-08-30T09:59:17.027Z |  |
| 46 | 02 | deviation | internal/httpapi/middleware/audit_test.go | 126 | [from 02-18] One fixture will not follow the next re-rooting automatically: audit_test.go:126 spells the problem-type base literally, by necessity (import cycle) and by intent (it stands in for an upstream writer). A future re-rooting must edit it by hand, and nothing fails first if it is forgotten, because the middleware never reads the field. | open |  | 2026-08-30T09:59:17.262Z |  |
| 47 | 02 | deviation | docs/api-contract.md |  | [from 02-18] The URN namespace identifier holzkube-manager is unregistered with IANA. Documented in both problem.go and docs/api-contract.md as acceptable -- RFC 9457 asks for a URI, not a registered namespace -- and recorded here so the decision is visible rather than assumed. | open |  | 2026-08-30T09:59:17.526Z |  |
| 48 | 02 | deviation | internal/imagefactory/installer.go |  | [from 02-19] READ WITH ENTRY 20, WHICH STAYS OPEN. Plan 02-19 changed what an unresolved installer RETURNS (502 with no body became 200 with installer:null plus installer_error), not the cold 2 x 30s serial candidate walk that produces it. The 49.8s and 60.002s observations recorded in G-02-15 are that composition and are unchanged. 'The panel now shows four references on a timeout' must not be read as the timeout having been addressed; the ceiling against writeTimeout=60s is still owned by 02-DECISION-probe-budget.md, and the milder symptom makes deferring that decision easier rather than more defensible. | fixed |  | 2026-08-30T09:59:17.754Z | 2026-09-03T19:57:58.102Z |
| 49 | 02 | deviation | docs/api-contract.md |  | [from 02-19] The assets route's response shape changed for two outcomes: 502 with no body became 200 with installer:null plus installer_error, so installer is now nullable on every answer. No external clients exist, which is why this is a note and not a migration. Matches T-02-91/T-02-92. | open |  | 2026-08-30T09:59:18.030Z |  |
| 50 | 02 | todo | docs/api-contract.md | 646 | [from 02-19] Entries 25 and 26 (R2-IN-02, R2-IN-03) sit inside the assets section plan 02-19 rewrote and were deliberately left alone: both are marked 'scoped out by the user', and a scoping decision is not an executor's to overturn. Entry 25's SecureBoot example still omits the warnings field the same section calls mandatory, and it now sits beside a table enumerating what warnings means in every outcome, so the inconsistency is more visible than before. Worth re-offering to the user rather than closing silently. | open |  | 2026-08-30T09:59:18.285Z |  |
| 51 | 02 | todo | web/src/api.ts | 344 | [from 02-19] AMENDS ENTRY 27, WHICH STAYS OPEN (no amend verb exists). Entry 27 records WARNING_INSTALLER_REPO_FALLBACK_UNVERIFIED as exported, pinned by the Go drift guard and unused by application code. Still true, and now true of a second constant: WARNING_INSTALLER_SECUREBOOT_REPO_FALLBACK_UNVERIFIED is also unused, because SchematicWarnings renders code/detail generically with no per-code branch. That is the intended design -- an unknown code still reaches the operator -- so the entry is about the constants' purpose being drift-pinning rather than rendering. Re-word rather than close. | open |  | 2026-08-30T09:59:18.604Z |  |
| 52 | 02 | todo | internal/httpapi/handlers/schematics.go | 66 | [from 02-20] AMENDS ENTRY 13, WHICH STAYS OPEN (no amend verb exists). Entry 13 reads 'schematicInput.Cluster is accepted, unvalidated and never sent'. The UNVALIDATED half is closed: cluster now goes through NotRepresentableReason in validate, and a refused codepoint in it is a 400 naming cluster. The NEVER SENT half is unchanged and still true -- cluster is stored on the record, reaches no other layer, and the authoring form offers no input for it. Amended text: 'schematicInput.Cluster is validated and stored but never sent anywhere and has no form input.' | open |  | 2026-08-30T09:59:18.930Z |  |
| 53 | 02 | deviation | internal/httpapi/handlers/schematics.go |  | [from 02-20] Records stored before plan 02-20 may carry refused codepoints in name or cluster. Nothing migrates them and there is no backfill to write, because the values are the operator's own text and repairing them would be the silent rewrite that plan exists to prevent. Such a record is readable but not re-creatable, and there is no edit route -- POST, GET, GET /{id}, GET /{id}/assets and DELETE /{id} are the whole surface -- so an operator holding one must delete and re-author it. The saved table renders it safely in the meantime via StoredText, which is why this is a residual and not a defect. | open |  | 2026-08-30T09:59:19.158Z |  |
| 54 | 02 | todo | web/src/routes/images.tsx | 329 | [from 02-20] The client cannot guard cluster because the authoring form has no cluster input; the server does guard it. When a cluster input is added it belongs in the single hasUnusableValue computation in images.tsx, which already carries a comment saying so. | open |  | 2026-08-30T09:59:19.461Z |  |
| 55 | 02 | todo | internal/httpapi/handlers/schematics.go |  | [from 02-20] createProblem's NotRepresentableError branch is now unreachable from the HTTP route in practice: refuseUnrepresentable covers every document path the request vocabulary names, so the branch fires only for a future field that forgets a check, or for an in-process caller. It is kept deliberately -- deleting it would turn the first of those into a 502 blaming the Factory, which is G-02-6 -- but it is a branch no route-level test can reach, the same shape as the utf8.ValidString note that produced entry 29. | open |  | 2026-08-30T09:59:19.676Z |  |
| 56 | 02 | unrun-verify | internal/imagefactory/guard_drift_test.go | 65 | [from 02-20] The codepoint sweep in guard_drift_test.go is exhaustive over Unicode but the SERVER set behind it is not exhaustively measured; it inherits 02-14's extrapolation (U+FDD1-U+FDEF, twenty-six C1 codepoints, and the interior of the range above U+FFFF are refused on the strength of measured ends). The guard proves the two layers agree; it cannot prove the set is right. Extends the 02-14 floor entry rather than starting a new claim. | open |  | 2026-08-30T09:59:19.914Z |  |
| 57 | 02 | deviation | internal/imagefactory/installer.go |  | [from 02-24] SUPERSEDES ENTRIES 20 AND 48, BOTH NOW MARKED FIXED (no amend verb exists). What moved: resolveInstallerRepo no longer walks its candidates serially -- plan 02-23 issues every candidate at once on a context derived from the caller's and decides in declared candidate order, so the route's worst case is one candidate's budget rather than the sum. The candidates are bounded by imagefactory.ManifestTimeout=30s (plan 02-22) and not by DefaultTimeout; the manifest GET and the ISO probe have budgets of their own (ManifestTimeout=30s, ProbeTimeout=90s); both Factory routes declare a shared route deadline (handlers.AssetsRouteBudget = ManifestTimeout + 5s = 35s, handlers.CreateRouteBudget = ProbeTimeout + DefaultTimeout = 120s); writeTimeout rose from 60s to 130s and now names the budgets it covers; and cmd/holzkube-managerd/budget_test.go computes the composition from the four constants and declares BOTH Factory routes withinBudget with an empty deferredTo (assets: 30s sum, largest call 30s, route deadline 35s, slack 5s, writeTimeout 2m10s; create: 2m30s sum, largest call 1m30s, route deadline 2m0s). Entry 20's '2x30 = 60.000s worst case against writeTimeout=60s' has no surviving true reading. Marked fixed on exactly that evidence and no other: the composition guard passes and the offline elapsed-time tests pass (TestAssetsRouteAnswersInsideItsCeiling answered in 603.727542ms against a 700ms scaled ceiling; TestCreateRouteAnswersInsideItsCeiling in 623.888083ms against 2.4s). What did NOT move: the 43.42s and 60.002907792s figures entries 20 and 48 record are what the SERIAL walk cost and remain the correct record of it -- they now live in a comment beside installerRepoRetryInterval in internal/imagefactory/installer.go, measured 2026-08-29, so a reader who greps for those numbers finds why they are still written down rather than assuming they are stale. NOTHING re-measured the installer resolution against factory.talos.dev after the change: plan 02-22's two live runs on 2026-09-03 measured the ISO probe (cold 28.445563875s and 31.491771083s, warm 3.752628125s and 2.493417375s) and not the installer resolution. The concurrency improvement is measured offline against fakes and unmeasured against factory.talos.dev -- 305.96ms against 606.93ms serialised at the client, 603.57ms against 1.0599s serialised at the HTTP route. Entry 48's warning survives into this entry: a route that declares a ceiling is not the same as a route that is fast. | open |  | 2026-09-03T19:57:04.257Z |  |
| 58 | 02 | deviation | internal/httpapi/handlers/schematics.go |  | [from 02-24] G-02-9 REMAINS OPEN AFTER ROUND 4. Plan 02-24 gives it the mitigation 02-DECISION-probe-budget.md's ratified Option 2 asks for and no more: the store.ErrConflict branch of POST /api/v1/schematics now writes the probe verdict that request just computed -- Usable, ProbedAt and ProbeReason and no other field, under a compare-and-swap on the read Rev -- instead of discarding it. G-02-9 measured the discard as HTTP 409 in 4.186898875s, a probe that ran and succeeded at that latency against a record that stayed usable=false with probed_at zero. What is still missing, which is why this is a window and not a closure: there is no re-probe route, no button and no job. The recovery is re-submitting the identical customisation, which answers 409 and refreshes the verdict as a side effect. That recovery is not discoverable from the saved list, it is surfaced to the operator as an error on the create form rather than as an action, and it is DECLINED in two cases -- when the stored record's Arch is not the architecture the fresh probe asked about (a record written before Arch existed carries an empty one and is therefore never refreshed), and when the fresh probe does not answer either. A probe that times out again leaves the record exactly as it was, which is 02-DECISION-probe-budget.md's own sentence and the reason it recorded G-02-9 as a known window rather than as fixed. The structural answer is that document's Option 1 -- split the create route, add a third probe state, add an explicit re-probe endpoint -- which the user did not take. | open |  | 2026-09-03T19:57:30.343Z |  |
| 59 | 02 | deviation | cmd/holzkube-managerd/main.go | 64 | [from 02-22, filed by 02-24] writeTimeout was raised from 60s to 130s process-wide rather than scoped per route, so every route on this server carries the response budget only the two Factory routes need. The per-route alternative requires Flush, Unwrap and Hijack on the three middleware ResponseWriter wrappers, which is exactly the Phase 5 entry blocker recorded in 02-CONTEXT.md <deadline_policy> (lines 256-271: none of the three wrappers implements them, so a streaming endpoint on this chain silently buffers). Cross-referenced here so Phase 5 finds this rather than re-deriving it. | open |  | 2026-09-03T19:57:30.475Z |  |
| 60 | 02 | deviation | internal/httpapi/handlers/schematics.go | 58 | [from 02-22, filed by 02-24] CreateRouteBudget = ProbeTimeout + DefaultTimeout = 120s is deliberately smaller than the sum of its route's three per-call budgets, which is DefaultTimeout 30s (Extensions) + DefaultTimeout 30s (CreateSchematic) + ProbeTimeout 90s (ProbeBuildable) = 150s. On a pathological upstream the probe is therefore cut by the route deadline before its own budget expires, and the clipping is exactly one JSON budget wide. Recorded rather than fixed because the outcome is the existing fail-safe: ProbedAt stays zero and the record is stored unprobed, which reads as 'no verdict' and never as 'the Factory refused'. The composition guard logs both numbers on every run (2m30s sum against a 2m0s route deadline). | open |  | 2026-09-03T19:57:30.611Z |  |
| 61 | 02 | deviation | internal/httpapi/middleware/audit.go |  | [from 02-24] The audit middleware derives its outcome from the response status (audit.go lines 85-105), so a 409 from POST /api/v1/schematics that REFRESHED the record's three probe fields is filed as OutcomeError with cause store.conflict -- byte-identical to a 409 that changed nothing. There is no handler-side enrichment and this plan added none. The sentence an auditor needs: the permanent archive does not show the refresh, so a record whose Usable, ProbedAt and ProbeReason changed on a conflicting POST has no archive entry saying so, and the only evidence is the record itself. Left as it is deliberately (T-02-112, disposition accept): changing it reopens Phase 1's fail-closed intent/outcome contract, which is not this round's to touch. | open |  | 2026-09-03T19:57:30.743Z |  |
| 62 | 02 | todo | web/src/routes/images.tsx |  | [from 02-23, filed by 02-24] The progress indicator .planning/research/PITFALLS.md:164 requires is still not built, and stating a ceiling is a different thing. Missing by name: elapsed time, the current sub-step, an expected duration, and a disabled action button carrying its reason. PITFALLS.md:646 names 'A spinner during the post-apply install window' as the anti-pattern and 'Named state, elapsed time, expected duration, current sub-step' as the approach. What plan 02-23 shipped is one static sentence per waiting state naming the server's own ceiling -- CREATE_WAIT_SECONDS=120 and ASSETS_WAIT_SECONDS=35 in web/src/routes/images.tsx -- which is a maximum rather than a prediction, and the create button is disabled while pending with no reason rendered beside it. The comment beside those two constants says so in the code, so a reader who finds a number there cannot mistake it for the requirement having been met. 02-DECISION-probe-budget.md names this as the risk its ratified Option 2 accepts. | open |  | 2026-09-03T19:57:30.876Z |  |
| 63 | 02 | todo | internal/imagefactory/client.go | 103 | [from 02-24] SUPERSEDES ENTRY 8, WHICH IS NOW MARKED FIXED. The fixed verb carries no reason, so the reason lives here. Entry 8 reads '[WR-01] WithHTTPClient silently discards the configured timeout'. That was true while the budget lived on http.Client.Timeout: the option shallow-copied the caller's client and the copy's zero Timeout overwrote whatever WithTimeout had set, silently and order-dependently. Plan 02-22 moved the three budgets off http.Client.Timeout entirely and applies them per call from the Client, so there is no client-wide timeout left to discard -- WithHTTPClient now clears the copy's Timeout explicitly, and TestClientCarriesNoClientWideTimeout asserts the zero duration both for a default client and for one built through WithHTTPClient with a 7s timeout set. The discarding is gone and the order-dependence with it. This entry is a record and not a defect, and is marked fixed on creation for that reason. | fixed |  | 2026-09-03T19:57:48.980Z | 2026-09-03T19:57:58.235Z |
| 64 | 02 | unrun-verify | internal/imagefactory/live_test.go |  | [from 02-24] SUPERSEDES ENTRY 5, WHICH STAYS OPEN (no amend verb exists). Entry 5 records that TestLiveFactory is the only drift guard against factory.talos.dev, is opt-in, and is planned by nothing. All three of those are unchanged: this round did not make it non-optional and did not schedule it. What DID change, from plan 02-22: TestLiveFactory now bounds the elapsed time of the probe it runs rather than asserting only err == nil; it measures a genuinely cold probe behind a per-run nonce, so each run authors a schematic the Factory has demonstrably never built; it re-applies ProbeTimeout's derivation rule to the widened sample of seven cold observations (28.45, 30.50, 30.59, 31.18, 31.52, 31.49, 32.69 -- slowest 32.69s, doubled 65.38s, rounded up to 90s, which is the shipped constant), so the next observation that would move the constant arrives as a red test; and a throttled cold measurement now Skipf's with 'NOT OBSERVED' rather than passing, so a run that measured nothing can no longer read as a run that measured something. | open |  | 2026-09-03T19:57:49.113Z |  |
| 65 | 02 | todo | cmd/holzkube-managerd/budget_test.go |  | [from 02-23, filed by 02-24] Known limitation of the composition guard, recorded as a limitation and not as a defect. Relisting the assets row to one declared call while leaving AssetsRouteBudget at two manifest budgets computes uncut against a declared uncut and passes both the R1 and R2 ratchets, so the guard stays green on a route over-provisioned by thirty seconds. Driven locally by plan 02-23 and confirmed green. Tolerated because the failure direction it leaves open is a ceiling that is too generous rather than one that cuts a candidate before it can answer, which is the silent fallback G-02-3 already cost this phase once. Teaching R4 to catch it would require the guard to know how many candidates the handler actually issues, which is exactly the derived count the table's own comment argues against. | open |  | 2026-09-03T19:57:49.247Z |  |
| 66 | 02 | deviation | internal/httpapi/handlers/schematics.go |  | [from 02-26, uebergeben von Plan 02-25] AMENDS ENTRY 58, WHICH STAYS OPEN (no amend verb exists). Eintrag 58 zaehlt die ablehnenden Faelle von `refreshTheStoredVerdict` auf und nennt zwei: die Probe hat nicht geantwortet, und die Architektur stimmt nicht. Seit Plan 02-25 sind es drei — die dritte ist die Talos-Version (`internal/httpapi/handlers/schematics.go`, `if stored.TalosVersion != fresh.TalosVersion`), aus demselben Grund wie die Architektur, weil `Schematic.Canonical()` weder das eine noch das andere emittiert; gemessen hat es Runde 4 der Verifikation (`02-VERIFICATION.md`, `gaps[0]`, `adversarial_checks_run[0]`). Was 58 weiterhin korrekt aufzeichnet, bleibt korrekt: **G-02-9 ist offen**, es gibt weiterhin keine Re-Probe-Route, keinen Button und keinen Job, und die Erholung ist eine Wieder-Einreichung, die als Fehler beantwortet wird. Ueberholt ist allein die Aufzaehlung der Bedingungen. | open |  | 2026-09-04T05:59:09.533Z |  |
| 67 | 02 | deviation | internal/httpapi/handlers/schematics.go |  | [from 02-26, uebergeben von Plan 02-25] Plan 02-24 hat den von `02-DECISION-probe-budget.md` ratifizierten Refresh mit zwei Bedingungen ausgeliefert. Die dritte fehlte, und keine Zeile in `.planning/WINDOWS.md` hat sie bis Runde 4 getragen. Runde 4 der Verifikation hat sie reproduziert statt sie zu erschliessen (`02-VERIFICATION.md`, `adversarial_checks_run[0]`): `siderolabs/cross-version-ext` bei `v1.12.0` authored, wo der Image-Endpunkt ihn ablehnt — Datensatz `talos_version v1.12.0, usable=false, probe_reason "... at v1.12.0/amd64 answered HTTP 400"` —, dann die Ablehnung aufgehoben und dieselbe Customisation bei `v1.13.9` erneut gepostet. Ergebnis: dieselbe Id, HTTP 409, und der gespeicherte Datensatz wurde zu `talos_version: v1.12.0, usable: true, probe_reason: ""`, rev 1 -> 2. Die wahre v1.12.0-Ablehnung wurde geloescht. Ursache: `Schematic.Canonical()` emittiert weder Architektur noch Version, also sind zwei Versuche, die sich nur in `talos_version` unterscheiden, ein Datensatz. Geschlossen von Plan 02-25 (Commits `45420ee`, `d4ee1f5`, `846d540`, `061b906`); die Reproduktion laeuft seither als `TestConflictAtAnotherTalosVersionErasesNoStoredRefusal`. | fixed |  | 2026-09-04T05:59:17.534Z | 2026-09-04T05:59:33.098Z |
| 68 | 02 | deviation | .planning/WINDOWS.md |  | [from 02-26, uebergeben von Plan 02-25] **Task 2 von Plan 02-25 hat keine verbleibende Ungenauigkeit gefunden.** Ausdrueckliche Feststellung, kein Ausbleiben einer Pruefung: die Vorpruefung des Tests auf eine echte gespeicherte Ablehnung (`usable: false` und `probe_reason` nennt `otherCatalogVersion`) laeuft und bricht ab, wenn sie nicht zutrifft; die Byte-Identitaet wird mit `marshalRecord` ueber den ganzen Datensatz geprueft und nicht mit `exceptTheProbeFields`; und `web/src/api.ts:203-209` und `:216-219` sowie das Badge `UsabilityVerdict` (`web/src/routes/images.tsx:942-944`) wurden geprueft und sind nach dieser Korrektur weiterhin wahr — `api.ts` nennt die Bedingungen des Refresh gar nicht, sondern verweist auf `docs/api-contract.md`, und das Badge sagt von selbst die Wahrheit, weil der Server keine Unwahrheit mehr speichert. Keine Datei unter `web/` wurde beruehrt. | open |  | 2026-09-04T05:59:17.668Z |  |
| 69 | 02 | deviation | internal/imagefactory/guard_drift_test.go | 49 | [from 02-26] Der Browser-Ablehnungs-Waechter war zwischen Plan 02-20 und dieser Runde an keinen Bezeichner verankert: `browserRefusalRange` (`internal/imagefactory/guard_drift_test.go:49-50`) ist ein regulaerer Ausdruck auf `{ from: 0x.., to: 0x.. }`, und `browserRefusalRanges` liess ihn ueber die **ganze** `web/src/routes/images.tsx` laufen. Gemessen in Runde 4 der Verifikation (`02-VERIFICATION.md`, `adversarial_checks_run[1]`, `gaps[1]`; `02-REVIEW.md` WR-04): `REFUSED_RANGES` durchgaengig in `RENAMED_BY_VERIFIER` umbenannt, `TestBrowserRefusalSetEqualsTheServers` erneut gefahren, `ok github.com/holzcloud/holzkube-manager/internal/imagefactory 0.546s` — ein gruener Waechter gegen eine Deklaration, die es nicht mehr gab, waehrend sein eigener `t.Fatalf`-Kommentar genau diese Eigenschaft verspricht ("A guard that silently passes when it can no longer find what it guards is worse than no guard"). Nichts war maskiert: die beiden Ablehnungsmengen stimmten und stimmen ueberein, und das Loeschen des U+FEFF-Eintrags liess den Waechter auch vorher schon scheitern — es ging um das kuenftige Verhalten des Waechters. Plan 02-26 hat es geschlossen: `refusedRangesDecl` schneidet die Deklaration zuerst heraus (Zeilenanfangs-Anker mit `regexp.QuoteMeta` um den Bezeichner, wie `budget_drift_test.go:89-90`, und Zuweisungs-Verankerung wie `stringArrayLiteral`), ihr Fehlen ist ein Fehler statt eines kuerzeren Ergebnisses, ein eintragsfoermiges Literal ausserhalb der Deklaration wird nicht mehr eingefaltet und faellt als Abweichung der beiden Trefferzahlen auf (heute gemessen: sechs im Deklarationsrumpf und sechs in der ganzen Datei), und ein Eintrag innerhalb der Deklaration, den der Ausdruck nicht lesen kann, ist ein Fehlschlag mit dem Literal im Text statt eines stillen Ueberspringens. Die Pruefung ist als reine Funktion `parseBrowserRefusalRanges` ohne `*testing.T` herausgeloest, damit ihre Fehlerfaelle selbst pruefbar sind; `TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration` (vier Zeilen) und `TestBrowserRefusalGuardRefusesAnEntryItCannotRead` (zwei Zeilen) pruefen sie. Die Falsifikation des Verifiers wurde zusaetzlich als Lebendbeleg am echten Baum wiederholt, mit umgekehrtem Ergebnis, und `web/src/routes/images.tsx` byteweise wiederhergestellt. Eintrag 56 bleibt daneben offen und traegt eine andere Aussage: dort geht es darum, dass die SERVER-Menge hinter dem Codepoint-Sweep nicht erschoepfend gemessen ist und 02-14s Extrapolation erbt — daran hat diese Runde nichts geaendert. Dieser Eintrag ist eine Aufzeichnung eines in derselben Runde geschlossenen Defekts und wird bei der Anlage als `fixed` markiert, in der Form, die Eintrag 63 etabliert hat; das `fixed`-Verb traegt keinen Grund, also steht er hier. | fixed |  | 2026-09-04T05:59:17.805Z | 2026-09-04T05:59:33.231Z |
| 70 | 02 | unmet-truth | internal/imagefactory/guard_drift_test.go | 83 | [from 02-28] SUPERSEDES ENTRY 69, WHICH STAYS FIXED (no amend verb exists). WAS UEBERHOLT IST: Eintrag 69 sagt 'Plan 02-26 hat es geschlossen: refusedRangesDecl schneidet die Deklaration zuerst heraus ..., ihr Fehlen ist ein Fehler statt eines kuerzeren Ergebnisses' und ist bei der Anlage fixed markiert worden. Runde 5 der Verifikation (02-VERIFICATION.md, gaps[0]) hat gemessen, dass das fuer eine praefixierende Umbenennung nicht galt: REFUSED_RANGES_LEGACY allein lieferte ranges=[{0 0}] err=<nil>, REFUSED_RANGESX lieferte ranges=[{0 1}] err=<nil>, und am echten Baum blieben mit REFUSED_RANGES_LEGACY sowohl TestBrowserRefusalSetEqualsTheServers als auch TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration mit allen vier Untertests gruen. Dazu die zweite Haelfte: die gelesenen Bounds waren nicht validiert, { from: 0xFFFFFFFF, to: 0xFFFFFFFF } fiel auf {-1 -1}, { from: 0x10FFFF, to: 0xFFFFFFFF } auf {1114111 -1} und { from: 0x001f, to: 0x0000 } auf {31 0}, jeweils err=<nil> -- jeder dieser Eintraege deckt keinen Codepoint ab und wurde still uebergangen. WAS AN 69 WEITERHIN GILT: die Beschreibung des unverankerten Zustands zwischen Plan 02-20 und Runde 5, die Messung ok github.com/holzcloud/holzkube-manager/internal/imagefactory 0.546s, und die Feststellung, dass nichts maskiert war -- die beiden Ablehnungsmengen stimmen ueberein und das Loeschen des U+FEFF-Eintrags laesst den Waechter scheitern. Diese Saetze bleiben richtig und werden nicht ueberholt. WAS RUNDE 6 GETAN HAT: Plan 02-27 hat vier Korrekturen an internal/imagefactory/guard_drift_test.go geliefert, jede mit ihrer gemessenen Rot-Ausgabe aus 02-27-SUMMARY.md. Erstens der gebundene Anker: hinter regexp.QuoteMeta(refusedRangesName) sind nur noch Leerraum, eine mit einem Doppelpunkt beginnende Typannotation und das Gleichheitszeichen erlaubt, das verlangte : oder = ist die Wortgrenze, die der Unterstrich in REFUSED_RANGES_LEGACY nicht ist. Rot gemessen als Lebendbeleg am echten Baum: REFUSED_RANGES durchgaengig in REFUSED_RANGES_LEGACY umbenannt, --- FAIL: TestBrowserRefusalSetEqualsTheServers und --- FAIL: TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration/the_real_route beobachtet, wo Runde 5 beide PASS gemessen hatte, Datei danach byteweise wiederhergestellt (sha256 ca13cf5f5bf2abc1f1e4c9376355363f70d232305ef7beaf9716a42f618ee505 vorher wie nachher, git diff leer). Commits da5c54c (RED) und 2ed7f6b (GREEN). Zweitens die semantische Bound-Validierung to > utf8.MaxRune oder from > to vor dem append, weil strconv.ParseUint(m[1], 16, 32) bis 0xFFFFFFFF akzeptiert und rune(uint64) still verliert; die drei Tabellenzeilen pruefen from 0x0000 to 0xFFFFFFFF, from 0x0061 to 0xFFFFFFFF und from 0x001f to 0x0000 als Teilzeichenketten der Meldung, und die vier in Runde 5 gemessenen Werte erreichen den Codepoint-Sweep nicht mehr. Commits e373099 (RED) und bdaec63 (GREEN). Drittens die getrennte Abschneide-Diagnose: der Zweig len(matches) == 0 && whole > 0 meldet das Abschneiden VOR dem ersten Eintrag mit der Zahl der Eintraege, waehrend der Leer-Zweig weiterhin die woertliche Leer-Meldung liefert, beide Zweige einzeln gemessen. Commits ecdd083 (RED) und 6652eab (GREEN). Viertens die drei Falsifikationstabellen mit jetzt 6, 3 und 2 Zeilen, keine bestehende Zeile entfernt oder umbenannt, Rumpf- wie Dateizahl weiterhin 6. WARUM DIESER EINTRAG OPEN BLEIBT UND WAS IHN SCHLIESST: die Eigenschaft ist allquantifiziert ueber kuenftige Umbenennungen und kuenftige Eintragsformen, und diese Phase hat daran in drei aufeinanderfolgenden Runden je ein engeres Loch gemessen. Ein bei der Anlage geschlossener Eintrag ueber eine solche Eigenschaft war genau der Fehler von 69, und er ist mechanisch und nicht bloss stilistisch: workflow.windows_enforce blockiert /gsd-ship, solange open_count groesser als 0 ist, und ein fixed-Eintrag ist an dieser Grenze unsichtbar. Die Schliessbedingung ist pruefbar und lautet: eine Verifikationsrunde falsifiziert den Waechter selbst in beiden Richtungen -- sie benennt REFUSED_RANGES praefixiert um und sieht TestBrowserRefusalSetEqualsTheServers rot, und sie setzt einen Eintrag mit einer Grenze oberhalb utf8.MaxRune und sieht den Waechter rot -- und erst dann wird dieser Eintrag mit windows fixed markiert. Ob die allquantifizierte Wahrheit nach Plan 02-27 gilt, stellt eine Verifikationsrunde fest; dieser Eintrag behauptet es nicht. DIE NACHBARSCHAFT: Eintrag 56 bleibt daneben offen und traegt eine andere Aussage, naemlich dass die SERVER-Menge hinter dem Codepoint-Sweep nicht erschoepfend gemessen ist und 02-14s Extrapolation erbt; er wird von diesem Eintrag nicht ueberholt. Die Eintraege 58 und 66 tragen G-02-9 und bleiben unberuehrt offen. G-02-9 IST OFFEN. | open |  | 2026-09-04T19:29:24.329Z |  |
| 71 | 02 | deviation | internal/httpapi/handlers/schematics.go | 458 | [from 02-28] Der Doc-Verweis bei internal/httpapi/handlers/schematics.go:458 nannte zwei Bedingungen, 55 Zeilen ueber einer Funktion, die seit d4ee1f5 drei prueft (fresh.ProbedAt.IsZero(), stored.Arch != fresh.Arch, stored.TalosVersion != fresh.TalosVersion). Gefunden von Runde 5 der Verifikation (02-VERIFICATION.md, gaps[2], Wahrheitszeile 13 der Tabelle). WARUM ER ENTSTANDEN IST: die Wahrheit aus Plan 02-25 ist allquantifiziert -- jede Aussage im Baum, die bisher zwei Bedingungen des Refresh nennt, nennt jetzt drei --, aber ihr Task 3 hat drei Stellen namentlich aufgezaehlt (internal/model/model.go bei ID und bei TalosVersion sowie docs/api-contract.md:620-627) und genau diese nachgezogen. Eine Aufzaehlung findet keine vierte Stelle, und die vierte lag im selben File wie die in Plan 02-25 geaenderte Funktion. WAS RUNDE 6 GETAN HAT: das Zahlwort in Zeile 458 korrigiert, so dass der ueber :458-459 verteilte Satz jetzt 'see refreshTheStoredVerdict for the three conditions and for why a failed refresh is silent' lautet (Commit 3a060ef, Diff genau eine Kommentarzeile), und die Aufzaehlung durch zwei mechanische Sweeps ersetzt. Sweep A ist grep -rn nach refreshTheStoredVerdict mit --include fuer .go, .md, .ts und .tsx ueber internal, docs, web und cmd, dessen Ausgabe anschliessend mit grep -c auf das englische Zahlwort fuer 2 als eigenstaendiges Wort gezaehlt wird: gemessen 1 vor und 0 nach der Aenderung. Sweep A ist als Abnahmebedingung von Plan 02-28 festgenagelt und ersetzt damit die Aufzaehlung dauerhaft durch eine Suche. Sweep B ist grep -rniE nach den Mustern refresh oder 409 ueber dieselben Pfade und Endungen, dessen Ausgabe mit grep -icE auf die Zahlwoerter two, zwei oder both gezaehlt wird: gemessen 5 vor und 4 nach der Aenderung. Beide Kommandos stehen in ihrer vollstaendigen Form mit den Rohrzeichen in 02-28-SUMMARY.md; hier sind sie beschrieben statt zitiert, weil die Markdown-Repraesentation dieses Ledgers rohrgetrennt ist. DIE VIER VERBLEIBENDEN FUNDSTELLEN sind einzeln beurteilt und richtig: schematics.go:628 (versionMismatchReason spricht aus einer der drei Bedingungen heraus ueber die beiden anderen und zaehlt sie nicht) sowie schematics_test.go:3143, :3178 und :3244 (sie sagen both architectures bzw. both Talos versions ueber je zwei benannte Werte in einer Fehlermeldung -- ArchAMD64 gegen ArchARM64 und catalogVersion gegen otherCatalogVersion -- und nicht ueber eine Zahl von Bedingungen). Eintrag 66 bleibt daneben stehen: er amendiert 58 um die dritte ablehnende Bedingung und ist von dieser Runde unberuehrt. DIE GRENZE DIESER MESSUNG: die beiden Sweeps decken internal, docs, web und cmd ueber die Endungen .go, .md, .ts und .tsx ab und nichts darueber hinaus. Dieser Eintrag wird bei der Anlage als fixed markiert, in der Form, die Eintrag 63 etabliert hat, weil der Defekt mit einer endlichen, wiederholbaren Messung ueber den ganzen Baum geschlossen ist und nicht mit einer Behauptung ueber kuenftiges Verhalten; das fixed-Verb traegt keinen Grund, also steht er hier. | fixed |  | 2026-09-04T19:29:31.724Z | 2026-09-04T19:29:36.022Z |
| 72 | 02 | unmet-truth | internal/imagefactory/guard_drift_test.go |  | [from 02-31] AMENDS ENTRY 70, WHICH STAYS OPEN (no amend verb exists). DIE ZEILENSPALTE DIESES EINTRAGS IST ABSICHTLICH LEER, nicht vergessen: er zeigt auf den Symbolnamen refusedRangesDecl in internal/imagefactory/guard_drift_test.go statt auf eine Zeilennummer, und damit ist der stale Zeiger von Eintrag 70 auf Zeile 83 -- eine Zahl von vor Plan 02-27, die heute im Doc-Kommentar liegt -- miterledigt, ohne dass 70 bearbeitet wird (02-REVIEW.md IN-05); kuenftige Eintraege ueber Code zitieren Symbole statt Zeilen. WAS UEBERHOLT IST: Eintrag 70 traegt eine Schliessbedingung, die von Runde 6 der Verifikation woertlich ausgefuehrt und in beiden Richtungen erfuellt wurde (02-VERIFICATION.md:27, :265-266), und der Eintrag steht trotzdem weiter offen, weil sein tatsaechlicher Offen-Grund ein anderer ist als seine geschriebene Bedingung. DIESE DIVERGENZ IST ES, DIE DIESER EINTRAG UEBERHOLT, nicht die Messungen von 70. Der tatsaechliche Grund ist benannt und gemessen: der Anker ist ein regulaerer Ausdruck ueber TEXT und nicht ueber ein Programm, er bindet an ein LITERAL und nicht an den WERT, den ein Ausdruck um ihn herum erzeugt, und an einen BEZEICHNER und nicht an Code, den der Browser ausfuehrt. Plan 02-30 hat das ueber den Live-Lesepfad gemessen, je Form mit Bereichszahl und Fehlerzustand: eine Deklaration, die nur in einem Block-Kommentar ueberlebt, ranges=6 err=nil; nur in einem eingerueckten JSX-Kommentar, ranges=6 err=nil; nur in einem Template-Literal, ranges=6 err=nil; nur als JSX-Text in einem pre-Block, ranges=6 err=nil; ein Literal, das vor seiner Bindung gefiltert wird, also ].filter((range) => range.class !== 'byte order mark'), ranges=6 err=nil; eine Deklaration, die in die wirklich benutzte Tabelle gespreizt wird, ranges=6 err=nil; und eine eingerueckte Rest-Deklaration neben dem Import der echten Tabelle, ranges=2 err=nil, wo die kuerzere Zahl selbst der Schaden ist. WAS AN 70 WEITERHIN GILT: die Beschreibung des unverankerten Zustands zwischen Plan 02-20 und Runde 5 und des praefix-durchlaessigen Zustands bis Plan 02-27; die sechs Messwerte der Runde 5 (REFUSED_RANGES_LEGACY liefert ranges=[{0 0}] err=nil, REFUSED_RANGESX liefert ranges=[{0 1}] err=nil, und die vier unvalidierten Bounds fielen auf {-1 -1}, {1114111 -1} und {31 0}); die vier Korrekturen der Runde 6 mit ihren Commits da5c54c, 2ed7f6b, e373099, bdaec63, ecdd083 und 6652eab; und die Feststellung, dass nichts maskiert ist, weil die beiden Ablehnungsmengen uebereinstimmen und das Loeschen des U+FEFF-Eintrags den Waechter scheitern laesst. Diese Saetze bleiben richtig und werden von diesem Eintrag nicht ueberholt. WAS RUNDE 7 GETAN HAT, aus 02-29-SUMMARY.md und 02-30-SUMMARY.md und nicht aus dem Plan: der Spiegel-Defekt der Runde 6 ist ueber die Form des Rumpfes beseitigt statt gegen eine andere falsche Ursache getauscht -- strings.TrimSpace(body) entscheidet die Leer-Frage, whole beantwortet nur noch die Frage nach dem Literal ausserhalb der Deklaration, und der aus einer Dateizaehlung erschlossene Satz "The declaration is NOT empty" ist mit dieser Zaehlung verschwunden; Ueberlauf und Inversion sind zwei Zweige mit zwei eigenen Ursachen, und die Inversions-Meldung nennt utf8.MaxRune nicht mehr (WR-03); keine Tabellenzeile pinnt mehr eine Zeichenkette, die jeder Fehlerausgang traegt -- die drei wantErr-Zeilen tragen je eine unterscheidende Teilzeichenkette und wantErr ist in allen drei Tabellen []string (WR-04); der 49-zeilige Kommentarblock ist geteilt und beide Regexp-Deklarationen tragen ihren eigenen Doc-Kommentar (WR-05); der Waechter stellt die Zahl der Deklarationen fest und macht mehr als eine zu einem eigenen Fehlerausgang vor dem Lesen des Rumpfes; die Funktion hat jetzt zehn Fehlerausgaenge, und keiner erschliesst seine Ursache aus einer Zaehlung; die Abschneide-Meldung zitiert den gelesenen Rumpf mit %q und traegt darum keine Zahl mehr, womit 02-REVIEW.md IN-01 als Nebenwirkung entfaellt (grep -c '%d entries' liefert 1 statt 2); der allquantifizierte Satz "Renamed, moved or deleted is the same as never having been there" ist aus dem Doc-Kommentar und aus dem Fehlertext verschwunden (grep -c liefert 0) und durch eine Aussage ueber den Text plus eine als Daten gefuehrte Ausschlussliste ersetzt; guardBlindSpots fuehrt die Blindheit nach MECHANISMUS skopiert (text-not-code, literal-not-value, declaration-not-use, surrogate-interior) und honestClaim rendert die Behauptung aus dieser Liste, so dass zwei Listen verschiedener Laenge nicht entstehen koennen; der Lesepfad ist parametrisiert und wrapper-frei (browserRefusalRanges(t, path)), so dass die sieben Blindheitszeilen ueber denselben Pfad messen wie der Live-Waechter und ein kuenftiges stripNonCode am Aufrufer sie rot faerbt statt sie zu umgehen -- das ist als dritte Rot-Ausgabe belegt; und Punkt 6 der Schliessbedingung wurde einmal ausgefuehrt und protokolliert. WARUM DIESER EINTRAG OPEN BLEIBT UND WAS IHN SCHLIESST: er bleibt offen, weil die Verengung die BEHAUPTUNG auf null Ueberhang bringt und den DEFEKT nicht beseitigt. Die Leiche bleibt lesbar, der Waechter bleibt gruen darueber, und der Zeitpunkt, an dem das schadet, ist unveraendert. Ein bei der Anlage geschlossener Eintrag ueber eine solche Eigenschaft war genau der Fehler von 69, und er ist mechanisch und nicht bloss stilistisch: workflow.windows_enforce blockiert /gsd-ship, solange open_count groesser als 0 ist, und ein fixed-Eintrag ist an dieser Grenze unsichtbar. Die Schliessbedingung ist pruefbar und endlich und lautet, dass eine Verifikationsrunde gegen den dann geltenden Waechter alle sechs Punkte misst und protokolliert: (1) REFUSED_RANGES praefixiert umbenannt, etwa _LEGACY, ergibt ROT; (2) die Deklaration nur hinter // ergibt ROT; (3) ein Eintrag mit einer Grenze oberhalb utf8.MaxRune ergibt ROT; (4) eine wirklich leere Zuweisung = [] neben einer zweiten eintragsfoermigen Tabelle ergibt die LEER-Meldung und nicht "Look for that bracket"; (5) die geschriebene Ausschlussliste wird gegen jede darin genannte Form EINZELN gemessen und die gemessene Ausgabe im Protokoll zitiert, mindestens blosser Blockkommentar, eingerruecktes JSX-Kommentar, Template-Literal (je mit dem Import daneben, sonst ist die Fixture kein Schadensfall, sondern ein TS-Fehler) sowie die Ableitungsformen ].filter(...) und ...SPREAD, und jede Form muss die Ausgabe liefern, die die Liste behauptet; (6) die Runde versucht AUSDRUECKLICH, eine SIEBTE Form zu bauen, die die Liste nicht nennt (JSX-Text, String.raw, Regex-Literal, ${...}-Interpolation, #private-Name), und protokolliert das Ergebnis -- findet sie eine, die der Waechter liest und deren Mechanismus die Liste nicht nennt, BLEIBT DIESER EINTRAG OFFEN und die Liste wird um sie erweitert. Punkt 6 ist die eigentliche Huerde: EIN EINTRAG, DESSEN BEDINGUNG NUR DIE SCHON BEKANNTEN FORMEN ABFRAGT, WAERE EIN EINTRAG, DER SICH SELBST SCHLIESST. Plan 02-30 hat Punkt 6 einmal ausgefuehrt, mit dem Ergebnis "kein Fund" -- String.raw, Regex-Literal und ${...}-Interpolation werden gelesen (je ranges=6 err=nil) und sind Auspraegungen des bereits gelisteten Mechanismus text-not-code, Objekt-Property und Re-Export werden korrekt abgelehnt --, und dieser Nicht-Fund schliesst ausdruecklich nichts: die Schliessbedingung verlangt den Versuch von der Verifikationsrunde, die den Eintrag schliesst, nicht von der Runde, die ihn anlegt. DIE RATIFIKATION FEHLT: die Wahl zwischen B und A wurde vom Betreiber am 2026-09-05 ausdruecklich an den autonomen Lauf delegiert; der Lauf hat B gewaehlt. Der Betreiber hat B nicht selbst ratifiziert. Das Entscheidungsdokument 02-DECISION-drift-guard-lexik.md traegt in seinem Statusblock unveraendert "offen -- vorgelegt, nicht ratifiziert", wurde von einem autonomen Lauf am 2026-09-05 erarbeitet, und Variante A liegt PUNKTGLEICH daneben, 7,0 gegen 7,0; der Verifier hatte die Frage ausdruecklich dem Betreiber zugewiesen (02-VERIFICATION.md Human-Verification-Punkt 1, :430-440 und :522-523), und der Code-Review ordnet begruendet anders (02-REVIEW.md:160-162 stellt das Entfernen von Kommentaren und Zeichenketten vor die Verengung, die dort erst bei :182-186 und ausdruecklich als "Falls das als zu teuer gilt" erscheint). Was ein Wechsel auf Variante A kostet: dieser Eintrag beschriebe einen anderen Stand, die Abschnitte ueber die Ausschlussliste entfielen zugunsten einer Beschreibung dessen, was ein Lexer schliesst und was nicht, und die Arbeit von Plan 02-30 an guard_drift_test.go waere neu zu planen -- der unbedingte Teil (Spiegel-Defekt, WR-03, WR-04, WR-05, IN-05) bliebe unberuehrt. Ebenfalls festzuhalten, damit ein spaeterer Leser die Beweisdichte dieser Runde richtig einordnet: der Betreiber hat fuer den Rest dieser Runde ausdruecklich um schnelleres Vorgehen mit weniger Verifikation gebeten, und die Evidenz der Runde 7 ist duenner als die der Runden 5 bis 7 zuvor. OVERRIDES WIRD NIRGENDS BENUTZT UND NICHTS WIRD GEWAIVT: waived_count bleibt 0, beide Verifikationen tragen overrides_applied: 0, und das Ledger-Verb fuer bewusste Annahme heisst waive. Dieser Rest wird nicht angenommen, sondern offen gefuehrt. WAS AUCH NACH DIESER RUNDE BLEIBT, fuenf benannte Reste: (1) DIE ABLEITUNG -- der Anker bindet an ein Literal und nicht an einen Wert, gemessen an lebendem, kompilierendem, bei images.tsx:159 referenziertem Code, wo ].filter((range) => range.class !== 'byte order mark') sechs Bereiche lesen laesst und err=nil liefert, waehrend das Formular U+FEFF wieder annimmt und der Server es weiter mit 400 ablehnt; das ist GRUEN AUF ECHTER DRIFT und damit von derselben Schwere wie G-02-11 selbst, und es ist der gefaehrlichste bekannte Rest. (2) DIE EINE TRAGENDE VERBINDUNG -- images.tsx:159 ist die einzige lebende Referenz auf REFUSED_RANGES im ganzen Baum; faellt der Aufruf von hasControlCharacter aus dem Validierungspfad, ist die Tabelle eine wohlgeformte, lebende, uebereinstimmende Leiche, und jede Variante ist gruen darueber; G-02-17 war genau dieser Fall an einem einzelnen Feld, und gefunden hat ihn ein Mensch. (3) DER TRIMSPACE-REST -- strings.TrimSpace(body) haelt einen Rumpf aus // oder aus einem leeren Blockkommentar fuer nicht leer und nimmt den Abschneide-Zweig; kleiner als der Defekt, den er ersetzt, weil die Meldung ihren Beleg mit %q zeigt statt ihn zu behaupten, aber dieselbe Gattung, innerhalb der Aenderung, die diese Gattung beseitigen soll, und von einer Tabellenzeile festgenagelt. (4) DIE DREI HINWEISZEILEN UEBER images.tsx:105, die der Entwurf von Variante B vorsieht und die diese Runde BEWUSST NICHT GESCHRIEBEN hat, weil sie alles darunter um drei Zeilen verschoeben und rund ein Dutzend images.tsx:NNN-Verweise im Planungsbestand ungueltig machten; damit fehlt der Verengung ihr einziges Geraet, das den Menschen erreicht, der die Leiche erzeugt. (5) DIE 2046 NIE VERGLICHENEN CODEPOINTS im Inneren des Surrogat-Bereichs -- der Codepoint-Sweep ueberspringt U+D800..U+DFFF mit einem continue und behauptet die Surrogat-Menge nur an ihren beiden Endpunkten, weshalb fuer die 2046 Codepoints dazwischen nichts verglichen wird, und ihr Server-Zwilling rawBodyRefusal wird von hier nie aufgerufen; das Entscheidungsdokument nennt an dieser Stelle eine Zahl, die um die beiden einzeln behaupteten Endpunkte zu hoch ist, und der Nachtrag vom 2026-09-05 an jenem Dokument haelt die Korrektur auf 2046 fest. Kein Eintrag von guardBlindSpots kann diesen Rest als Zeile messen, weil Go keinen unpaarigen Surrogat in einem String halten kann; er ist dort als rowless mit Grund gefuehrt. DIE NACHBARSCHAFT: Eintrag 56 bleibt daneben OFFEN und traegt eine andere Aussage -- die SERVER-Menge hinter dem Codepoint-Sweep ist nicht erschoepfend gegen factory.talos.dev gemessen und erbt 02-14s Extrapolation --, und er wird von diesem Eintrag AUSDRUECKLICH NICHT ueberholt. Die Eintraege 58 und 66 tragen G-02-9 und bleiben unberuehrt OFFEN; G-02-9 IST OFFEN. Eintrag 69 bleibt FIXED und wird nicht angefasst. Eintrag 70 bleibt woertlich unveraendert und OFFEN. DIE GESCHWISTER-LESER SIND VON DIESEM EINTRAG NICHT GEDECKT -- stringArrayLiteral in derselben Datei, der Anker in internal/httpapi/handlers/budget_drift_test.go und die beiden strings.Contains-Sweeps in internal/imagefactory/warnings_test.go tragen dieselbe Gattung Loch und bekommen ihren eigenen offenen Eintrag in derselben Runde, Eintrag 74, in dem jeder der drei einzeln gemessen ist. | open |  | 2026-09-05T09:27:11.222Z |  |
| 73 | 02 | deviation | .planning/REQUIREMENTS.md |  | [from 02-31] Der Werkzeugdefekt, der einen Release-Blocker faelschlich als Complete stehen laesst: requirements revert-phase erreicht keine Traceability-Zeile, deren Id-Zelle Markup traegt. WAS GEMESSEN WURDE: eine Kopie von .planning/REQUIREMENTS.md wurde in ein Temp-Projektverzeichnis AUSSERHALB dieses Repositories gelegt, dort die undekorierte Phase-2-Zeile FACT-03 als Kontrolle kuenstlich auf Complete gesetzt und `gsd-tools requirements revert-phase TRANS-06 FACT-03 --project-dir <temp>` gefahren. Ergebnis woertlich: reverted enthaelt FACT-03, unchanged enthaelt TRANS-06, total 2. In der Kopie stand danach `FACT-03 -- Phase 2 -- Gaps Found` und unveraendert `**TRANS-06** (Emoji) -- Phase 2 -- Complete`. Beide Ids durchliefen denselben Aufruf, beide Checkboxen waren vorher schon leer, und der einzige Unterschied zwischen ihnen ist die Dekoration der Id-Zelle. Das echte Register wurde von diesem Messlauf nicht beruehrt. WARUM ES PASSIERT, an der Quelle gelesen statt aus der Beobachtung erschlossen: die Funktion cmdRequirementsRevertPhase in gsd-core/bin/lib/milestone.cjs hat zwei Wirkflaechen, und die beiden sind sich uneinig darueber, wie eine Id-Zelle aussehen darf. Die Checkbox-Haelfte traegt den Fett-Wrapper AUSDRUECKLICH in ihrem Muster: `const checkboxPattern = new RegExp("(-\\\\s*\\\\[)x(\\\\]\\\\s*\\\\*\\\\*" + reqEscaped + "\\\\*\\\\*)", "gi");`. Die Traceability-Haelfte sucht die Zeile dagegen ueber ein Praedikat, das die ERSTE ZELLE der Zeile nach Trimmen und Kleinschreibung auf EXAKTE GLEICHHEIT mit der Id prueft: `const rowMatch = (row) => (Object.values(row)[0] ?? "").trim().toLowerCase() === reqId.toLowerCase();`. "**trans-06** (Emoji)" ist nicht "trans-06", also findet das Praedikat die Zeile nicht, tableHit bleibt false, die Id landet unter unchanged und es wird nichts geschrieben. Eine dekorierte Zeile ist damit fuer die eine Haelfte derselben Funktion sichtbar und fuer die andere nicht. WAS ES ANGERICHTET HAT: weder der Revert der Runde 5 (2e57ee3 docs(phase-02): revert premature Complete requirements after gaps found) noch der dieser Runde (b7a1ab9) hat die TRANS-06-Zeile erreicht, waehrend alle 13 undekorierten Phase-2-Ids zurueckgenommen wurden. Der einzige Release-Blocker der Phase las damit Complete, waehrend die Verifikation der Phase gaps_found sagt -- woertlich die Aussage, die der Betreiber mit 2e57ee3 von Hand zurueckgenommen und in derselben Commit-Nachricht als Regel ausgesprochen hat. Ein Werkzeugdefekt, der als falscher Projektzustand auftaucht, an genau der Stelle, an der niemand nachfragt. WIE WEIT ES REICHT: gemessen ueber das ganze Register, nicht ueber diese Phase. 16 Release-Blocker-Zeilen tragen eine dekorierte Id-Zelle und werden beim naechsten gaps_found ihrer Phase denselben Weg nehmen: TRANS-06 (Phase 2), INV-01, INV-03, INV-04, INV-05 und INV-07 (Phase 3), JOB-07 (Phase 6), CFG-02 (Phase 7), PROV-05, PROV-09 und PROV-10 (Phase 8), UPG-02, UPG-03, UPG-06 und UPG-07 (Phase 9) sowie OPS-05 (Phase 10). Das sind alle 16 Release-Blocker des Projekts; die Dekoration IST die Release-Blocker-Auszeichnung, also ist jeder einzelne betroffen. WAS RUNDE 7 GETAN HAT: die Statuszelle der TRANS-06-Zeile von Hand von Complete auf Gaps Found gesetzt, und sonst nichts an dieser Zeile -- die Id-Zelle behaelt ihre Dekoration, weil sie zu entfernen die bequeme Reparatur am falschen Ort waere und die Release-Blocker-Auszeichnung des Registers beschaedigte, um einen Werkzeugdefekt zu umgehen. Die Handkorrektur ist die ANWENDUNG der Regel, die der Betreiber mit 2e57ee3 selbst ausgesprochen hat, und keine neue Entscheidung. Danach lesen alle vierzehn Phase-2-Zeilen Gaps Found und keine liest Complete. WARUM DIESER EINTRAG OFFEN BLEIBT: der Defekt sitzt in der GSD-Laufzeit unter gsd-core/bin/lib/milestone.cjs, ausserhalb dieses Repositories, nicht versioniert und nicht im Umfang dieser Phase. Nichts in diesem Repository kann ihn schliessen, und der naechste mark-complete- oder revert-phase-Zyklus erzeugt ihn erneut -- fuer diese Zeile und fuer die fuenfzehn anderen. SEINE SCHLIESSBEDINGUNG IST EIN LAUF, IN DEM EINE DEKORIERTE ZEILE UNTER reverted ERSCHEINT: dieselbe Messung wie oben, eine dekorierte und eine undekorierte Id in einem Aufruf gegen eine Kopie, und beide erscheinen unter reverted statt eine unter unchanged. Solange das nicht gemessen ist, bleibt dieser Eintrag offen, auch wenn die eine Zeile im Repository von Hand richtig steht -- die Handkorrektur behebt den Zustand, nicht die Ursache. DIE GRENZE DIESER MESSUNG: geprueft wurde requirements revert-phase, nicht requirements mark-complete und nicht ready-ids. Ob dieselbe Uneinigkeit zwischen Checkbox- und Traceability-Haelfte auch dort besteht, hat diese Runde nicht festgestellt. | open |  | 2026-09-05T09:34:19.051Z |  |
| 74 | 02 | unmet-truth | internal/imagefactory/guard_drift_test.go |  | [from 02-31] DIE GESCHWISTER-LESER DIESES BESTANDS TRAGEN DIESELBE GATTUNG LOCH WIE DER BROWSER-ABLEHNUNGS-WAECHTER, JEDER IN EIGENEM UMFANG -- HIER EINZELN GEMESSEN, NICHT AUS EINEM PLANUNGSDOKUMENT UEBERNOMMEN. Die Zeilenspalte ist absichtlich leer: der Eintrag zeigt auf den Symbolnamen stringArrayLiteral in internal/imagefactory/guard_drift_test.go, und die beiden anderen Fundstellen sind im Text mit Datei und Symbol benannt. WELCHE DREI LESER: (1) stringArrayLiteral in internal/imagefactory/guard_drift_test.go, Muster NAME[^=]*=\\s*\\[([^\\]]*)\\] ohne Zeilenanfangs-Anker und ohne Wortgrenze hinter dem Namen; (2) der Anker in TestBudgetWaitsStatedInTheUIMatchTheRouteBudgets in internal/httpapi/handlers/budget_drift_test.go, Muster (?m)^\\s*(?:export\\s+)?const\\s+NAME\\s*=\\s*([0-9]+)\\b; (3) die beiden strings.Contains-Sweeps in TestWarningDetailsMatchTheUI in internal/imagefactory/warnings_test.go, die den Detail-Satz gegen web/src/components/SchematicWarnings.tsx und den Code gegen web/src/api.ts pruefen, samt der Schleife ueber exportedWarningCodes, die denselben Contains-Sweep je Code fahrt. WAS EINZELN GEMESSEN WURDE, mit einer wegwerfbaren Sonde ausserhalb dieses Repositories, die die drei Muster woertlich kopiert und gegen synthetische Quellen faehrt; die Sonde ist entfernt. LESER 1 stringArrayLiteral: Kontrolle auf der echten Deklaration GELESEN mit members=2; Praefix-Loch (nur INSTALLER_REPOSITORY_NAMES_LITERAL_LEGACY vorhanden) GELESEN mit members=2, also JA; Blindheit in einem Zeilenkommentar GELESEN mit members=2 und in einem Template-Literal GELESEN mit members=2, also JA. Er traegt beide Loecher unveraendert. LESER 2 der Budget-Anker: Kontrolle GELESEN mit wert=45; Praefix-Loch (nur SCHEMATIC_WAIT_SECONDS_LEGACY vorhanden) NICHT GELESEN, also NEIN; Blindheit in einem Zeilenkommentar NICHT GELESEN und in einem eingerueckten JSX-Kommentar NICHT GELESEN, aber in einem Blockkommentar GELESEN mit wert=45 und in einem Template-Literal GELESEN mit wert=45, also JA und zwar begrenzt. LESER 3 die beiden Contains-Sweeps: Kontrolle GELESEN; Praefix-Loch (nur installer.repo-fallback-unverified vorhanden, gesucht installer.repo-fallback) GELESEN, also JA; Blindheit in einem Zeilenkommentar, in einem Blockkommentar, in einem Template-Literal und als JSX-Text jeweils GELESEN, also JA in jeder gepruefen Form. Er hat weder Anker noch Wortgrenze und ist der durchlaessigste der drei. WORIN DIE MESSUNG DEM ENTSCHEIDUNGSDOKUMENT WIDERSPRICHT, und das gehoert genannt, weil eine uebernommene Vermutung in einem Ledger genau die Sorte Behauptung waere, die dieser Ledger fuehrt: 02-DECISION-drift-guard-lexik.md schreibt im Abschnitt "Was keine der vier Varianten schliesst", Unterpunkt "Die Geschwister", dass stringArrayLiteral Runde 5s Praefix-Loch unveraendert weitertraegt und dass budget_drift_test.go:89-90 sowie warnings_test.go:281-336 "ebenso" betroffen sind. FUER DEN BUDGET-ANKER STIMMT DAS NICHT: er verlangt hinter dem Bezeichner unmittelbar Leerraum und das Gleichheitszeichen, weshalb der Unterstrich in SCHEMATIC_WAIT_SECONDS_LEGACY das Muster bricht -- er hat das Praefix-Loch NICHT, und er hatte es nie. Zweitens ist auch die Beschreibung seiner lexikalischen Blindheit zu weit: sein Zeilenanfangs-Anker laesst zwar Einrueckung zu, aber weil auf den Leerraum unmittelbar `export` oder `const` folgen muss, faellt eine mit // oder mit {/* beginnende Zeile durch. Er ist blind gegen einen Blockkommentar und gegen ein Template-Literal, weil die Deklarationszeile IM Inneren dieser Formen wieder mit const beginnt -- nicht gegen Kommentarzeichen im Allgemeinen. Die Vermutung des Dokuments trifft also fuer Leser 1 und Leser 3 zu und fuer Leser 2 nur zur Haelfte und aus einem anderen Grund als dort angenommen. WARUM SIE EINEN EIGENEN EINTRAG BEKOMMEN: Ledger-Eintrag 72 deckt sie AUSDRUECKLICH NICHT -- er handelt von refusedRangesDecl und von der Behauptung, die der Browser-Ablehnungs-Waechter ueber REFUSED_RANGES fuehrt. Keine der vier Varianten des Entscheidungsdokuments fasst die Geschwister an. Sie stillschweigend stehen zu lassen waere dieselbe Operation, die Eintrag 69 falsch gemacht hat: eine Eigenschaft, die weiterlebt, an einem Eintrag vorbei, der sie nicht fuehrt. WAS IHN SCHLIESST: jeder der drei traegt entweder die Verankerungs-Disziplin aus Plan 02-27 -- hinter regexp.QuoteMeta(name) nur Leerraum, eine mit Doppelpunkt beginnende Typannotation und das Gleichheitszeichen, so dass das verlangte Zeichen die Wortgrenze ist, die der Unterstrich nicht ist -- oder einen GESCHRIEBENEN, GEMESSENEN Ausschluss in der Form, die Plan 02-30 fuer guardBlindSpots etabliert hat, also je Eintrag Mechanismus, gemessene Ausgabe und eine Zeile, die ihn misst. Eine Runde, die das fuer alle drei tut und je die beiden Messungen dieses Eintrags protokolliert -- Praefix-Loch und lexikalische Blindheit, mit der gemessenen Ausgabe und nicht mit einem Urteil --, schliesst diesen Eintrag. Fuer Leser 2 heisst das ausdruecklich: der Praefix-Teil ist bereits erfuellt und gemessen, offen ist allein der lexikalische Teil. DIE GRENZE DIESER MESSUNG: DREI LESER WURDEN GEPRUEFT UND KEINE WEITEREN. Ob der Baum weitere Transkriptions-Waechter mit derselben Gattung Loch traegt, hat diese Runde NICHT festgestellt; es wurde keine Suche ueber den ganzen Baum nach solchen Mustern gefahren. Gemessen wurden je Leser genau zwei Fragen (Praefix-Loch und lexikalische Blindheit) an genau den oben genannten synthetischen Formen; andere Formen wurden nicht versucht. | open |  | 2026-09-05T09:36:55.361Z |  |
| 75 | 02 | unmet-truth | .planning/ROADMAP.md |  | [aus Runde 8, Human-Decision-Punkt 2] DIE PLANZAHL IN ROADMAP.md ZEILE 81 IST DREIMAL HINTEREINANDER GEDRIFTET, WEIL WERKZEUGAUSGABE UND HANDSCHRIFT IN EINEM SATZ STEHEN. Der Zustand ist heute richtig, gemessen 31 von 31 an beiden Stellen; der Befund ist die Wiederholung und nicht der Stand. Runde 6 hinterliess 28 von 28 neben 26 von 28, der Fix 77ed0f3 erfasste nur die vordere Haelfte, Runde 7 hinterliess 30 von 31 neben 28 von 31, und Plan 02-31 korrigierte auf 31 von 31 ohne vierten Eintrag, weil seine Erfolgskriterien die Endsignatur auf 74 nagelten, und uebergab den Fall ausdruecklich. Nach dem Massstab von Eintrag 74, sie stillschweigend stehen zu lassen waere dieselbe Operation, die Eintrag 69 falsch gemacht hat, gehoert sie geledgert. Schliessbedingung: entweder die von Hand gefuehrte Zahl ersatzlos streichen, so dass nur die vom Werkzeug gefuehrte bleibt, oder eine Pruefung, die beide Zahlen im selben Satz gegeneinander haelt. | open |  | 2026-09-05T10:14:39.906Z |  |
| 76 | 03 | unrun-verify | internal/talos/contract_real_test.go |  | TRANS-08 / Erfolgskriterium 5 ist NICHT erfuellt. Die Contract-Suite laeuft gruen gegen talossim (Tier 0); die reale Haelfte existiert als realTransport in internal/talos/contract_real_test.go und der Tier-1-Provisioner als sandbox/cmd/talos-sandbox, aber sie wurde in dieser Sitzung NIE AUSGEFUEHRT: der Docker-Daemon laeuft in dieser Umgebung nicht (docker info scheitert), und ohne ihn kann pkg/pr | open |  | 2026-09-11T16:30:00.000Z |  |
| 77 | 03 | unrun-verify | Taskfile.yml |  | 'task lint:go' (golangci-lint run) konnte in dieser Sitzung nicht ausgefuehrt werden: das installierte golangci-lint ist mit go1.25 gebaut und weigert sich gegen ein go1.26.7-Ziel zu laufen ('the Go language version used to build golangci-lint is lower than the targeted Go version'). gofmt -l und go vet sind ueber cmd/ und internal/ sauber, und die Go- sowie Web-Testsuiten sind gruen. Derselbe Ris | open |  | 2026-09-11T16:30:00.000Z |  |
| 78 | 03 | todo | internal/inventory/observe.go |  | Refresh liest den Machine-Record, baut den Snapshot und schreibt zurueck -- ohne die Rev-Kollision zu behandeln. Zwei Ereignisse koennen sie ausloesen: der Supervisor-Heartbeat und ein manuelles POST /machines/{id}/refresh treffen zusammen, oder ein Refresh laeuft waehrend recordMachine denselben Record anfasst. Der Verlierer bekommt store.ErrConflict, der Fehlschlag wird geloggt und der Snapshot  | open |  | 2026-09-11T16:30:00.000Z |  |
| 79 | 03 | deviation | internal/inventory/observe.go |  | Die Supervisors sind Heartbeat-Poller ueber COSI-Reads, keine COSI-Watches. INV-13 und D-19 verlangen 'Watch primaer, Poll als Heartbeat'; gebaut ist der Heartbeat mit Jitter (45s +/-20%) ueber genau die Ressourcen, die ein Watch beobachten wuerde. Die Leserichtung stimmt -- Ressourcenzustand statt unaerer RPCs, was das eigentliche Verbot von INV-13 ist -- und die Antwortform (health.Field[T]) ist | open |  | 2026-09-11T16:30:00.000Z |  |

````json
[
  {
    "id": 1,
    "kind": "stub",
    "phase": "02",
    "file": "internal/talos/dial_direct.go",
    "line": null,
    "description": "directDialer.Probe leaves Identity.Version empty: the version is not in the TLS certificate and Dialer.Probe carries no Creds to make an authenticated RPC",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-28T19:58:41.457Z",
    "resolved_at": null
  },
  {
    "id": 2,
    "kind": "stub",
    "phase": "02",
    "file": "internal/talossim/machine.go",
    "line": null,
    "description": "talossim implements 2 of 54 MachineService RPCs; the rest inherit Unimplemented. Scoped to plan 02-08 by the plan's own scope_decision",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-28T19:58:41.580Z",
    "resolved_at": null
  },
  {
    "id": 3,
    "kind": "stub",
    "phase": "02",
    "file": "internal/talossim/machine.go",
    "line": null,
    "description": "ApplyConfiguration counts an applied config but does not parse it: applying a config that sets a hostname does not change what Hostname reports. Server.SetHostname/SetVersion give a scenario the same effect explicitly.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-29T05:01:33.912Z",
    "resolved_at": null
  },
  {
    "id": 4,
    "kind": "stub",
    "phase": "02",
    "file": "internal/talossim/stream.go",
    "line": null,
    "description": "Events emits an identical MachineStatusEvent{Stage: RUNNING} payload per message; the event stream is not driven by the node's actual state transitions. Correlating events with Bootstrap/Reboot/Reset belongs to plan 02-03's scenario engine.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-29T05:01:34.036Z",
    "resolved_at": null
  },
  {
    "id": 5,
    "kind": "unrun-verify",
    "phase": "02",
    "file": "internal/imagefactory/live_test.go",
    "line": null,
    "description": "TestLiveFactory ist der einzige Drift-Waechter gegen factory.talos.dev, ist opt-in und wird von nichts geplant; factory.talos.dev hat in dieser Sitzung nachweislich gedrosselt, ein Retry fehlt.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-29T05:30:01.609Z",
    "resolved_at": null
  },
  {
    "id": 6,
    "kind": "unrun-verify",
    "phase": "02",
    "file": "docs/api-contract.md",
    "line": null,
    "description": "golangci-lint run could not be executed on this host (binary not installed); go vet and gofmt are clean. Plan 02-06 task acceptance criterion 'golangci-lint run exits 0' is unverified.",
    "status": "fixed",
    "reason": "Not installed on the subagent PATH, but present at ~/go/bin/golangci-lint (installed during plan 02-01 at the version CI pins). Orchestrator ran it at the wave-3 gate with that path exported: 0 issues.",
    "recorded_at": "2026-08-29T06:53:45.252Z",
    "resolved_at": "2026-08-29T09:05:00.000Z"
  },
  {
    "id": 7,
    "kind": "deviation",
    "phase": "02",
    "file": "internal/talossim/scenario_conn.go",
    "line": null,
    "description": "ip_changes_on_reboot: rebind() and Reboot() carried comments claiming established connections survive the reboot so the Reboot reply is delivered, while closeListener severs them. Plan 02-05 corrected the comments rather than the behaviour (severing is what makes the address change observable to an already-connected client), but the simulator is now harder to satisfy than hardware for this one RPC: talosctl reboot does return a reply. Closing it properly means delivering the reply and severing after a short grace, which needs a scenario-owned goroutine.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-29T07:54:43.845Z",
    "resolved_at": null
  },
  {
    "id": 8,
    "kind": "todo",
    "phase": "02",
    "file": "internal/imagefactory/client.go",
    "line": 103,
    "description": "[WR-01] WithHTTPClient silently discards the configured timeout. Code review 02-REVIEW.md, scoped out of the phase-2 fix pass.",
    "status": "fixed",
    "reason": "",
    "recorded_at": "2026-08-29T12:40:00.000Z",
    "resolved_at": "2026-09-03T19:57:57.830Z"
  },
  {
    "id": 9,
    "kind": "todo",
    "phase": "02",
    "file": "internal/imagefactory/client.go",
    "line": 312,
    "description": "[WR-05] One additive upstream field takes the whole Images screen down — DisallowUnknownFields on the catalog read turns an upstream addition into a 502. Verifier confirmed still present and still touching a phase-2 success criterion.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-29T12:40:00.000Z",
    "resolved_at": null
  },
  {
    "id": 10,
    "kind": "todo",
    "phase": "02",
    "file": "web/src/routes/images.tsx",
    "line": 455,
    "description": "[WR-06] A cleared META key silently becomes slot 0 via Number(''); out-of-range keys surface as a raw decoder error. Verifier confirmed still present and still touching a phase-2 success criterion.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-29T12:40:00.000Z",
    "resolved_at": null
  },
  {
    "id": 11,
    "kind": "todo",
    "phase": "02",
    "file": "internal/talossim/talossim.go",
    "line": 183,
    "description": "[WR-07] talossim.New leaks both listeners when seeding fails. Test infrastructure, not production code.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-29T12:40:00.000Z",
    "resolved_at": null
  },
  {
    "id": 12,
    "kind": "todo",
    "phase": "02",
    "file": "internal/talos/client.go",
    "line": 357,
    "description": "[CR-04] The stream error path never classifies: RecvMsg calls s.cancel() then classify(s.Context(), ...), and classify returns the error untouched when ctx.Err() is context.Canceled — so KindUnreachable/KindRejected are unreachable on streams. Found by the fixer while writing the WR-02 tests, confirmed independently by the verifier. Lands on Phase 5, which consumes streams.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-29T12:40:00.000Z",
    "resolved_at": null
  },
  {
    "id": 13,
    "kind": "todo",
    "phase": "02",
    "file": "internal/httpapi/handlers/schematics.go",
    "line": 66,
    "description": "[IN-01] schematicInput.Cluster is accepted, unvalidated and never sent.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-29T12:40:00.000Z",
    "resolved_at": null
  },
  {
    "id": 14,
    "kind": "todo",
    "phase": "02",
    "file": "web/src/routes/images.tsx",
    "line": 927,
    "description": "[IN-02] CopyButton never returns to its resting label.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-29T12:40:00.000Z",
    "resolved_at": null
  },
  {
    "id": 15,
    "kind": "todo",
    "phase": "02",
    "file": "internal/imagefactory/client.go",
    "line": 85,
    "description": "[IN-03] The installer repository cache never expires.",
    "status": "fixed",
    "reason": "",
    "recorded_at": "2026-08-29T12:40:00.000Z",
    "resolved_at": "2026-08-29T21:31:38.594Z"
  },
  {
    "id": 16,
    "kind": "todo",
    "phase": "02",
    "file": "internal/httpapi/handlers/schematics.go",
    "line": 205,
    "description": "[IN-04] Raw JSON decoder errors reach the client.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-29T12:40:00.000Z",
    "resolved_at": null
  },
  {
    "id": 17,
    "kind": "todo",
    "phase": "02",
    "file": "internal/imagefactory/imagefactory.go",
    "line": 37,
    "description": "[IN-05] DefaultBaseURL is documented as overridable but nothing overrides it.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-29T12:40:00.000Z",
    "resolved_at": null
  },
  {
    "id": 18,
    "kind": "todo",
    "phase": "02",
    "file": "internal/httpapi/handlers/schematics.go",
    "line": 156,
    "description": "[IN-06] NewestStable's reason is discarded entirely.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-29T12:40:00.000Z",
    "resolved_at": null
  },
  {
    "id": 19,
    "kind": "unrun-verify",
    "phase": "02",
    "file": "Taskfile.yml",
    "line": null,
    "description": "Plan 02-11 could not run 'task lint:go' (golangci-lint run): golangci-lint is not installed on this host. gofmt -l and go vet are clean. Same gap as the 02-06 entry.",
    "status": "fixed",
    "reason": "",
    "recorded_at": "2026-08-29T17:10:58.006Z",
    "resolved_at": "2026-08-29T17:16:43.845Z"
  },
  {
    "id": 20,
    "kind": "deviation",
    "phase": "02",
    "file": "internal/imagefactory/installer.go",
    "line": null,
    "description": "A cold resolveInstallerRepo still walks two candidates serially at DefaultTimeout=30s each, so GET /schematics/{id}/assets keeps a 2x30=60.000s worst case against writeTimeout=60s -- the composition G-02-2 measured as status=502 duration=1m0.002907792s, unchanged by plan 02-12. The bounded re-question that plan adds asks only the never-ruled-out candidate, costs at most 1x30s and cannot reach that ceiling. Bounding the cold path needs the per-route deadline 02-DECISION-probe-budget.md owns; G-02-2 is deferred to cluster A, and cmd/holzkubed/budget_test.go declares the route as known-over-budget.",
    "status": "fixed",
    "reason": "",
    "recorded_at": "2026-08-29T17:27:33.353Z",
    "resolved_at": "2026-09-03T19:57:57.966Z"
  },
  {
    "id": 21,
    "kind": "deviation",
    "phase": "02",
    "file": "internal/model/model.go",
    "line": null,
    "description": "model.Schematic.Arch is additive and unversioned, so every schematic stored before plan 02-13 carries an empty architecture and renders its verdict unqualified forever; those are precisely the records created while the G-02-8 arch leak was live, and the architecture a past probe used is not recoverable from the record, so there is nothing to backfill -- new records are qualified, old ones are readable only by deleting and re-authoring.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-29T17:51:17.158Z",
    "resolved_at": null
  },
  {
    "id": 22,
    "kind": "todo",
    "phase": "02",
    "file": "internal/imagefactory/installer.go",
    "line": 244,
    "description": "[R2-WR-02] No single-flight: every concurrent request on a cold or stale installer-repo key issues its own registry resolution. Round-2 code review, scoped out by the user. Also the only fix that would make two concurrent callers agree on one reference in the mirror ordering storeInstallerRepo does not close (see its doc comment).",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-29T21:31:26.855Z",
    "resolved_at": null
  },
  {
    "id": 23,
    "kind": "todo",
    "phase": "02",
    "file": "internal/httpapi/handlers/schematics_test.go",
    "line": 1195,
    "description": "[R2-WR-06] The provisional-warning branch of GET /assets has no handler-level test: the warning is asserted in imagefactory and rendered in the web suite, but nothing pins that it survives the handler. Round-2 code review, scoped out by the user.",
    "status": "fixed",
    "reason": "",
    "recorded_at": "2026-08-29T21:31:27.059Z",
    "resolved_at": "2026-08-30T09:59:55.342Z"
  },
  {
    "id": 24,
    "kind": "todo",
    "phase": "02",
    "file": "internal/imagefactory/installer.go",
    "line": 360,
    "description": "[R2-IN-01] The raw transport error is echoed verbatim into the operator-facing fallback warning detail, which is long-lived because it rides every cached answer. Via probe.go:104. Round-2 code review, scoped out by the user.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-29T21:31:27.326Z",
    "resolved_at": null
  },
  {
    "id": 25,
    "kind": "todo",
    "phase": "02",
    "file": "docs/api-contract.md",
    "line": 646,
    "description": "[R2-IN-02] The SecureBoot assets example omits the warnings field that the same section calls mandatory. Round-2 code review, scoped out by the user.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-29T21:31:27.604Z",
    "resolved_at": null
  },
  {
    "id": 26,
    "kind": "todo",
    "phase": "02",
    "file": "docs/api-contract.md",
    "line": 679,
    "description": "[R2-IN-03] The contract describes the provisional installer outcome more narrowly than the code produces it. Round-2 code review, scoped out by the user.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-29T21:31:27.863Z",
    "resolved_at": null
  },
  {
    "id": 27,
    "kind": "todo",
    "phase": "02",
    "file": "web/src/api.ts",
    "line": 283,
    "description": "[R2-IN-04] WARNING_INSTALLER_REPO_FALLBACK_UNVERIFIED is exported and pinned by the Go drift guard but unused by application code. Round-2 code review, scoped out by the user.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-29T21:31:28.120Z",
    "resolved_at": null
  },
  {
    "id": 28,
    "kind": "todo",
    "phase": "02",
    "file": "web/src/routes/images.tsx",
    "line": 100,
    "description": "[R2-IN-05] Two small dead or silent branches in the images route, also at images.tsx:579. Round-2 code review, scoped out by the user.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-29T21:31:28.399Z",
    "resolved_at": null
  },
  {
    "id": 29,
    "kind": "todo",
    "phase": "02",
    "file": "internal/httpapi/handlers/schematics.go",
    "line": 66,
    "description": "[R2-RESIDUAL] A lone surrogate submitted through the API rather than the browser is still silently rewritten to U+FFFD by encoding/json before the schematic id is computed, so a non-browser caller gets an id over a character it never sent. Commit ec10e08 refused it at the input, which makes the utf8.ValidString branch unreachable from the HTTP route but leaves API clients exposed. T-02-67. The honest fix is a raw-bytes check in schematicInput.validate before decoding; widening the canonical serialiser would move the precomputed id FACT-06 rests on.",
    "status": "fixed",
    "reason": "",
    "recorded_at": "2026-08-29T21:31:28.635Z",
    "resolved_at": "2026-08-30T09:59:55.616Z"
  },
  {
    "id": 30,
    "kind": "unrun-verify",
    "phase": "02",
    "file": "internal/imagefactory/live_test.go",
    "line": null,
    "description": "The SecureBoot installer matrix is a partial observation and the ledger never said so. 02-09-PLAN.md:447 required conjunctively that a throttled live run be recorded AND that this entry be appended; it throttled (first run: metal-installer and metal-installer-secureboot at v1.12.0 both exceeded the 60s client timeout without an HTTP response, the SecureBoot pairing subtest failed the same way after 235s) and the entry was never filed, so a ship gate reading the register learned none of the following. (1) The matrix is two versions wide: v1.13.9, the pin, and v1.12.0, MinSupportedVersion. (2) talos.MaxSupportedVersion is v1.14, a range bound and not a concrete tag, so it has never been probed and cannot be probed as one; live_test.go:499-517 says so in place. (3) No v1.13.x below the pin has ever been probed, and the Factory's newest stable inside v1.12..v1.14 is the pin itself, so no concrete version exists in that window to probe today. (4) The fake's v1.9.0 SecureBoot cell remains an assumption labelled as one. Filed by plan 02-21 closing G-02-21; WINDOWS entry 5 (the throttle itself) stays open independently. See also the version-range entry handed over by plan 02-16.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:13.016Z",
    "resolved_at": null
  },
  {
    "id": 31,
    "kind": "deviation",
    "phase": "02",
    "file": "internal/model/model.go",
    "line": null,
    "description": "SUPERSEDES THE RECOVERABILITY CLAUSE OF ENTRY 21, which stays open for the window itself (the tool has append/fixed/waive and no amend, so a correction has to be a new entry). Entry 21 says flatly that 'the architecture a past probe used is not recoverable from the record'. That is true of a record whose probe succeeded or never answered, and FALSE of one it refused: a refusal stores the architecture verbatim in probe_reason, in the shape '<id> at <version>/<arch> answered HTTP <status>' produced by imagefactory/probe.go and now pinned by handlers.TestRefusalReasonNamesTheArchitectureItAskedAbout, so it is machine-parseable out of that sentence. The claim is narrowed and not inverted: recovering an architecture that way means parsing prose written for an operator to read, which is weaker and more fragile than a field and one rewording away from being wrong, and nothing does or should. The window entry 21 records is unchanged -- pre-02-13 records still render unqualified verdicts, and there is still nothing to backfill, because a refused record has no usable verdict to qualify. Filed by plan 02-21 closing G-02-22.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:13.303Z",
    "resolved_at": null
  },
  {
    "id": 32,
    "kind": "unmet-truth",
    "phase": "02",
    "file": "docs/api-contract.md",
    "line": null,
    "description": "[from 02-14] The refusal set stated in docs/api-contract.md is a floor, not a measurement. Six regions are refused by extrapolation from measured ends rather than by observation: twenty-six of the thirty-two C1 codepoints, the interior of the range above U+FFFF, U+FDD1-U+FDEF, the leading and trailing positions for all but three groups, and the surrogates. The contract states the set as if every member had been observed refused by the Factory.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:13.572Z",
    "resolved_at": null
  },
  {
    "id": 33,
    "kind": "stub",
    "phase": "02",
    "file": "web/src/routes/images.tsx",
    "line": null,
    "description": "[from 02-14, coverage item D7] images.tsx under-refuses relative to the server: hasControlCharacter and the doc comment above it claim to transcribe the server's representable set, but the guard stops only runes below U+0020, U+007F and lone surrogates. An operator typing an emoji, a BOM or U+2028 into a kernel argument learns of it from the server's 400 instead of from the row while they are still looking at it. Behaviour is safe (the 400 is the documented backstop).",
    "status": "fixed",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:13.819Z",
    "resolved_at": "2026-08-30T09:59:55.875Z"
  },
  {
    "id": 34,
    "kind": "unmet-truth",
    "phase": "02",
    "file": "Taskfile.yml",
    "line": null,
    "description": "[from 02-14 item 3, SUPERSEDED-AS-WRITTEN, and from 02-16] golangci-lint is NOT absent from this host, and two plans recorded that it was. It is installed at ~/go/bin/golangci-lint (2.13.1, the version CI pins) and is merely off the default PATH, so a bare 'golangci-lint run' or 'task lint:go' fails with command-not-found and reads as a missing tool. That misreading has now produced two unrun-verify entries (6 and 19, both since marked fixed) and one SUMMARY claiming a permanent tooling gap, which invites the next plan to skip the Go lint gate for a reason that is not true. What is filed here is the PATH trap, not an absent tool: 02-14's entry as drafted would have re-filed the falsehood. The repo lints clean at 0 issues with that path exported. Open until the lint task resolves the binary rather than depending on the caller's PATH.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:14.049Z",
    "resolved_at": null
  },
  {
    "id": 35,
    "kind": "unrun-verify",
    "phase": "02",
    "file": "internal/imagefactory/canonical_live_test.go",
    "line": null,
    "description": "[from 02-14] TestLiveCanonical is opt-in and runs against a public service; nothing in CI executes it. A clean run is 16 POSTs to factory.talos.dev. The class it guards is invisible to the offline suite because the fake accepts documents the real Factory refuses, so the offline suite passing says nothing about canonicalisation drift.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:14.271Z",
    "resolved_at": null
  },
  {
    "id": 36,
    "kind": "skipped-test",
    "phase": "02",
    "file": "internal/talossim/scenario_test.go",
    "line": 311,
    "description": "[from 02-14] TestScenarioGoSilent flaked once under -race and passed on re-run. Not investigated; it was out of 02-14's scope. It asserts that the client's own deadline is what fires, which is a timing assertion, so a flake there is either a real race or a machine-load artefact and the entry does not claim to know which.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:14.503Z",
    "resolved_at": null
  },
  {
    "id": 37,
    "kind": "deviation",
    "phase": "02",
    "file": "internal/imagefactory/canonical.go",
    "line": null,
    "description": "[from 02-14] Schematics stored before plan 02-14 are not re-validated against the codepoints that plan started refusing. Any record whose kernel_args or meta carried one holds an id the Factory did not assign. There is no re-validation pass and no migration, nothing enumerates such records, and the Factory will not enumerate schematics either. Same shape as WINDOWS entry 21 and as the name/cluster residual handed over by plan 02-20 -- different fields, same unmigratable population.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:14.817Z",
    "resolved_at": null
  },
  {
    "id": 38,
    "kind": "lint-warning",
    "phase": "02",
    "file": "internal/imagefactory/canonical_live_test.go",
    "line": 627,
    "description": "[from 02-16] QF1012 staticcheck: WriteString(fmt.Sprintf(...)) should be fmt.Fprintf(...). Pre-existing from plan 02-14, unchanged since bc9be70. golangci-lint run does not exit 0 until it is fixed, and plan 02-16 was prohibited from editing the file.",
    "status": "fixed",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:15.074Z",
    "resolved_at": "2026-08-30T09:59:56.138Z"
  },
  {
    "id": 39,
    "kind": "deviation",
    "phase": "02",
    "file": "internal/imagefactory/warnings.go",
    "line": null,
    "description": "[from 02-16] warnings.go was edited outside plan 02-16's declared files_modified, to declare installer.secureboot-repo-fallback-unverified. Unavoidable for the option that plan chose; recorded so the file set a later reader reconstructs from the plan frontmatter is known to be incomplete.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:15.347Z",
    "resolved_at": null
  },
  {
    "id": 40,
    "kind": "todo",
    "phase": "02",
    "file": "web/src/api.ts",
    "line": null,
    "description": "[from 02-16] The installer.secureboot-repo-fallback-unverified warning code had no TypeScript mirror constant and no row in the contract's warning table when 02-16 introduced it. Handed to plan 02-19 task 4. The contract calls the warning taxonomy closed while it had gained a fourth member unannounced.",
    "status": "fixed",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:15.606Z",
    "resolved_at": "2026-08-30T09:59:56.462Z"
  },
  {
    "id": 41,
    "kind": "unmet-truth",
    "phase": "02",
    "file": "internal/imagefactory/live_test.go",
    "line": null,
    "description": "[from 02-16] The installer matrix has still never probed any v1.13.x below the pin, and v1.14.x does not exist yet. installerCandidates' comment now says this explicitly rather than claiming the range is settled, and live_test.go's third row will probe v1.14.x automatically once upstream ships it -- so the gap closes itself on that side and does not on the other. Distinct from the entry plan 02-21 filed for G-02-21: that one records that 02-09's acceptance criterion went unfiled, this one records the version range itself and the self-updating row.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:15.890Z",
    "resolved_at": null
  },
  {
    "id": 42,
    "kind": "unmet-truth",
    "phase": "02",
    "file": "web/src/routes/images.browser.test.tsx",
    "line": null,
    "description": "[from 02-17] Only one browser engine is measured. UAX #14's hyphen rule is a specification and every engine implements it, but the fix is a CSS rule and the measurement is one engine's: Chromium, via playwright 1.62.1, headless, at a 1200x900 viewport. Firefox and WebKit are unmeasured. Severity low -- white-space: nowrap is not a corner of CSS where engines disagree -- but the claim 'occupies one line box' is true of Chromium and inferred elsewhere.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:16.242Z",
    "resolved_at": null
  },
  {
    "id": 43,
    "kind": "todo",
    "phase": "02",
    "file": "internal/imagefactory/guard_drift_test.go",
    "line": 138,
    "description": "[from 02-17, residual 2] INSTALLER_REPOSITORY_NAMES was not pinned to installerCandidates: a fifth Go candidate would leave the web sweep measuring four stale strings and still passing -- a smaller instance of exactly the defect 02-17 closed. Marked open until plan 02-20 landed the guard.",
    "status": "fixed",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:16.507Z",
    "resolved_at": "2026-08-30T09:59:56.712Z"
  },
  {
    "id": 44,
    "kind": "unrun-verify",
    "phase": "02",
    "file": ".github/workflows/ci.yml",
    "line": 71,
    "description": "[from 02-17] The CI browser-install step has never run. Its form and ordering are verified by reading the file and its local equivalent was exercised, but no CI run has happened against this commit, and 'playwright install --with-deps chromium' on ubuntu-latest is untested. The '--' argument-forwarding bug 02-17 fixed in that same line is precisely the class of failure only a real run confirms is gone.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:16.753Z",
    "resolved_at": null
  },
  {
    "id": 45,
    "kind": "deviation",
    "phase": "02",
    "file": "internal/httpapi/problem.go",
    "line": null,
    "description": "[from 02-18] Wire-format change to a field the contract calls stable: every problem type moved from https://holzkube.dev/problems/<suffix> to urn:holzkube-manager:problem:<suffix>. A client matching on type rather than on code breaks. The project has no external clients today, which is why this is a note and not a migration -- the note is the difference between having decided that and not having noticed it. Matches T-02-90 (Repudiation, disposition accept).",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:17.027Z",
    "resolved_at": null
  },
  {
    "id": 46,
    "kind": "deviation",
    "phase": "02",
    "file": "internal/httpapi/middleware/audit_test.go",
    "line": 126,
    "description": "[from 02-18] One fixture will not follow the next re-rooting automatically: audit_test.go:126 spells the problem-type base literally, by necessity (import cycle) and by intent (it stands in for an upstream writer). A future re-rooting must edit it by hand, and nothing fails first if it is forgotten, because the middleware never reads the field.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:17.262Z",
    "resolved_at": null
  },
  {
    "id": 47,
    "kind": "deviation",
    "phase": "02",
    "file": "docs/api-contract.md",
    "line": null,
    "description": "[from 02-18] The URN namespace identifier holzkube-manager is unregistered with IANA. Documented in both problem.go and docs/api-contract.md as acceptable -- RFC 9457 asks for a URI, not a registered namespace -- and recorded here so the decision is visible rather than assumed.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:17.526Z",
    "resolved_at": null
  },
  {
    "id": 48,
    "kind": "deviation",
    "phase": "02",
    "file": "internal/imagefactory/installer.go",
    "line": null,
    "description": "[from 02-19] READ WITH ENTRY 20, WHICH STAYS OPEN. Plan 02-19 changed what an unresolved installer RETURNS (502 with no body became 200 with installer:null plus installer_error), not the cold 2 x 30s serial candidate walk that produces it. The 49.8s and 60.002s observations recorded in G-02-15 are that composition and are unchanged. 'The panel now shows four references on a timeout' must not be read as the timeout having been addressed; the ceiling against writeTimeout=60s is still owned by 02-DECISION-probe-budget.md, and the milder symptom makes deferring that decision easier rather than more defensible.",
    "status": "fixed",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:17.754Z",
    "resolved_at": "2026-09-03T19:57:58.102Z"
  },
  {
    "id": 49,
    "kind": "deviation",
    "phase": "02",
    "file": "docs/api-contract.md",
    "line": null,
    "description": "[from 02-19] The assets route's response shape changed for two outcomes: 502 with no body became 200 with installer:null plus installer_error, so installer is now nullable on every answer. No external clients exist, which is why this is a note and not a migration. Matches T-02-91/T-02-92.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:18.030Z",
    "resolved_at": null
  },
  {
    "id": 50,
    "kind": "todo",
    "phase": "02",
    "file": "docs/api-contract.md",
    "line": 646,
    "description": "[from 02-19] Entries 25 and 26 (R2-IN-02, R2-IN-03) sit inside the assets section plan 02-19 rewrote and were deliberately left alone: both are marked 'scoped out by the user', and a scoping decision is not an executor's to overturn. Entry 25's SecureBoot example still omits the warnings field the same section calls mandatory, and it now sits beside a table enumerating what warnings means in every outcome, so the inconsistency is more visible than before. Worth re-offering to the user rather than closing silently.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:18.285Z",
    "resolved_at": null
  },
  {
    "id": 51,
    "kind": "todo",
    "phase": "02",
    "file": "web/src/api.ts",
    "line": 344,
    "description": "[from 02-19] AMENDS ENTRY 27, WHICH STAYS OPEN (no amend verb exists). Entry 27 records WARNING_INSTALLER_REPO_FALLBACK_UNVERIFIED as exported, pinned by the Go drift guard and unused by application code. Still true, and now true of a second constant: WARNING_INSTALLER_SECUREBOOT_REPO_FALLBACK_UNVERIFIED is also unused, because SchematicWarnings renders code/detail generically with no per-code branch. That is the intended design -- an unknown code still reaches the operator -- so the entry is about the constants' purpose being drift-pinning rather than rendering. Re-word rather than close.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:18.604Z",
    "resolved_at": null
  },
  {
    "id": 52,
    "kind": "todo",
    "phase": "02",
    "file": "internal/httpapi/handlers/schematics.go",
    "line": 66,
    "description": "[from 02-20] AMENDS ENTRY 13, WHICH STAYS OPEN (no amend verb exists). Entry 13 reads 'schematicInput.Cluster is accepted, unvalidated and never sent'. The UNVALIDATED half is closed: cluster now goes through NotRepresentableReason in validate, and a refused codepoint in it is a 400 naming cluster. The NEVER SENT half is unchanged and still true -- cluster is stored on the record, reaches no other layer, and the authoring form offers no input for it. Amended text: 'schematicInput.Cluster is validated and stored but never sent anywhere and has no form input.'",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:18.930Z",
    "resolved_at": null
  },
  {
    "id": 53,
    "kind": "deviation",
    "phase": "02",
    "file": "internal/httpapi/handlers/schematics.go",
    "line": null,
    "description": "[from 02-20] Records stored before plan 02-20 may carry refused codepoints in name or cluster. Nothing migrates them and there is no backfill to write, because the values are the operator's own text and repairing them would be the silent rewrite that plan exists to prevent. Such a record is readable but not re-creatable, and there is no edit route -- POST, GET, GET /{id}, GET /{id}/assets and DELETE /{id} are the whole surface -- so an operator holding one must delete and re-author it. The saved table renders it safely in the meantime via StoredText, which is why this is a residual and not a defect.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:19.158Z",
    "resolved_at": null
  },
  {
    "id": 54,
    "kind": "todo",
    "phase": "02",
    "file": "web/src/routes/images.tsx",
    "line": 329,
    "description": "[from 02-20] The client cannot guard cluster because the authoring form has no cluster input; the server does guard it. When a cluster input is added it belongs in the single hasUnusableValue computation in images.tsx, which already carries a comment saying so.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:19.461Z",
    "resolved_at": null
  },
  {
    "id": 55,
    "kind": "todo",
    "phase": "02",
    "file": "internal/httpapi/handlers/schematics.go",
    "line": null,
    "description": "[from 02-20] createProblem's NotRepresentableError branch is now unreachable from the HTTP route in practice: refuseUnrepresentable covers every document path the request vocabulary names, so the branch fires only for a future field that forgets a check, or for an in-process caller. It is kept deliberately -- deleting it would turn the first of those into a 502 blaming the Factory, which is G-02-6 -- but it is a branch no route-level test can reach, the same shape as the utf8.ValidString note that produced entry 29.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:19.676Z",
    "resolved_at": null
  },
  {
    "id": 56,
    "kind": "unrun-verify",
    "phase": "02",
    "file": "internal/imagefactory/guard_drift_test.go",
    "line": 65,
    "description": "[from 02-20] The codepoint sweep in guard_drift_test.go is exhaustive over Unicode but the SERVER set behind it is not exhaustively measured; it inherits 02-14's extrapolation (U+FDD1-U+FDEF, twenty-six C1 codepoints, and the interior of the range above U+FFFF are refused on the strength of measured ends). The guard proves the two layers agree; it cannot prove the set is right. Extends the 02-14 floor entry rather than starting a new claim.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:19.914Z",
    "resolved_at": null
  },
  {
    "id": 57,
    "kind": "deviation",
    "phase": "02",
    "file": "internal/imagefactory/installer.go",
    "line": null,
    "description": "[from 02-24] SUPERSEDES ENTRIES 20 AND 48, BOTH NOW MARKED FIXED (no amend verb exists). What moved: resolveInstallerRepo no longer walks its candidates serially -- plan 02-23 issues every candidate at once on a context derived from the caller's and decides in declared candidate order, so the route's worst case is one candidate's budget rather than the sum. The candidates are bounded by imagefactory.ManifestTimeout=30s (plan 02-22) and not by DefaultTimeout; the manifest GET and the ISO probe have budgets of their own (ManifestTimeout=30s, ProbeTimeout=90s); both Factory routes declare a shared route deadline (handlers.AssetsRouteBudget = ManifestTimeout + 5s = 35s, handlers.CreateRouteBudget = ProbeTimeout + DefaultTimeout = 120s); writeTimeout rose from 60s to 130s and now names the budgets it covers; and cmd/holzkube-managerd/budget_test.go computes the composition from the four constants and declares BOTH Factory routes withinBudget with an empty deferredTo (assets: 30s sum, largest call 30s, route deadline 35s, slack 5s, writeTimeout 2m10s; create: 2m30s sum, largest call 1m30s, route deadline 2m0s). Entry 20's '2x30 = 60.000s worst case against writeTimeout=60s' has no surviving true reading. Marked fixed on exactly that evidence and no other: the composition guard passes and the offline elapsed-time tests pass (TestAssetsRouteAnswersInsideItsCeiling answered in 603.727542ms against a 700ms scaled ceiling; TestCreateRouteAnswersInsideItsCeiling in 623.888083ms against 2.4s). What did NOT move: the 43.42s and 60.002907792s figures entries 20 and 48 record are what the SERIAL walk cost and remain the correct record of it -- they now live in a comment beside installerRepoRetryInterval in internal/imagefactory/installer.go, measured 2026-08-29, so a reader who greps for those numbers finds why they are still written down rather than assuming they are stale. NOTHING re-measured the installer resolution against factory.talos.dev after the change: plan 02-22's two live runs on 2026-09-03 measured the ISO probe (cold 28.445563875s and 31.491771083s, warm 3.752628125s and 2.493417375s) and not the installer resolution. The concurrency improvement is measured offline against fakes and unmeasured against factory.talos.dev -- 305.96ms against 606.93ms serialised at the client, 603.57ms against 1.0599s serialised at the HTTP route. Entry 48's warning survives into this entry: a route that declares a ceiling is not the same as a route that is fast.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-03T19:57:04.257Z",
    "resolved_at": null
  },
  {
    "id": 58,
    "kind": "deviation",
    "phase": "02",
    "file": "internal/httpapi/handlers/schematics.go",
    "line": null,
    "description": "[from 02-24] G-02-9 REMAINS OPEN AFTER ROUND 4. Plan 02-24 gives it the mitigation 02-DECISION-probe-budget.md's ratified Option 2 asks for and no more: the store.ErrConflict branch of POST /api/v1/schematics now writes the probe verdict that request just computed -- Usable, ProbedAt and ProbeReason and no other field, under a compare-and-swap on the read Rev -- instead of discarding it. G-02-9 measured the discard as HTTP 409 in 4.186898875s, a probe that ran and succeeded at that latency against a record that stayed usable=false with probed_at zero. What is still missing, which is why this is a window and not a closure: there is no re-probe route, no button and no job. The recovery is re-submitting the identical customisation, which answers 409 and refreshes the verdict as a side effect. That recovery is not discoverable from the saved list, it is surfaced to the operator as an error on the create form rather than as an action, and it is DECLINED in two cases -- when the stored record's Arch is not the architecture the fresh probe asked about (a record written before Arch existed carries an empty one and is therefore never refreshed), and when the fresh probe does not answer either. A probe that times out again leaves the record exactly as it was, which is 02-DECISION-probe-budget.md's own sentence and the reason it recorded G-02-9 as a known window rather than as fixed. The structural answer is that document's Option 1 -- split the create route, add a third probe state, add an explicit re-probe endpoint -- which the user did not take.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-03T19:57:30.343Z",
    "resolved_at": null
  },
  {
    "id": 59,
    "kind": "deviation",
    "phase": "02",
    "file": "cmd/holzkube-managerd/main.go",
    "line": 64,
    "description": "[from 02-22, filed by 02-24] writeTimeout was raised from 60s to 130s process-wide rather than scoped per route, so every route on this server carries the response budget only the two Factory routes need. The per-route alternative requires Flush, Unwrap and Hijack on the three middleware ResponseWriter wrappers, which is exactly the Phase 5 entry blocker recorded in 02-CONTEXT.md <deadline_policy> (lines 256-271: none of the three wrappers implements them, so a streaming endpoint on this chain silently buffers). Cross-referenced here so Phase 5 finds this rather than re-deriving it.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-03T19:57:30.475Z",
    "resolved_at": null
  },
  {
    "id": 60,
    "kind": "deviation",
    "phase": "02",
    "file": "internal/httpapi/handlers/schematics.go",
    "line": 58,
    "description": "[from 02-22, filed by 02-24] CreateRouteBudget = ProbeTimeout + DefaultTimeout = 120s is deliberately smaller than the sum of its route's three per-call budgets, which is DefaultTimeout 30s (Extensions) + DefaultTimeout 30s (CreateSchematic) + ProbeTimeout 90s (ProbeBuildable) = 150s. On a pathological upstream the probe is therefore cut by the route deadline before its own budget expires, and the clipping is exactly one JSON budget wide. Recorded rather than fixed because the outcome is the existing fail-safe: ProbedAt stays zero and the record is stored unprobed, which reads as 'no verdict' and never as 'the Factory refused'. The composition guard logs both numbers on every run (2m30s sum against a 2m0s route deadline).",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-03T19:57:30.611Z",
    "resolved_at": null
  },
  {
    "id": 61,
    "kind": "deviation",
    "phase": "02",
    "file": "internal/httpapi/middleware/audit.go",
    "line": null,
    "description": "[from 02-24] The audit middleware derives its outcome from the response status (audit.go lines 85-105), so a 409 from POST /api/v1/schematics that REFRESHED the record's three probe fields is filed as OutcomeError with cause store.conflict -- byte-identical to a 409 that changed nothing. There is no handler-side enrichment and this plan added none. The sentence an auditor needs: the permanent archive does not show the refresh, so a record whose Usable, ProbedAt and ProbeReason changed on a conflicting POST has no archive entry saying so, and the only evidence is the record itself. Left as it is deliberately (T-02-112, disposition accept): changing it reopens Phase 1's fail-closed intent/outcome contract, which is not this round's to touch.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-03T19:57:30.743Z",
    "resolved_at": null
  },
  {
    "id": 62,
    "kind": "todo",
    "phase": "02",
    "file": "web/src/routes/images.tsx",
    "line": null,
    "description": "[from 02-23, filed by 02-24] The progress indicator .planning/research/PITFALLS.md:164 requires is still not built, and stating a ceiling is a different thing. Missing by name: elapsed time, the current sub-step, an expected duration, and a disabled action button carrying its reason. PITFALLS.md:646 names 'A spinner during the post-apply install window' as the anti-pattern and 'Named state, elapsed time, expected duration, current sub-step' as the approach. What plan 02-23 shipped is one static sentence per waiting state naming the server's own ceiling -- CREATE_WAIT_SECONDS=120 and ASSETS_WAIT_SECONDS=35 in web/src/routes/images.tsx -- which is a maximum rather than a prediction, and the create button is disabled while pending with no reason rendered beside it. The comment beside those two constants says so in the code, so a reader who finds a number there cannot mistake it for the requirement having been met. 02-DECISION-probe-budget.md names this as the risk its ratified Option 2 accepts.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-03T19:57:30.876Z",
    "resolved_at": null
  },
  {
    "id": 63,
    "kind": "todo",
    "phase": "02",
    "file": "internal/imagefactory/client.go",
    "line": 103,
    "description": "[from 02-24] SUPERSEDES ENTRY 8, WHICH IS NOW MARKED FIXED. The fixed verb carries no reason, so the reason lives here. Entry 8 reads '[WR-01] WithHTTPClient silently discards the configured timeout'. That was true while the budget lived on http.Client.Timeout: the option shallow-copied the caller's client and the copy's zero Timeout overwrote whatever WithTimeout had set, silently and order-dependently. Plan 02-22 moved the three budgets off http.Client.Timeout entirely and applies them per call from the Client, so there is no client-wide timeout left to discard -- WithHTTPClient now clears the copy's Timeout explicitly, and TestClientCarriesNoClientWideTimeout asserts the zero duration both for a default client and for one built through WithHTTPClient with a 7s timeout set. The discarding is gone and the order-dependence with it. This entry is a record and not a defect, and is marked fixed on creation for that reason.",
    "status": "fixed",
    "reason": "",
    "recorded_at": "2026-09-03T19:57:48.980Z",
    "resolved_at": "2026-09-03T19:57:58.235Z"
  },
  {
    "id": 64,
    "kind": "unrun-verify",
    "phase": "02",
    "file": "internal/imagefactory/live_test.go",
    "line": null,
    "description": "[from 02-24] SUPERSEDES ENTRY 5, WHICH STAYS OPEN (no amend verb exists). Entry 5 records that TestLiveFactory is the only drift guard against factory.talos.dev, is opt-in, and is planned by nothing. All three of those are unchanged: this round did not make it non-optional and did not schedule it. What DID change, from plan 02-22: TestLiveFactory now bounds the elapsed time of the probe it runs rather than asserting only err == nil; it measures a genuinely cold probe behind a per-run nonce, so each run authors a schematic the Factory has demonstrably never built; it re-applies ProbeTimeout's derivation rule to the widened sample of seven cold observations (28.45, 30.50, 30.59, 31.18, 31.52, 31.49, 32.69 -- slowest 32.69s, doubled 65.38s, rounded up to 90s, which is the shipped constant), so the next observation that would move the constant arrives as a red test; and a throttled cold measurement now Skipf's with 'NOT OBSERVED' rather than passing, so a run that measured nothing can no longer read as a run that measured something.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-03T19:57:49.113Z",
    "resolved_at": null
  },
  {
    "id": 65,
    "kind": "todo",
    "phase": "02",
    "file": "cmd/holzkube-managerd/budget_test.go",
    "line": null,
    "description": "[from 02-23, filed by 02-24] Known limitation of the composition guard, recorded as a limitation and not as a defect. Relisting the assets row to one declared call while leaving AssetsRouteBudget at two manifest budgets computes uncut against a declared uncut and passes both the R1 and R2 ratchets, so the guard stays green on a route over-provisioned by thirty seconds. Driven locally by plan 02-23 and confirmed green. Tolerated because the failure direction it leaves open is a ceiling that is too generous rather than one that cuts a candidate before it can answer, which is the silent fallback G-02-3 already cost this phase once. Teaching R4 to catch it would require the guard to know how many candidates the handler actually issues, which is exactly the derived count the table's own comment argues against.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-03T19:57:49.247Z",
    "resolved_at": null
  },
  {
    "id": 66,
    "kind": "deviation",
    "phase": "02",
    "file": "internal/httpapi/handlers/schematics.go",
    "line": null,
    "description": "[from 02-26, uebergeben von Plan 02-25] AMENDS ENTRY 58, WHICH STAYS OPEN (no amend verb exists). Eintrag 58 zaehlt die ablehnenden Faelle von `refreshTheStoredVerdict` auf und nennt zwei: die Probe hat nicht geantwortet, und die Architektur stimmt nicht. Seit Plan 02-25 sind es drei — die dritte ist die Talos-Version (`internal/httpapi/handlers/schematics.go`, `if stored.TalosVersion != fresh.TalosVersion`), aus demselben Grund wie die Architektur, weil `Schematic.Canonical()` weder das eine noch das andere emittiert; gemessen hat es Runde 4 der Verifikation (`02-VERIFICATION.md`, `gaps[0]`, `adversarial_checks_run[0]`). Was 58 weiterhin korrekt aufzeichnet, bleibt korrekt: **G-02-9 ist offen**, es gibt weiterhin keine Re-Probe-Route, keinen Button und keinen Job, und die Erholung ist eine Wieder-Einreichung, die als Fehler beantwortet wird. Ueberholt ist allein die Aufzaehlung der Bedingungen.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-04T05:59:09.533Z",
    "resolved_at": null
  },
  {
    "id": 67,
    "kind": "deviation",
    "phase": "02",
    "file": "internal/httpapi/handlers/schematics.go",
    "line": null,
    "description": "[from 02-26, uebergeben von Plan 02-25] Plan 02-24 hat den von `02-DECISION-probe-budget.md` ratifizierten Refresh mit zwei Bedingungen ausgeliefert. Die dritte fehlte, und keine Zeile in `.planning/WINDOWS.md` hat sie bis Runde 4 getragen. Runde 4 der Verifikation hat sie reproduziert statt sie zu erschliessen (`02-VERIFICATION.md`, `adversarial_checks_run[0]`): `siderolabs/cross-version-ext` bei `v1.12.0` authored, wo der Image-Endpunkt ihn ablehnt — Datensatz `talos_version v1.12.0, usable=false, probe_reason \"... at v1.12.0/amd64 answered HTTP 400\"` —, dann die Ablehnung aufgehoben und dieselbe Customisation bei `v1.13.9` erneut gepostet. Ergebnis: dieselbe Id, HTTP 409, und der gespeicherte Datensatz wurde zu `talos_version: v1.12.0, usable: true, probe_reason: \"\"`, rev 1 -> 2. Die wahre v1.12.0-Ablehnung wurde geloescht. Ursache: `Schematic.Canonical()` emittiert weder Architektur noch Version, also sind zwei Versuche, die sich nur in `talos_version` unterscheiden, ein Datensatz. Geschlossen von Plan 02-25 (Commits `45420ee`, `d4ee1f5`, `846d540`, `061b906`); die Reproduktion laeuft seither als `TestConflictAtAnotherTalosVersionErasesNoStoredRefusal`.",
    "status": "fixed",
    "reason": "",
    "recorded_at": "2026-09-04T05:59:17.534Z",
    "resolved_at": "2026-09-04T05:59:33.098Z"
  },
  {
    "id": 68,
    "kind": "deviation",
    "phase": "02",
    "file": ".planning/WINDOWS.md",
    "line": null,
    "description": "[from 02-26, uebergeben von Plan 02-25] **Task 2 von Plan 02-25 hat keine verbleibende Ungenauigkeit gefunden.** Ausdrueckliche Feststellung, kein Ausbleiben einer Pruefung: die Vorpruefung des Tests auf eine echte gespeicherte Ablehnung (`usable: false` und `probe_reason` nennt `otherCatalogVersion`) laeuft und bricht ab, wenn sie nicht zutrifft; die Byte-Identitaet wird mit `marshalRecord` ueber den ganzen Datensatz geprueft und nicht mit `exceptTheProbeFields`; und `web/src/api.ts:203-209` und `:216-219` sowie das Badge `UsabilityVerdict` (`web/src/routes/images.tsx:942-944`) wurden geprueft und sind nach dieser Korrektur weiterhin wahr — `api.ts` nennt die Bedingungen des Refresh gar nicht, sondern verweist auf `docs/api-contract.md`, und das Badge sagt von selbst die Wahrheit, weil der Server keine Unwahrheit mehr speichert. Keine Datei unter `web/` wurde beruehrt.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-04T05:59:17.668Z",
    "resolved_at": null
  },
  {
    "id": 69,
    "kind": "deviation",
    "phase": "02",
    "file": "internal/imagefactory/guard_drift_test.go",
    "line": 49,
    "description": "[from 02-26] Der Browser-Ablehnungs-Waechter war zwischen Plan 02-20 und dieser Runde an keinen Bezeichner verankert: `browserRefusalRange` (`internal/imagefactory/guard_drift_test.go:49-50`) ist ein regulaerer Ausdruck auf `{ from: 0x.., to: 0x.. }`, und `browserRefusalRanges` liess ihn ueber die **ganze** `web/src/routes/images.tsx` laufen. Gemessen in Runde 4 der Verifikation (`02-VERIFICATION.md`, `adversarial_checks_run[1]`, `gaps[1]`; `02-REVIEW.md` WR-04): `REFUSED_RANGES` durchgaengig in `RENAMED_BY_VERIFIER` umbenannt, `TestBrowserRefusalSetEqualsTheServers` erneut gefahren, `ok github.com/holzcloud/holzkube-manager/internal/imagefactory 0.546s` — ein gruener Waechter gegen eine Deklaration, die es nicht mehr gab, waehrend sein eigener `t.Fatalf`-Kommentar genau diese Eigenschaft verspricht (\"A guard that silently passes when it can no longer find what it guards is worse than no guard\"). Nichts war maskiert: die beiden Ablehnungsmengen stimmten und stimmen ueberein, und das Loeschen des U+FEFF-Eintrags liess den Waechter auch vorher schon scheitern — es ging um das kuenftige Verhalten des Waechters. Plan 02-26 hat es geschlossen: `refusedRangesDecl` schneidet die Deklaration zuerst heraus (Zeilenanfangs-Anker mit `regexp.QuoteMeta` um den Bezeichner, wie `budget_drift_test.go:89-90`, und Zuweisungs-Verankerung wie `stringArrayLiteral`), ihr Fehlen ist ein Fehler statt eines kuerzeren Ergebnisses, ein eintragsfoermiges Literal ausserhalb der Deklaration wird nicht mehr eingefaltet und faellt als Abweichung der beiden Trefferzahlen auf (heute gemessen: sechs im Deklarationsrumpf und sechs in der ganzen Datei), und ein Eintrag innerhalb der Deklaration, den der Ausdruck nicht lesen kann, ist ein Fehlschlag mit dem Literal im Text statt eines stillen Ueberspringens. Die Pruefung ist als reine Funktion `parseBrowserRefusalRanges` ohne `*testing.T` herausgeloest, damit ihre Fehlerfaelle selbst pruefbar sind; `TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration` (vier Zeilen) und `TestBrowserRefusalGuardRefusesAnEntryItCannotRead` (zwei Zeilen) pruefen sie. Die Falsifikation des Verifiers wurde zusaetzlich als Lebendbeleg am echten Baum wiederholt, mit umgekehrtem Ergebnis, und `web/src/routes/images.tsx` byteweise wiederhergestellt. Eintrag 56 bleibt daneben offen und traegt eine andere Aussage: dort geht es darum, dass die SERVER-Menge hinter dem Codepoint-Sweep nicht erschoepfend gemessen ist und 02-14s Extrapolation erbt — daran hat diese Runde nichts geaendert. Dieser Eintrag ist eine Aufzeichnung eines in derselben Runde geschlossenen Defekts und wird bei der Anlage als `fixed` markiert, in der Form, die Eintrag 63 etabliert hat; das `fixed`-Verb traegt keinen Grund, also steht er hier.",
    "status": "fixed",
    "reason": "",
    "recorded_at": "2026-09-04T05:59:17.805Z",
    "resolved_at": "2026-09-04T05:59:33.231Z"
  },
  {
    "id": 70,
    "kind": "unmet-truth",
    "phase": "02",
    "file": "internal/imagefactory/guard_drift_test.go",
    "line": 83,
    "description": "[from 02-28] SUPERSEDES ENTRY 69, WHICH STAYS FIXED (no amend verb exists). WAS UEBERHOLT IST: Eintrag 69 sagt 'Plan 02-26 hat es geschlossen: refusedRangesDecl schneidet die Deklaration zuerst heraus ..., ihr Fehlen ist ein Fehler statt eines kuerzeren Ergebnisses' und ist bei der Anlage fixed markiert worden. Runde 5 der Verifikation (02-VERIFICATION.md, gaps[0]) hat gemessen, dass das fuer eine praefixierende Umbenennung nicht galt: REFUSED_RANGES_LEGACY allein lieferte ranges=[{0 0}] err=<nil>, REFUSED_RANGESX lieferte ranges=[{0 1}] err=<nil>, und am echten Baum blieben mit REFUSED_RANGES_LEGACY sowohl TestBrowserRefusalSetEqualsTheServers als auch TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration mit allen vier Untertests gruen. Dazu die zweite Haelfte: die gelesenen Bounds waren nicht validiert, { from: 0xFFFFFFFF, to: 0xFFFFFFFF } fiel auf {-1 -1}, { from: 0x10FFFF, to: 0xFFFFFFFF } auf {1114111 -1} und { from: 0x001f, to: 0x0000 } auf {31 0}, jeweils err=<nil> -- jeder dieser Eintraege deckt keinen Codepoint ab und wurde still uebergangen. WAS AN 69 WEITERHIN GILT: die Beschreibung des unverankerten Zustands zwischen Plan 02-20 und Runde 5, die Messung ok github.com/holzcloud/holzkube-manager/internal/imagefactory 0.546s, und die Feststellung, dass nichts maskiert war -- die beiden Ablehnungsmengen stimmen ueberein und das Loeschen des U+FEFF-Eintrags laesst den Waechter scheitern. Diese Saetze bleiben richtig und werden nicht ueberholt. WAS RUNDE 6 GETAN HAT: Plan 02-27 hat vier Korrekturen an internal/imagefactory/guard_drift_test.go geliefert, jede mit ihrer gemessenen Rot-Ausgabe aus 02-27-SUMMARY.md. Erstens der gebundene Anker: hinter regexp.QuoteMeta(refusedRangesName) sind nur noch Leerraum, eine mit einem Doppelpunkt beginnende Typannotation und das Gleichheitszeichen erlaubt, das verlangte : oder = ist die Wortgrenze, die der Unterstrich in REFUSED_RANGES_LEGACY nicht ist. Rot gemessen als Lebendbeleg am echten Baum: REFUSED_RANGES durchgaengig in REFUSED_RANGES_LEGACY umbenannt, --- FAIL: TestBrowserRefusalSetEqualsTheServers und --- FAIL: TestBrowserRefusalGuardRefusesToPassWithoutItsDeclaration/the_real_route beobachtet, wo Runde 5 beide PASS gemessen hatte, Datei danach byteweise wiederhergestellt (sha256 ca13cf5f5bf2abc1f1e4c9376355363f70d232305ef7beaf9716a42f618ee505 vorher wie nachher, git diff leer). Commits da5c54c (RED) und 2ed7f6b (GREEN). Zweitens die semantische Bound-Validierung to > utf8.MaxRune oder from > to vor dem append, weil strconv.ParseUint(m[1], 16, 32) bis 0xFFFFFFFF akzeptiert und rune(uint64) still verliert; die drei Tabellenzeilen pruefen from 0x0000 to 0xFFFFFFFF, from 0x0061 to 0xFFFFFFFF und from 0x001f to 0x0000 als Teilzeichenketten der Meldung, und die vier in Runde 5 gemessenen Werte erreichen den Codepoint-Sweep nicht mehr. Commits e373099 (RED) und bdaec63 (GREEN). Drittens die getrennte Abschneide-Diagnose: der Zweig len(matches) == 0 && whole > 0 meldet das Abschneiden VOR dem ersten Eintrag mit der Zahl der Eintraege, waehrend der Leer-Zweig weiterhin die woertliche Leer-Meldung liefert, beide Zweige einzeln gemessen. Commits ecdd083 (RED) und 6652eab (GREEN). Viertens die drei Falsifikationstabellen mit jetzt 6, 3 und 2 Zeilen, keine bestehende Zeile entfernt oder umbenannt, Rumpf- wie Dateizahl weiterhin 6. WARUM DIESER EINTRAG OPEN BLEIBT UND WAS IHN SCHLIESST: die Eigenschaft ist allquantifiziert ueber kuenftige Umbenennungen und kuenftige Eintragsformen, und diese Phase hat daran in drei aufeinanderfolgenden Runden je ein engeres Loch gemessen. Ein bei der Anlage geschlossener Eintrag ueber eine solche Eigenschaft war genau der Fehler von 69, und er ist mechanisch und nicht bloss stilistisch: workflow.windows_enforce blockiert /gsd-ship, solange open_count groesser als 0 ist, und ein fixed-Eintrag ist an dieser Grenze unsichtbar. Die Schliessbedingung ist pruefbar und lautet: eine Verifikationsrunde falsifiziert den Waechter selbst in beiden Richtungen -- sie benennt REFUSED_RANGES praefixiert um und sieht TestBrowserRefusalSetEqualsTheServers rot, und sie setzt einen Eintrag mit einer Grenze oberhalb utf8.MaxRune und sieht den Waechter rot -- und erst dann wird dieser Eintrag mit windows fixed markiert. Ob die allquantifizierte Wahrheit nach Plan 02-27 gilt, stellt eine Verifikationsrunde fest; dieser Eintrag behauptet es nicht. DIE NACHBARSCHAFT: Eintrag 56 bleibt daneben offen und traegt eine andere Aussage, naemlich dass die SERVER-Menge hinter dem Codepoint-Sweep nicht erschoepfend gemessen ist und 02-14s Extrapolation erbt; er wird von diesem Eintrag nicht ueberholt. Die Eintraege 58 und 66 tragen G-02-9 und bleiben unberuehrt offen. G-02-9 IST OFFEN.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-04T19:29:24.329Z",
    "resolved_at": null
  },
  {
    "id": 71,
    "kind": "deviation",
    "phase": "02",
    "file": "internal/httpapi/handlers/schematics.go",
    "line": 458,
    "description": "[from 02-28] Der Doc-Verweis bei internal/httpapi/handlers/schematics.go:458 nannte zwei Bedingungen, 55 Zeilen ueber einer Funktion, die seit d4ee1f5 drei prueft (fresh.ProbedAt.IsZero(), stored.Arch != fresh.Arch, stored.TalosVersion != fresh.TalosVersion). Gefunden von Runde 5 der Verifikation (02-VERIFICATION.md, gaps[2], Wahrheitszeile 13 der Tabelle). WARUM ER ENTSTANDEN IST: die Wahrheit aus Plan 02-25 ist allquantifiziert -- jede Aussage im Baum, die bisher zwei Bedingungen des Refresh nennt, nennt jetzt drei --, aber ihr Task 3 hat drei Stellen namentlich aufgezaehlt (internal/model/model.go bei ID und bei TalosVersion sowie docs/api-contract.md:620-627) und genau diese nachgezogen. Eine Aufzaehlung findet keine vierte Stelle, und die vierte lag im selben File wie die in Plan 02-25 geaenderte Funktion. WAS RUNDE 6 GETAN HAT: das Zahlwort in Zeile 458 korrigiert, so dass der ueber :458-459 verteilte Satz jetzt 'see refreshTheStoredVerdict for the three conditions and for why a failed refresh is silent' lautet (Commit 3a060ef, Diff genau eine Kommentarzeile), und die Aufzaehlung durch zwei mechanische Sweeps ersetzt. Sweep A ist grep -rn nach refreshTheStoredVerdict mit --include fuer .go, .md, .ts und .tsx ueber internal, docs, web und cmd, dessen Ausgabe anschliessend mit grep -c auf das englische Zahlwort fuer 2 als eigenstaendiges Wort gezaehlt wird: gemessen 1 vor und 0 nach der Aenderung. Sweep A ist als Abnahmebedingung von Plan 02-28 festgenagelt und ersetzt damit die Aufzaehlung dauerhaft durch eine Suche. Sweep B ist grep -rniE nach den Mustern refresh oder 409 ueber dieselben Pfade und Endungen, dessen Ausgabe mit grep -icE auf die Zahlwoerter two, zwei oder both gezaehlt wird: gemessen 5 vor und 4 nach der Aenderung. Beide Kommandos stehen in ihrer vollstaendigen Form mit den Rohrzeichen in 02-28-SUMMARY.md; hier sind sie beschrieben statt zitiert, weil die Markdown-Repraesentation dieses Ledgers rohrgetrennt ist. DIE VIER VERBLEIBENDEN FUNDSTELLEN sind einzeln beurteilt und richtig: schematics.go:628 (versionMismatchReason spricht aus einer der drei Bedingungen heraus ueber die beiden anderen und zaehlt sie nicht) sowie schematics_test.go:3143, :3178 und :3244 (sie sagen both architectures bzw. both Talos versions ueber je zwei benannte Werte in einer Fehlermeldung -- ArchAMD64 gegen ArchARM64 und catalogVersion gegen otherCatalogVersion -- und nicht ueber eine Zahl von Bedingungen). Eintrag 66 bleibt daneben stehen: er amendiert 58 um die dritte ablehnende Bedingung und ist von dieser Runde unberuehrt. DIE GRENZE DIESER MESSUNG: die beiden Sweeps decken internal, docs, web und cmd ueber die Endungen .go, .md, .ts und .tsx ab und nichts darueber hinaus. Dieser Eintrag wird bei der Anlage als fixed markiert, in der Form, die Eintrag 63 etabliert hat, weil der Defekt mit einer endlichen, wiederholbaren Messung ueber den ganzen Baum geschlossen ist und nicht mit einer Behauptung ueber kuenftiges Verhalten; das fixed-Verb traegt keinen Grund, also steht er hier.",
    "status": "fixed",
    "reason": "",
    "recorded_at": "2026-09-04T19:29:31.724Z",
    "resolved_at": "2026-09-04T19:29:36.022Z"
  },
  {
    "id": 72,
    "kind": "unmet-truth",
    "phase": "02",
    "file": "internal/imagefactory/guard_drift_test.go",
    "line": null,
    "description": "[from 02-31] AMENDS ENTRY 70, WHICH STAYS OPEN (no amend verb exists). DIE ZEILENSPALTE DIESES EINTRAGS IST ABSICHTLICH LEER, nicht vergessen: er zeigt auf den Symbolnamen refusedRangesDecl in internal/imagefactory/guard_drift_test.go statt auf eine Zeilennummer, und damit ist der stale Zeiger von Eintrag 70 auf Zeile 83 -- eine Zahl von vor Plan 02-27, die heute im Doc-Kommentar liegt -- miterledigt, ohne dass 70 bearbeitet wird (02-REVIEW.md IN-05); kuenftige Eintraege ueber Code zitieren Symbole statt Zeilen. WAS UEBERHOLT IST: Eintrag 70 traegt eine Schliessbedingung, die von Runde 6 der Verifikation woertlich ausgefuehrt und in beiden Richtungen erfuellt wurde (02-VERIFICATION.md:27, :265-266), und der Eintrag steht trotzdem weiter offen, weil sein tatsaechlicher Offen-Grund ein anderer ist als seine geschriebene Bedingung. DIESE DIVERGENZ IST ES, DIE DIESER EINTRAG UEBERHOLT, nicht die Messungen von 70. Der tatsaechliche Grund ist benannt und gemessen: der Anker ist ein regulaerer Ausdruck ueber TEXT und nicht ueber ein Programm, er bindet an ein LITERAL und nicht an den WERT, den ein Ausdruck um ihn herum erzeugt, und an einen BEZEICHNER und nicht an Code, den der Browser ausfuehrt. Plan 02-30 hat das ueber den Live-Lesepfad gemessen, je Form mit Bereichszahl und Fehlerzustand: eine Deklaration, die nur in einem Block-Kommentar ueberlebt, ranges=6 err=nil; nur in einem eingerueckten JSX-Kommentar, ranges=6 err=nil; nur in einem Template-Literal, ranges=6 err=nil; nur als JSX-Text in einem pre-Block, ranges=6 err=nil; ein Literal, das vor seiner Bindung gefiltert wird, also ].filter((range) => range.class !== 'byte order mark'), ranges=6 err=nil; eine Deklaration, die in die wirklich benutzte Tabelle gespreizt wird, ranges=6 err=nil; und eine eingerueckte Rest-Deklaration neben dem Import der echten Tabelle, ranges=2 err=nil, wo die kuerzere Zahl selbst der Schaden ist. WAS AN 70 WEITERHIN GILT: die Beschreibung des unverankerten Zustands zwischen Plan 02-20 und Runde 5 und des praefix-durchlaessigen Zustands bis Plan 02-27; die sechs Messwerte der Runde 5 (REFUSED_RANGES_LEGACY liefert ranges=[{0 0}] err=nil, REFUSED_RANGESX liefert ranges=[{0 1}] err=nil, und die vier unvalidierten Bounds fielen auf {-1 -1}, {1114111 -1} und {31 0}); die vier Korrekturen der Runde 6 mit ihren Commits da5c54c, 2ed7f6b, e373099, bdaec63, ecdd083 und 6652eab; und die Feststellung, dass nichts maskiert ist, weil die beiden Ablehnungsmengen uebereinstimmen und das Loeschen des U+FEFF-Eintrags den Waechter scheitern laesst. Diese Saetze bleiben richtig und werden von diesem Eintrag nicht ueberholt. WAS RUNDE 7 GETAN HAT, aus 02-29-SUMMARY.md und 02-30-SUMMARY.md und nicht aus dem Plan: der Spiegel-Defekt der Runde 6 ist ueber die Form des Rumpfes beseitigt statt gegen eine andere falsche Ursache getauscht -- strings.TrimSpace(body) entscheidet die Leer-Frage, whole beantwortet nur noch die Frage nach dem Literal ausserhalb der Deklaration, und der aus einer Dateizaehlung erschlossene Satz \"The declaration is NOT empty\" ist mit dieser Zaehlung verschwunden; Ueberlauf und Inversion sind zwei Zweige mit zwei eigenen Ursachen, und die Inversions-Meldung nennt utf8.MaxRune nicht mehr (WR-03); keine Tabellenzeile pinnt mehr eine Zeichenkette, die jeder Fehlerausgang traegt -- die drei wantErr-Zeilen tragen je eine unterscheidende Teilzeichenkette und wantErr ist in allen drei Tabellen []string (WR-04); der 49-zeilige Kommentarblock ist geteilt und beide Regexp-Deklarationen tragen ihren eigenen Doc-Kommentar (WR-05); der Waechter stellt die Zahl der Deklarationen fest und macht mehr als eine zu einem eigenen Fehlerausgang vor dem Lesen des Rumpfes; die Funktion hat jetzt zehn Fehlerausgaenge, und keiner erschliesst seine Ursache aus einer Zaehlung; die Abschneide-Meldung zitiert den gelesenen Rumpf mit %q und traegt darum keine Zahl mehr, womit 02-REVIEW.md IN-01 als Nebenwirkung entfaellt (grep -c '%d entries' liefert 1 statt 2); der allquantifizierte Satz \"Renamed, moved or deleted is the same as never having been there\" ist aus dem Doc-Kommentar und aus dem Fehlertext verschwunden (grep -c liefert 0) und durch eine Aussage ueber den Text plus eine als Daten gefuehrte Ausschlussliste ersetzt; guardBlindSpots fuehrt die Blindheit nach MECHANISMUS skopiert (text-not-code, literal-not-value, declaration-not-use, surrogate-interior) und honestClaim rendert die Behauptung aus dieser Liste, so dass zwei Listen verschiedener Laenge nicht entstehen koennen; der Lesepfad ist parametrisiert und wrapper-frei (browserRefusalRanges(t, path)), so dass die sieben Blindheitszeilen ueber denselben Pfad messen wie der Live-Waechter und ein kuenftiges stripNonCode am Aufrufer sie rot faerbt statt sie zu umgehen -- das ist als dritte Rot-Ausgabe belegt; und Punkt 6 der Schliessbedingung wurde einmal ausgefuehrt und protokolliert. WARUM DIESER EINTRAG OPEN BLEIBT UND WAS IHN SCHLIESST: er bleibt offen, weil die Verengung die BEHAUPTUNG auf null Ueberhang bringt und den DEFEKT nicht beseitigt. Die Leiche bleibt lesbar, der Waechter bleibt gruen darueber, und der Zeitpunkt, an dem das schadet, ist unveraendert. Ein bei der Anlage geschlossener Eintrag ueber eine solche Eigenschaft war genau der Fehler von 69, und er ist mechanisch und nicht bloss stilistisch: workflow.windows_enforce blockiert /gsd-ship, solange open_count groesser als 0 ist, und ein fixed-Eintrag ist an dieser Grenze unsichtbar. Die Schliessbedingung ist pruefbar und endlich und lautet, dass eine Verifikationsrunde gegen den dann geltenden Waechter alle sechs Punkte misst und protokolliert: (1) REFUSED_RANGES praefixiert umbenannt, etwa _LEGACY, ergibt ROT; (2) die Deklaration nur hinter // ergibt ROT; (3) ein Eintrag mit einer Grenze oberhalb utf8.MaxRune ergibt ROT; (4) eine wirklich leere Zuweisung = [] neben einer zweiten eintragsfoermigen Tabelle ergibt die LEER-Meldung und nicht \"Look for that bracket\"; (5) die geschriebene Ausschlussliste wird gegen jede darin genannte Form EINZELN gemessen und die gemessene Ausgabe im Protokoll zitiert, mindestens blosser Blockkommentar, eingerruecktes JSX-Kommentar, Template-Literal (je mit dem Import daneben, sonst ist die Fixture kein Schadensfall, sondern ein TS-Fehler) sowie die Ableitungsformen ].filter(...) und ...SPREAD, und jede Form muss die Ausgabe liefern, die die Liste behauptet; (6) die Runde versucht AUSDRUECKLICH, eine SIEBTE Form zu bauen, die die Liste nicht nennt (JSX-Text, String.raw, Regex-Literal, ${...}-Interpolation, #private-Name), und protokolliert das Ergebnis -- findet sie eine, die der Waechter liest und deren Mechanismus die Liste nicht nennt, BLEIBT DIESER EINTRAG OFFEN und die Liste wird um sie erweitert. Punkt 6 ist die eigentliche Huerde: EIN EINTRAG, DESSEN BEDINGUNG NUR DIE SCHON BEKANNTEN FORMEN ABFRAGT, WAERE EIN EINTRAG, DER SICH SELBST SCHLIESST. Plan 02-30 hat Punkt 6 einmal ausgefuehrt, mit dem Ergebnis \"kein Fund\" -- String.raw, Regex-Literal und ${...}-Interpolation werden gelesen (je ranges=6 err=nil) und sind Auspraegungen des bereits gelisteten Mechanismus text-not-code, Objekt-Property und Re-Export werden korrekt abgelehnt --, und dieser Nicht-Fund schliesst ausdruecklich nichts: die Schliessbedingung verlangt den Versuch von der Verifikationsrunde, die den Eintrag schliesst, nicht von der Runde, die ihn anlegt. DIE RATIFIKATION FEHLT: die Wahl zwischen B und A wurde vom Betreiber am 2026-09-05 ausdruecklich an den autonomen Lauf delegiert; der Lauf hat B gewaehlt. Der Betreiber hat B nicht selbst ratifiziert. Das Entscheidungsdokument 02-DECISION-drift-guard-lexik.md traegt in seinem Statusblock unveraendert \"offen -- vorgelegt, nicht ratifiziert\", wurde von einem autonomen Lauf am 2026-09-05 erarbeitet, und Variante A liegt PUNKTGLEICH daneben, 7,0 gegen 7,0; der Verifier hatte die Frage ausdruecklich dem Betreiber zugewiesen (02-VERIFICATION.md Human-Verification-Punkt 1, :430-440 und :522-523), und der Code-Review ordnet begruendet anders (02-REVIEW.md:160-162 stellt das Entfernen von Kommentaren und Zeichenketten vor die Verengung, die dort erst bei :182-186 und ausdruecklich als \"Falls das als zu teuer gilt\" erscheint). Was ein Wechsel auf Variante A kostet: dieser Eintrag beschriebe einen anderen Stand, die Abschnitte ueber die Ausschlussliste entfielen zugunsten einer Beschreibung dessen, was ein Lexer schliesst und was nicht, und die Arbeit von Plan 02-30 an guard_drift_test.go waere neu zu planen -- der unbedingte Teil (Spiegel-Defekt, WR-03, WR-04, WR-05, IN-05) bliebe unberuehrt. Ebenfalls festzuhalten, damit ein spaeterer Leser die Beweisdichte dieser Runde richtig einordnet: der Betreiber hat fuer den Rest dieser Runde ausdruecklich um schnelleres Vorgehen mit weniger Verifikation gebeten, und die Evidenz der Runde 7 ist duenner als die der Runden 5 bis 7 zuvor. OVERRIDES WIRD NIRGENDS BENUTZT UND NICHTS WIRD GEWAIVT: waived_count bleibt 0, beide Verifikationen tragen overrides_applied: 0, und das Ledger-Verb fuer bewusste Annahme heisst waive. Dieser Rest wird nicht angenommen, sondern offen gefuehrt. WAS AUCH NACH DIESER RUNDE BLEIBT, fuenf benannte Reste: (1) DIE ABLEITUNG -- der Anker bindet an ein Literal und nicht an einen Wert, gemessen an lebendem, kompilierendem, bei images.tsx:159 referenziertem Code, wo ].filter((range) => range.class !== 'byte order mark') sechs Bereiche lesen laesst und err=nil liefert, waehrend das Formular U+FEFF wieder annimmt und der Server es weiter mit 400 ablehnt; das ist GRUEN AUF ECHTER DRIFT und damit von derselben Schwere wie G-02-11 selbst, und es ist der gefaehrlichste bekannte Rest. (2) DIE EINE TRAGENDE VERBINDUNG -- images.tsx:159 ist die einzige lebende Referenz auf REFUSED_RANGES im ganzen Baum; faellt der Aufruf von hasControlCharacter aus dem Validierungspfad, ist die Tabelle eine wohlgeformte, lebende, uebereinstimmende Leiche, und jede Variante ist gruen darueber; G-02-17 war genau dieser Fall an einem einzelnen Feld, und gefunden hat ihn ein Mensch. (3) DER TRIMSPACE-REST -- strings.TrimSpace(body) haelt einen Rumpf aus // oder aus einem leeren Blockkommentar fuer nicht leer und nimmt den Abschneide-Zweig; kleiner als der Defekt, den er ersetzt, weil die Meldung ihren Beleg mit %q zeigt statt ihn zu behaupten, aber dieselbe Gattung, innerhalb der Aenderung, die diese Gattung beseitigen soll, und von einer Tabellenzeile festgenagelt. (4) DIE DREI HINWEISZEILEN UEBER images.tsx:105, die der Entwurf von Variante B vorsieht und die diese Runde BEWUSST NICHT GESCHRIEBEN hat, weil sie alles darunter um drei Zeilen verschoeben und rund ein Dutzend images.tsx:NNN-Verweise im Planungsbestand ungueltig machten; damit fehlt der Verengung ihr einziges Geraet, das den Menschen erreicht, der die Leiche erzeugt. (5) DIE 2046 NIE VERGLICHENEN CODEPOINTS im Inneren des Surrogat-Bereichs -- der Codepoint-Sweep ueberspringt U+D800..U+DFFF mit einem continue und behauptet die Surrogat-Menge nur an ihren beiden Endpunkten, weshalb fuer die 2046 Codepoints dazwischen nichts verglichen wird, und ihr Server-Zwilling rawBodyRefusal wird von hier nie aufgerufen; das Entscheidungsdokument nennt an dieser Stelle eine Zahl, die um die beiden einzeln behaupteten Endpunkte zu hoch ist, und der Nachtrag vom 2026-09-05 an jenem Dokument haelt die Korrektur auf 2046 fest. Kein Eintrag von guardBlindSpots kann diesen Rest als Zeile messen, weil Go keinen unpaarigen Surrogat in einem String halten kann; er ist dort als rowless mit Grund gefuehrt. DIE NACHBARSCHAFT: Eintrag 56 bleibt daneben OFFEN und traegt eine andere Aussage -- die SERVER-Menge hinter dem Codepoint-Sweep ist nicht erschoepfend gegen factory.talos.dev gemessen und erbt 02-14s Extrapolation --, und er wird von diesem Eintrag AUSDRUECKLICH NICHT ueberholt. Die Eintraege 58 und 66 tragen G-02-9 und bleiben unberuehrt OFFEN; G-02-9 IST OFFEN. Eintrag 69 bleibt FIXED und wird nicht angefasst. Eintrag 70 bleibt woertlich unveraendert und OFFEN. DIE GESCHWISTER-LESER SIND VON DIESEM EINTRAG NICHT GEDECKT -- stringArrayLiteral in derselben Datei, der Anker in internal/httpapi/handlers/budget_drift_test.go und die beiden strings.Contains-Sweeps in internal/imagefactory/warnings_test.go tragen dieselbe Gattung Loch und bekommen ihren eigenen offenen Eintrag in derselben Runde, Eintrag 74, in dem jeder der drei einzeln gemessen ist.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-05T09:27:11.222Z",
    "resolved_at": null
  },
  {
    "id": 73,
    "kind": "deviation",
    "phase": "02",
    "file": ".planning/REQUIREMENTS.md",
    "line": null,
    "description": "[from 02-31] Der Werkzeugdefekt, der einen Release-Blocker faelschlich als Complete stehen laesst: requirements revert-phase erreicht keine Traceability-Zeile, deren Id-Zelle Markup traegt. WAS GEMESSEN WURDE: eine Kopie von .planning/REQUIREMENTS.md wurde in ein Temp-Projektverzeichnis AUSSERHALB dieses Repositories gelegt, dort die undekorierte Phase-2-Zeile FACT-03 als Kontrolle kuenstlich auf Complete gesetzt und `gsd-tools requirements revert-phase TRANS-06 FACT-03 --project-dir <temp>` gefahren. Ergebnis woertlich: reverted enthaelt FACT-03, unchanged enthaelt TRANS-06, total 2. In der Kopie stand danach `FACT-03 -- Phase 2 -- Gaps Found` und unveraendert `**TRANS-06** (Emoji) -- Phase 2 -- Complete`. Beide Ids durchliefen denselben Aufruf, beide Checkboxen waren vorher schon leer, und der einzige Unterschied zwischen ihnen ist die Dekoration der Id-Zelle. Das echte Register wurde von diesem Messlauf nicht beruehrt. WARUM ES PASSIERT, an der Quelle gelesen statt aus der Beobachtung erschlossen: die Funktion cmdRequirementsRevertPhase in gsd-core/bin/lib/milestone.cjs hat zwei Wirkflaechen, und die beiden sind sich uneinig darueber, wie eine Id-Zelle aussehen darf. Die Checkbox-Haelfte traegt den Fett-Wrapper AUSDRUECKLICH in ihrem Muster: `const checkboxPattern = new RegExp(\"(-\\\\s*\\\\[)x(\\\\]\\\\s*\\\\*\\\\*\" + reqEscaped + \"\\\\*\\\\*)\", \"gi\");`. Die Traceability-Haelfte sucht die Zeile dagegen ueber ein Praedikat, das die ERSTE ZELLE der Zeile nach Trimmen und Kleinschreibung auf EXAKTE GLEICHHEIT mit der Id prueft: `const rowMatch = (row) => (Object.values(row)[0] ?? \"\").trim().toLowerCase() === reqId.toLowerCase();`. \"**trans-06** (Emoji)\" ist nicht \"trans-06\", also findet das Praedikat die Zeile nicht, tableHit bleibt false, die Id landet unter unchanged und es wird nichts geschrieben. Eine dekorierte Zeile ist damit fuer die eine Haelfte derselben Funktion sichtbar und fuer die andere nicht. WAS ES ANGERICHTET HAT: weder der Revert der Runde 5 (2e57ee3 docs(phase-02): revert premature Complete requirements after gaps found) noch der dieser Runde (b7a1ab9) hat die TRANS-06-Zeile erreicht, waehrend alle 13 undekorierten Phase-2-Ids zurueckgenommen wurden. Der einzige Release-Blocker der Phase las damit Complete, waehrend die Verifikation der Phase gaps_found sagt -- woertlich die Aussage, die der Betreiber mit 2e57ee3 von Hand zurueckgenommen und in derselben Commit-Nachricht als Regel ausgesprochen hat. Ein Werkzeugdefekt, der als falscher Projektzustand auftaucht, an genau der Stelle, an der niemand nachfragt. WIE WEIT ES REICHT: gemessen ueber das ganze Register, nicht ueber diese Phase. 16 Release-Blocker-Zeilen tragen eine dekorierte Id-Zelle und werden beim naechsten gaps_found ihrer Phase denselben Weg nehmen: TRANS-06 (Phase 2), INV-01, INV-03, INV-04, INV-05 und INV-07 (Phase 3), JOB-07 (Phase 6), CFG-02 (Phase 7), PROV-05, PROV-09 und PROV-10 (Phase 8), UPG-02, UPG-03, UPG-06 und UPG-07 (Phase 9) sowie OPS-05 (Phase 10). Das sind alle 16 Release-Blocker des Projekts; die Dekoration IST die Release-Blocker-Auszeichnung, also ist jeder einzelne betroffen. WAS RUNDE 7 GETAN HAT: die Statuszelle der TRANS-06-Zeile von Hand von Complete auf Gaps Found gesetzt, und sonst nichts an dieser Zeile -- die Id-Zelle behaelt ihre Dekoration, weil sie zu entfernen die bequeme Reparatur am falschen Ort waere und die Release-Blocker-Auszeichnung des Registers beschaedigte, um einen Werkzeugdefekt zu umgehen. Die Handkorrektur ist die ANWENDUNG der Regel, die der Betreiber mit 2e57ee3 selbst ausgesprochen hat, und keine neue Entscheidung. Danach lesen alle vierzehn Phase-2-Zeilen Gaps Found und keine liest Complete. WARUM DIESER EINTRAG OFFEN BLEIBT: der Defekt sitzt in der GSD-Laufzeit unter gsd-core/bin/lib/milestone.cjs, ausserhalb dieses Repositories, nicht versioniert und nicht im Umfang dieser Phase. Nichts in diesem Repository kann ihn schliessen, und der naechste mark-complete- oder revert-phase-Zyklus erzeugt ihn erneut -- fuer diese Zeile und fuer die fuenfzehn anderen. SEINE SCHLIESSBEDINGUNG IST EIN LAUF, IN DEM EINE DEKORIERTE ZEILE UNTER reverted ERSCHEINT: dieselbe Messung wie oben, eine dekorierte und eine undekorierte Id in einem Aufruf gegen eine Kopie, und beide erscheinen unter reverted statt eine unter unchanged. Solange das nicht gemessen ist, bleibt dieser Eintrag offen, auch wenn die eine Zeile im Repository von Hand richtig steht -- die Handkorrektur behebt den Zustand, nicht die Ursache. DIE GRENZE DIESER MESSUNG: geprueft wurde requirements revert-phase, nicht requirements mark-complete und nicht ready-ids. Ob dieselbe Uneinigkeit zwischen Checkbox- und Traceability-Haelfte auch dort besteht, hat diese Runde nicht festgestellt.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-05T09:34:19.051Z",
    "resolved_at": null
  },
  {
    "id": 74,
    "kind": "unmet-truth",
    "phase": "02",
    "file": "internal/imagefactory/guard_drift_test.go",
    "line": null,
    "description": "[from 02-31] DIE GESCHWISTER-LESER DIESES BESTANDS TRAGEN DIESELBE GATTUNG LOCH WIE DER BROWSER-ABLEHNUNGS-WAECHTER, JEDER IN EIGENEM UMFANG -- HIER EINZELN GEMESSEN, NICHT AUS EINEM PLANUNGSDOKUMENT UEBERNOMMEN. Die Zeilenspalte ist absichtlich leer: der Eintrag zeigt auf den Symbolnamen stringArrayLiteral in internal/imagefactory/guard_drift_test.go, und die beiden anderen Fundstellen sind im Text mit Datei und Symbol benannt. WELCHE DREI LESER: (1) stringArrayLiteral in internal/imagefactory/guard_drift_test.go, Muster NAME[^=]*=\\s*\\[([^\\]]*)\\] ohne Zeilenanfangs-Anker und ohne Wortgrenze hinter dem Namen; (2) der Anker in TestBudgetWaitsStatedInTheUIMatchTheRouteBudgets in internal/httpapi/handlers/budget_drift_test.go, Muster (?m)^\\s*(?:export\\s+)?const\\s+NAME\\s*=\\s*([0-9]+)\\b; (3) die beiden strings.Contains-Sweeps in TestWarningDetailsMatchTheUI in internal/imagefactory/warnings_test.go, die den Detail-Satz gegen web/src/components/SchematicWarnings.tsx und den Code gegen web/src/api.ts pruefen, samt der Schleife ueber exportedWarningCodes, die denselben Contains-Sweep je Code fahrt. WAS EINZELN GEMESSEN WURDE, mit einer wegwerfbaren Sonde ausserhalb dieses Repositories, die die drei Muster woertlich kopiert und gegen synthetische Quellen faehrt; die Sonde ist entfernt. LESER 1 stringArrayLiteral: Kontrolle auf der echten Deklaration GELESEN mit members=2; Praefix-Loch (nur INSTALLER_REPOSITORY_NAMES_LITERAL_LEGACY vorhanden) GELESEN mit members=2, also JA; Blindheit in einem Zeilenkommentar GELESEN mit members=2 und in einem Template-Literal GELESEN mit members=2, also JA. Er traegt beide Loecher unveraendert. LESER 2 der Budget-Anker: Kontrolle GELESEN mit wert=45; Praefix-Loch (nur SCHEMATIC_WAIT_SECONDS_LEGACY vorhanden) NICHT GELESEN, also NEIN; Blindheit in einem Zeilenkommentar NICHT GELESEN und in einem eingerueckten JSX-Kommentar NICHT GELESEN, aber in einem Blockkommentar GELESEN mit wert=45 und in einem Template-Literal GELESEN mit wert=45, also JA und zwar begrenzt. LESER 3 die beiden Contains-Sweeps: Kontrolle GELESEN; Praefix-Loch (nur installer.repo-fallback-unverified vorhanden, gesucht installer.repo-fallback) GELESEN, also JA; Blindheit in einem Zeilenkommentar, in einem Blockkommentar, in einem Template-Literal und als JSX-Text jeweils GELESEN, also JA in jeder gepruefen Form. Er hat weder Anker noch Wortgrenze und ist der durchlaessigste der drei. WORIN DIE MESSUNG DEM ENTSCHEIDUNGSDOKUMENT WIDERSPRICHT, und das gehoert genannt, weil eine uebernommene Vermutung in einem Ledger genau die Sorte Behauptung waere, die dieser Ledger fuehrt: 02-DECISION-drift-guard-lexik.md schreibt im Abschnitt \"Was keine der vier Varianten schliesst\", Unterpunkt \"Die Geschwister\", dass stringArrayLiteral Runde 5s Praefix-Loch unveraendert weitertraegt und dass budget_drift_test.go:89-90 sowie warnings_test.go:281-336 \"ebenso\" betroffen sind. FUER DEN BUDGET-ANKER STIMMT DAS NICHT: er verlangt hinter dem Bezeichner unmittelbar Leerraum und das Gleichheitszeichen, weshalb der Unterstrich in SCHEMATIC_WAIT_SECONDS_LEGACY das Muster bricht -- er hat das Praefix-Loch NICHT, und er hatte es nie. Zweitens ist auch die Beschreibung seiner lexikalischen Blindheit zu weit: sein Zeilenanfangs-Anker laesst zwar Einrueckung zu, aber weil auf den Leerraum unmittelbar `export` oder `const` folgen muss, faellt eine mit // oder mit {/* beginnende Zeile durch. Er ist blind gegen einen Blockkommentar und gegen ein Template-Literal, weil die Deklarationszeile IM Inneren dieser Formen wieder mit const beginnt -- nicht gegen Kommentarzeichen im Allgemeinen. Die Vermutung des Dokuments trifft also fuer Leser 1 und Leser 3 zu und fuer Leser 2 nur zur Haelfte und aus einem anderen Grund als dort angenommen. WARUM SIE EINEN EIGENEN EINTRAG BEKOMMEN: Ledger-Eintrag 72 deckt sie AUSDRUECKLICH NICHT -- er handelt von refusedRangesDecl und von der Behauptung, die der Browser-Ablehnungs-Waechter ueber REFUSED_RANGES fuehrt. Keine der vier Varianten des Entscheidungsdokuments fasst die Geschwister an. Sie stillschweigend stehen zu lassen waere dieselbe Operation, die Eintrag 69 falsch gemacht hat: eine Eigenschaft, die weiterlebt, an einem Eintrag vorbei, der sie nicht fuehrt. WAS IHN SCHLIESST: jeder der drei traegt entweder die Verankerungs-Disziplin aus Plan 02-27 -- hinter regexp.QuoteMeta(name) nur Leerraum, eine mit Doppelpunkt beginnende Typannotation und das Gleichheitszeichen, so dass das verlangte Zeichen die Wortgrenze ist, die der Unterstrich nicht ist -- oder einen GESCHRIEBENEN, GEMESSENEN Ausschluss in der Form, die Plan 02-30 fuer guardBlindSpots etabliert hat, also je Eintrag Mechanismus, gemessene Ausgabe und eine Zeile, die ihn misst. Eine Runde, die das fuer alle drei tut und je die beiden Messungen dieses Eintrags protokolliert -- Praefix-Loch und lexikalische Blindheit, mit der gemessenen Ausgabe und nicht mit einem Urteil --, schliesst diesen Eintrag. Fuer Leser 2 heisst das ausdruecklich: der Praefix-Teil ist bereits erfuellt und gemessen, offen ist allein der lexikalische Teil. DIE GRENZE DIESER MESSUNG: DREI LESER WURDEN GEPRUEFT UND KEINE WEITEREN. Ob der Baum weitere Transkriptions-Waechter mit derselben Gattung Loch traegt, hat diese Runde NICHT festgestellt; es wurde keine Suche ueber den ganzen Baum nach solchen Mustern gefahren. Gemessen wurden je Leser genau zwei Fragen (Praefix-Loch und lexikalische Blindheit) an genau den oben genannten synthetischen Formen; andere Formen wurden nicht versucht.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-05T09:36:55.361Z",
    "resolved_at": null
  },
  {
    "id": 75,
    "kind": "unmet-truth",
    "phase": "02",
    "file": ".planning/ROADMAP.md",
    "line": null,
    "description": "[aus Runde 8, Human-Decision-Punkt 2] DIE PLANZAHL IN ROADMAP.md ZEILE 81 IST DREIMAL HINTEREINANDER GEDRIFTET, WEIL WERKZEUGAUSGABE UND HANDSCHRIFT IN EINEM SATZ STEHEN. Der Zustand ist heute richtig, gemessen 31 von 31 an beiden Stellen; der Befund ist die Wiederholung und nicht der Stand. Runde 6 hinterliess 28 von 28 neben 26 von 28, der Fix 77ed0f3 erfasste nur die vordere Haelfte, Runde 7 hinterliess 30 von 31 neben 28 von 31, und Plan 02-31 korrigierte auf 31 von 31 ohne vierten Eintrag, weil seine Erfolgskriterien die Endsignatur auf 74 nagelten, und uebergab den Fall ausdruecklich. Nach dem Massstab von Eintrag 74, sie stillschweigend stehen zu lassen waere dieselbe Operation, die Eintrag 69 falsch gemacht hat, gehoert sie geledgert. Schliessbedingung: entweder die von Hand gefuehrte Zahl ersatzlos streichen, so dass nur die vom Werkzeug gefuehrte bleibt, oder eine Pruefung, die beide Zahlen im selben Satz gegeneinander haelt.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-05T10:14:39.906Z",
    "resolved_at": null
  },
  {
    "id": 76,
    "kind": "unrun-verify",
    "phase": "03",
    "file": "internal/talos/contract_real_test.go",
    "line": null,
    "description": "TRANS-08 / Erfolgskriterium 5 ist NICHT erfuellt. Die Contract-Suite laeuft gruen gegen talossim (Tier 0); die reale Haelfte existiert als realTransport in internal/talos/contract_real_test.go und der Tier-1-Provisioner als sandbox/cmd/talos-sandbox, aber sie wurde in dieser Sitzung NIE AUSGEFUEHRT: der Docker-Daemon laeuft in dieser Umgebung nicht (docker info scheitert), und ohne ihn kann pkg/provision keinen Talos-Container starten. Der Test ueberspringt sich sichtbar mit der Anleitung statt still durchzulaufen (D-27). ZWEITE, DAVON UNABHAENGIGE EINSCHRAENKUNG: selbst ein gruener Tier-1-Lauf deckt nur zwei der neun Szenarien ab -- second_bootstrap_returns_AlreadyExists und reject_apply, die beiden, deren Fehler das dokumentierte Eigenverhalten des Knotens ist. Die uebrigen sieben verlangen, dass jemand eine Verbindung kappt, eine Ressource entfernt oder eine Adresse aendert, und werden namentlich uebersprungen. Fake-Drift bleibt damit fuer sieben von neun Szenarien ungemessen. SCHLIESSBEDINGUNG: ein Lauf auf einem Host mit laufendem Docker, protokolliert, plus eine Entscheidung, ob und wie die sieben nicht induzierbaren Szenarien auf Tier 2 (echte Hardware, Phase 10) geprueft werden.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-11T16:30:00.000Z",
    "resolved_at": null
  },
  {
    "id": 77,
    "kind": "unrun-verify",
    "phase": "03",
    "file": "Taskfile.yml",
    "line": null,
    "description": "'task lint:go' (golangci-lint run) konnte in dieser Sitzung nicht ausgefuehrt werden: das installierte golangci-lint ist mit go1.25 gebaut und weigert sich gegen ein go1.26.7-Ziel zu laufen ('the Go language version used to build golangci-lint is lower than the targeted Go version'). gofmt -l und go vet sind ueber cmd/ und internal/ sauber, und die Go- sowie Web-Testsuiten sind gruen. Derselbe Riss wie Eintraege 6 und 19, aber aus einem anderen Grund: damals fehlte das Binary, hier ist es zu alt. SCHLIESSBEDINGUNG: ein Lauf mit einem golangci-lint, das gegen dieselbe Go-Version gebaut ist, die .golangci.yml als Ziel fuehrt.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-11T16:30:00.000Z",
    "resolved_at": null
  },
  {
    "id": 78,
    "kind": "todo",
    "phase": "03",
    "file": "internal/inventory/observe.go",
    "line": null,
    "description": "Refresh liest den Machine-Record, baut den Snapshot und schreibt zurueck -- ohne die Rev-Kollision zu behandeln. Zwei Ereignisse koennen sie ausloesen: der Supervisor-Heartbeat und ein manuelles POST /machines/{id}/refresh treffen zusammen, oder ein Refresh laeuft waehrend recordMachine denselben Record anfasst. Der Verlierer bekommt store.ErrConflict, der Fehlschlag wird geloggt und der Snapshot dieser Runde ist verloren; der naechste Heartbeat holt ihn nach. Kein Datenverlust und keine falsche Anzeige, aber ein Fehlerpfad, der nur im Log existiert und den nichts misst. SCHLIESSBEDINGUNG: entweder ein Retry mit frisch gelesener Rev, oder eine ausdrueckliche Entscheidung samt Test, dass ein verlorener Snapshot der gewollte Ausgang ist.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-11T16:30:00.000Z",
    "resolved_at": null
  },
  {
    "id": 79,
    "kind": "deviation",
    "phase": "03",
    "file": "internal/inventory/observe.go",
    "line": null,
    "description": "Die Supervisors sind Heartbeat-Poller ueber COSI-Reads, keine COSI-Watches. INV-13 und D-19 verlangen 'Watch primaer, Poll als Heartbeat'; gebaut ist der Heartbeat mit Jitter (45s +/-20%) ueber genau die Ressourcen, die ein Watch beobachten wuerde. Die Leserichtung stimmt -- Ressourcenzustand statt unaerer RPCs, was das eigentliche Verbot von INV-13 ist -- und die Antwortform (health.Field[T]) ist dieselbe, die ein Watch fuellen wuerde, also ist der Tausch clientseitig unsichtbar. Was fehlt, ist die Latenz: eine Aenderung wird im Mittel nach ~22s sichtbar statt sofort. SCHLIESSBEDINGUNG: entweder Watches in Phase 5 (wo die SSE-Route ohnehin einen Aenderungsstrom braucht), oder eine ausdrueckliche Entscheidung, dass der Heartbeat fuer 5-20 Nodes die richtige Aufloesung ist.",
    "status": "open",
    "reason": "",
    "recorded_at": "2026-09-11T16:30:00.000Z",
    "resolved_at": null
  }
]
````
