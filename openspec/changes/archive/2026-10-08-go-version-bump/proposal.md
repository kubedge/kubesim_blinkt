# Bump the Go toolchain

## Why

The Go implementation's module (`go.mod`, repo root) declares `go 1.23`, which is past end of support: Go
supports the two newest releases, currently 1.26 and 1.27. The DRA driver module (`dra-driver/`) is already on
`go 1.26` because its Kubernetes v0.36 libraries require it. Two modules in one repo on different Go lines,
built with different `golang` images, is drift with no benefit.

(This change was first seeded from a sim-base template listing modules this repo doesn't have. It is
rewritten here for kubesim_blinkt's two Go modules.)

## What Changes

- Raise the root module's `go` directive from `1.23` to `1.26.0`, matching `dra-driver/go.mod`. Run
  `go mod tidy`.
- Build every Go binary with the matching `golang:1.26` image: `build/Dockerfile.golang`, `hack/interop/Dockerfile`
  (`dra-driver/Dockerfile` already does).
- Keep `go vet` and `go test -race` green in both modules, and keep the Go image's behaviour unchanged.

## Capabilities

### New Capabilities
- `go-toolchain`: which Go release the repo's Go modules and Go images target.

### Modified Capabilities
- None.

## Impact

- `go.mod`, `go.sum`, `build/Dockerfile.golang`, `hack/interop/Dockerfile`, README toolchain note.
- No behaviour change; the next published Go image is built with Go 1.26.
