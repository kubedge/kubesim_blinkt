# Tasks — test-coverage-uplift

Done by the 2026-10-08 work; the repo had no test files before (commit 4aca31f).

- [x] `go test ./... -cover`; note 0% packages. Evidence: only the `main` entry points are at 0% (`cmd`, `dra-driver/cmd/blinkt-dra-driver`); they are exercised by the home-pi hardware runs and the kind run.
- [x] Add unit tests for the core logic in each `cmd/*` / internal package. Evidence: pkg/config 94.1%, pkg/ledstate 90.8%, pkg/periBlink 79.5%; dra-driver internal/driver 94.7%, internal/gpio 85.7%, internal/cdi 75.0%; plus 34 Rust tests and the Go↔Rust golden fixtures and interop check.
- [x] Keep `go test ./... -race` green; record the coverage delta. Evidence: `make go-test dra-test` green under `-race`; delta 0 test files → the figures above.
