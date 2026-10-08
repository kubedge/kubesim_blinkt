//go:build !linux

package periBlink

import "errors"

func requestOutput(offset int) (outputPin, error) {
	return nil, errors.New("periBlink: GPIO character device is only available on Linux")
}
