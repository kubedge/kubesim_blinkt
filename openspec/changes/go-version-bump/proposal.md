# Bump the Go toolchain

> **Superseded** by `port-to-rust` (2026-10-08): the Go code this change targeted was replaced by the Rust crate. Rust CI and test coverage would need a fresh proposal. Archive or delete this change.

## Why

The sim base modules are on `go 1.20` and the consumers span `go 1.15`–`1.20` — well
behind current. The sim family froze under the old "compile + start" bar. Bringing the go
directive up (to a current, supported line) is the base half of modernizing the sims and
unblocks newer deps/tooling.

## What Changes

- Raise the `go` directive in each sim-base module (`config/`, `connected/`, `grpc/go/`,
  and root/health/arpscan) to a current line, `go mod tidy`, and fix any breakage.
- Keep `go build/vet/test ./...` green per module.

## Capabilities

### New Capabilities
- go-toolchain: the Go language/toolchain version the sim base targets.
