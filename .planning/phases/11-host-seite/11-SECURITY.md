---
phase: 11
slug: host-seite
status: verified
threats_total: 23
threats_closed: 23
threats_open: 0
asvs_level: 1
block_on: high
audited: 2026-09-28
---

# Phase 11 — Security

Retroactive audit of the threat registers in 11-01..11-07-PLAN.md against the code at
2e8d719, after the review fixes (11-REVIEW-FIX.md). Ran on the operator's Pi (aarch64),
Go 1.27.1; the production service and its data directory were not touched.

## Trust Boundaries

| Boundary | Description |
|---|---|
| browser → GET /api/v1/host | an authenticated reader reads the host's namespace |
| daemon → /proc, /sys, /etc, syscalls | kernel interfaces read by the unprivileged service |
| GitHub release metadata → root script | tag and asset names reach a root process and the status file |
| root script → /var/lib/holzkube-manager-update | root writes a file the daemon reads |
| status file → daemon → page | a file on disk becomes a statement on the page |
| verification session → production host / repository | measuring beside the live service; measured values must not reach the public tree |

## Threat Register

| ID | Category | Component | Severity | Disposition | Status | Evidence |
|---|---|---|---|---|---|---|
| T-11-01 | EoP | handlers/host.go | medium | mitigate | CLOSED | host.go:37-38 RequiresSession + MinRole reader, no WallLink; router.go:423-455; TestHostAPI/no_session → 401 |
| T-11-02 | Info Disc. | /api/v1/host device section | low | accept | CLOSED | Accepted Risks Log |
| T-11-03 | Info Disc. | identity.go detectContainer | medium | mitigate | CLOSED | identity.go:185-201 returns bool only; TestContainer checks the view for environ contents |
| T-11-04 | DoS | collector.go readBounded | medium | mitigate | CLOSED | collector.go:26-27, 413-429; every file read in internal/host goes through it |
| T-11-05 | Repudiation | /api/v1/host not audited | low | accept | CLOSED | Accepted Risks Log |
| T-11-06 | EoP | record_status (root write) | high | mitigate | CLOSED | update script :73, :139, :143, :152-155 (lstat uid 0, no g/o write), :159, :178; TestUpdateScriptAsRoot symlink/mode/foreign-uid pass |
| T-11-07 | Tampering | status JSON from tag names | medium | mitigate | CLOSED | script :163-172 argv → json.dump; status.go:77, :244 |
| T-11-08 | DoS | EXIT trap vs. outcome | high | mitigate | CLOSED | one EXIT trap :223; on_exit :188-200 set +e, `|| true`, no exit; rc-unchanged tests pass |
| T-11-09 | DoS | updatestatus.Read | medium | mitigate | CLOSED | status.go:121, :124, :144, :155-193 |
| T-11-10 | EoP | HOLZKUBE_MANAGER_* overrides | low | accept | CLOSED | Accepted Risks Log |
| T-11-11 | Tampering | live CPU/memory integrity | medium | mitigate | CLOSED | host.go:74-88; live_test.go:79; api.ts:2941-2946, CPU/memory values without .default |
| T-11-12 | DoS | parseMountinfo | low | mitigate | CLOSED | collector.go:215; mountinfo.go:42; linear loop |
| T-11-13 | Info Disc. | read-failed reasons | low | accept | CLOSED | Accepted Risks Log |
| T-11-14 | DoS | dirSizer walk | medium | mitigate | CLOSED | filesystems.go:209-212 (60 s TTL, 5 s deadline), :251-257 singleflight; TestDataDirSizeIsCached |
| T-11-15 | Info Disc. | data dir / mount / versions | low | accept | CLOSED | Accepted Risks Log |
| T-11-16 | Tampering | --update-status-file | low | mitigate | CLOSED | config.go:283-285 IsAbs; status.go 4 KiB cap and per-field rules |
| T-11-17 | DoS | readSensors walk | low | mitigate | CLOSED | sensors.go:26-30, :65, :92, :220; readTrimmed → readBounded |
| T-11-18 | Info Disc. | sensor names/labels | low | accept | CLOSED | Accepted Risks Log |
| T-11-19 | Info Disc. | interface names/counters | low | accept | CLOSED | Accepted Risks Log |
| T-11-20 | DoS | readLinks | low | mitigate | CLOSED | network.go:24, :89-91; each file through readBounded |
| T-11-21 | Info Disc. | docs after measuring the host | medium | mitigate | CLOSED | publicrepo passes at HEAD; the phase-11 files carry no identifier of this host; compare/harden scripts untracked |
| T-11-22 | DoS | production service | high | mitigate | CLOSED | 11-07-SUMMARY before/after ActiveEnterTimestamp; unchanged since, NRestarts=0; ports 18443/18444 free |
| T-11-SC | Tampering | dependencies | low | accept | CLOSED | Accepted Risks Log |
| T-11-23 | EoP | root python3 import from caller's cwd (review CR-01, registered here) | high | mitigate | CLOSED | script :43 `cd /`; python3 -I at :153, :163, :301, :325; PYTHONPATH/cwd test passes |

## Accepted Risks Log

| ID | Risk | Rationale | Premise checked |
|---|---|---|---|
| T-11-02 | Readers see hostname, board model, kernel | Readers already see version and cluster topology; no secrets, environment or mount list | Device section is hostname, model, arch, cores, os, kernel, uptime only |
| T-11-05 | GET /api/v1/host is not audited | A read that changes nothing, polled every 3 s; auditing it would flood a permanent archive | Route has no Action; TestHostAPI asserts nothing audited |
| T-11-10 | HOLZKUBE_MANAGER_* override root script paths | Only someone controlling root's environment can set them; the unit sets none; sudo env_reset strips them | Update unit has ExecStart only, no Environment |
| T-11-13 | Reason messages show paths and error text | No environment or secret file contents | Parse errors can quote pieces of /proc/stat, /proc/meminfo, /proc/loadavg and the CPU online list; these are non-sensitive kernel numbers. Mountinfo parse errors are not surfaced |
| T-11-15 | Readers see data dir path, mount device, versions | Readers already see version and topology | At most two filesystem rows; no mount table |
| T-11-18 | Readers see sensor chip names and labels | Driver strings only | No serial, model or voltage file is read |
| T-11-19 | Readers see interface names and byte counters | No MAC | `address` never opened (network_test.go:158-163); no address file in testdata |
| T-11-SC | Supply chain | No package installed | Only change: x/sys indirect → direct in go.mod; go.sum and package-lock.json unchanged |

## Residual Observations (non-blocking)

1. The foreign-uid test uses /tmp (1777), which the mode check refuses as well; the `st_uid == 0` check has no guard of its own.
2. status.go:164 wraps encoding/json's syntax error, which quotes one byte of the file.
3. api.ts:1247 `fanSchema.rpm` has `.default(0)` inside the sensors reading, against the comment at api.ts:2937. It cannot invent a value because Go always emits `rpm` and skips unparsable fans.
4. `install -d` would chown a symlink's target before the lstat check refuses it, if STATUS_DIR's parent were writable by others. The default parent /var/lib is root-only.
5. Summaries 11-01..11-06 have no `## Threat Flags` section.

## Security Audit Trail

| Date | Auditor | Result | Notes |
|---|---|---|---|
| 2026-09-28 | gsd-security-auditor | SECURED, 23/23 closed, threats_open 0 | ASVS L1, block_on high; after 11-REVIEW-FIX; Pi aarch64 |
