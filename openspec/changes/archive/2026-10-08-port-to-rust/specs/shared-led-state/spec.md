# Spec Delta

## Purpose

Lets several blinkt processes on one node, typically the blinkt sidecars of co-located kubesim simulator pods,
share the eight LEDs. Each process lights only its own pixels, and the drawn frame is the merge of all of them.

## ADDED Requirements

### Requirement: State lives in a shared directory
The shared state directory SHALL be `$BLINKT_STATE_DIR` when that variable is set, and `/etc/kubedge` otherwise. It SHALL hold a lock file `blinkt.lock`, created with mode 0666 subject to umask, and a state file `blinkt_state.json`.

#### Scenario: Default directory
- **WHEN** `BLINKT_STATE_DIR` is unset
- **THEN** the program uses `/etc/kubedge/blinkt.lock` and `/etc/kubedge/blinkt_state.json`

#### Scenario: Directory override
- **WHEN** `BLINKT_STATE_DIR=/tmp/s` is set
- **THEN** the program uses `/tmp/s/blinkt.lock` and `/tmp/s/blinkt_state.json`

### Requirement: Updates are serialized by an exclusive flock
Each update SHALL take an exclusive `flock` on `blinkt.lock` and hold it while it reads the state, applies its change, drops expired entries, draws the merged frame and saves the state. It SHALL then release the lock.

#### Scenario: Concurrent publishers
- **WHEN** four processes publish different pixels concurrently, 25 times each
- **THEN** a frame drawn after all of them have published shows all four pixels

### Requirement: State file format
The state file SHALL be a JSON object `{"owners": {<owner>: {"pixels": {"<index>": {"r":R,"g":G,"b":B,"l":L}}, "updated": <RFC 3339 timestamp, nanosecond precision>, "ttl": <nanoseconds>}}}`. Pixel keys SHALL be decimal strings. A missing, empty or unparsable file, or a missing or null `owners`, SHALL be read as having no owners.

#### Scenario: Written entry
- **WHEN** owner `kubesim-lte-7d9f` publishes pixel 6 blue 255 luminance 5 at 2026-10-08T19:00:00.123456789Z with a 5 s TTL
- **THEN** the file contains `{"owners":{"kubesim-lte-7d9f":{"pixels":{"6":{"r":0,"g":0,"b":255,"l":5}},"updated":"2026-10-08T19:00:00.123456789Z","ttl":5000000000}}}`

#### Scenario: Corrupt file
- **WHEN** the state file contains `{not json`
- **THEN** the next update starts from no owners and succeeds

#### Scenario: Pixels null
- **WHEN** an owner entry has `"pixels": null`
- **THEN** that owner contributes no lit pixels and the file is still accepted

### Requirement: State is saved atomically
The state SHALL be written to `blinkt_state.json.tmp` in the same directory with mode 0644, then renamed over `blinkt_state.json`.

#### Scenario: Reader during a save
- **WHEN** a process reads the state file while another is saving
- **THEN** it sees either the complete previous state or the complete new state

### Requirement: Merged frame resolves conflicts by owner name
The drawn frame SHALL be built by applying owners in ascending byte order of their names, each overwriting the LEDs it claims. So for a contested LED, the owner whose name sorts last wins. Pixel indices outside 0 to 7 SHALL be ignored, and LEDs nobody claims SHALL be dark.

#### Scenario: Co-located simulators
- **WHEN** owner `kubesim-lte-a` claims pixel 6 blue and owner `kubesim-elte-b` claims pixel 4 green
- **THEN** the frame lights pixel 4 green and pixel 6 blue, and all other LEDs are dark

#### Scenario: Contested pixel
- **WHEN** `kubesim-5gc` and `kubesim-nr` both claim pixel 7
- **THEN** pixel 7 shows `kubesim-nr`'s colour

### Requirement: Entries expire after their TTL
On every update, entries whose `updated` is more than `ttl` before the current time SHALL be removed before the frame is drawn.

#### Scenario: Crashed owner
- **WHEN** an owner published with a 5 s TTL and was killed without withdrawing, and another owner updates 6 s later
- **THEN** the crashed owner's pixels are no longer drawn and its entry is removed from the file

### Requirement: Withdrawal removes only the caller's pixels
On withdrawal, the program SHALL delete its own entry and redraw the frame from the remaining owners.

#### Scenario: One of two owners stops
- **WHEN** owners A (pixel 0) and B (pixel 7) are active and B withdraws
- **THEN** the frame shows only pixel 0

### Requirement: Solo fallback when the directory is unusable
If the state directory is configured as empty, or `blinkt.lock` cannot be opened or created, the program SHALL keep running in solo mode. In solo mode it draws only its own pixels, keeps its state in memory, and records the reason for the start-up log line.

#### Scenario: Read-only directory
- **WHEN** the state directory is not writable by the process
- **THEN** the program still lights its own pixels and reports `solo (<reason>)`

### Requirement: Rust and Go implementations interoperate
A Rust build and the Go 0.4.x build SHALL be able to run on the same node against the same state directory. Each SHALL read the other's entries, honour its lock and draw the same merged frame.

#### Scenario: Mixed sidecars
- **WHEN** a Go 0.4.0 process owns pixel 4 and a Rust process owns pixel 6 on the same node
- **THEN** both pixels stay lit, and stopping either one leaves the other lit
