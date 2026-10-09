# Proposal

## Why

Running `blinkt-operator` on piclustera and piclusterb (2026-10-09) and reviewing node cleanup against the
code, the DRA helper and kubelet 1.36 sources showed four problems:

- **A. Orphaned pixels.** Narrowing `BlinktConfig.spec.nodeSelector` removes the node component from nodes that
  still hold allocated blinkt claims. In agent mode their simulators' LEDs go dark while the pods run on. In cdi
  mode their claims can no longer be unprepared.
- **B. Leaked CDI specs.** A claim prepared in cdi mode leaves `/var/run/cdi/blinkt.kubedge.io-<uid>.json`
  behind if the node switches to agent mode before the claim is unprepared, because the agent's unprepare does
  not know about that file.
- **C. Frozen LEDs.** If the agent dies without its SIGTERM handler (kill -9, OOM) and the node then leaves the
  agent's node set, the APA102 keeps its last frame forever: nothing runs there to clear it.
- **D. Leader-election crash loop.** On a loaded Pi 3 control plane, the single-replica manager lost its lease
  whenever the API server stalled, and exited. The kit was patched by hand (`--leader-elect=false`,
  `strategy: Recreate`), but `dist/install.yaml` still renders the failing default.

## What Changes

- **Guard for selector narrowing (A):** when a new `spec.nodeSelector` would drop nodes whose blinkt claims are
  still allocated, the operator keeps the DaemonSet's current selector. It sets `Ready=False` with reason
  `NodeClaimsInUse`, naming the nodes and claims, until those claims are gone. Removing a node's *label* cannot
  be intercepted (the DaemonSet controller acts on it directly); the docs require draining blinkt claims first.
- **Stale CDI cleanup (B):** the node agent's unprepare also deletes a cdi-mode spec file for that claim if one
  exists. The agent DaemonSet mounts `/var/run/cdi` for this.
- **LED clear when a node leaves (C):** the operator records the nodes the agent covers. When a node leaves that
  set (selector change, label removed, or mode leaving `agent`), and no agent pod remains there, the operator
  runs a one-shot Job pinned to that node. The Job runs `/node-agent --clear`, which withdraws the agent's pixels
  and leaves the rest of the strip to its other owners.
- **Single-replica default (D):** the manager Deployment renders with `--leader-elect=false` and
  `strategy: Recreate`. Leader election stays available through the flag for anyone running more than one
  replica.
- **Unchanged:** the published images' names, the DRA driver (`dra-driver/`), the blinkt implementations, and
  the `blinkt-operator` change's existing behaviour.

## Capabilities

### New Capabilities
- `blinkt-node-lifecycle`: what happens on a node when it joins or leaves the set the operator's node component
  covers, or when that component stops: no orphaned pixels, no leaked files, no frozen LEDs. Also how the
  manager survives an unresponsive API server.

### Modified Capabilities
- None. `blinkt-operator` and `blinkt-node-agent` are still in flight in the `blinkt-operator` change, so these
  rules are added as a new capability rather than as deltas to specs that are not yet main specs.

## Impact

- `blinkt-operator/`:
  - `internal/controller`: selector guard, node tracking, clear Jobs, plus the Job RBAC;
  - `internal/agent` and `cmd/node-agent`: stale-CDI removal, `--clear`, `--cdi-dir`;
  - `api/v1alpha1`: `status.agentNodes`;
  - `config/manager`: leader election off, Recreate;
  - `dist/install.yaml` re-rendered; README.
- Release: a new operator image is needed to ship B and C. The version is set at release time.
