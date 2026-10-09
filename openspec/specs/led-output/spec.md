# led-output Specification

## Summary
The Blinkt! is driven by bit-banging two GPIO lines through the Linux GPIO character device (go-gpiocdev in Go, gpiocdev in Rust), found by name (`GPIO23` data, `GPIO24` clock) with a `gpiochip0` fallback. A frame is the APA102 wire format: a 32-bit zero start, `0xE0|luminance`, B, G, R per pixel, then 36 zero bits. Values are masked to the hardware range. The lines are held only while a frame is written, and a busy line (EBUSY) is retried for at most 2 s, which lets several processes share them.

## Purpose
Drives the Pimoroni Blinkt! (eight APA102 LEDs) on a Raspberry Pi by bit-banging two GPIO lines through the
Linux GPIO character device, so it works on 32- and 64-bit kernels without /dev/gpiomem or sysfs numbering.

## Requirements

### Requirement: Data and clock lines are found by name
The program SHALL drive BCM GPIO23 as data and BCM GPIO24 as clock through the GPIO character device. It SHALL locate each line by its kernel line name (`GPIO23`, `GPIO24`) on any chip, and fall back to offsets 23 and 24 on `gpiochip0` when no chip publishes that name. Lines SHALL be requested as outputs, initially low, with consumer label `kubesim_blinkt`.

#### Scenario: Kernel publishes line names
- **WHEN** a chip exposes lines named `GPIO23` and `GPIO24`
- **THEN** those lines are requested, whatever the chip's number or the sysfs GPIO base

#### Scenario: Kernel publishes no line names
- **WHEN** no chip has a line named `GPIO23`
- **THEN** offset 23 on `gpiochip0` is requested for data and offset 24 for clock

#### Scenario: Lines report their location
- **WHEN** both lines are acquired
- **THEN** they are described as `data=<chip>:<offset> clock=<chip>:<offset>`, e.g. `data=gpiochip0:23 clock=gpiochip0:24`

### Requirement: Frames use the APA102 wire format
A frame SHALL be clocked out MSB-first: 32 clock pulses with data low, then for each LED 0 to 7 the four bytes `0xE0|luminance`, blue, green, red, then 36 clock pulses with data low. For each bit, data SHALL be set before the clock goes high and then low.

#### Scenario: One lit pixel
- **WHEN** LED 0 is red 0x12, green 0x34, blue 0x56, luminance 7 and the other LEDs are zero
- **THEN** the 324 bits clocked are 32 zeros, bytes `E7 56 34 12`, seven times `E0 00 00 00`, then 36 zeros

### Requirement: Pixel values are masked to the hardware range
Red, green and blue SHALL be masked to 8 bits (`value & 255`) and luminance to 5 bits (`value & 31`) before encoding. Out-of-range values SHALL NOT be rejected.

#### Scenario: Oversized and negative values
- **WHEN** a pixel is set to red 261, green -1, blue 300, luminance 40
- **THEN** it is drawn as red 5, green 255, blue 44, luminance 8

### Requirement: Lines are held only while a frame is written
The program SHALL acquire both lines before writing a frame and release them right after, so that another process on the node can write the next frame.

#### Scenario: Another process draws between frames
- **WHEN** a process has finished writing a frame
- **THEN** a second process can acquire GPIO23 and GPIO24 without error

### Requirement: Busy lines are retried for a bounded time
If acquiring a line fails because another process holds it (`EBUSY`), the program SHALL retry about every 5 ms for up to 2 seconds. It SHALL then fail with that error. Errors other than `EBUSY` SHALL fail immediately, without retrying.

#### Scenario: Line freed within the window
- **WHEN** the line is busy for three attempts and then free
- **THEN** acquisition succeeds

#### Scenario: Line held too long
- **WHEN** the line stays busy for more than 2 seconds
- **THEN** acquisition fails with the `EBUSY` error

#### Scenario: No GPIO device
- **WHEN** `/dev/gpiochip0` does not exist
- **THEN** acquisition fails at once with an error naming the chip, the line offset and `BCM GPIO23`
