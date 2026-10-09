# blinkt-operator

Controls how a cluster runs its Pimoroni Blinkt! LEDs, through one custom resource:

```yaml
apiVersion: blinkt.kubedge.io/v1alpha1
kind: BlinktConfig
metadata:
  name: cluster          # the only instance acted on
spec:
  mode: agent            # legacy | cdi | agent
  legacyCompat: true     # agent shares the strip with legacy sidecars while migrating
  defaults:              # DeviceClass PixelConfig; claims override it field by field
    intensity: 5
    algorithm: fixed5
```

| Mode | Node component | Who writes GPIO23/24 | A simulator pod needs |
|---|---|---|---|
| `legacy` | none | each privileged blinkt sidecar | today's charts |
| `cdi` | `dra-driver` DaemonSet (the DaemonSet approach) | each sidecar, handed the device via CDI | sidecar + ResourceClaim |
| `agent` | node agent DaemonSet (`/node-agent`) | **only the agent** | **just a ResourceClaim** with a `PixelConfig` |

The operator:
- owns DeviceClass `blinkt-pixel.kubedge.io`, carrying `defaults` as its configuration, and the selected mode's
  DaemonSet in `blinkt-system`, and repairs either if it drifts;
- never runs two `blinkt.kubedge.io` drivers on a node: a mode switch removes the old pods before creating the
  new ones;
- keeps the DRA components while blinkt claims exist (`ClaimsInUse`), including when the BlinktConfig is
  deleted;
- steps aside when it finds a hand-applied driver (`ForeignDriver`).

The plain-YAML DaemonSet approach in `../dra-driver/deploy` still works without the operator.

## Agent mode: the claim is the whole LED

```yaml
apiVersion: resource.k8s.io/v1
kind: ResourceClaimTemplate
metadata: {name: lte-led}
spec:
  spec:
    devices:
      requests:
        - name: led
          exactly:
            deviceClassName: blinkt-pixel.kubedge.io
            selectors: [{cel: {expression: 'device.attributes["blinkt.kubedge.io"].index == 6'}}]
      config:
        - requests: [led]
          opaque:
            driver: blinkt.kubedge.io
            parameters:
              apiVersion: blinkt.kubedge.io/v1alpha1
              kind: PixelConfig
              color: [0, 0, 255]      # 0-255 each; default white
              intensity: 5            # 0-31; default 5
              algorithm: fixed5       # steady | fixed5 (lit frequency, dark 10 ms) | fixed (lit/dark frequency); default fixed5
              frequency: 1000         # ms; default 1000
```

A pod that lists this in `resourceClaims` needs no container reference, no device, no volume and no
securityContext. The agent:
- watches ResourceClaims and lights every claim allocated on its node that a pod has reserved;
- darkens a pixel when its pod is gone;
- rebuilds the strip from the API after a restart, keeping no files;
- clears the strip on shutdown.

An invalid PixelConfig fails the pod start with an error naming the field.

## Install

```sh
kubectl apply -f dist/install.yaml           # CRD, namespace blinkt-system, RBAC, manager Deployment
kubectl apply -f config/samples/blinktconfig.yaml
kubectl get blinkt                           # MODE / ACTIVE / READY / NODES / DEVICES
```

Log lines:
- manager: `blinkt-operator: starting`;
- agent: `blinkt-agent: registered driver=blinkt.kubedge.io node=<node>`, then
  `blinkt-agent: published devices=8 node=<node> writer=exclusive data=gpiochip0:23 clock=gpiochip0:24`;
- per claim: `blinkt-agent: prepared claim=… pixels=[6]` and `unprepared claim=…`.

Requirements: Kubernetes ≥ 1.34 (DRA `resource.k8s.io/v1`). Agent mode does not need CDI; `cdi` mode does.

## Develop

From the repo root:

```sh
make operator-generate        # DeepCopy, CRD and RBAC from api/ and the +kubebuilder markers
make operator-manifests       # + render dist/install.yaml (kubectl kustomize config/default)
make operator-test            # unit tests + envtest controller tests (Kubernetes 1.36.x API server)
make operator-lint
make operator-buildx-check    # one image, /manager + /node-agent, linux/arm64 + linux/amd64
make operator-buildx          # push kubedge1/blinkt-operator:<version> + :latest
```

Test-only manager flags:
- `--agent-extra-args=--fake-gpio`: the agent logs frames instead of driving GPIO;
- `--driver-extra-args=--assume-gpio,--gpio-device=/dev/null`.

## Verified on kind (2026-10-09)

kind v0.33.0, `kindest/node:v1.36.4`, operator image built from this tree, `dra-driver` from the published
`kubedge1/blinkt-dra-driver:0.5.1`.

```console
$ kubectl get blinkt                                      # mode: legacy
NAME      MODE     ACTIVE   READY   NODES   DEVICES
cluster   legacy   legacy   True    0                     # no DaemonSet, no DeviceClass
$ kubectl patch blinkt cluster --type merge -p '{"spec":{"mode":"cdi","defaults":{"intensity":3}}}'
cluster   cdi      cdi      True    1       8             # dra-driver published 8 devices; a cdi claim pod got BLINKT_PIXELS=6
$ kubectl patch blinkt cluster --type merge -p '{"spec":{"mode":"agent"}}'
# pod lifecycle: blinkt-dra-driver deleted 23:55:40 and Succeeded → blinkt-node-agent created 23:55:42
cluster   agent    agent    True    1       8
```

Two busybox pods with no securityContext and no volumes claimed `index == 4` (green, steady) and
`index == 6` (blue, `fixed` at 2000 ms):

```console
$ kubectl logs elte                                       # the pod sees no device and no BLINKT_* env
ls: /dev/gpiochip0: No such file or directory
0
$ kubectl -n blinkt-system logs ds/blinkt-node-agent      # intensity 3 comes from the class defaults
blinkt-agent: prepared claim=default/elte-led-mk4mh uid=… pixels=[4]
blinkt-agent: frame [- - - - 0,255,0,3 - 0,0,255,3 -]
blinkt-agent: frame [- - - - 0,255,0,3 - - -]               # pixel 6 blinking
$ kubectl delete pod elte
blinkt-agent: frame [- - - - - - 0,0,255,3 -]               # pixel 4 dark, pixel 6 unaffected
```

Guards:

| Scenario | Result |
|---|---|
| Agent pod deleted | A new agent redrew `0,0,255,3` on pixel 6 within 1 s of starting, from the API |
| `mode: legacy` while lte's claim exists | `Ready=False ClaimsInUse: … default/lte-led-…`; the agent is kept |
| `dra-driver/deploy/blinkt-dra-driver.yaml` hand-applied | `Ready=False ForeignDriver: blinkt-dra/blinkt-dra-driver`; the operator removed its own DaemonSet; after deleting the hand-applied one, `agent` became Ready again |

Peak memory (VmHWM): manager 35.7 MB (limit 96 Mi), agent 36.7 MB (limit 64 Mi).
