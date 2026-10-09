# Architecture

How the parts of kubesim_blinkt fit together. What each part does and how to build and run it is in
[README.md](README.md); the normative behaviour is in `openspec/specs/`.

## The hardware and the problem

A Pimoroni Blinkt! is eight APA102 LEDs on two GPIO lines of a Raspberry Pi (BCM GPIO23 data, GPIO24 clock).
One strip per node is shared by many pods: the kubesim simulator sidecars (5gc, nr, epc, lte, elte) and the
`charts/kubesim-blinkt` demo replicas each want "their" LED. Two things must hold:

- only one process writes a frame at a time (the lines carry one serial stream);
- every frame shows every owner's pixels, not just the writer's.

## Components

| Component | Where | Role |
|---|---|---|
| `blinkt5` | `go-blinkt/` (default), `rust-blinkt/` | The LED program: reads its config, draws its own pixels, blinks them |
| shared LED state | `ledstate` package / module in both | Locked state file that merges the pixels of every `blinkt5` on a node |
| DRA driver | `dra-driver/` | DaemonSet kubelet plugin `blinkt.kubedge.io`: publishes `pixel-0..7`, hands claims the device through CDI |
| operator | `blinkt-operator/cmd/manager` | Reconciles the `BlinktConfig` singleton into the components of one mode |
| node agent | `blinkt-operator/cmd/node-agent` | Agent-mode kubelet plugin: the only writer of the strip, draws from the ResourceClaims |
| demo chart | `charts/kubesim-blinkt` | One LED per replica in the release colour; `deploy/` is rendered from it |

`blinkt5` exists twice on purpose. Go and Rust build the same `/blinkt5` and are held to the same golden frames
(`tests/fixtures/`) and to each other (`make interop`), so either image can run next to the other on one node.

## Layer 1: driving the LEDs (`led-output`)

`blinkt5` opens the GPIO character device (`/dev/gpiochip0`), looks the two lines up by name and bit-bangs an
APA102 frame: 32 zero bits, then `0xE0|luminance`, B, G, R per pixel, then the end frame. It requests the lines
only for the duration of one frame and retries `EBUSY` for up to 2 s, which is what lets several processes on
one node take turns on the same lines.

## Layer 2: sharing the strip (`shared-led-state`)

Taking turns is not enough: the last writer would erase everyone else. Each `blinkt5` therefore publishes its
pixels into `blinkt_state.json` (in `/etc/kubedge`, or `/run/blinkt` under CDI) under an exclusive flock on
`blinkt.lock`:

```
lock -> read state -> prune expired owners -> set my pixels -> save atomically -> draw merged frame -> unlock
```

Each owner entry has a TTL, so a crashed process's pixels age out; on SIGTERM a process withdraws only its own
pixels. On a pixel conflict the owner whose name sorts last wins. If the directory is unusable, `blinkt5` runs
solo and draws only its own pixels.

## Layer 3: allocating LEDs (`blinkt-pixel-claims`, DRA)

A label and a hostPath cannot stop two pods from claiming the same LED; DRA can. The node publishes one
ResourceSlice with eight devices (`pixel-0..7`, attributes `index`, `model`) under the DeviceClass
`blinkt-pixel.kubedge.io`. A workload's ResourceClaim asks for one pixel, a specific `index` or any free one,
and the scheduler gives each pixel to at most one claim: a ninth claim on a full node stays Pending.

## The three modes (`blinkt-operator`)

`BlinktConfig` `cluster` picks one, for the nodes in its `nodeSelector`:

| Mode | Who draws | Pod needs | How the pixel reaches the LED |
|---|---|---|---|
| `legacy` | each pod's `blinkt5` | privileged, hostPath `/etc/kubedge` | config file names the pixel; shared state merges |
| `cdi` | each pod's `blinkt5` | a claim; nothing privileged | the DRA driver's CDI spec injects `/dev/gpiochip0`, `/run/blinkt`, `BLINKT_PIXELS` |
| `agent` | the node agent only | a claim with a `PixelConfig`; no device at all | the agent rebuilds the strip from the claims allocated on its node |

In `agent` mode the GPIO lines never leave the agent: the pod is a pause container holding a claim, and the
claim's opaque `PixelConfig` (colour, intensity, pattern; class defaults, claim overrides) is the whole
description of the LED. Prepare and unprepare are only triggers; after a restart the agent redraws from the API.
With `legacyCompat` it also writes through the shared state so legacy sidecars can coexist.

The operator keeps the switch safe: it never runs two drivers on a node and steps aside for a foreign
`blinkt.kubedge.io` driver; it refuses to leave a DRA mode while claims are live (`ClaimsInUse`); it waits to
narrow the node set while a dropped node holds a claim (`NodeClaimsInUse`); and when a node leaves the agent it
runs a `blinkt-clear-<node>` Job that withdraws the agent's pixels (`blinkt-node-lifecycle`).

## Build and delivery

- Images: `kubesim_blinkt_go` (also `kubesim_blinkt`), `kubesim_blinkt_rs`, `blinkt-dra-driver`,
  `blinkt-operator`. Each is one buildx manifest for linux/arm64 + linux/amd64, cross-compiled on
  `$BUILDPLATFORM` into a static binary `FROM scratch` (`container-packaging`).
- CI: `.github/workflows/code.yml` runs the Makefile targets for the four code roots, `make interop` and the
  image builds; `code-ok` and `secret-scan` are the required checks (`continuous-integration`). `ci.yml` comes
  from claude-meta.
