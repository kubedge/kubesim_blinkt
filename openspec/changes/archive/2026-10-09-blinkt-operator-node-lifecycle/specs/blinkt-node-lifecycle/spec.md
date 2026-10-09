# Spec Delta

## Purpose

Keeps each node consistent when it joins or leaves the set covered by the blinkt-operator's node component, or
when that component stops: no orphaned pixels, no leaked CDI files, no frozen LEDs. Also keeps the operator
running when the API server is slow.

## ADDED Requirements

### Requirement: Selector narrowing waits for claims
When a change to `BlinktConfig.spec.nodeSelector` would stop the active node component on nodes that hold allocated `blinkt.kubedge.io` claims, the operator SHALL keep the DaemonSet's current node selector. It SHALL report `Ready=False` with reason `NodeClaimsInUse`, naming those nodes and claims. It SHALL apply the new selector once none of the dropped nodes holds a claim.

#### Scenario: Narrowing with a claim on the dropped node
- **WHEN** the agent DaemonSet runs on home-pi and nas-pi, a claim is allocated on nas-pi, and `spec.nodeSelector` is changed so that it matches only home-pi
- **THEN** the DaemonSet's node selector is unchanged, and the condition reads `NodeClaimsInUse` naming nas-pi and the claim

#### Scenario: Claim released
- **WHEN** the claim on nas-pi is deleted
- **THEN** the DaemonSet takes the new selector, and its pod leaves nas-pi

### Requirement: The agent removes stale cdi-mode specs
When the node agent unprepares a claim, it SHALL delete `<cdi-dir>/blinkt.kubedge.io-<claim-uid>.json` if that file exists, and SHALL succeed if it does not.

#### Scenario: Claim prepared in cdi mode, unprepared in agent mode
- **WHEN** a claim was prepared by `dra-driver` (CDI file present), the mode switched to `agent`, and the kubelet then unprepares the claim through the agent
- **THEN** the CDI file is gone

### Requirement: LEDs cleared when a node leaves the agent
The operator SHALL record in `status.agentNodes` the nodes the agent DaemonSet covers. When a recorded node is no longer covered and no agent pod remains on it, the operator SHALL run a one-shot Job pinned to that node that executes `/node-agent --clear`, then drop the node from `status.agentNodes`.
- Without legacy compatibility, `--clear` SHALL write a dark frame and release the lines.
- With legacy compatibility, `--clear` SHALL withdraw only the agent's pixels from the shared state and redraw the rest.

#### Scenario: Agent killed, then the node unlabelled
- **WHEN** the agent on nas-pi is killed with SIGKILL (the strip keeps its last frame) and nas-pi then loses `blinktInstalled=true`
- **THEN** a Job runs `/node-agent --clear` on nas-pi, the strip goes dark, and nas-pi leaves `status.agentNodes`

#### Scenario: Leaving agent mode
- **WHEN** the mode changes from `agent` to `cdi` or `legacy`
- **THEN** every node in `status.agentNodes` gets a clear Job after its agent pod is gone, and `status.agentNodes` ends empty

### Requirement: Single-replica manager without leader election
The rendered install manifest SHALL run the manager as one replica, with `--leader-elect=false` and Deployment strategy `Recreate`, so that an unresponsive API server delays reconciles instead of stopping the manager. Leader election SHALL remain available through `--leader-elect=true`.

#### Scenario: API server stalls
- **WHEN** the API server does not answer for longer than a lease renew deadline
- **THEN** the manager keeps running, and reconciles once the API server answers again
