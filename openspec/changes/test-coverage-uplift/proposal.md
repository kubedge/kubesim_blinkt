# Uplift test coverage

> **Superseded** by `port-to-rust` (2026-10-08): the Go code this change targeted was replaced by the Rust crate. Rust CI and test coverage would need a fresh proposal. Archive or delete this change.

## Why

This simulator has little/no test coverage — a legacy of the "compile + start" bar. Since
it runs as a workload (often deployed by an operator), its core logic should be tested.

## What Changes

- Add unit tests for the simulator's core logic (message handling / behavior in its
  `cmd/*` and internal packages).
- Run `go test ./... -race` and wire it into CI.

## Capabilities

### New Capabilities
- test-coverage: this simulator's core logic is protected by tests.
