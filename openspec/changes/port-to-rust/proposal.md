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
- The container image keeps its name (`docker.io/kubedge1/kubesim_blinkt`), entrypoint (`/blinkt5`), base
  (`FROM scratch`) and platforms (`linux/arm64`, `linux/amd64`). The Rust build replaces the Go build in
  `build/Dockerfile.buildkit` and `make docker-buildx`.
- The Go sources are removed once the Rust binary passes the same hardware checks. **BREAKING** only for anyone
  importing the Go packages as a library; none are known (the repo is standalone in the kubedge org).
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

- **Code:** `cmd/`, `pkg/` (Go) are replaced by a Cargo crate. `go.mod`/`go.sum` go away. `Makefile` targets
  `fmt`/`vet-v1`/`docker-build-*` are replaced by `cargo fmt`/`clippy`/`test` equivalents.
- **Build:** `build/Dockerfile.buildkit` switches to a Rust toolchain that cross-compiles static musl binaries for
  `aarch64` and `x86_64`. The image stays `FROM scratch`.
- **Dependencies:** a Rust GPIO character-device crate, serde/serde_json, and a flock binding. Exact crates are
  in design.md.
- **Release:** published as `kubedge1/kubesim_blinkt` 0.5.0. The simulator charts' `blinktTag` moves only after
  hardware verification.
- **Related:** the open `go-version-bump`, `adopt-go-ci` and `test-coverage-uplift` changes target the Go code
  and become moot. `buildx-multiarch-image` still applies (same image and platforms).
