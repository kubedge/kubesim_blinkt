
# Image URL to use all building/pushing image targets
COMPONENT        ?= kubesim_blinkt
VERSION_V1       ?= 0.2.24
DHUBREPO         ?= kubedge1/${COMPONENT}
DHUBREPO_ARM32V7 ?= kubedge1/${COMPONENT}-arm32v7
DHUBREPO_ARM64V8 ?= kubedge1/${COMPONENT}-arm64v8
DOCKER_NAMESPACE ?= kubedge1
IMG              ?= ${DHUBREPO}:${VERSION_V1}
IMG_ARM32V7      ?= ${DHUBREPO_ARM32V7}:${VERSION_V1}
IMG_ARM64V8      ?= ${DHUBREPO_ARM64V8}:${VERSION_V1}
K8S_NAMESPACE    ?= default

all: docker-build

setup:
ifndef GOPATH
	$(error GOPATH not defined, please define GOPATH. Run "go help gopath" to learn more about GOPATH)
endif
	# dep ensure

clean:
	rm -fr build/_output
	rm -fr config/crds
	rm -fr go.sum

# Run go fmt against code
fmt: setup
	go fmt ./pkg/... ./cmd/...

# Run go vet against code
vet-v1: fmt
	go vet -composites=false -tags=v1 ./pkg/... ./cmd/...

# Build the docker image
docker-build: fmt vet-v1 docker-build-arm32v7 docker-build-arm64v8

docker-build-arm32v7:
	GOOS=linux GOARM=7 GOARCH=arm CGO_ENABLED=0 go build -o build/_output/arm32v7/blinkt5 -gcflags all=-trimpath=${GOPATH} -asmflags all=-trimpath=${GOPATH} -tags=v1 ./cmd/...
	docker build . -f build/Dockerfile.arm32v7 -t ${IMG_ARM32V7}
	docker tag ${IMG_ARM32V7} ${DHUBREPO_ARM32V7}:latest

docker-build-arm64v8:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o build/_output/arm64v8/blinkt5 -gcflags all=-trimpath=${GOPATH} -asmflags all=-trimpath=${GOPATH} -tags=v1 ./cmd/...
	docker build . -f build/Dockerfile.arm64v8 -t ${IMG_ARM64V8}
	docker tag ${IMG_ARM64V8} ${DHUBREPO_ARM64V8}:latest

PLATFORMS ?= linux/arm64,linux/amd64,linux/arm/v7
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' build/Dockerfile.buildkit > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name project-v3-builder
	$(CONTAINER_TOOL) buildx use project-v3-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} --tag ${DHUBREPO}:latest -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm project-v3-builder
	rm Dockerfile.cross

# Push the docker image
docker-push: docker-push-arm32v7 docker-push-arm64v8

docker-push-arm32v7:
	docker push ${IMG_ARM32V7}

docker-push-arm64v8:
	docker push ${IMG_ARM64V8}

# Run against the configured Kubernetes cluster in ~/.kube/config
install: install-arm32v7

install-arm32v7:
	helm install --name blinkt5 chart --set images.tags.operator=${IMG_ARM32V7},images.pull_policy=Always --namespace ${K8S_NAMESPACE}

install-arm64v8:
	helm install --name blinkt5 chart --set images.tags.operator=${IMG_ARM64V8},images.pull_policy=Always --namespace ${K8S_NAMESPACE}

purge: setup
	helm delete --purge blinkt5

# Build the docker image for cross-plaform support
CONTAINER_TOOL ?= docker
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

