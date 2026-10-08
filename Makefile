
# Two implementations live side by side and build the same /blinkt5 program:
#   Rust (src/, Cargo.toml)     - the default image
#   Go   (cmd/, pkg/, go.mod)   - image tags carry a -go suffix
# Both are checked against tests/fixtures and against each other (make interop).
#
#   make test lint                     both implementations
#   make docker-buildx                 push the Rust image (IMPL=rust, default)
#   make docker-buildx IMPL=go         push the Go image

# Image URL to use all building/pushing image targets
COMPONENT        ?= kubesim_blinkt
VERSION_V1       ?= 0.5.0
DHUBREPO         ?= kubedge1/${COMPONENT}
DOCKER_NAMESPACE ?= kubedge1
K8S_NAMESPACE    ?= default

IMPL ?= rust
ifeq ($(IMPL),rust)
DOCKERFILE := build/Dockerfile.rust
TAG_SUFFIX :=
else ifeq ($(IMPL),go)
DOCKERFILE := build/Dockerfile.golang
TAG_SUFFIX := -go
else
$(error IMPL must be rust or go, not '$(IMPL)')
endif
IMG        ?= ${DHUBREPO}:${VERSION_V1}${TAG_SUFFIX}
IMG_LATEST ?= ${DHUBREPO}:latest${TAG_SUFFIX}

.PHONY: all test lint fmt interop clean \
	rust-test rust-lint rust-fmt go-test go-lint go-fmt \
	docker-buildx docker-buildx-check deploy undeploy install purge

all: test lint

test: rust-test go-test
lint: rust-lint go-lint
fmt: rust-fmt go-fmt

# Rust: cargo runs in the pinned toolchain container (hack/cargo.sh), so no
# local Rust install is needed; colima/docker with buildx is.
rust-test:
	hack/cargo.sh test

rust-lint:
	hack/cargo.sh fmt --check
	hack/cargo.sh clippy --all-targets -- -D warnings

rust-fmt:
	hack/fmt.sh

# Go: uses the local go toolchain (go.mod: go 1.23).
go-test:
	go test -race ./cmd/... ./pkg/...

go-lint:
	@test -z "$$(gofmt -l cmd pkg)" || { gofmt -l cmd pkg; echo "gofmt: files above need formatting"; exit 1; }
	go vet ./cmd/... ./pkg/...

go-fmt:
	gofmt -w cmd pkg

# Go <-> Rust shared-state interop check, built from this tree
interop:
	hack/interop/check.sh

clean:
	rm -fr target build/_output

# Both Dockerfiles build on $$BUILDPLATFORM and cross-compile static binaries,
# so no per-arch emulation is needed. arm/v7 is retired: every Pi now runs arm64.
# Requires a live buildx builder (e.g. `colima start`). buildx cannot --load a
# manifest list, so this pushes.
PLATFORMS ?= linux/arm64,linux/amd64
docker-buildx: $(IMPL)-test $(IMPL)-lint ## Build and push the multi-arch image for IMPL
	$(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) -t ${IMG} -t ${IMG_LATEST} -f $(DOCKERFILE) .

# Same multi-arch build without pushing
docker-buildx-check:
	$(CONTAINER_TOOL) buildx build --platform=$(PLATFORMS) -f $(DOCKERFILE) .

# Helm 3 install of the standalone chart against ~/.kube/config
install:
	helm install blinkt5 charts/kubesim-blinkt --set image.repository=${DHUBREPO},image.tag=${VERSION_V1}${TAG_SUFFIX} --namespace ${K8S_NAMESPACE}

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
