# sandbox — a separate Go module, deliberately

This directory is its own Go module (`github.com/holzcloud/holzkube-manager/sandbox`) and
is **not** part of the product build. Nothing under `sandbox/` is compiled into
`holzkube-managerd`, and `go list ./...` in the repository root does not report it: Go's
package patterns stop at a nested `go.mod`.

## Why

The Docker and QEMU provisioners used to bring up a throwaway Talos cluster for
development live in the **Talos root module** (`github.com/siderolabs/talos`),
not in the lightweight `pkg/machinery` the product talks to. The root module
pulls in a substantial part of an operating system: containerd, CNI plugins,
image handling, and their transitive dependencies.

If that arrives in the product module, three things happen at once:

1. `holzkube-managerd` grows from roughly 10 MB to something on the order of 200 MB.
2. The supply-chain surface of a tool that holds cluster PKI expands to every
   dependency of a container runtime, none of which the product uses.
3. `goreleaser` cross-compilation gets slower and more fragile, because the
   requirement of a pure-Go dependency tree becomes much harder to keep.

Unwinding this later is painful: by then imports have spread and the split has to
be made while everything is moving. The module boundary is therefore drawn in
phase 1, while it costs nothing and there is not yet a single Talos import to
move.

## The rule

`cmd/holzkube-managerd` depends on `pkg/machinery` **only**. Anything that needs the
Talos root module goes here.

`internal/depguard_test.go` in the root module enforces it: it walks the full
dependency graph of `./cmd/holzkube-managerd` and fails if any package of the Talos root
module appears. It is running now, before phase 2 adds the first Talos import, so
the guard is in place at the moment it starts to matter rather than after.

## Status

Empty. Phase 1 has no Talos interaction at all — the module exists so that the
first provisioner has an obvious place to land, and so that the guard has a
boundary to point at.
