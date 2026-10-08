# Tasks

## 1. Prerequisites

- [ ] 1.1 Track picluster-automation's planned upgrade changes (Kubernetes 1.36.5, containerd 2.x with CDI, piclustera first). It is not done in this repo. Verify picluster reports `resource.k8s.io/v1` served and dra-example-driver passing on piclustera.
- [x] 1.2 Stand up a local test cluster (kind node image 1.36.x, CDI enabled) for driver development. Verify `kubectl api-resources | grep resource.k8s.io` lists `deviceclasses`, `resourceclaims` and `resourceslices` at `v1`.

## 2. Driver skeleton (`blinkt-dra-driver`)

- [x] 2.1 Create `dra-driver/` as its own Go module, starting from `kubernetes-sigs/dra-example-driver`'s structure and using `k8s.io/dynamic-resource-allocation/kubeletplugin`. Verify `go build ./...` and `go vet ./...` pass in `dra-driver/`.
- [x] 2.2 Log the success lines picluster verifies on: `blinkt-dra: registered driver=blinkt.kubedge.io node=<node>` and `blinkt-dra: published devices=8 node=<node>` (or `devices=0 reason=<why>`). Implement start-up: the `resource.k8s.io/v1` presence check, kubelet registration as `blinkt.kubedge.io`, and line discovery (names `GPIO23`/`GPIO24`, `gpiochip0` fallback, read-only). Verify with unit tests over a fake chip listing, and a start-up failure test for a missing API.
- [x] 2.3 Publish the node ResourceSlice with `pixel-0`…`pixel-7` (`index`, `model` attributes), or nothing when the lines are absent. Verify in kind that `kubectl get resourceslices -o yaml` shows 8 devices per node.

## 3. Claim preparation (`blinkt-dra-driver`)

- [x] 3.1 Implement `PrepareResourceClaims`: write the per-claim CDI spec (device node, state-dir mount at `/run/blinkt`, `BLINKT_STATE_DIR`, `BLINKT_PIXELS` ascending) and return the CDI ID. Verify with unit tests on the generated CDI JSON for one-pixel and all-pixel claims.
- [x] 3.2 Implement `UnprepareResourceClaims` and restart idempotency. Verify unit tests cover prepare twice, unprepare twice, and unprepare after restart with no leaked files.
- [x] 3.3 Add the configurable host state directory (default `/etc/kubedge`). Verify a test that the CDI mount source follows the setting.

## 4. Cluster objects and packaging (`blinkt-pixel-claims`)

- [x] 4.1 Add plain-YAML manifests: the DeviceClass `blinkt-pixel.kubedge.io`, driver RBAC, the driver DaemonSet, and example ResourceClaimTemplates (fixed `index == 6`, any free). The DaemonSet uses an arm64 nodeSelector, the control-plane toleration, host mounts for `/var/lib/kubelet/plugins`, `/var/lib/kubelet/plugins_registry`, `/var/run/cdi` and `/dev`, and a memory limit. Verify `kubeconform -strict -kubernetes-version 1.36.5`.
- [ ] 4.2 Build and publish the multi-arch `kubedge1/blinkt-dra-driver` image via buildx. Verify `imagetools inspect` shows linux/arm64 and linux/amd64.
- [x] 4.3 In kind, verify exclusivity and placement: two pods claiming `index == 6` on one node leave the second Pending, and an any-free claim gets a different pixel. Record the commands and output in `dra-driver/README.md`.

## 5. Blinkt program allocation input (`blinkt-pixel-claims`)

- [x] 5.1 Implement `BLINKT_PIXELS` in the blinkt program, in both the Go and Rust implementations (they live side by side): owned indices, colour from the same index or the first configured colour, fatal `blinkt: invalid BLINKT_PIXELS: <v>`. Verify unit tests for each `blinkt-pixel-claims` program scenario, and that unset `BLINKT_PIXELS` keeps current behaviour.
- [x] 5.2 Document `BLINKT_PIXELS` and `/run/blinkt` in README.md. Verify the documented example runs.

## 6. Simulator charts (follow-up PRs in kubesim_5gc, nr, epc, lte, elte)

- [ ] 6.1 Add the `blinkt.mode: legacy|dra` value. `dra` renders the ResourceClaimTemplate with the chart's pixel selector and `resourceClaims`, and drops `privileged` and the hostPath from the sidecar. Verify with `helm template` in both modes (legacy unchanged byte-for-byte) and `kubeconform`.

## 7. Hardware rollout (after 1.1 is done)

- [ ] 7.1 Deploy the driver on piclustera and verify 8 devices per Pi in its ResourceSlices.
- [ ] 7.2 Deploy kubesim_lte and kubesim_elte in DRA mode on one node. Verify that:
  - both sidecars run unprivileged;
  - pixels 4 and 6 are lit (operator confirmation);
  - deleting one pod turns off only its LED.
- [ ] 7.3 Verify the mixed mode: one legacy 0.4.x sidecar and one DRA pod on the same node both keep their LEDs lit.
