# Proposal

## Why

In the `dra-blinkt-driver` design (the "DaemonSet approach"), DRA decides which LED each pod owns, but the
code that drives GPIO23/24 still runs inside **every** simulator pod's blinkt sidecar. The sidecars share the
bus frame by frame through a lock file in `/etc/kubedge`. So every simulator ships GPIO code, gets
`/dev/gpiochip0`, depends on a host-file protocol, and keeps its colour in a separate ConfigMap. The driver
writes a CDI spec file per claim that must be cleaned up.

A single owner of the strip on each node is simpler and more accurate. If the claim also carries the colour,
the node component knows everything needed to light an LED, and it can rebuild the strip from the
ResourceClaims in the API at any time (the operator pattern: reconcile desired against actual). Nothing is
left on disk to create or clean up beyond the kubelet plugin sockets. Choosing between the old and new way
of running the Blinkt! per cluster is also a lifecycle decision that belongs in a custom resource, not in
hand-applied manifests.

This change adds that path **in parallel**. The DaemonSet approach (`dra-driver/`, plain YAML, CDI hand-off to
sidecars) stays as is and remains one of the selectable modes.

## What Changes

- **New node agent** (`blinkt-operator`, binary `blinkt-node-agent`): a DRA kubelet plugin for
  `blinkt.kubedge.io` that is also the **sole GPIO writer** on its node.
  - It publishes `pixel-0` … `pixel-7` exactly as `dra-driver` does: same driver name, DeviceClass and
    attributes, so the same claims work.
  - It watches ResourceClaims allocated on its node and draws the frame they describe.
  - Prepare and unprepare validate the claim and trigger a redraw. They return no CDI devices, so claiming
    pods get **no** device, mount or GPIO code.
- **Colour in the claim**: an opaque device configuration `PixelConfig` (`blinkt.kubedge.io/v1alpha1`) with
  `color`, `intensity`, `algorithm` and `frequency`. The agent validates it when the claim is prepared.
- **New cluster operator** (binary `blinkt-operator`, CRD `BlinktConfig`, cluster-scoped singleton), selecting
  `spec.mode`:
  - `legacy`: no DRA components; simulators use their privileged sidecars;
  - `cdi`: deploys the existing `dra-driver` DaemonSet (the DaemonSet approach);
  - `agent`: deploys the node agent DaemonSet.

  It also owns the DeviceClass, never runs two modes' DaemonSets on one node, refuses to remove DRA components
  while blinkt claims exist, and reports status.
- **Legacy compatibility** (agent mode, optional): the agent can take part in the 0.4.x shared-state protocol,
  so legacy sidecars on the same node keep their LEDs during migration.
- **Not changing:** `dra-driver/` and its specs; `go-blinkt` / `rust-blinkt`; the plain-YAML
  `dra-driver/deploy/`. Simulator chart changes for agent mode are follow-up work in their own repos.

## Capabilities

### New Capabilities
- `blinkt-pixel-config`: the opaque per-claim LED configuration (colour, intensity, pattern): schema,
  defaults and validation.
- `blinkt-node-agent`: the node component in agent mode, covering device publication, sole ownership of the
  GPIO lines, reconciling the strip from ResourceClaims, prepare/unprepare semantics, restart recovery and
  legacy compatibility.
- `blinkt-operator`: the cluster operator, covering the `BlinktConfig` resource, mode selection and switching,
  the components it manages, its safety rules and its status.

### Modified Capabilities
- None. `dra-blinkt-driver`'s capabilities (`blinkt-dra-driver`, `blinkt-pixel-claims`) are still in flight
  and describe the `cdi` mode, which this change keeps.

## Impact

- **New code:** `blinkt-operator/`, its own Go module in the kubebuilder layout
  (`cmd/`, `api/v1alpha1/`, `internal/controller/`, `internal/agent/`, `config/`) on controller-runtime v0.24 /
  Kubernetes v0.36 libraries, matching `kubedge-operator-base`. It reuses `go-blinkt/pkg/periBlink` for frame
  writing.
- **New image:** `kubedge1/blinkt-operator`, multi-arch, holding both binaries (`/manager`, `/node-agent`).
- **New CRD:** `blinktconfigs.blinkt.kubedge.io`.
- **Clusters:** same prerequisites as the DaemonSet approach (Kubernetes ≥ 1.34, DRA stable from 1.35).
  Agent mode does **not** need CDI.
- **Charts** (follow-up, other repos): an agent-mode variant needs no blinkt sidecar, only a
  ResourceClaimTemplate with `PixelConfig`, plus `strategy: Recreate`.
