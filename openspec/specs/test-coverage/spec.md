# test-coverage Specification

## Summary
The simulator's core logic, meaning frame encoding, shared LED state, configuration and DRA claim handling, has unit tests. They run under `go test ./... -race` and must pass. The requirement exists so the core packages never fall back to zero coverage and a behaviour change is caught before it reaches the hardware.

## Purpose
Keeps the simulator's core logic (frame encoding, shared LED state, configuration, DRA claim handling) covered by passing unit tests, so behaviour changes are caught before they reach the hardware.

## Requirements

### Requirement: The simulator's core logic is unit-tested

This simulator SHALL have unit tests covering its core message-handling / simulation
logic, run under `go test ./... -race`.

#### Scenario: the simulator has passing tests
- **WHEN** `go test ./... -race` runs
- **THEN** its core packages have passing tests (no longer 0 coverage)
