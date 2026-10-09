# blinkt-pixel-claims Specification

## Summary
Workloads request LEDs through the DeviceClass `blinkt-pixel.kubedge.io`, either a specific pixel by its `index` attribute or any free one. The scheduler gives each pixel to at most one claim at a time, so a ninth claim on an eight-LED node stays Pending. The blinkt program lights exactly the pixels in `BLINKT_PIXELS` and exits on an invalid allocation.

## Purpose
How workloads request Blinkt! LEDs through DRA, how the scheduler places them, and how the blinkt program
lights the pixels it was actually allocated.

## Requirements

### Requirement: DeviceClass for Blinkt! pixels
A DeviceClass `blinkt-pixel.kubedge.io` SHALL select all devices published by driver `blinkt.kubedge.io`.

#### Scenario: Class lists node pixels
- **WHEN** the driver runs on three Pis
- **THEN** a claim against `blinkt-pixel.kubedge.io` can be satisfied by any of the 24 published devices

### Requirement: Workloads claim a specific pixel or any free pixel
A workload SHALL be able to claim a specific LED, with a CEL selector on the `index` attribute (e.g. `device.attributes["blinkt.kubedge.io"].index == 6`), or any free LED, with no selector.

#### Scenario: Fixed assignment, as the charts use today
- **WHEN** kubesim_lte's pod template claims one device with `index == 6`
- **THEN** it is allocated `pixel-6` on the node it is scheduled to

#### Scenario: Any free pixel
- **WHEN** a pod claims one device with no selector on a node where pixels 4 and 6 are taken
- **THEN** it is allocated one of the other six pixels

### Requirement: A pixel is allocated to at most one claim at a time
Two claims SHALL NOT hold the same pixel on the same node at the same time. A pod whose claim cannot be satisfied on a node SHALL NOT be scheduled there.

#### Scenario: Two pods want pixel 6 on a one-node cluster
- **WHEN** kubesim_lte holds `pixel-6` on the only node and kubesim_epc also claims `index == 6`
- **THEN** the kubesim_epc pod stays Pending with an unschedulable reason, instead of fighting over the LED

#### Scenario: Placement follows free pixels
- **WHEN** `pixel-6` is taken on node A and free on node B, and both match the pod's other constraints
- **THEN** the pod is scheduled to node B

### Requirement: The blinkt program lights exactly its allocation
When `BLINKT_PIXELS` is set, the blinkt program SHALL own exactly those pixel indices. Each one SHALL use the colour configured for that index if present, otherwise the first configured colour in index order. When `BLINKT_PIXELS` is unset, the program SHALL behave as without DRA.

#### Scenario: Allocation matches config
- **WHEN** the config sets `pixel6: [0, 0, 255]` and `BLINKT_PIXELS=6`
- **THEN** the program owns pixel 6 blue, exactly as today

#### Scenario: Allocated a different pixel
- **WHEN** the config sets only `pixel6: [0, 0, 255]` and the claim was allocated `BLINKT_PIXELS=2`
- **THEN** the program owns pixel 2 blue and does not touch pixel 6

#### Scenario: No allocation
- **WHEN** `BLINKT_PIXELS` is unset
- **THEN** the program owns the pixels configured in its config file, as before

### Requirement: Invalid allocations are fatal
If `BLINKT_PIXELS` is set but empty, or contains a value that is not an integer from 0 to 7, the blinkt program SHALL log `blinkt: invalid BLINKT_PIXELS: <value>` and exit with status 1.

#### Scenario: Out of range
- **WHEN** `BLINKT_PIXELS=9`
- **THEN** the program exits 1 with `blinkt: invalid BLINKT_PIXELS: 9`
