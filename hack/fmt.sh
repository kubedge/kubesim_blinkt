#!/usr/bin/env bash
# Run `cargo fmt` (and refresh Cargo.lock) in the pinned toolchain; writes back to src/, examples/ and Cargo.lock.
set -euo pipefail
cd "$(dirname "$0")/.."
docker buildx build -q -f hack/Dockerfile.fmt --output type=local,dest=. . >/dev/null
