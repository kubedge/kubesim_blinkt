// Package gpio decides whether a node can drive a Blinkt!: whether the GPIO
// character device exposes BCM GPIO23 (data) and GPIO24 (clock). It only
// reads line info and never requests the lines, so it never competes with
// the blinkt processes that draw.
package gpio

import "fmt"

// Lines reports what the node's GPIO chips expose.
type Lines interface {
	// FindLine returns the chip holding the named line, or an error.
	FindLine(name string) (chip string, offset int, err error)
	// NumLines returns how many lines a chip (e.g. "gpiochip0") has.
	NumLines(chip string) (int, error)
}

// Result of a discovery.
type Result struct {
	Found  bool
	Chip   string // e.g. "gpiochip0"
	Reason string // why not found
}

// FallbackChip is checked when the kernel publishes no line names.
const FallbackChip = "gpiochip0"

// Discover looks for lines named GPIO23 and GPIO24 on one chip, falling back
// to gpiochip0 having at least 25 lines (offsets 23 and 24 exist).
func Discover(l Lines) Result {
	dChip, _, dErr := l.FindLine("GPIO23")
	cChip, _, cErr := l.FindLine("GPIO24")
	if dErr == nil && cErr == nil {
		if dChip != cChip {
			return Result{Reason: fmt.Sprintf("GPIO23 on %s and GPIO24 on %s", dChip, cChip)}
		}
		return Result{Found: true, Chip: dChip}
	}
	n, err := l.NumLines(FallbackChip)
	if err != nil {
		return Result{Reason: fmt.Sprintf("no lines named GPIO23/GPIO24 and %s unavailable: %v", FallbackChip, err)}
	}
	if n < 25 {
		return Result{Reason: fmt.Sprintf("no lines named GPIO23/GPIO24 and %s has only %d lines", FallbackChip, n)}
	}
	return Result{Found: true, Chip: FallbackChip}
}
