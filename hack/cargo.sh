#!/usr/bin/env bash
# Run a cargo command in the pinned Rust toolchain container (linux, host arch),
# e.g. `hack/cargo.sh test`, `hack/cargo.sh clippy --all-targets -- -D warnings`.
# Goes through `docker buildx build` so it works without host bind mounts (colima).
set -euo pipefail
cd "$(dirname "$0")/.."
exec docker buildx build --no-cache-filter=cargo --progress=plain -f hack/Dockerfile.cargo \
  --build-arg "CARGO_ARGS=$*" --output type=cacheonly . 2>&1 \
  | sed -n 's/^#[0-9]* [0-9.]* //p'
