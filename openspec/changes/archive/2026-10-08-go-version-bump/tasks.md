# Tasks — go-version-bump

## 1. Bump

- [x] 1.1 Raise the root `go.mod` directive to `1.26.0` and run `go mod tidy`. Verify `grep '^go ' go.mod dra-driver/go.mod` shows `1.26.0` twice.
- [x] 1.2 Switch `build/Dockerfile.golang` and `hack/interop/Dockerfile` to `golang:1.26`, and update the README's Go toolchain note. Verify every `FROM golang:` line names 1.26.

## 2. Verify

- [x] 2.1 Verify `make go-test go-lint dra-test dra-lint` and `make interop` pass, `make docker-buildx-check` (Go image) builds, and the arm64 Go image still exits 1 with `blinkt: GPIO setup failed: …` when there is no GPIO.
