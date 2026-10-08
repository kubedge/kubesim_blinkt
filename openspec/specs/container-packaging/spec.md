# container-packaging Specification

## Purpose
How the repo packages its programs: each image (kubesim_blinkt in Go and Rust, and the DRA driver) is one multi-arch image built with docker buildx, so every node pulls the same name whatever its architecture.

## Requirements

### Requirement: Each built image is a single multi-arch image via buildx

Each image this simulator builds SHALL be produced as one multi-arch image
`<image>:<tag>` covering at least `linux/arm64` via `docker buildx`, replacing the
arch-suffixed variants. Where the image is deployed by an operator, that operator's
example CR SHALL reference the arch-independent name.

#### Scenario: an image serves all target arches
- **WHEN** `docker buildx imagetools inspect <image>:<tag>` runs after a build
- **THEN** it resolves to a manifest list including `linux/arm64`, with no arch-suffixed variant
