# kubesim_blinkt

Use the Blinkt to illustrate Helm and Kubernetes behavior.

`blinkt5` (implemented in both Rust and Go, see Develop) drives a Pimoroni Blinkt! (8 × APA102 LEDs) on a Raspberry Pi through the Linux GPIO character
device (`/dev/gpiochip0`, BCM GPIO23 data / GPIO24 clock). It mostly runs as the `blinkt` sidecar of the
kubesim simulator charts (5gc, nr, epc, lte, elte), each lighting its own LED. Several blinkt processes on
one Pi share the strip through a locked state file, and each draws only its own pixels.

## Configuration

| Setting | Default | Meaning |
|---|---|---|
| `BLINKT_CONFIG` | `/etc/kubedge/blinkt_conf.yaml` | YAML config (`algorithm`, `intensity`, `frequency`, `pixel0`…`pixel7`) |
| `BLINKT_STATE_DIR` | `/etc/kubedge` | Shared state directory (`blinkt.lock`, `blinkt_state.json`); set it empty to run solo |
| `BLINKT_OWNER` | hostname (pod name) | Name of this process in the shared state |

Example config, the kubesim_lte sidecar's:

```yaml
algorithm: fixed5   # lit `frequency` ms, dark 10 ms; any other name: lit/dark `frequency` ms each; blinkt5: random
intensity: 5        # luminance 0-31
frequency: 1000     # ms, defaults to 1000 when missing
pixel6: [0, 0, 255] # LED 6 blue; lists with fewer than 3 values are ignored
```

On start it logs `blinkt: GPIO ready (data=gpiochip0:23 clock=gpiochip0:24)`, then
`blinkt: running algorithm=… frequency=…ms config=… owner=… shared state=/etc/kubedge`, or `solo (…)` when
the state directory isn't usable. GPIO, config or drawing failures exit 1 with a `blinkt: …` line.
SIGTERM turns off only this process's LEDs.

## Run on a Pi

```sh
BLINKT_CONFIG=./blinkt.yaml BLINKT_STATE_DIR=/tmp/blinkt ./blinkt5   # user in group dialout, or sudo
```

## Develop

Two implementations of the same program live side by side and build the same `/blinkt5` image:

| | Rust (default image) | Go |
|---|---|---|
| Source | `Cargo.toml`, `src/` | `go.mod`, `cmd/`, `pkg/` |
| Toolchain | pinned `rust:1.99` container (`hack/cargo.sh`), no local Rust needed | local `go` (go.mod: 1.23) |
| Image | `build/Dockerfile.rust` → `kubedge1/kubesim_blinkt:<version>`, `:latest` | `build/Dockerfile.golang` → `:<version>-go`, `:latest-go` |

Both test suites check the same golden frames and state file in `tests/fixtures/`, and `make interop` runs
both against one state directory. A behaviour change has to land in both implementations and the fixtures.

```sh
make test                         # rust-test + go-test
make lint                         # rust: rustfmt --check + clippy -D warnings; go: gofmt + vet
make fmt                          # format both
make interop                      # Go <-> Rust shared-state check, both built from this tree
make docker-buildx-check          # Rust image, linux/arm64 + linux/amd64, no push
make docker-buildx-check IMPL=go  # Go image, no push
make docker-buildx                # build and push the Rust image
make docker-buildx IMPL=go        # build and push the Go image (-go tags)
```

Docker with buildx is needed for the Rust targets and the images (e.g. `colima start`).

Deploy the standalone DaemonSet with `make deploy` (plain YAML in `deploy/`, after
`kubectl label node <node> blinktInstalled=true`).

## Main tutorials

[Original Fork](https://github.com/richrarobi/periBlink)
