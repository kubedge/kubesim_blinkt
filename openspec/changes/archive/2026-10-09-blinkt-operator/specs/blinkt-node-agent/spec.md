# Spec Delta

## Purpose

The node component of the operator's `agent` mode: a DRA kubelet plugin for `blinkt.kubedge.io` that is the
only process driving the Blinkt! on its node, and that keeps the strip equal to what the ResourceClaims in the
API describe.

## ADDED Requirements

### Requirement: Same devices as the DaemonSet driver
The agent SHALL register as DRA driver `blinkt.kubedge.io`. On a node exposing GPIO23 and GPIO24, it SHALL publish one ResourceSlice with devices `pixel-0` … `pixel-7` and attributes `index` and `model`, identical to `dra-driver`, so that the same DeviceClass and claims work in either mode.

#### Scenario: Claims portable between modes
- **WHEN** a claim template written for `cdi` mode (`index == 6`) is used in `agent` mode
- **THEN** it is allocated `pixel-6` without change

### Requirement: Sole owner of the GPIO lines
The agent SHALL hold GPIO23 and GPIO24 for as long as it runs, and SHALL be the only process on its node that writes frames. Claiming pods SHALL receive no device node, mount or environment from the agent.

#### Scenario: Unprivileged pod without GPIO access lights its LED
- **WHEN** a pod with no securityContext and no volumes claims `index == 4` with `color: [0, 255, 0]`
- **THEN** pixel 4 turns green, and the pod's containers have no `/dev/gpiochip0`

### Requirement: Strip reconciled from ResourceClaims
The agent SHALL watch ResourceClaims and treat a claim as lit when it has an allocation result for driver `blinkt.kubedge.io` in this node's pool and a non-empty `status.reservedFor`. The drawn frame SHALL be the union of all lit claims' pixels with their effective PixelConfig. The agent SHALL redraw whenever that set changes.

#### Scenario: Pod deleted
- **WHEN** the only pod using a claim for pixel 6 is deleted
- **THEN** pixel 6 goes dark once the claim is no longer reserved, and other pixels are unchanged

#### Scenario: Two claims on one node
- **WHEN** claims for pixel 4 (green) and pixel 6 (blue) are both reserved on the node
- **THEN** both LEDs are lit, each with its own pattern

### Requirement: Prepare and unprepare are idempotent triggers
Prepare SHALL validate the claim's PixelConfig, trigger a redraw, and return the allocated devices with no CDI device IDs. Unprepare SHALL trigger a redraw and succeed. Neither SHALL create or delete files.

#### Scenario: Prepare called twice
- **WHEN** the kubelet prepares the same claim twice
- **THEN** both calls succeed and the strip is unchanged by the second

### Requirement: Restart recovery without local state
The agent SHALL keep no local checkpoint. After a restart it SHALL rebuild the frame from the API before drawing.

#### Scenario: Agent restarted while pods run
- **WHEN** the agent pod is deleted and recreated while claims for pixels 4 and 6 are reserved
- **THEN** within one reconcile period after its start, pixels 4 and 6 are lit again with their configurations

### Requirement: Dark strip on shutdown
On SIGTERM, the agent SHALL clear the strip, release the lines and stop serving the plugin.

#### Scenario: Mode switched away from agent
- **WHEN** the operator removes the agent DaemonSet
- **THEN** the strip on each node goes dark

### Requirement: Optional legacy compatibility
With legacy compatibility enabled, the agent SHALL instead act as one owner (`blinkt-node-agent`) in the shared-state protocol of `shared-led-state`: it publishes its pixels, honours the flock, and holds the lines only per frame. Legacy blinkt sidecars on the node then keep their LEDs.

#### Scenario: Mixed node during migration
- **WHEN** legacy compatibility is on, a legacy sidecar owns pixel 7, and a claim lights pixel 4
- **THEN** both pixels are lit
