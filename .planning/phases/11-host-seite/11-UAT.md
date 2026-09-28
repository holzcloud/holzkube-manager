---
status: partial
phase: 11-host-seite
source: [11-VERIFICATION.md]
started: 2026-09-28T19:00:00Z
updated: 2026-09-28T19:00:00Z
---

## Current Test

number: 1

## Tests

### 1. /host on the production service after the next release
expected: ProcSubset notice shown; one merged filesystem meter; cpu_thermal and rp1_adc; "No fan reported"; eth0 and wlan0; update check "Not recorded" until the new script's first run, then a recorded outcome.
result: pending
reason: needs a release (operator's call) and a look in the browser

### 2. Local gate flake in internal/upgrade
expected: `./bin/task ci` green on the Pi under normal load
result: pending
reason: TestANodeSaysHowItBooted times out under load; passes alone

## Summary

total: 2
passed: 0
issues: 0
pending: 2
skipped: 0

## Gaps
