# Design

## Context

Today's Go program is about 450 lines across four units, and each maps one-to-one onto a spec in this change:

| Go unit | Spec |
|---|---|
| `pkg/periBlink` (GPIO + APA102 frame) | `led-output` |
| `pkg/ledstate` (shared state) | `shared-led-state` |
| `pkg/config` + `cmd/main.go` | `blinkt-runtime` |

It ships as a static binary `/blinkt5` in a `FROM scratch` image, built by `build/Dockerfile.buildkit` with
`FROM --platform=$BUILDPLATFORM` and native cross-compilation. Building under QEMU emulation is known to crash
(arm/v7, 2026-10-08), so cross-compilation is a hard constraint. Deployed consumers:
- the `blinkt` sidecar of the five simulator charts (`kubedge1/kubesim_blinkt:0.4.0`);
- the optional `deploy/kubesim-blinkt.yaml` DaemonSet;
- the host binary installed by picluster-automation on all 8 Pis (installed, not running).

## Goals / Non-Goals

**Goals:**
- Same observable behaviour, byte-for-byte where the specs say so: frame bits, state file, log lines.
- Go 0.4.x and Rust processes coexist on one node during migration.
- Static, dependency-free binaries for `aarch64` and `x86_64` Linux. The image stays `FROM scratch`.
- Port the Go unit tests one-for-one, plus an interop test against the Go build.

**Non-Goals:**
- No new behaviour: no DRA, no new algorithms, no API server.
- No arm/v7 image, since it's retired.
- No change to Helm chart or manifest structure; only image tags move.

## Decisions

### D1. Single binary crate, modules mirroring the Go packages
Layout: `Cargo.toml` at repo root; `src/main.rs` (runtime), `src/config.rs`, `src/led_output.rs`,
`src/ledstate.rs`. The binary name is `blinkt5`.
- Why: a 1:1 mapping makes parity review a side-by-side read, and keeps tests in the same place as in Go.
- Alternative: a workspace with a library crate. Rejected because nothing else consumes it yet. The
  `dra-blinkt-driver` change may later want `ledstate` as a library, and splitting then is cheap.

### D2. GPIO: the `gpiocdev` crate (warthog618)
It is the Rust counterpart of `go-gpiocdev`, by the same author, with the same line-request model: find a line
by name, request it as an output, set its value, release it on drop.
- Why: the closest semantic match, so the EBUSY, find-by-name and fallback behaviour carry over directly.
  It's pure Rust (no libgpiod C dependency), which keeps musl static builds simple.
- Alternatives:
  - `gpio-cdev` (rust-embedded): older, v1 uAPI only, less maintained.
  - `libgpiod` bindings: need the C library and complicate static linking.
- Testability: the frame encoder writes to a small `OutputPin` trait (`set(bool)`); `gpiocdev` implements it
  in production and a recording fake in tests. This mirrors the Go `outputPin` interface.

### D3. Shared state: serde_json + `flock` via `rustix`
`State { owners: BTreeMap<String, Entry> }` and `Entry { pixels: Option<BTreeMap<String, Pixel>>, updated:
<RFC 3339 nano>, ttl: u64 nanos }`, with serde attributes matching the Go field names exactly.
- `BTreeMap` iteration is ascending byte order, which gives the merge rule directly.
- Pixel keys are kept as strings in the file and parsed to an index when merging, which ignores out-of-range
  or non-numeric keys as Go does.
- `rustix::fs::flock(LockExclusive)` on the opened `blinkt.lock`, the same syscall as Go's `syscall.Flock`.
  That makes Go and Rust locks mutually exclusive.
- Timestamps use the `time` crate's RFC 3339 format with nanoseconds. Go accepts any fractional precision on
  read, and Rust parses Go's trimmed fractions.
- Alternative: `nix` for flock. Fine too, but `rustix` is already pulled in by the GPIO stack and avoids libc.

### D4. Config: a maintained YAML crate
`serde_yaml` is archived, so use a maintained successor (`serde_yaml_ng` or equivalent) with
`#[serde(default)]` on every field to match Go's zero-value decoding.
- Alternative: hand-parse the eight known keys. Rejected as fragile.

### D5. Runtime details that must match Go
- **Log lines:** a minimal logger writes `YYYY/MM/DD HH:MM:SS <msg>` to stderr. `Stopping on Interrupt` /
  `Stopping on Terminate` / `Stopping` go to stdout, as with Go's `fmt.Println`. In a `scratch` image without
  zoneinfo the time is UTC, as with Go.
- **Signals:** `signal-hook` sets an `AtomicBool`; the loops check it between waits, as the Go `atomic.Bool`
  does.
- **Fatal paths:** exit with status 1 after logging.
- **Randomness:** `fastrand`; the blinkt5 pattern only needs to be random, not reproducible.

### D6. Build: cross-compile to musl with `cargo-zigbuild`
`build/Dockerfile.buildkit` builder stage: `FROM --platform=$BUILDPLATFORM rust:<stable>` with `cargo-zigbuild`
and zig. It maps `TARGETARCH` to `aarch64-unknown-linux-musl` or `x86_64-unknown-linux-musl`, builds
`--release`, and copies the binary to `/blinkt5` in `FROM scratch`.
- Why: native toolchain, no QEMU, fully static output, one Dockerfile for both arches.
- Alternatives:
  - `cross`: needs Docker-in-Docker.
  - Per-arch `rust:alpine` under emulation: the QEMU crash class seen on 2026-10-08.
- `make docker-buildx` keeps its interface: same `IMG`, `PLATFORMS` and tags.

### D7. Verification against the Go build before Go is deleted
- Interop test: both full binaries exit at the GPIO check off-Pi, so interop runs at the state-library level.
  A small Go test program (Go `ledstate` with a recording renderer) and a Rust integration test publish into
  the same temp state directory. The test then asserts both entries persist, the merged frame matches, and
  withdrawing either keeps the other. Fixtures written by each side are also read by the other.
- Frame golden test: the Rust encoder's bit stream for the spec scenarios equals Go's (fixture captured from
  the Go test).
- Hardware: rerun the home-pi shared-LED test with one Go and one Rust process.

## Risks / Trade-offs

- [Rust GPIO crate behaves differently on EBUSY or name lookup] → covered by the `led-output` scenarios, plus a
  hardware check on a Pi 3 before release.
- [Subtle JSON differences, e.g. timestamp precision or `null` vs `{}` pixels] → both are spec scenarios,
  exercised by fixtures written by Go and read by Rust, and the reverse.
- [Log format drift breaks the picluster verify step, which greps `blinkt: running algorithm=`] → exact-string
  tests on both start-up lines.
- [Binary size grows versus Go's 2.2 MB image] → acceptable; the release profile uses `lto`, `strip` and
  `opt-level="z"` if needed.
- [Two languages in the repo during migration] → Go is removed in the same change once parity tasks pass; no
  long-lived dual build.

## Migration Plan

1. Land the Rust crate and build. Publish `0.5.0` to `kubedge1/kubesim_blinkt`.
2. Hardware test on home-pi, mixed with Go 0.4.0.
3. Bump `blinktTag` to `0.5.0` in the simulator charts (separate PRs). Update `deploy/` and the
   picluster-installed host binary.
4. **Rollback:** set the charts back to `0.4.0`. The state format is shared, so mixed or rolled-back nodes keep
   working.
