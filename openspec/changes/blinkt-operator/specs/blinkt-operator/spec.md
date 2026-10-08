# Spec Delta

## Purpose

A cluster operator that controls how the Blinkt! is run on a cluster: legacy sidecars, the DaemonSet DRA
driver (CDI hand-off), or the sole-writer node agent. It is selected and reported through one custom resource.

## ADDED Requirements

### Requirement: BlinktConfig resource
The operator SHALL serve a cluster-scoped custom resource `BlinktConfig` (`blinkt.kubedge.io/v1alpha1`) and act only on the instance named `cluster`. Its spec SHALL include:
- `mode`: `legacy`, `cdi` or `agent`, default `legacy`;
- `nodeSelector` and `tolerations` for the node components;
- `images`: the driver and agent image references;
- `defaults`: a PixelConfig used as the DeviceClass configuration;
- `legacyCompat`: boolean.

#### Scenario: Other instances ignored
- **WHEN** a `BlinktConfig` named `test` is created
- **THEN** the operator sets its `Ready` condition to `False` with reason `IgnoredNotSingleton`, and changes nothing

### Requirement: Components per mode
For mode `cdi`, the operator SHALL run the `dra-driver` DaemonSet. For mode `agent`, it SHALL run the node-agent DaemonSet. In both modes it SHALL ensure DeviceClass `blinkt-pixel.kubedge.io`, carrying `defaults` as class configuration. For mode `legacy`, it SHALL run neither DaemonSet. It SHALL own and re-create these objects if they are changed or deleted.

#### Scenario: Selecting agent mode
- **WHEN** `spec.mode` is set to `agent`
- **THEN** the node-agent DaemonSet and the DeviceClass exist, and no `dra-driver` DaemonSet runs

#### Scenario: Drift repaired
- **WHEN** someone deletes the managed DaemonSet
- **THEN** the operator re-creates it

### Requirement: One driver per node
The operator SHALL never let the `dra-driver` and node-agent DaemonSets run pods on the same node at the same time. Both register `blinkt.kubedge.io`. When switching between `cdi` and `agent`, it SHALL remove the old DaemonSet and wait until its pods are gone before creating the new one.

#### Scenario: Switching cdi to agent
- **WHEN** `spec.mode` changes from `cdi` to `agent`
- **THEN** the driver pods terminate before any agent pod is created, and the status reports the switch in progress until the agent is ready

#### Scenario: Hand-applied driver already present
- **WHEN** mode is `cdi` or `agent` and a DaemonSet labelled `app.kubernetes.io/name=blinkt-dra-driver` that the operator does not own exists
- **THEN** the operator creates no DaemonSet of its own, and sets `Ready=False` with reason `ForeignDriver`, naming that DaemonSet

### Requirement: No removal under live claims
The operator SHALL NOT remove the DRA components (switching to `legacy`, or deleting the BlinktConfig) while ResourceClaims allocated by `blinkt.kubedge.io` exist. It SHALL keep them running and report the blocking claims instead.

#### Scenario: Switch to legacy with claims present
- **WHEN** `spec.mode` changes to `legacy` while two blinkt claims are allocated
- **THEN** the components stay, and the `Ready` condition is `False` with reason `ClaimsInUse`, naming the claims

### Requirement: Status
The operator SHALL report:
- `status.mode` (the mode in effect);
- `status.nodes.desired` and `status.nodes.ready` for the active DaemonSet;
- `status.devices` (the total of published blinkt devices);
- a `Ready` condition that is `True` only when the active components are fully rolled out.

#### Scenario: Agent mode on piclustera
- **WHEN** mode is `agent` on a three-Pi cluster with all agents running
- **THEN** status shows `mode: agent`, `nodes: {desired: 3, ready: 3}`, `devices: 24`, and `Ready=True`

### Requirement: DaemonSet approach still usable without the operator
The plain-YAML deployment of `dra-driver` SHALL keep working without the operator installed. The operator SHALL NOT modify objects it did not create.

#### Scenario: Hand-applied driver
- **WHEN** `dra-driver/deploy/blinkt-dra-driver.yaml` is applied on a cluster without the operator
- **THEN** the driver works as specified in `dra-blinkt-driver`
