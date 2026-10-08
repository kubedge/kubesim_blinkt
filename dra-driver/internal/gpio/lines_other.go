//go:build !linux

package gpio

import "errors"

// System reads the node's real GPIO chips; there are none off Linux.
type System struct{}

var errNotLinux = errors.New("GPIO character device is only available on Linux")

func (System) FindLine(string) (string, int, error) { return "", 0, errNotLinux }
func (System) NumLines(string) (int, error)         { return 0, errNotLinux }
