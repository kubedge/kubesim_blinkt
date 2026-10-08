# Tasks

## 1. Scaffold and parity fixtures

- [x] 1.1 Capture Go golden fixtures before any Rust code: the frame bit streams for the `led-output` scenarios and a `blinkt_state.json` written by Go 0.4.0. Commit them under `tests/fixtures/` and verify they come from the current Go tests (`go test ./...` passes).
- [x] 1.2 Create the Cargo crate (binary `blinkt5`, modules `config`, `led_output`, `ledstate`) with the release profile, and add `rust-toolchain.toml`. Verify `cargo build` and `cargo clippy -- -D warnings` succeed on macOS.

## 2. LED output (`led-output`)

- [x] 2.1 Implement the APA102 encoder over an `OutputPin` trait with value masking. Verify unit tests reproduce the Go frame fixture and the masking scenario.
- [x] 2.2 Implement line lookup by name with the `gpiochip0` fallback, the output request, and the `data=… clock=…` description using `gpiocdev`, gated to Linux with a non-Linux stub. Verify it cross-compiles for `aarch64-unknown-linux-musl`.
- [x] 2.3 Implement per-frame acquire/release and EBUSY retry (5 ms steps, 2 s cap; other errors fail at once). Verify with fake-request unit tests for freed-in-time, held-too-long and no-device.

## 3. Shared state (`shared-led-state`)

- [x] 3.1 Implement the State/Entry serde model, load (missing, corrupt or null tolerant) and atomic save. Verify the Go-written fixture round-trips and Go can read a Rust-written file (Go test reading the Rust fixture).
- [x] 3.2 Implement flock-serialized update, merge by owner name, out-of-range pixel filtering, TTL pruning, withdraw and solo fallback. Verify by porting every Go `ledstate` test, including 4 concurrent publishers.
- [x] 3.3 Add the interop check: a Go process and a Rust process publishing into one temp state directory. Verify both entries persist, the merged frame matches, and withdrawing either keeps the other.

## 4. Runtime (`blinkt-runtime`)

- [x] 4.1 Implement the config loader (`BLINKT_CONFIG`, defaults, pixel lists with fewer than 3 values ignored, fatal read/parse errors with exact messages). Verify with unit tests for each `blinkt-runtime` config scenario.
- [x] 4.2 Implement main: GPIO check, then config, frequency default, owner/TTL computation, both start-up log lines (exact format), the fixed/fixed5/blinkt5 loops, SIGINT/SIGTERM handling, withdraw on exit, and exit codes. Verify with tests on the log-line strings and TTL, and a run on macOS showing `blinkt: GPIO setup failed:` and exit 1.
- [x] 4.3 Update README.md (build, run, env vars) for the Rust toolchain. Verify the documented commands run as written.

## 5. Build and image

- [x] 5.1 Switch `build/Dockerfile.buildkit` to the cargo-zigbuild musl cross-build and update Makefile targets (`fmt`, `lint`, `test`, `docker-buildx`). Verify `docker buildx build --platform linux/arm64,linux/amd64` succeeds without pushing, and the arm64 image smoke-run prints `blinkt: GPIO setup failed`.
- [x] 5.2 Keep the Go implementation side by side (amended 2026-10-08; the first pass removed it). `build/Dockerfile.golang` and `make … IMPL=go` build it, `go-test` and `go-lint` check it, Go tests read the shared `tests/fixtures`, and interop builds both from the tree. Remove only the Go-only legacy build (arm32v7/arm64v8 Dockerfiles, Travis). Verify `make test lint interop` passes and `make docker-buildx-check IMPL=rust` and `IMPL=go` both build.

## 6. Hardware and release

- [x] 6.1 Hardware test on a Pi 3 (home-pi): the Rust binary alone (8-colour pattern), then Rust plus Go 0.4.0 sharing LEDs (the 4-phase shared test). Verify the operator confirms each phase and there are no GPIO errors.
- [x] 6.2 Publish `kubedge1/kubesim_blinkt:0.5.0` via `make docker-buildx`. Verify `imagetools inspect` shows linux/arm64 and linux/amd64, and an anonymous pull works.
