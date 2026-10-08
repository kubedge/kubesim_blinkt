#!/usr/bin/env bash
# Go <-> Rust shared-state interop check (both built from this tree); prints INTEROP OK or fails.
set -euo pipefail
cd "$(dirname "$0")/../.."
docker buildx build --no-cache-filter=check --progress=plain -f hack/interop/Dockerfile \
  --output type=cacheonly "$@" . 2>&1 | grep -E "^#[0-9]+ [0-9.]+ (ok |INTEROP)|ERROR|error" \
  | sed -E 's/^#[0-9]+ [0-9.]+ //'
