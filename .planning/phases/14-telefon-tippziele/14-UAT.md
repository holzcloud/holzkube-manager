---
status: testing
phase: 14-telefon-tippziele
source: [14-VERIFICATION.md]
started: 2026-09-30T13:05:00Z
updated: 2026-09-30T13:05:00Z
---

## Current Test

number: 1
name: D-11 hand test on a phone (criterion 4, phone half)
expected: |
  On a phone, against a development daemon over TLS on the LAN (never production) with the helper installed:
  on /host tap Restart host, type the host name, confirm turns on, tap Keep running, never submit.
  Nothing needs zooming anywhere, and focus returns to Restart host.
awaiting: user response

## Tests

### 1. D-11 hand test on a phone (criterion 4, phone half)
expected: On /host at phone width, Restart host -> type the host name -> confirm turns on -> Keep running. No zoom needed anywhere; focus returns to Restart host. Never submitted.
result: [pending]

### 2. CR-01 dialog lock on a phone
expected: While an order is in flight (or the sudo prompt is on top), Keep running, a tap outside and the X leave the host action dialog open; no order is placed after a cancel.
result: [pending]

### 3. Repeat after 13-12
expected: D-08 dumps before and after 13-12's changes show no ONLY AFTER / MOVED lines outside the new fifth action; the D-11 hand test holds with five actions.
result: [pending]

## Summary

total: 3
passed: 0
issues: 0
pending: 3
skipped: 0
blocked: 0

## Gaps
