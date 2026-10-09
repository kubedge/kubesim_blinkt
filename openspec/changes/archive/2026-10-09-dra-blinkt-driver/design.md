# Design

## Context

- **Clusters today:** piclustera (3 Pi 3) and piclusterb (5 Pi 3) run Kubernetes **1.29.14** with containerd
  **1.7.25**. DRA there is an early alpha whose API was later replaced. The `resource.k8s.io/v1` API needs
  **≥ 1.34**, and DRA has been **stable (locked on) since 1.35**.
- **Target platform** (picluster-automation, four planned changes, piclustera first):
  - Kubernetes **1.36.5**, with `resource.k8s.io/v1` served and no feature gates;
  - containerd **2.x** (config v3) with CDI enabled and the default spec dirs `/etc/cdi` and `/var/run/cdi`;
  - kubeadm-default plugin paths `/var/lib/kubelet/plugins` and `/var/lib/kubelet/plugins_registry`;
  - arm64-only nodes: Pi 3, 899 MB RAM, Ubuntu 24.04, kernel 6.8.
  Their validation runs the upstream dra-example-driver first, then this driver.
- **Blinkt! sharing today** (0.4.0): each simulator's privileged sidecar draws its own pixels into a flock-guarded
  `/etc/kubedge/blinkt_state.json`. Pixel ownership comes from chart config, collisions are settled by owner name,
  and node placement relies on labels.
- **Hardware limit:** the APA102 strip is write-only, so a driver cannot detect whether a Blinkt! is plugged in.
  It can only see that the GPIO lines exist. Every Pi in both clusters has one (operator, 2026-10-08).

## Goals / Non-Goals

**Goals:**
- Conflict-free LED ownership decided by the scheduler, not by name sorting.
- Unprivileged blinkt containers: the only privileged piece is the driver DaemonSet.
- Coexistence with legacy (0.4.x hostPath) sidecars on the same node during migration.
- Only stable DRA features (`resource.k8s.io/v1`), nothing alpha.

**Non-Goals:**
- Upgrading the clusters. That's a picluster-automation change; this design only states the requirement.
- Replacing `kubedgeNodeType` role placement.
- Moving LED drawing into the driver (the "one LED server per node" option). Drawing stays in the workload's
  blinkt process.
- Detecting physical Blinkt! presence.

## Decisions

### D1. Driver in Go with `k8s.io/dynamic-resource-allocation/kubeletplugin`
The helper provides `Start`, kubelet registration, the `DRAPlugin` interface (`PrepareResourceClaims`,
`UnprepareResourceClaims`) and `PublishResources` for ResourceSlices. `kubernetes-sigs/dra-example-driver` is
the reference to fork from.
- Why: it's the officially maintained path. The gRPC protocol, registration and ResourceSlice reconciliation are
  already solved and track Kubernetes releases.
- Alternative: write the driver in Rust (kube-rs + tonic), hand-implementing the DRA gRPC and registration
  protocols. Rejected for now: it means owning a moving protocol with no reference implementation. It's the one
  place the two changes could converge (a single-language repo), so it can be revisited after `port-to-rust`
  ships.
- Relationship to `port-to-rust`: independent. The driver never draws LEDs. The only shared contract is the
  `BLINKT_PIXELS` / `BLINKT_STATE_DIR` env and the state directory, which both implementations already honour or
  will.

### D2. Driver lives in `dra-driver/` as its own Go module and image
Its own `go.mod` and `kubedge1/blinkt-dra-driver` image, built multi-arch with the repo's existing buildx
pattern.
- Why: `port-to-rust` removes the root Go module, and a sub-module keeps the driver unaffected. It also keeps
  driver and LED program versioned together, since they share the env contract.
- Alternative: a separate `kubedge/blinkt-dra-driver` repo. Reasonable later; one repo is simpler while the
  contract evolves.

### D3. Eight per-pixel devices, exclusive allocation
Device names `pixel-0` to `pixel-7`, attributes `index` and `model`.
- Why: plain exclusive allocation of separate devices is stable DRA and maps directly onto "each simulator owns
  an LED". The scheduler's normal allocation gives the conflict-freedom requirement with no custom logic.
- Alternatives:
  - One `blinkt` device shared through one ResourceClaim: loses per-LED conflict detection.
  - One device with consumable or partitionable capacity: depends on non-stable features.

### D4. CDI injection, one spec file per claim
On prepare, the driver writes `/var/run/cdi/blinkt.kubedge.io-<claim-uid>.json`, defining kind
`blinkt.kubedge.io/claim` and device `<claim-uid>` with:
- deviceNodes `/dev/gpiochip0`;
- a bind mount of the configured state directory to `/run/blinkt`;
- env `BLINKT_STATE_DIR=/run/blinkt` and `BLINKT_PIXELS=<indices>`.

It returns CDI ID `blinkt.kubedge.io/claim=<claim-uid>`. Unprepare deletes the file, and missing files are not
an error, which keeps both calls idempotent across restarts.
- Why: CDI device nodes are added to the container's device cgroup by the runtime. That removes the need for
  `privileged`. Env and mounts travel with the same spec.
- Runtime requirement: CDI enabled in containerd with `/var/run/cdi` among its spec dirs. The target containerd
  2.x does this by default. On containerd 1.7 it would need `enable_cdi = true`, but that's not targeted.

### D5. Shared state directory defaults to `/etc/kubedge`, mounted at `/run/blinkt`
- Why: legacy sidecars already use host `/etc/kubedge`, so the same host directory makes DRA pods and legacy
  pods draw cooperatively on one node.
- Why `/run/blinkt` and not `/etc/kubedge` inside the container: the charts already subPath-mount
  `/etc/kubedge/blinkt_conf.yaml` from a ConfigMap, and overlaying a CDI mount on that path would be fragile.

### D6. Line discovery without holding lines
At start-up, and on a slow resync, the driver opens `/dev/gpiochip*` read-only and checks for lines named
`GPIO23` and `GPIO24`, falling back to `gpiochip0` having at least 25 lines. It never requests the lines, so it
never competes with blinkt processes.

### D7. Allocation input to the blinkt program
`BLINKT_PIXELS` overrides which indices are owned. Colours come from the config for that index, else the first
configured colour. This keeps each chart's ConfigMap valid in both modes.
- Implementation: in whichever language is current when this lands. If `port-to-rust` is in flight, implement
  it once in Rust.

### D8. Charts gain an opt-in DRA mode
A values switch, `blinkt.mode: legacy | dra` (default `legacy`):
- `dra` adds a `ResourceClaimTemplate`, requesting class `blinkt-pixel.kubedge.io` with a CEL `index ==`
  selector for the chart's pixel, plus `pod.spec.resourceClaims`;
- `dra` removes the sidecar's `privileged` and hostPath.

## Risks / Trade-offs

- [Seven minor Kubernetes upgrades (1.29→1.36.5) on 899 MB Pi 3 nodes, one minor at a time with kubeadm] →
  owned by picluster-automation's planned changes. This driver is not deployed until a cluster reports ≥ 1.35.
- [CDI not active in containerd → pods fail with an "unresolvable CDI device" error] → the target is containerd
  2.x with CDI on by default, and picluster validates CDI with dra-example-driver before this driver.
- [Memory on 899 MB nodes] → the driver is a single small Go binary with a memory limit in the DaemonSet
  (target ≤ 32 Mi RSS). Verify on a Pi.
- [The driver advertises pixels on a Pi with no Blinkt! attached] → accepted. DaemonSet placement (nodeSelector)
  decides which nodes run the driver, and all current Pis have one.
- [Mixed legacy and DRA pods can still collide on a pixel, since legacy pods aren't allocated] → the shared state
  still settles it by name, as today. Migrate a node's simulators together.
- [Scheduler and driver overhead on Pi control planes] → small: eight devices per node, a handful of claims.
- [Name-based fallback (`gpiochip0`, ≥ 25 lines) misidentifies a non-Pi board] → the DaemonSet only runs on the
  Pis (arm64 nodeSelector).

## Migration Plan

1. **Prerequisite (picluster-automation):** a cluster on Kubernetes 1.36.5 with containerd 2.x and CDI enabled,
   validated with dra-example-driver. piclustera comes first.
2. Ship the driver image, DeviceClass, RBAC and DaemonSet. Verify the ResourceSlices: 8 devices per Pi.
3. Ship `BLINKT_PIXELS` support in the blinkt program (backward compatible: unset means today's behaviour).
4. Per simulator chart, enable `blinkt.mode=dra` on one node type at a time. Verify the sidecar is
   unprivileged and its LED is lit.
5. **Rollback:** switch the chart back to `legacy`. The driver can stay installed, since unused devices are
   harmless.

## Open Questions

- Exact cluster-upgrade sequence and timing: it lives in picluster-automation and doesn't change this design.
