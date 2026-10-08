# blinkt-runtime Specification

## Purpose
The operator-facing contract of the kubesim_blinkt program: how it is configured, which LED pattern it
runs, how it names itself, and what it logs and returns on start, stop and failure.

## Requirements

### Requirement: Configuration file
The program SHALL read YAML from `$BLINKT_CONFIG` when set, otherwise from `/etc/kubedge/blinkt_conf.yaml`. It SHALL read the keys `algorithm` (string), `intensity` (int), `frequency` (int, milliseconds) and `pixel0` to `pixel7` (lists of ints). Only pixel lists with at least three values SHALL light that LED, as red, green and blue at luminance `intensity`.

#### Scenario: Sidecar config
- **WHEN** the config is `algorithm: fixed5`, `intensity: 5`, `pixel6: [0, 0, 255]` and other pixel lists are empty
- **THEN** the program owns only pixel 6, blue 255, luminance 5

#### Scenario: Short pixel list
- **WHEN** `pixel2: [255]` is configured
- **THEN** pixel 2 is not lit, and the program does not crash

### Requirement: Missing or invalid configuration is fatal
If the config file cannot be read, the program SHALL log `blinkt: read config: <error>` and exit with status 1. If it is not valid YAML for these keys, the program SHALL log `Unmarshal: <error>` and exit with status 1.

#### Scenario: No config file
- **WHEN** the config path does not exist
- **THEN** the program exits 1 and logs `blinkt: read config: ...`, without lighting anything

### Requirement: Frequency defaults to 1000 ms
A missing, zero or negative `frequency` SHALL be treated as 1000.

#### Scenario: Chart config without frequency
- **WHEN** the config has no `frequency` key
- **THEN** the program runs and logs `frequency=1000ms`

### Requirement: Blinking algorithms
With `algorithm: blinkt5`, the program SHALL every 60 ms set one random LED (0 to 7) to random red, green and blue (0 to 255) and luminance (0 to 2), and publish all LEDs set so far. With any other algorithm, it SHALL repeatedly publish its configured pixels, wait `frequency` ms, publish no pixels, then wait 10 ms if the algorithm is `fixed5` and `frequency` ms otherwise.

#### Scenario: fixed5
- **WHEN** `algorithm: fixed5` and `frequency: 1000`
- **THEN** the owned LEDs are lit for 1000 ms and dark for 10 ms, repeatedly

#### Scenario: Other algorithm name
- **WHEN** `algorithm: fixed` and `frequency: 500`
- **THEN** the owned LEDs are lit for 500 ms and dark for 500 ms, repeatedly

### Requirement: Owner name and entry lifetime
The program SHALL publish under owner name `$BLINKT_OWNER` when set, else the host name (the pod name in Kubernetes), else `pid-<pid>`. Its entries' TTL SHALL be `3 × (frequency + dark time) ms + 2 s`, where the dark time is 10 ms for `fixed5` and `frequency` otherwise.

#### Scenario: Kubernetes sidecar
- **WHEN** the program runs in pod `kubesim-lte-5f7c9-abcde` with `BLINKT_OWNER` unset
- **THEN** its state entry is named `kubesim-lte-5f7c9-abcde`

#### Scenario: TTL for fixed5 at 1000 ms
- **WHEN** `algorithm: fixed5` and `frequency: 1000`
- **THEN** the entry TTL is 5.03 s

### Requirement: Start-up checks and log lines
Before reading the config, the program SHALL acquire and release the GPIO lines once. It SHALL then log, in this order and with the standard `YYYY/MM/DD HH:MM:SS ` prefix:
1. `blinkt: GPIO ready (data=<chip>:<offset> clock=<chip>:<offset>)`
2. `blinkt: running algorithm=<a> frequency=<f>ms config=<path> owner=<owner> <mode>`

`<mode>` SHALL be `shared state=<dir>` or `solo (<reason>)`. The program SHALL NOT change other owners' LEDs at start-up.

#### Scenario: Healthy start in a sidecar
- **WHEN** GPIO, config and state directory are all usable
- **THEN** both lines are logged, and the second ends with `shared state=/etc/kubedge`

#### Scenario: GPIO unavailable
- **WHEN** the lines cannot be acquired
- **THEN** the program logs `blinkt: GPIO setup failed: <error>` and exits with status 1

### Requirement: Drawing failures are fatal
If publishing a frame fails (GPIO, lock or save error), the program SHALL log `blinkt: <error>` and exit with status 1.

#### Scenario: Device disappears while running
- **WHEN** `/dev/gpiochip0` stops being accessible while the program runs
- **THEN** the program exits 1 with a `blinkt: ...` log line instead of running on with dark LEDs

### Requirement: Graceful stop on SIGINT and SIGTERM
On SIGINT the program SHALL print `Stopping on Interrupt`, and on SIGTERM `Stopping on Terminate`. It SHALL then finish its current wait, print `Stopping`, and withdraw its pixels. If withdrawal fails, it SHALL log `blinkt: exit: <error>`. It SHALL exit 0 after a clean stop.

#### Scenario: Pod deletion
- **WHEN** Kubernetes sends SIGTERM to the blinkt sidecar
- **THEN** within about one blink period its LEDs go dark, other owners' LEDs stay lit, and the process exits 0
