# Adopt CI for the four code roots

## Why

The repo has four code roots and no root `go.mod`:

| Root | Kind | Local checks |
|---|---|---|
| `go-blinkt/` | Go module (default blinkt5) | `make go-test go-lint` |
| `dra-driver/` | Go module | `make dra-test dra-lint` |
| `blinkt-operator/` | Go module, envtest (k8s 1.36) | `make operator-test operator-lint` |
| `rust-blinkt/` | Cargo crate (rust 1.99) | `make rust-test rust-lint` |

plus `make interop` (Go <-> Rust shared state, built from the tree) and the image builds.
The claude-meta delivery (alemax 0.3.3, #18) landed the class-M set, but its `ci.yml`
detects only a root `go.mod` / `pyproject.toml`, so `alemax update citrim` trimmed it to
`secret-scan`. Nothing gates build, test or lint on a push today.

## What changes

- New workflow `.github/workflows/code.yml`, separate from the class-M `ci.yml` so the next
  meta broadcast 3-way-merges `ci.yml` without touching it.
- Jobs call the same Makefile targets as the local loop, so CI and `make test lint` cannot
  drift:
  - `go` (matrix `go-blinkt`, `dra-driver`): `make <root>-test <root>-lint`;
  - `operator`: `make operator-test operator-lint` (setup-envtest fetches the 1.36 assets);
  - `rust`: `make rust-test rust-lint` (pinned `rust:1.99` container via `hack/cargo.sh`);
  - `interop`: `make interop`, which must print `INTEROP OK`;
  - `images`: `docker-buildx-check` for go and rust, `dra-buildx-check`,
    `operator-buildx-check` (no push, linux/arm64 + linux/amd64, cross-compiled).
- Go toolchain from each module's `go.mod` (`actions/setup-go`, `go-version-file`).
- Path filters so a docs-only change skips the code jobs; `secret-scan` stays in `ci.yml`.

## Non-goals

- golangci-lint: the project's lint is `gofmt` + `go vet` (Go) and `rustfmt` + clippy
  `-D warnings` (Rust); a new linter is a separate change.
- Shell lint (shellcheck/shfmt): `hack/*.sh` does not pass yet; fixing it is a separate
  change.
- Publishing images from CI: pushes stay manual (`make docker-buildx …`).
- Changing the class-M `ci.yml` beyond the citrim already applied.

## Impact

Every PR is gated on all four roots, the interop check and the image builds. Required
checks on `main` are set in the repo settings once the jobs are green.
