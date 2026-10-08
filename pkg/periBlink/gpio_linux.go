//go:build linux

package periBlink

import (
	"fmt"

	"github.com/warthog618/go-gpiocdev"
)

// findLine resolves a BCM GPIO by its device-tree line name ("GPIO23"),
// falling back to the given offset on defaultChip.
func findLine(offset int) (string, int) {
	if chip, off, err := gpiocdev.FindLine(fmt.Sprintf("GPIO%d", offset)); err == nil {
		return chip, off
	}
	return defaultChip, offset
}

func requestOutput(offset int) (outputPin, error) {
	chip, off := findLine(offset)
	l, err := gpiocdev.RequestLine(chip, off, gpiocdev.AsOutput(0), gpiocdev.WithConsumer(consumer))
	if err != nil {
		return nil, fmt.Errorf("periBlink: request %s line %d (BCM GPIO%d): %w", chip, off, offset, err)
	}
	return l, nil
}
