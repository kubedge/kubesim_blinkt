# Design

## Context

See proposal.md, Why. The relevant facts, checked against source:
- The kubelet-plugin helper (k8s.io/dynamic-resource-allocation v0.36.5) removes its unix sockets on a clean stop,
  and removes stale ones before listening. It never deletes ResourceSlices.
- The kubelet (release-1.36, `pkg/kubelet/cm/dra`) wipes a driver's slices on the node `defaultWipingDelay = 30s`
  after the driver becomes unusable, and wipes all of them at kubelet start.
- The kernel frees GPIO line requests on any process exit. The APA102 keeps its last frame with no power-off
  default.

## Decisions

### D1. The selector guard keeps the DaemonSet's selector, not the BlinktConfig's
On each reconcile, the operator compares the existing DaemonSet's `spec.template.spec.nodeSelector` with the
desired selector. It lists the nodes matching the old selector but not the new one, and checks
`status.allocation.devices.results[].pool` of blinkt claims against them. If any match, the DaemonSet is applied
with the old selector; everything else, such as images, still updates. The user's `BlinktConfig` is never
rewritten.
- Why: one source of truth (the spec), with the delay visible in status. No extra fields are needed, because the
  DaemonSet already records what is applied.
- Toleration changes that drop tainted nodes are not guarded; nodeSelector is the supported way to choose nodes.

### D2. Node set from the API, not from pods
`status.agentNodes` is the sorted list of Node names matching the agent DaemonSet's applied selector (pods may be
briefly absent during rollouts). A node leaves when it is in `status.agentNodes` but no longer matches, or when
the mode is no longer `agent`. The clear Job is created only once no agent pod is on that node (`spec.nodeName`
field selector), so it never competes with a running agent for the lines.

### D3. The clear Job reuses the agent image and its flags
Job `blinkt-clear-<node>` in the operator namespace has `nodeName: <node>`, the agent's privileged root container
and `/dev` + `/etc/kubedge` mounts, args `--clear` plus the agent's `--legacy-compat` / extra args,
`backoffLimit: 3` and `ttlSecondsAfterFinished: 600`.
- `--clear` acquires the lines with the usual EBUSY retry, draws, releases and exits 0. It needs no API access,
  so it uses no ServiceAccount token (`automountServiceAccountToken: false`).
- The node leaves `status.agentNodes` when its Job succeeds. A failed Job keeps the node listed and is retried by
  re-creation on the next reconcile.

### D4. Stale CDI removal is best effort and idempotent
The agent DaemonSet mounts host `/var/run/cdi`. Unprepare calls `os.Remove` on the cdi-mode file name and
ignores "not exist". Prepare doesn't touch CDI; agent mode never creates specs.

### D5. Leader election off by default in the rendered manifest
`config/manager/manager.yaml` gets `args: [--leader-elect=false]` and `strategy: Recreate`. The binary's flag
default stays `true` (library convention); the manifest decides.

## Risks / Trade-offs

- [A node is unlabelled with claims still on it (unguardable)] → its claims stay allocated. The clear Job darkens
  the agent's pixels; the README requires draining claims first and gives the query.
- [A clear Job races with a cdi-mode sidecar starting on that node] → in exclusive mode the Job clears all 8
  pixels once. Sidecars redraw on their next blink (≤ 1 s); with legacyCompat only the agent's entry is
  withdrawn.
- [The operator restarts while a node is leaving] → `status.agentNodes` persists in the API, so the next
  reconcile resumes.
