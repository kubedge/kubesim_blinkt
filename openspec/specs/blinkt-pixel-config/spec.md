# blinkt-pixel-config Specification

## Summary
`PixelConfig` is opaque configuration for the `blinkt.kubedge.io` driver, carried in the ResourceClaim. It sets colour, intensity and pattern, so a claim describes its LED completely. Class-level values are defaults that claim-level values override. A configuration that fails validation fails the pod start rather than drawing something unexpected.

## Purpose
The per-claim LED configuration for the Blinkt!: which colour, intensity and blink pattern a claimed pixel
shows. It is carried in the ResourceClaim itself, so a claim fully describes the LED.

## Requirements

### Requirement: PixelConfig opaque configuration
A claim SHALL be able to configure its pixels with an opaque device configuration for driver `blinkt.kubedge.io` whose parameters are `apiVersion: blinkt.kubedge.io/v1alpha1`, `kind: PixelConfig`, and fields `color` (three integers 0–255), `intensity` (0–31), `algorithm` (`fixed5`, `fixed` or `steady`) and `frequency` (milliseconds, > 0).

#### Scenario: Claim with a colour
- **WHEN** a ResourceClaimTemplate requests `index == 6` with config `{apiVersion: blinkt.kubedge.io/v1alpha1, kind: PixelConfig, color: [0, 0, 255]}` for request `led`
- **THEN** the allocated pixel is shown blue

### Requirement: Defaults
Omitted fields SHALL default to `color: [255, 255, 255]`, `intensity: 5`, `algorithm: fixed5` and `frequency: 1000`. A configuration supplied by the DeviceClass SHALL apply first, and one supplied by the claim SHALL override it field by field.

#### Scenario: Only a colour given
- **WHEN** the claim's PixelConfig sets only `color: [0, 255, 0]` and the DeviceClass sets no configuration
- **THEN** the pixel is green, intensity 5, lit 1000 ms and dark 10 ms (`fixed5`)

#### Scenario: Class default overridden by claim
- **WHEN** the DeviceClass configures `intensity: 3` and the claim configures `intensity: 10`
- **THEN** the pixel uses intensity 10

### Requirement: Patterns
`steady` SHALL keep the pixel lit. `fixed5` SHALL light it for `frequency` ms, then dark for 10 ms, repeatedly. `fixed` SHALL light it for `frequency` ms, then dark for `frequency` ms, repeatedly.

#### Scenario: Blinking pixel
- **WHEN** a claim configures `algorithm: fixed, frequency: 500`
- **THEN** its pixel alternates 500 ms lit and 500 ms dark, without affecting other claims' pixels

### Requirement: Invalid configuration fails the pod start
A PixelConfig with an unknown `apiVersion` or `kind`, an unknown field, or an out-of-range value SHALL make claim preparation fail. The error SHALL name the field, and the pod SHALL NOT start.

#### Scenario: Colour out of range
- **WHEN** a claim configures `color: [0, 0, 300]`
- **THEN** the pod's events show a prepare failure naming `color`, and the pod stays in `ContainerCreating`

#### Scenario: Unknown kind
- **WHEN** a claim configures `kind: GPUConfig` for driver `blinkt.kubedge.io`
- **THEN** preparation fails with an error naming the unsupported kind
