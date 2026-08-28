## What this changes

<!-- One or two sentences. The diff shows how; say what and why. -->

## What this serves

<!-- The issue this fixes, or the behaviour a user gains. If the change belongs
     to an area the sidebar still marks "Coming in phase N", say so. -->

## Gates

Run against this change, not against an earlier build:

- [ ] `task build`
- [ ] `golangci-lint run` — no issues
- [ ] `go test ./...` (`task test` runs the same suite with `-race`)
- [ ] `npm --prefix web test`
- [ ] `npm --prefix web run lint`

All five run for a change on either side. The binary embeds the UI bundle, so a
frontend change that fails to build is a Go build failure, and a Go change can
break the API contract the frontend tests assert against.

- [ ] Documentation updated, or nothing user-visible changed
- [ ] No credentials, kubeconfigs, talosconfigs or private keys in the diff,
      including in test fixtures
