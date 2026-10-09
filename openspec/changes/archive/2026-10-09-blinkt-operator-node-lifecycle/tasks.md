# Tasks

## 1. Agent (`blinkt-node-lifecycle`)

- [x] 1.1 Add `--cdi-dir` (default `/var/run/cdi`) and stale cdi-spec removal in unprepare. Verify a unit test where a file is removed and a missing file is fine.
- [x] 1.2 Add `--clear`: exclusive mode draws a dark frame and releases; compat mode withdraws owner `blinkt-node-agent` and redraws the others; fake mode logs a dark frame; exit 0 without API access. Verify unit tests through the writers, and a smoke run of `--fake-gpio --clear`.

## 2. Operator (`blinkt-node-lifecycle`)

- [x] 2.1 Selector guard: dropped nodes from the old vs new selector, the claim pools check, keep the old selector, `NodeClaimsInUse`. Verify envtest: narrowing blocked by a claim, then applied after the claim is deleted.
- [x] 2.2 `status.agentNodes`, clear Jobs for nodes leaving the agent (selector, label, mode change) once no agent pod remains on the node, removal on Job success, Job RBAC. Verify envtest: unlabel, then the Job is pinned to the node with `--clear`; Job succeeded, then the node leaves status; agent→cdi, then Jobs for all agent nodes.
- [x] 2.3 Agent DaemonSet mounts `/var/run/cdi`. Manager manifest: `--leader-elect=false`, `strategy: Recreate`; re-render `dist/install.yaml`. Verify kubeconform and the rendered args.

## 3. Verify and document

- [x] 3.1 kind 1.36: (a) narrowing guard with a live claim; (b) cdi claim prepared, switch to agent, delete pod, CDI file gone; (c) kill -9 agent, unlabel node, clear Job logs a dark frame (`--fake-gpio`), node leaves `status.agentNodes`. Record in the README.
- [x] 3.2 Update the README (node lifecycle, drain-before-unlabel query) and `06-node-cleanup-check.md` expectations in the picluster kit. Verify `make operator-test operator-lint` and `openspec validate --strict`.
