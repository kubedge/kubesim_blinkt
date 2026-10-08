
# Image URL to use all building/pushing image targets
COMPONENT        ?= kubesim_blinkt
VERSION_V1       ?= 0.5.0
DHUBREPO         ?= kubedge1/${COMPONENT}
DOCKER_NAMESPACE ?= kubedge1
IMG              ?= ${DHUBREPO}:${VERSION_V1}
K8S_NAMESPACE    ?= default

# Cargo runs in the pinned Rust toolchain container (hack/cargo.sh), so no
# local Rust install is needed; colima/docker with buildx is.
all: test lint

# Format the crate (writes back to src/ and refreshes Cargo.lock)
fmt:
	hack/fmt.sh

# Lint: rustfmt check + clippy with warnings denied
lint:
	hack/cargo.sh fmt --check
	hack/cargo.sh clippy --all-targets -- -D warnings

# Unit tests (linux, host arch)
test:
	hack/cargo.sh test

# Go 0.4.x <-> Rust shared-state interop check
interop:
	hack/interop/check.sh

clean:
	rm -fr target build/_output

# build/Dockerfile.buildkit builds on $$BUILDPLATFORM and cross-compiles static
# musl binaries, so no per-arch emulation is needed. arm/v7 is retired: every
# Pi now runs arm64. Requires a live buildx builder (e.g. `colima start`).
# buildx cannot --load a manifest list, so this pushes.
PLATFORMS ?= linux/arm64,linux/amd64
.PHONY: all fmt lint test interop clean docker-buildx docker-buildx-check deploy undeploy install purge
docker-buildx: test lint ## Build and push the multi-arch image
	$(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) -t ${IMG} -t ${DHUBREPO}:latest -f build/Dockerfile.buildkit .

# Same multi-arch build without pushing
docker-buildx-check:
	$(CONTAINER_TOOL) buildx build --platform=$(PLATFORMS) -f build/Dockerfile.buildkit .

# Helm 3 install of the standalone chart against ~/.kube/config
install:
	helm install blinkt5 charts/kubesim-blinkt --set image.repository=${DHUBREPO},image.tag=${VERSION_V1} --namespace ${K8S_NAMESPACE}

purge:
	helm uninstall blinkt5 --namespace ${K8S_NAMESPACE}

# Plain manifests (no Helm). Label each Blinkt! node first:
#   kubectl label node <node> blinktInstalled=true
deploy:
	kubectl apply -f deploy/kubesim-blinkt.yaml

undeploy:
	kubectl delete -f deploy/kubesim-blinkt.yaml

CONTAINER_TOOL ?= docker
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec
