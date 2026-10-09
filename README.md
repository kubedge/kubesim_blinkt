# kubesim_blinkt

Use the Blinkt to illustrate Helm and Kubernetes behavior.

`blinkt5` (implemented in both Go and Rust, see Develop) drives a Pimoroni Blinkt! (8 × APA102 LEDs) on a Raspberry Pi through the Linux GPIO character
device (`/dev/gpiochip0`, BCM GPIO23 data / GPIO24 clock). It mostly runs as the `blinkt` sidecar of the
kubesim simulator charts (5gc, nr, epc, lte, elte), each lighting its own LED. Several blinkt processes on
one Pi share the strip through a locked state file, and each draws only its own pixels.

## Configuration

| Setting | Default | Meaning |
|---|---|---|
| `BLINKT_CONFIG` | `/etc/kubedge/blinkt_conf.yaml` | YAML config (`algorithm`, `intensity`, `frequency`, `pixel0`…`pixel7`) |
| `BLINKT_STATE_DIR` | `/etc/kubedge` | Shared state directory (`blinkt.lock`, `blinkt_state.json`); set it empty to run solo |
| `BLINKT_OWNER` | hostname (pod name) | Name of this process in the shared state |
| `BLINKT_PIXELS` | unset | LEDs allocated by a DRA claim, e.g. `6` or `0,1,2`. The process lights exactly these, in the colour configured for each index, else the first configured colour. Set by the DRA driver. |

With the DRA driver (`dra-driver/`), a claiming container gets `/dev/gpiochip0`, the shared state at
`/run/blinkt` (`BLINKT_STATE_DIR=/run/blinkt`) and `BLINKT_PIXELS` through CDI. It needs no
`privileged` and no hostPath. See [dra-driver/README.md](dra-driver/README.md).

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

```
go-blinkt/       Go implementation of blinkt5 (module github.com/kubedge/kubesim_blinkt/go-blinkt)
rust-blinkt/     Rust implementation of blinkt5 (crate kubesim_blinkt)
dra-driver/      DRA driver blinkt.kubedge.io (module github.com/kubedge/kubesim_blinkt/dra-driver)
blinkt-operator/ operator (BlinktConfig: legacy | cdi | agent) + sole-writer node agent (module …/blinkt-operator)
tests/fixtures/  golden frames + state file both implementations are tested against
build/           Dockerfiles (golang, rust) and CA bundle
hack/            container-run cargo/fmt and the Go<->Rust interop check
charts/          kubesim-blinkt Helm chart (blinkt.mode agent | cdi | legacy)
deploy/          test-pattern manifests rendered from the chart (make deploy-manifests)
openspec/        specs and changes
```

Two implementations of the same program live side by side and build the same `/blinkt5` image:

| | Go (default) | Rust |
|---|---|---|
| Source | `go-blinkt/` (`go.mod`, `cmd/`, `pkg/`) | `rust-blinkt/` (`Cargo.toml`, `src/`, `examples/`) |
| Toolchain | local `go` (go-blinkt/go.mod: 1.26) | pinned `rust:1.99` container (`hack/cargo.sh`), no local Rust needed |
| Dockerfile | `build/Dockerfile.golang` | `build/Dockerfile.rust` |
| Image | `kubedge1/kubesim_blinkt_go:<version>`, also published as `kubedge1/kubesim_blinkt:<version>` | `kubedge1/kubesim_blinkt_rs:<version>` |

Both test suites check the same golden frames and state file in `tests/fixtures/`, and `make interop` runs
both against one state directory. A behaviour change has to land in both implementations and the fixtures.

```sh
make test                         # rust-test + go-test
make lint                         # rust: rustfmt --check + clippy -D warnings; go: gofmt + vet
make fmt                          # format both
make interop                      # Go <-> Rust shared-state check, both built from this tree
make docker-buildx-check            # Go image, linux/arm64 + linux/amd64, no push
make docker-buildx-check IMPL=rust  # Rust image, no push
make docker-buildx                  # push kubesim_blinkt_go + kubesim_blinkt (the default)
make docker-buildx IMPL=rust        # push kubesim_blinkt_rs
make docker-buildx-all              # push both
```

Each image gets `<version>` and `latest` tags. Docker with buildx is needed for the Rust targets and the images (e.g. `colima start`).

## Deploy

`charts/kubesim-blinkt` lights the LEDs listed in `blinkt.pixels` (a fixed `index` 0-7, or none for any free
LED). `blinkt.mode` chooses how:

| mode | needs | pod |
|---|---|---|
| `agent` (default) | blinkt-operator, `BlinktConfig` mode `agent` | DRA claim with a PixelConfig per LED + a pause holder; the node agent draws. No blinkt process, device, privileges or hostPath |
| `cdi` | blinkt-operator mode `cdi` (or `dra-driver/deploy`), CDI in containerd | DRA claim + unprivileged blinkt; CDI hands it `/dev/gpiochip0`, `/run/blinkt`, `BLINKT_PIXELS` |
| `legacy` | nothing | privileged blinkt with hostPath `/etc/kubedge` |

```sh
helm install red charts/kubesim-blinkt                      # pixel 0 red, agent mode
helm install any charts/kubesim-blinkt -f anyfree.yaml      # pixels: [{color: [0,255,255]}]: any free LED
helm install t charts/kubesim-blinkt --set kind=DaemonSet   # one pod per node labelled blinktInstalled=true
```

Without Helm, `deploy/` holds a test pattern (all 8 LEDs in distinct colours, one pod per labelled node) rendered
from the chart: `deploy/kubesim-blinkt.yaml` (agent), `-cdi.yaml` and `-legacy.yaml`. Regenerate them with
`make deploy-manifests`, then apply with `make deploy`
(`kubectl label node <node> blinktInstalled=true` first). In DRA modes the test pattern claims all 8 LEDs of its
node, so simulator pods asking for a pixel there stay Pending until it is removed.

## Main tutorials

[Original Fork](https://github.com/richrarobi/periBlink)
