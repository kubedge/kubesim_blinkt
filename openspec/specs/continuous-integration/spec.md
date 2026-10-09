# continuous-integration Specification

## Purpose
Gates every change on the same checks as the local `make test lint` for the four code roots (`go-blinkt`,
`dra-driver`, `blinkt-operator`, `rust-blinkt`), plus the Go <-> Rust interop check and the multi-arch image
builds, in a workflow kept apart from the class-M `ci.yml` that claude-meta delivers.

## Requirements
### Requirement: CI checks every code root on every push

The repository SHALL run, on every pull request to `main` and every push to `main` that
touches code, the same checks as the local `make test lint` for each code root:
`go-blinkt` and `dra-driver` (`go test -race`, `gofmt`, `go vet`), `blinkt-operator`
(the same, with envtest), and `rust-blinkt` (`cargo test`, `rustfmt --check`, clippy
`-D warnings`). Each job SHALL invoke the Makefile target, not a copy of its commands.

#### Scenario: A failing test blocks the PR
- **WHEN** a pull request breaks a test in any of the four code roots
- **THEN** that root's job fails and the PR shows a failed check

#### Scenario: CI and local agree
- **WHEN** `make test lint` passes locally on a commit
- **THEN** the code jobs pass on that commit

### Requirement: CI checks Go <-> Rust interop and the images

CI SHALL run `make interop` and fail unless it prints `INTEROP OK`, and SHALL build,
without pushing, the Go and Rust blinkt5 images, the DRA driver image and the operator
image for linux/arm64 and linux/amd64.

#### Scenario: A Dockerfile breaks
- **WHEN** a change breaks `build/Dockerfile.rust` for linux/arm64
- **THEN** the images job fails and nothing is pushed

### Requirement: Code CI lives outside the class-M ci.yml

The code jobs SHALL live in `.github/workflows/code.yml`. `.github/workflows/ci.yml`
SHALL stay as delivered by claude-meta after `citrim` (`secret-scan`), so a meta
broadcast merges it without conflicts from project jobs.

#### Scenario: Next meta broadcast
- **WHEN** a claude-meta delivery changes `ci.yml`
- **THEN** the cherry-pick touches only `ci.yml` and the code jobs in `code.yml` are unchanged

#### Scenario: Docs-only change
- **WHEN** a pull request changes only Markdown or `openspec/`
- **THEN** the code jobs are skipped and `secret-scan` still runs

#### Scenario: One required code check
- **WHEN** the code jobs finish, run or skipped
- **THEN** `code-ok` passes only if each of them passed or was skipped, so `code-ok` and `secret-scan` are the
  required checks on `main`
