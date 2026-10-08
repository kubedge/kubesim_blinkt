# syntax=docker/dockerfile:1.7
# Go 0.4.x <-> Rust shared-state interop check (port-to-rust task 3.3).
# The Go side is built from GitHub at the last Go release, so this keeps
# working after the Go sources leave this repo. Run: hack/interop/check.sh
# Last Go release (0.4.0): main @ 0c6ebcc.
ARG GO_REF=0c6ebcc5d31318a9f8d05f83b054765040cbe1b6
FROM golang:1.23-bookworm AS go
ARG GO_REF
ADD https://github.com/kubedge/kubesim_blinkt.git#${GO_REF} /gosrc
WORKDIR /gosrc
RUN mkdir -p cmd/interop && cat > cmd/interop/main.go <<'GO'
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/kubedge/kubesim_blinkt/pkg/ledstate"
)

func main() {
	a := os.Args
	num := func(i int) int { n, err := strconv.Atoi(a[i]); if err != nil { panic(err) }; return n }
	b, err := ledstate.Open(a[1], a[2], time.Minute, func(f ledstate.Frame) error {
		j, err := json.Marshal(f)
		fmt.Println(string(j))
		return err
	})
	if err != nil {
		panic(err)
	}
	if a[3] == "withdraw" {
		if err := b.Withdraw(); err != nil { panic(err) }
		return
	}
	count := 1
	if len(a) > 8 { count = num(8) }
	px := map[int]ledstate.Pixel{num(4): {R: num(5), G: num(6), B: num(7), L: 5}}
	for i := 0; i < count; i++ {
		if err := b.Publish(px); err != nil { panic(err) }
	}
}
GO
RUN CGO_ENABLED=0 go build -o /interop-go ./cmd/interop

FROM rust:1.99-bookworm AS rust
WORKDIR /src
COPY . .
RUN --mount=type=cache,target=/usr/local/cargo/registry \
    --mount=type=cache,target=/target \
    CARGO_TARGET_DIR=/target cargo build --example interop && cp /target/debug/examples/interop /interop-rs

FROM debian:bookworm-slim
RUN apt-get update -qq && apt-get install -y -qq jq >/dev/null
COPY --from=go /interop-go /usr/local/bin/
COPY --from=rust /interop-rs /usr/local/bin/
COPY hack/interop/run.sh /run.sh
RUN --mount=type=tmpfs,target=/s sh /run.sh /s
