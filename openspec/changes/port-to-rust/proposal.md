# Proposal

## Why

The operator wants kubesim_blinkt in Rust. The Go code is now small and fully characterized: GPIO via the
character device, APA102 framing, shared LED state and config/runtime (PRs #1, #3, #5). It was hardware-verified
on the 64-bit Pi 3 nodes on 2026-10-08. That makes this the cheapest point to port it like-for-like, before
more features land on the Go side.

## What Changes

- Re-implement the current Go program (`cmd/main.go`, `pkg/config`, `pkg/periBlink`, `pkg/ledstate`) in Rust,
  with **identical externally visible behaviour**:
  - same environment variables, config file format, algorithms, log lines and exit codes;
  - same GPIO lines and APA102 frame bytes;
  - same shared-state file format and locking.
- A Rust binary and a Go 0.4.x binary SHALL interoperate on the same node: same `blinkt.lock` and
  `blinkt_state.json`, same merge rule. Simulator pods can therefore move from one to the other one at a time.
- The Rust image keeps the entrypoint (`/blinkt5`), base (`FROM scratch`) and platforms (`linux/arm64`,
  `linux/amd64`). It is published as `docker.io/kubedge1/kubesim_blinkt_rs` (`build/Dockerfile.rust`,
  `make docker-buildx IMPL=rust`). Go stays the default (operator decision, 2026-10-08):
  `kubedge1/kubesim_blinkt_go`, also published as `kubedge1/kubesim_blinkt`.
- The Go implementation stays in the repo **side by side** with the Rust one (operator decision, 2026-10-08).
  Either can be built: `make docker-buildx` (Go, default) or `make docker-buildx IMPL=rust`.
  `make test lint` covers both. Both are held to the same `tests/fixtures` and to each other (`make interop`).
- Out of scope:
  - new features, including any Kubernetes DRA integration (see the separate `dra-blinkt-driver` change);
  - changes to the Helm chart, `deploy/` manifests or the simulator charts beyond the image tag.

## Capabilities

There is no baseline spec yet, so this change records the current Go behaviour as the contract the Rust
implementation must meet.

### New Capabilities
- `led-output`: driving the Blinkt! hardware. GPIO line selection and hold/release, APA102 frame encoding,
  value masking, busy-line retry and GPIO error reporting.
- `shared-led-state`: sharing the eight LEDs between blinkt processes on one node. State directory, lock,
  file format, merge rule, expiry, withdrawal and solo fallback.
- `blinkt-runtime`: the program's operator-facing contract. Config file and env vars, algorithms and their
  timing, owner naming, start-up/shutdown log lines, signals and exit codes.

### Modified Capabilities
- None.

## Impact

- **Code:** a Cargo crate (`Cargo.toml`, `src/`) is added next to the Go module (`go.mod`, `cmd/`, `pkg/`).
  `Makefile` gets `rust-*` and `go-*` targets plus an `IMPL=rust|go` image switch. The Go-only
  `docker-build-*` targets, the arm32v7/arm64v8 Dockerfiles and Travis config are removed.
- **Build:** `build/Dockerfile.buildkit` switches to a Rust toolchain that cross-compiles static musl binaries for
  `aarch64` and `x86_64`. The image stays `FROM scratch`.
- **Dependencies:** a Rust GPIO character-device crate, serde/serde_json, and a flock binding. Exact crates are
  in design.md.
- **Release:** the Rust build was first published as `kubedge1/kubesim_blinkt:0.5.0`. From 0.5.1 it is
  `kubedge1/kubesim_blinkt_rs`, and `kubedge1/kubesim_blinkt` is the Go build again. The simulator charts'
  `blinktTag` moves only after hardware verification.
- **Related:** `go-version-bump`, `adopt-go-ci` and `test-coverage-uplift` still apply to the Go
  implementation. `buildx-multiarch-image` applies to both images.
