
# Two implementations live side by side and build the same /blinkt5 program:
#   Go   (cmd/, pkg/, go.mod)   -> kubedge1/kubesim_blinkt_go   (default)
#   Rust (src/, Cargo.toml)     -> kubedge1/kubesim_blinkt_rs
# The default implementation is also published as kubedge1/kubesim_blinkt.
# Both are checked against tests/fixtures and against each other (make interop).
#
#   make test lint                     both implementations
#   make docker-buildx                 push the Go image (IMPL=go, default) + kubesim_blinkt
#   make docker-buildx IMPL=rust       push the Rust image
#   make docker-buildx-all             push both
#   make dra-buildx                    push the DRA driver image (dra-driver/)

# Image URL to use all building/pushing image targets
COMPONENT        ?= kubesim_blinkt
VERSION_V1       ?= 0.5.1
DOCKER_NAMESPACE ?= kubedge1
DHUBREPO         ?= ${DOCKER_NAMESPACE}/${COMPONENT}
K8S_NAMESPACE    ?= default

# Which implementation kubesim_blinkt (no suffix) points to.
DEFAULT_IMPL ?= go
IMPL         ?= $(DEFAULT_IMPL)
ifeq ($(IMPL),go)
DOCKERFILE := build/Dockerfile.golang
IMPL_REPO  := ${DHUBREPO}_go
else ifeq ($(IMPL),rust)
DOCKERFILE := build/Dockerfile.rust
IMPL_REPO  := ${DHUBREPO}_rs
else
$(error IMPL must be go or rust, not '$(IMPL)')
endif
IMG  := ${IMPL_REPO}:${VERSION_V1}
TAGS := -t ${IMG} -t ${IMPL_REPO}:latest
ifeq ($(IMPL),$(DEFAULT_IMPL))
TAGS += -t ${DHUBREPO}:${VERSION_V1} -t ${DHUBREPO}:latest
endif

.PHONY: all test lint fmt interop clean \
	rust-test rust-lint rust-fmt go-test go-lint go-fmt \
	dra-test dra-lint dra-buildx dra-buildx-check \
	docker-buildx docker-buildx-all docker-buildx-check deploy undeploy install purge

all: test lint

test: rust-test go-test dra-test
lint: rust-lint go-lint dra-lint
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

# DRA driver blinkt.kubedge.io (dra-driver/, its own Go module)
DRA_REPO ?= ${DOCKER_NAMESPACE}/blinkt-dra-driver
dra-test:
	cd dra-driver && go test -race ./...

dra-lint:
	@test -z "$$(gofmt -l dra-driver)" || { gofmt -l dra-driver; echo "gofmt: files above need formatting"; exit 1; }
	cd dra-driver && go vet ./...

dra-buildx: dra-test dra-lint ## Build and push the multi-arch DRA driver image
	$(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) -t ${DRA_REPO}:${VERSION_V1} -t ${DRA_REPO}:latest \
	  --label org.opencontainers.image.revision=$$(git rev-parse HEAD) \
	  --label org.opencontainers.image.source=https://github.com/kubedge/kubesim_blinkt \
	  dra-driver

dra-buildx-check:
	$(CONTAINER_TOOL) buildx build --platform=$(PLATFORMS) dra-driver

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
	$(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) $(TAGS) \
	  --label org.opencontainers.image.revision=$$(git rev-parse HEAD) \
	  --label org.opencontainers.image.source=https://github.com/kubedge/kubesim_blinkt \
	  -f $(DOCKERFILE) .

# Push both implementations
docker-buildx-all:
	$(MAKE) docker-buildx IMPL=go
	$(MAKE) docker-buildx IMPL=rust

# Same multi-arch build without pushing
docker-buildx-check:
	$(CONTAINER_TOOL) buildx build --platform=$(PLATFORMS) -f $(DOCKERFILE) .

# Helm 3 install of the standalone chart against ~/.kube/config
install:
	helm install blinkt5 charts/kubesim-blinkt --set image.repository=${IMPL_REPO},image.tag=${VERSION_V1} --namespace ${K8S_NAMESPACE}

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
