# blinkt-dra-driver: native build on $BUILDPLATFORM, cross-compiled with GOARCH,
# static binary in a scratch image.
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /workspace
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags "-s -w" -o /blinkt-dra-driver ./cmd/blinkt-dra-driver

FROM scratch
COPY --from=builder /blinkt-dra-driver /blinkt-dra-driver
ENTRYPOINT ["/blinkt-dra-driver"]
