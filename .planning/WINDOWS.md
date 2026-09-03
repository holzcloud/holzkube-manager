---
schema_version: 1
open_count: 47
waived_count: 0
fixed_count: 9
total_count: 56
last_updated: 2026-08-30T09:59:56.712Z
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
| 8 | 02 | todo | internal/imagefactory/client.go | 103 | [WR-01] WithHTTPClient silently discards the configured timeout. Code review 02-REVIEW.md, scoped out of the phase-2 fix pass. | open |  | 2026-08-29T12:40:00.000Z |  |
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
| 20 | 02 | deviation | internal/imagefactory/installer.go |  | A cold resolveInstallerRepo still walks two candidates serially at DefaultTimeout=30s each, so GET /schematics/{id}/assets keeps a 2x30=60.000s worst case against writeTimeout=60s -- the composition G-02-2 measured as status=502 duration=1m0.002907792s, unchanged by plan 02-12. The bounded re-question that plan adds asks only the never-ruled-out candidate, costs at most 1x30s and cannot reach that ceiling. Bounding the cold path needs the per-route deadline 02-DECISION-probe-budget.md owns; G-02-2 is deferred to cluster A, and cmd/holzkubed/budget_test.go declares the route as known-over-budget. | open |  | 2026-08-29T17:27:33.353Z |  |
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
| 48 | 02 | deviation | internal/imagefactory/installer.go |  | [from 02-19] READ WITH ENTRY 20, WHICH STAYS OPEN. Plan 02-19 changed what an unresolved installer RETURNS (502 with no body became 200 with installer:null plus installer_error), not the cold 2 x 30s serial candidate walk that produces it. The 49.8s and 60.002s observations recorded in G-02-15 are that composition and are unchanged. 'The panel now shows four references on a timeout' must not be read as the timeout having been addressed; the ceiling against writeTimeout=60s is still owned by 02-DECISION-probe-budget.md, and the milder symptom makes deferring that decision easier rather than more defensible. | open |  | 2026-08-30T09:59:17.754Z |  |
| 49 | 02 | deviation | docs/api-contract.md |  | [from 02-19] The assets route's response shape changed for two outcomes: 502 with no body became 200 with installer:null plus installer_error, so installer is now nullable on every answer. No external clients exist, which is why this is a note and not a migration. Matches T-02-91/T-02-92. | open |  | 2026-08-30T09:59:18.030Z |  |
| 50 | 02 | todo | docs/api-contract.md | 646 | [from 02-19] Entries 25 and 26 (R2-IN-02, R2-IN-03) sit inside the assets section plan 02-19 rewrote and were deliberately left alone: both are marked 'scoped out by the user', and a scoping decision is not an executor's to overturn. Entry 25's SecureBoot example still omits the warnings field the same section calls mandatory, and it now sits beside a table enumerating what warnings means in every outcome, so the inconsistency is more visible than before. Worth re-offering to the user rather than closing silently. | open |  | 2026-08-30T09:59:18.285Z |  |
| 51 | 02 | todo | web/src/api.ts | 344 | [from 02-19] AMENDS ENTRY 27, WHICH STAYS OPEN (no amend verb exists). Entry 27 records WARNING_INSTALLER_REPO_FALLBACK_UNVERIFIED as exported, pinned by the Go drift guard and unused by application code. Still true, and now true of a second constant: WARNING_INSTALLER_SECUREBOOT_REPO_FALLBACK_UNVERIFIED is also unused, because SchematicWarnings renders code/detail generically with no per-code branch. That is the intended design -- an unknown code still reaches the operator -- so the entry is about the constants' purpose being drift-pinning rather than rendering. Re-word rather than close. | open |  | 2026-08-30T09:59:18.604Z |  |
| 52 | 02 | todo | internal/httpapi/handlers/schematics.go | 66 | [from 02-20] AMENDS ENTRY 13, WHICH STAYS OPEN (no amend verb exists). Entry 13 reads 'schematicInput.Cluster is accepted, unvalidated and never sent'. The UNVALIDATED half is closed: cluster now goes through NotRepresentableReason in validate, and a refused codepoint in it is a 400 naming cluster. The NEVER SENT half is unchanged and still true -- cluster is stored on the record, reaches no other layer, and the authoring form offers no input for it. Amended text: 'schematicInput.Cluster is validated and stored but never sent anywhere and has no form input.' | open |  | 2026-08-30T09:59:18.930Z |  |
| 53 | 02 | deviation | internal/httpapi/handlers/schematics.go |  | [from 02-20] Records stored before plan 02-20 may carry refused codepoints in name or cluster. Nothing migrates them and there is no backfill to write, because the values are the operator's own text and repairing them would be the silent rewrite that plan exists to prevent. Such a record is readable but not re-creatable, and there is no edit route -- POST, GET, GET /{id}, GET /{id}/assets and DELETE /{id} are the whole surface -- so an operator holding one must delete and re-author it. The saved table renders it safely in the meantime via StoredText, which is why this is a residual and not a defect. | open |  | 2026-08-30T09:59:19.158Z |  |
| 54 | 02 | todo | web/src/routes/images.tsx | 329 | [from 02-20] The client cannot guard cluster because the authoring form has no cluster input; the server does guard it. When a cluster input is added it belongs in the single hasUnusableValue computation in images.tsx, which already carries a comment saying so. | open |  | 2026-08-30T09:59:19.461Z |  |
| 55 | 02 | todo | internal/httpapi/handlers/schematics.go |  | [from 02-20] createProblem's NotRepresentableError branch is now unreachable from the HTTP route in practice: refuseUnrepresentable covers every document path the request vocabulary names, so the branch fires only for a future field that forgets a check, or for an in-process caller. It is kept deliberately -- deleting it would turn the first of those into a 502 blaming the Factory, which is G-02-6 -- but it is a branch no route-level test can reach, the same shape as the utf8.ValidString note that produced entry 29. | open |  | 2026-08-30T09:59:19.676Z |  |
| 56 | 02 | unrun-verify | internal/imagefactory/guard_drift_test.go | 65 | [from 02-20] The codepoint sweep in guard_drift_test.go is exhaustive over Unicode but the SERVER set behind it is not exhaustively measured; it inherits 02-14's extrapolation (U+FDD1-U+FDEF, twenty-six C1 codepoints, and the interior of the range above U+FFFF are refused on the strength of measured ends). The guard proves the two layers agree; it cannot prove the set is right. Extends the 02-14 floor entry rather than starting a new claim. | open |  | 2026-08-30T09:59:19.914Z |  |

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
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-29T12:40:00.000Z",
    "resolved_at": null
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
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-29T17:27:33.353Z",
    "resolved_at": null
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
    "status": "open",
    "reason": "",
    "recorded_at": "2026-08-30T09:59:17.754Z",
    "resolved_at": null
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
  }
]
````
