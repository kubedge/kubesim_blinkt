# Tasks

## 1. Scaffold

- [x] 1.1 Create `blinkt-operator/` as its own Go module (kubebuilder layout: `cmd/manager`, `cmd/node-agent`, `api/v1alpha1`, `internal/{controller,agent,pixelconfig}`, `config/`) on controller-runtime v0.24.x / Kubernetes v0.36.x, with `replace` to `../go-blinkt`. Verify `go build ./...` and `go vet ./...` pass.
- [x] 1.2 Define the `BlinktConfig` types (mode, nodeSelector, tolerations, images, defaults, legacyCompat; status mode/nodes/devices/conditions), generate the CRD and DeepCopy with controller-gen, and add Makefile targets `operator-test`, `operator-lint`, `operator-manifests`. Verify `make operator-manifests` produces a CRD that kubeconform accepts for 1.36.5.

## 2. PixelConfig (`blinkt-pixel-config`)

- [x] 2.1 Implement strict decoding, defaults, FromClass→FromClaim field-by-field merge filtered by request, range validation with field-naming errors, and the pattern phase function (`steady`, `fixed5`, `fixed`). Verify with unit tests for every `blinkt-pixel-config` scenario.

## 3. Node agent (`blinkt-node-agent`)

- [x] 3.1 Start-up: acquire GPIO23/24 via `go-blinkt/pkg/periBlink` (or, with `--legacy-compat`, a `ledstate.Board` as owner `blinkt-node-agent`), register as `blinkt.kubedge.io`, and publish `pixel-0..7` (or 0 devices with a reason). Log `blinkt-agent: registered …` and `blinkt-agent: published devices=… node=…`. Verify with unit tests over a fake line source.
- [x] 3.2 ResourceClaim informer and desired-frame computation (driver/pool filter, `reservedFor`, effective config per result). Verify with unit tests: pod deleted, two claims, other node, other driver, config precedence.
- [x] 3.3 Prepare (validate, enqueue, no CDI IDs) and unprepare (enqueue); health stream; 10 ms render loop drawing only on change; dark frame and release on SIGTERM. Verify with unit tests using a recording frame writer, including prepare-twice idempotency and the shutdown frame.
- [x] 3.4 Add `--fake-gpio`, which logs frames instead of writing GPIO (test clusters only). Verify the frame log lines in a unit test.

## 4. Operator (`blinkt-operator`)

- [x] 4.1 Reconcile `BlinktConfig/cluster`: ignore other names (`IgnoredNotSingleton`); ensure the DeviceClass with `defaults` as class config; DaemonSet per mode, built from embedded templates (the cdi one equal to `dra-driver/deploy`); owner references and the managed-by label; drift repair. Verify with envtest tests per mode.
- [x] 4.2 Mode switching state machine, `ClaimsInUse` guard, `ForeignDriver` detection, and status (mode, nodes desired/ready, devices from ResourceSlices, `Ready` condition). Verify with envtest tests for cdi→agent ordering, legacy with claims, a foreign DaemonSet, and status counts.

## 5. Packaging

- [x] 5.1 Dockerfile (native build, cross-compiled, scratch image with `/manager` and `/node-agent`), and `make operator-buildx(-check)` for `kubedge1/blinkt-operator`. Verify the multi-arch check build and an arm64 smoke run of both binaries' `--help`.
- [x] 5.2 Render `blinkt-operator/dist/install.yaml` (CRD, namespace `blinkt-system`, RBAC, manager Deployment with leader election, arm64 + control-plane toleration, 96 Mi limit; agent and driver ServiceAccounts/RBAC) and an example `BlinktConfig`. Verify kubeconform for 1.36.5, and document it in `blinkt-operator/README.md`.

## 6. kind end-to-end (Kubernetes 1.36.x)

- [x] 6.1 Install the operator; in `legacy`, nothing is deployed. Switch to `cdi`: the driver DaemonSet and DeviceClass appear and the earlier `dra-driver` claim checks still pass. Switch to `agent` (`--fake-gpio`): the driver pods are gone before agent pods start; an unprivileged pod with no volumes claiming `index == 4` + green gives a frame log with pixel 4 green; deleting the pod darkens it. Record the commands and output in the README.
- [x] 6.2 Verify the guards: `legacy` with live claims gives `ClaimsInUse`; a hand-applied driver gives `ForeignDriver`; agent restart recovery redraws from the API. Record the agent and manager memory peaks.

## 7. Hardware (after picluster's 1.36.5 upgrade)

- [x] 7.1 On piclustera, operator in `agent` mode. Two claims (lte pixel 6 blue, elte pixel 4 green) light correctly, and deleting a pod darkens only its LED (operator confirmation). **Done 2026-10-09:** piclusterb kube-node02 (and piclustera home-pi): lte pixel 6 blue blinking + elte pixel 4 green; pods had no /dev/gpiochip0; deleting elte darkened only pixel 4 (operator confirmed).
- [x] 7.2 Switch `agent` → `cdi` → `agent` on the live cluster with claims present. Verify the LEDs recover and no two drivers ran on a node. **Done 2026-10-09:** piclusterb with lte live: agent→cdi (pixel 6 dark by design) →agent (blinking again); driver pods Killing before agent pods created on every node; converged even with an overlapping second patch.
- [x] 7.3 With `legacyCompat: true`, a legacy 0.5.x sidecar and an agent-mode claim on one Pi both stay lit. **Done 2026-10-09:** piclusterb kube-node02: legacy 0.5.2 sidecar pixel 7 red + agent pixel 6 blinking, both lit; state file owners legacy-px7 and blinkt-node-agent; GPIO23/24 free between frames.

## 8. Simulator charts (follow-up PRs in kubesim_5gc, nr, epc, lte, elte)

- [x] 8.1 Add `blinkt.mode: agent`: render a ResourceClaimTemplate with the chart's pixel selector and PixelConfig (colour/intensity/algorithm from values), `resourceClaims` on the pod, no blinkt sidecar, and `strategy: Recreate`. Verify `helm template` in every mode, with legacy unchanged byte-for-byte. **Done 2026-10-09:** kubesim_5gc#6, kubesim_nr#8, kubesim_epc#6, kubesim_lte#9, kubesim_elte#9: `blinkt.mode` agent (default) | legacy (byte-identical); template name hashed on `.Values.blinkt` (spec immutable); kind: lte+elte charts drawn, uninstall darkens only its pixel, recolour upgrade redraws.
