# blinkt-dra-driver Specification

## Purpose
A Kubernetes DRA kubelet-plugin driver that advertises each node's Blinkt! LEDs as allocatable devices and
gives every allocated claim exactly the device access it needs, with no privileged containers.

## Requirements

### Requirement: Driver identity
The driver SHALL register with the kubelet under driver name `blinkt.kubedge.io`. It SHALL use the stable `resource.k8s.io/v1` API, and SHALL NOT start against an API server that lacks it.

#### Scenario: Cluster without DRA v1
- **WHEN** the driver starts on a cluster whose API server does not serve `resource.k8s.io/v1`
- **THEN** it exits with an error naming the missing API, and publishes nothing

### Requirement: One device per LED in the node's ResourceSlice
On a node where the GPIO character device exposes the Blinkt! data and clock lines (BCM GPIO23 and GPIO24), the driver SHALL publish one ResourceSlice for that node. It SHALL contain eight devices named `pixel-0` to `pixel-7`, each with attribute `index` (int, 0 to 7) and attribute `model` (string `pimoroni-blinkt`).

#### Scenario: Pi with Blinkt! lines
- **WHEN** the driver runs on a Pi 3 whose `gpiochip0` has lines `GPIO23` and `GPIO24`
- **THEN** `kubectl get resourceslices` shows one slice for that node with devices `pixel-0` … `pixel-7` and their `index` attributes

#### Scenario: Node without the GPIO lines
- **WHEN** the driver runs on a node where neither line can be found
- **THEN** it publishes no devices for that node and logs why

### Requirement: Claims are prepared through CDI
When the kubelet asks it to prepare a claim allocated on its node, the driver SHALL write a CDI spec and return the matching CDI device IDs. Containers using the claim then get:
- the device node `/dev/gpiochip0`;
- the node's shared LED state directory mounted read-write at `/run/blinkt`;
- `BLINKT_STATE_DIR=/run/blinkt`;
- `BLINKT_PIXELS`, set to the claim's allocated pixel indices on this node, ascending and comma-separated.

#### Scenario: Pod claims pixel 6
- **WHEN** a pod's claim is allocated `pixel-6` on node home-pi and the pod starts there
- **THEN** its container sees `/dev/gpiochip0`, `/run/blinkt`, `BLINKT_STATE_DIR=/run/blinkt` and `BLINKT_PIXELS=6`, without `privileged: true`

#### Scenario: Pod claims all pixels
- **WHEN** a claim requests all devices of the class on a node and is allocated `pixel-0` … `pixel-7`
- **THEN** the container sees `BLINKT_PIXELS=0,1,2,3,4,5,6,7`

### Requirement: The shared state directory is configurable and compatible
The host directory backing `/run/blinkt` SHALL be a driver setting, defaulting to `/etc/kubedge`. With the default, pods using DRA and legacy sidecars (hostPath `/etc/kubedge`) on the same node share one lock and state file.

#### Scenario: Mixed legacy and DRA pods
- **WHEN** a legacy privileged sidecar owns pixel 4 and a DRA-claimed pod owns pixel 6 on the same node
- **THEN** both LEDs stay lit

### Requirement: Unprepare removes what prepare created
When the kubelet asks it to unprepare a claim, the driver SHALL remove that claim's CDI spec. Both prepare and unprepare SHALL be idempotent, and SHALL survive a driver restart between them.

#### Scenario: Pod deleted
- **WHEN** the last pod using a claim is deleted and the kubelet unprepares the claim
- **THEN** the claim's CDI spec file is gone from the node

#### Scenario: Driver restarted mid-lifecycle
- **WHEN** the driver restarts after preparing a claim and is then asked to prepare it again or unprepare it
- **THEN** both calls succeed without duplicating or leaking CDI specs

### Requirement: The driver itself is the only privileged component
The driver DaemonSet SHALL be the only component needing host access: kubelet plugin and registration sockets, the CDI spec directory, and read access to `/dev/gpiochip0` for line discovery. Workload containers using claims SHALL NOT need `privileged`, host devices or hostPath volumes.

#### Scenario: Simulator chart in DRA mode
- **WHEN** a simulator chart is rendered with DRA mode enabled
- **THEN** its blinkt sidecar has no `privileged` security context and no hostPath volume
