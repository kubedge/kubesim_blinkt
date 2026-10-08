# blinkt-dra-driver

Kubernetes DRA driver `blinkt.kubedge.io` for the Pimoroni Blinkt! on Raspberry Pi nodes. It runs as a
DaemonSet kubelet plugin and does three things:

- publishes each node's eight LEDs as devices `pixel-0` … `pixel-7` (attributes `index`, `model`), under
  DeviceClass `blinkt-pixel.kubedge.io`;
- lets workloads claim a specific pixel or any free one. The scheduler never gives the same pixel to two
  claims on a node;
- prepares each claim through a CDI spec that injects `/dev/gpiochip0`, the shared LED state directory at
  `/run/blinkt`, `BLINKT_STATE_DIR=/run/blinkt` and `BLINKT_PIXELS=<allocated indices>`.

Claiming containers therefore need no `privileged` and no hostPath. The driver never requests the GPIO
lines: the workload's blinkt process draws only its allocated pixels, through the shared state that legacy
sidecars use too.

Requirements: Kubernetes ≥ 1.34 (`resource.k8s.io/v1`; DRA is stable from 1.35) and containerd with CDI
enabled. containerd 2.x enables it by default, with spec dirs `/etc/cdi` and `/var/run/cdi`.

## Deploy

```sh
kubectl apply -f deploy/blinkt-dra-driver.yaml   # namespace blinkt-dra, RBAC, DeviceClass, DaemonSet
kubectl apply -f deploy/examples.yaml            # claim templates (pixel 6 / any) + an example pod
```

Each driver pod logs:

```
blinkt-dra: registered driver=blinkt.kubedge.io node=<node>
blinkt-dra: published devices=8 node=<node> chip=gpiochip0
```

…or `published devices=0 node=<node> reason=<why>` on a node without GPIO23/GPIO24.

Claim a specific LED with a CEL selector on `index`, or any free LED with no selector:

```yaml
requests:
  - name: led
    exactly:
      deviceClassName: blinkt-pixel.kubedge.io
      selectors:
        - cel:
            expression: device.attributes["blinkt.kubedge.io"].index == 6
```

## Flags

| Flag | Default | |
|---|---|---|
| `--node-name` | `$NODE_NAME` | node served, also the pool name |
| `--state-dir` | `/etc/kubedge` | host dir bind-mounted at `/run/blinkt`; the default is shared with legacy sidecars |
| `--cdi-dir` | `/var/run/cdi` | per-claim spec files `blinkt.kubedge.io-<claim-uid>.json` |
| `--gpio-device` | `/dev/gpiochip0` | device node injected into claiming containers |
| `--assume-gpio` | `false` | publish pixels without GPIO discovery (test clusters only) |

## Develop

```sh
make dra-test dra-lint        # from the repo root
make dra-buildx-check         # linux/arm64 + linux/amd64 image, no push
make dra-buildx               # push kubedge1/blinkt-dra-driver:<version> + :latest
```

## Verified on kind (2026-10-08)

kind v0.33.0, `kindest/node:v1.36.4` (containerd 2.3.4, CDI on by default). There is no GPIO in kind, so
the driver ran with `--assume-gpio --gpio-device=/dev/null`.

```console
$ kind create cluster --name blinkt-dra --image kindest/node:v1.36.4
$ kind load docker-image kubedge1/blinkt-dra-driver:dev --name blinkt-dra
$ kubectl apply -f driver-kind.yaml     # deploy/blinkt-dra-driver.yaml + dev image + the two test flags
$ kubectl -n blinkt-dra logs ds/blinkt-dra-driver
blinkt-dra: registered driver=blinkt.kubedge.io node=blinkt-dra-control-plane
blinkt-dra: published devices=8 node=blinkt-dra-control-plane chip=assumed (--assume-gpio)
$ kubectl get resourceslices -o custom-columns=NODE:.spec.nodeName,DRIVER:.spec.driver,DEVICES:.spec.devices[*].name
NODE                       DRIVER              DEVICES
blinkt-dra-control-plane   blinkt.kubedge.io   pixel-0,pixel-1,pixel-2,pixel-3,pixel-4,pixel-5,pixel-6,pixel-7
```

Three unprivileged busybox pods with no hostPath, two claiming `index == 6` and one claiming any pixel:

```console
$ kubectl get resourceclaims -o custom-columns=CLAIM:.metadata.name,DEVICE:.status.allocation.devices.results[*].device
CLAIM             DEVICE
any-c-led-jl8fc   pixel-0
epc-b-led-l8j62   <none>          # pod Pending: "1 cannot allocate all claims"
lte-a-led-kvndg   pixel-6
$ kubectl logs lte-a                # env, mount, write to host state dir, device node
BLINKT_PIXELS=6 BLINKT_STATE_DIR=/run/blinkt
mount=/run/blinkt
state-dir-writable
crw-rw-rw-
$ kubectl delete pod lte-a          # unprepare removes its CDI spec; epc-b now gets pixel 6
$ kubectl logs epc-b
BLINKT_PIXELS=6
```

The driver's resident memory peaked at 15.8 MB, under the DaemonSet's 64 Mi limit.

Pods scheduled while the driver pod itself restarts fail with "DRA driver … is not registered". The kubelet
retries them on its next pod sync.
