# Spec Delta

## Purpose

Keeps every Go module in this repo, and the images built from them, on one current and supported Go
release, so the Go implementation and the DRA driver build and test the same way.

## ADDED Requirements

### Requirement: Go modules target one supported Go release
Both Go modules, the root module (Go implementation of blinkt5) and `dra-driver/`, SHALL declare the same `go` directive, and it SHALL be a Go release still supported upstream (one of the two newest).

#### Scenario: Modules agree
- **WHEN** the `go` directives of `go.mod` and `dra-driver/go.mod` are compared
- **THEN** they name the same release, currently `1.26.0`

#### Scenario: Both modules stay green
- **WHEN** `make go-test go-lint dra-test dra-lint` runs
- **THEN** vet and `go test -race` pass in both modules

### Requirement: Go images build with the matching toolchain
Every Dockerfile that compiles Go code SHALL use a `golang` builder image of the same release the modules declare.

#### Scenario: Builder images match
- **WHEN** the `FROM golang:` lines of `build/Dockerfile.golang`, `hack/interop/Dockerfile` and `dra-driver/Dockerfile` are listed
- **THEN** each names the release declared in `go.mod`
