# Proposal

## Why

The Blinkt! is a shared node device: several simulator pods, each owning one LED, can land on the same Pi.
Today this is handled by convention:
- every simulator's blinkt sidecar runs **privileged** with a hostPath on `/etc/kubedge`;
- LED ownership is fixed in each chart's config, and the 0.4.0 shared state file resolves collisions after
  the fact (the owner whose name sorts last wins);
- placement relies on hand-applied node labels (`blinktInstalled`).

Kubernetes Dynamic Resource Allocation (DRA, stable since v1.35) is built for exactly this: a driver
advertises devices per node, pods claim them, the scheduler allocates them without conflicts, and the
kubelet plugin hands each container only the device access it was granted.

## What Changes

- **New DRA driver `blinkt.kubedge.io`**, deployed as a DaemonSet kubelet plugin on the Pis. On each node it
  publishes a ResourceSlice with eight devices, one per LED, and prepares claims by injecting, through CDI,
  only:
  - `/dev/gpiochip0`;
  - the shared state directory;
  - the allocated pixel indices.
- **New DeviceClass `blinkt-pixel.kubedge.io`**, so workloads claim "pixel 6 on this node" (or "any free pixel")
  with a ResourceClaimTemplate. The scheduler then guarantees:
  - no two pods hold the same LED on a node;
  - a pod lands only on a node with that LED free.
- **The blinkt program honours an allocation**: when the claim provides allocated pixel indices, it lights
  exactly those, using the colour from its config. Without an allocation it behaves as today.
- **Follow-up chart changes** (kubesim_5gc, nr, epc, lte, elte): the blinkt sidecar drops `privileged: true`
  and the `/etc/kubedge` hostPath and gains a resource claim. **BREAKING** for those charts on clusters
  without DRA. That's why it needs a chart major/minor bump and a values switch to keep the legacy mode.
- **Prerequisite, owned by picluster-automation, not this repo:** both clusters must move from Kubernetes
  1.29.14 to at least 1.35, with CDI enabled in containerd.
- Not changing:
  - the LED wire protocol;
  - the shared-state mechanism, which is still needed because several pods draw on one strip;
  - `kubedgeNodeType` role placement, which DRA does not replace.

## Capabilities

### New Capabilities
- `blinkt-dra-driver`: the node-side DRA driver. ResourceSlice publication, claim preparation and
  unpreparation, CDI injection, and driver lifecycle on nodes with Blinkt! hardware.
- `blinkt-pixel-claims`: how workloads request and receive Blinkt! pixels through DRA. The DeviceClass, device
  attributes, exclusivity, and how the blinkt program consumes an allocation.

### Modified Capabilities
- None. The blinkt program's runtime capability is still being specified in the in-flight `port-to-rust`
  change and has no main spec yet. The allocation input is specified here under `blinkt-pixel-claims`.

## Impact

- **New code:** a driver binary (`cmd/blinkt-dra-driver` or a separate repo, see design.md). It uses
  `k8s.io/dynamic-resource-allocation/kubeletplugin`, `k8s.io/client-go` and the CDI spec format.
- **blinkt program:** reads the allocated pixel list, in Go or in Rust depending on whether `port-to-rust` has
  landed. Both changes touch the same env/config contract, so whichever lands second implements it.
- **Manifests:** driver DaemonSet with RBAC (ResourceSlices, ResourceClaims read), the DeviceClass, and example
  ResourceClaimTemplates.
- **Clusters:** Kubernetes ≥ 1.35 on piclustera and piclusterb, plus containerd with CDI enabled. Kubernetes
  1.35 is also the last release supporting containerd 1.x, so a later upgrade needs containerd 2.x.
- **Charts:** the five simulator charts gain an opt-in DRA mode.
