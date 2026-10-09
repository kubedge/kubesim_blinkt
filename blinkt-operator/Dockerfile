# blinkt-operator: /manager and /node-agent in one scratch image. Build from
# the repo root (the module replaces go-blinkt with ../go-blinkt):
#   docker buildx build -f blinkt-operator/Dockerfile .
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /workspace
COPY go-blinkt/ go-blinkt/
COPY blinkt-operator/go.mod blinkt-operator/go.sum blinkt-operator/
WORKDIR /workspace/blinkt-operator
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY blinkt-operator/api/ api/
COPY blinkt-operator/cmd/ cmd/
COPY blinkt-operator/internal/ internal/
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    for b in manager node-agent; do \
      CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
        go build -trimpath -ldflags "-s -w" -o /out/$b ./cmd/$b || exit 1; \
    done

FROM scratch
COPY --from=builder /out/manager /manager
COPY --from=builder /out/node-agent /node-agent
USER 65532:65532
ENTRYPOINT ["/manager"]
