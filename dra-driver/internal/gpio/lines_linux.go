//go:build linux

package gpio

import "github.com/warthog618/go-gpiocdev"

// System reads the node's real GPIO chips.
type System struct{}

func (System) FindLine(name string) (string, int, error) {
	return gpiocdev.FindLine(name)
}

func (System) NumLines(chip string) (int, error) {
	c, err := gpiocdev.NewChip(chip)
	if err != nil {
		return 0, err
	}
	defer c.Close()
	return c.Lines(), nil
}
