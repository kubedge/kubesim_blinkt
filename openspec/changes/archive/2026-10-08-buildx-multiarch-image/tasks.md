# Tasks — buildx-multiarch-image

Done by the 2026-10-08 work (PRs #4, #7, #8, #9); evidence per task.

- [x] Confirm buildx + a running builder (colima on Apple-Silicon). Evidence: colima docker 29.5.2 arm64; `make docker-buildx*` and `dra-buildx` build through it.
- [x] For each built image, add a buildx target `docker buildx build --platform linux/arm64 -t <image>:<tag>` (add `,linux/amd64` only if still needed). Evidence: `make docker-buildx [IMPL=go|rust]`, `make dra-buildx`; `PLATFORMS ?= linux/arm64,linux/amd64` (amd64 kept for dev/CI).
- [x] Make each Dockerfile multi-stage (compile per TARGETOS/TARGETARCH; don't copy a prebuilt amd64 binary). Evidence: `build/Dockerfile.golang`, `build/Dockerfile.rust` and `dra-driver/Dockerfile` all build on `$BUILDPLATFORM` and cross-compile for the target, with `FROM scratch` finals.
- [x] Remove the `_AMD64/_ARM64V8/_ARM32V7` image-name variables. Evidence: removed from the Makefile in #4/#7; arm/v7 retired (every Pi is arm64).
- [x] `docker buildx imagetools inspect` each image → manifest list incl. linux/arm64. Evidence: verified for kubesim_blinkt{,_go,_rs}:0.5.1 and blinkt-dra-driver:0.5.1 (linux/arm64 + linux/amd64, anonymous pull 200).
- [x] If deployed by an operator, update that operator's example CR to the arch-independent name. Evidence: no operator deploys this image. The simulator charts already use the arch-independent `kubedge1/kubesim_blinkt`.
