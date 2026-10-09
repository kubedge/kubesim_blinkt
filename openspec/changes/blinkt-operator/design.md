# Design

## Context

- **DaemonSet approach (`dra-driver/`, change `dra-blinkt-driver`)**: the driver publishes `pixel-0..7`
  and, per claim, writes a CDI spec giving the pod `/dev/gpiochip0`, the `/etc/kubedge` state directory at
  `/run/blinkt` and `BLINKT_PIXELS`. The pod's own blinkt process draws, sharing the bus through the
  flock-guarded state file. Verified on kind 1.36.4; published `kubedge1/blinkt-dra-driver:0.5.1`.
- **Drawing code**: `go-blinkt/pkg/periBlink` (acquire/draw/release with EBUSY retry) and
  `go-blinkt/pkg/ledstate` (shared-state Board) are importable Go packages.
- **Target clusters** (picluster-automation, planned): Kubernetes 1.36.5, containerd 2.x, arm64 Pi 3 with
  899 MB RAM; the control planes have about 190 MB free. No Helm on the clusters; plain YAML is expected.
- **DRA API facts** used here (k8s.io/api v0.36 `resource/v1`):
  - claim `spec.devices.config[].opaque{driver, parameters}` is copied into
    `status.allocation.devices.config[]`, with `source` FromClass or FromClaim and the request names;
  - `status.reservedFor` lists the consuming pods;
  - a `DeviceClass` can carry `spec.config` (FromClass).

## Goals / Non-Goals

**Goals:**
- One writer per node in agent mode, with the strip reconciled from the API: no CDI files, no state file, no
  checkpoint.
- Colour, intensity and pattern live in the claim; class-level defaults come from the operator.
- One declarative switch per cluster (`BlinktConfig.spec.mode`) across legacy, cdi and agent, with safe
  transitions.
- The DaemonSet approach stays intact: as the operator's `cdi` mode, and standalone via plain YAML.

**Non-Goals:**
- Changing `dra-driver/`, `go-blinkt/` or `rust-blinkt/` behaviour. The agent reuses `go-blinkt` packages
  without modifying them.
- Managing simulator workloads or their charts. Agent-mode chart variants are follow-up work in the
  simulator repos.
- Multi-cluster management, and webhooks (validation happens at prepare time and in the CRD schema).

## Decisions

### D1. One Go module `blinkt-operator/`, kubebuilder layout, one image with two binaries
Layout: `cmd/manager`, `cmd/node-agent`, `api/v1alpha1`, `internal/controller`, `internal/agent`,
`internal/pixelconfig`, and `config/` (kustomize). The stack is controller-runtime v0.24.x with Kubernetes
v0.36.x libraries and `controller-gen` for the CRD and DeepCopy, matching `kubedge-operator-base` and
`dra-driver`. One image, `kubedge1/blinkt-operator`, ships `/manager` and `/node-agent`.
- Why one image: a single build and release, and the agent DaemonSet uses the same image as the operator
  that deploys it, so versions can't skew.
- Alternative: two images. Rejected for now; it doubles release work for two small static binaries.
- `go-blinkt` is consumed through a `replace github.com/kubedge/kubesim_blinkt/go-blinkt => ../go-blinkt`
  directive, so the agent draws with the same code the Go sidecar uses.

### D2. The agent takes the GPIO lines once and keeps them
At start-up the agent acquires GPIO23/24 through `periBlink` (the same lookup by name with a `gpiochip0`
fallback). Success is the hardware discovery. If acquisition fails, it publishes 0 devices and logs the
reason.
- Why: a sole writer has no reason to release the lines between frames, and holding them stops a stray
  legacy process from interleaving bits.
- With `legacyCompat`: the agent instead uses `ledstate.Board` (owner `blinkt-node-agent`) with per-frame
  acquisition, exactly as a sidecar does, so legacy sidecars keep working.

### D3. Desired state comes from a ResourceClaim informer
- The agent runs a shared informer on ResourceClaims (cluster-wide list/watch; the scale is tiny). A claim
  counts toward this node when an allocation result has `driver == blinkt.kubedge.io` and `pool == <node>`,
  and `status.reservedFor` is non-empty.
- For each such result, the effective PixelConfig is computed: defaults, then FromClass, then FromClaim,
  filtered to configs whose `requests` include the result's request (or that list no requests).
- Prepare validates the config (error = pod does not start) and enqueues a redraw. It returns the devices with
  empty `CDIDeviceIDs`. Unprepare only enqueues a redraw.
- Why the informer is the truth: kubelet calls can repeat, reorder or be lost across restarts. The API is
  authoritative, so restart recovery needs no checkpoint.

### D4. Render loop
A 10 ms ticker computes each lit claim's on/off phase from its pattern (`steady`, `fixed5`, `fixed`) and its
start time. It draws only when the 8-pixel frame differs from the last one drawn. On SIGTERM it draws a dark
frame and releases the lines.
- Why: one goroutine, deterministic, cheap (a frame write is a few ms and happens only on change).

### D5. The operator manages objects it owns, through server-side apply
- In namespace `blinkt-system`, the operator owns:
  - the `dra-driver` DaemonSet (cdi mode), with the same spec as `dra-driver/deploy/blinkt-dra-driver.yaml`;
  - the node-agent DaemonSet (agent mode);
  - DeviceClass `blinkt-pixel.kubedge.io`, with `spec.config` = `defaults` as a PixelConfig.
- ServiceAccounts and RBAC for the node components ship statically in `config/rbac`, so the operator never
  needs to grant permissions (no `escalate`/`bind`).
- Owned objects carry an owner reference to the `BlinktConfig` and label `app.kubernetes.io/managed-by:
  blinkt-operator`. Anything without that label is "foreign" and never touched.
- Watches: BlinktConfig, owned DaemonSets, DeviceClass, and ResourceClaims (for the in-use guard and the
  device count via ResourceSlices).

### D6. Mode switching as a small state machine
`desired = spec.mode`, `current = status.mode`.

| Switch | Steps |
|---|---|
| cdi ↔ agent | delete the old DaemonSet → wait for zero pods → create the new one → status `Switching` until rolled out → `current = desired` |
| → legacy | allowed only with zero blinkt ResourceClaims; otherwise keep the components, `Ready=False/ClaimsInUse` |

`legacyCompat` is passed to the agent as a flag.

### D7. Packaging: plain YAML from kustomize
`make operator-manifests` renders `config/` into `blinkt-operator/dist/install.yaml` (CRD, namespace, RBAC,
manager Deployment). It is published as a release artifact and validated with kubeconform. The manager is a
single replica with leader election, on arm64 nodes with the control-plane toleration, with a 96 Mi memory
limit. The agent DaemonSet has a 64 Mi limit (32 Mi request).
- Measured on kind 1.36.4 (peak RSS): manager 35.7 MB, agent 36.7 MB. The agent's ResourceClaim informer costs
  about 20 MB over `dra-driver`'s 15.8 MB, so the planned 48 Mi limit was raised to 64 Mi for headroom.

## Risks / Trade-offs

- [LED semantics change: in agent mode the LED means "claim reserved by a pod", lit from scheduling until the
  pod is gone, not "the sidecar process is alive"] → documented. For a simulator status light this follows
  Kubernetes' own view and needs no code in the pod.
- [Agent holding the lines makes non-compat legacy sidecars on the same node fail with EBUSY] → the operator's
  `legacyCompat` turns on shared-state mode, and the docs say to enable it until a node's simulators are
  migrated.
- [Two drivers registering `blinkt.kubedge.io` on one node] → the operator serializes the switch and refuses
  to deploy when a foreign driver DaemonSet exists (`ForeignDriver`).
- [Memory on 899 MB Pis: manager plus agent plus informers] → measured on kind: manager 35.7 MB, agent 36.7 MB
  (limits 96 Mi and 64 Mi). A control plane hosting both uses about 73 MB of its ~190 MB free. Re-measure on a Pi.
- [Cluster-wide ResourceClaim watch from every agent] → negligible at this scale; a field-selector or
  label-based narrowing can come later.
- [Opaque config is free-form JSON] → strict decoding (unknown fields rejected) plus range checks, tested.

## Migration Plan

1. Install the operator (`dist/install.yaml`). The BlinktConfig defaults to `legacy`, so nothing changes.
2. On a cluster where the DaemonSet approach was hand-applied: delete it, then set `mode: cdi`. The operator
   now owns the same driver.
3. Per cluster, set `mode: agent` with `legacyCompat: true`; switch simulators to agent-mode charts (claim
   plus PixelConfig, no sidecar); then set `legacyCompat: false`.
4. **Rollback:** `mode: cdi`. The agent clears the strip, the driver comes back, and claims stay valid because
   the devices and class are identical. If PixelConfig is ignored in cdi mode, the pods' own blinkt sidecars
   draw instead.

## Open Questions

- Whether `cdi` mode should also honour PixelConfig, by passing it through the CDI env to sidecars. This
  doesn't change this change's specs, which leave `cdi` mode as `dra-blinkt-driver` defines it.
