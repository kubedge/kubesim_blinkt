package config

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// RGB is a configured colour.
type RGB [3]int

// ParsePixels parses BLINKT_PIXELS, the LED indices a DRA claim allocated
// (e.g. "6" or "0,1,2"). Each must be an integer from 0 to 7.
func ParsePixels(v string) ([]int, error) {
	if strings.TrimSpace(v) == "" {
		return nil, fmt.Errorf("invalid BLINKT_PIXELS: %s", v)
	}
	var out []int
	for _, f := range strings.Split(v, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil || n < 0 || n > 7 {
			return nil, fmt.Errorf("invalid BLINKT_PIXELS: %s", v)
		}
		out = append(out, n)
	}
	return out, nil
}

// Colours returns the configured pixels (lists of at least three values).
func (c BlinktConfigData) Colours() map[int]RGB {
	out := map[int]RGB{}
	for i, rgb := range [][]int{c.Pixel0, c.Pixel1, c.Pixel2, c.Pixel3, c.Pixel4, c.Pixel5, c.Pixel6, c.Pixel7} {
		if len(rgb) >= 3 {
			out[i] = RGB{rgb[0], rgb[1], rgb[2]}
		}
	}
	return out
}

// Allocate maps each allocated index to the colour configured for that
// index, otherwise to the first configured colour in index order.
func Allocate(configured map[int]RGB, alloc []int) map[int]RGB {
	keys := make([]int, 0, len(configured))
	for k := range configured {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	out := map[int]RGB{}
	for _, i := range alloc {
		if c, ok := configured[i]; ok {
			out[i] = c
		} else if len(keys) > 0 {
			out[i] = configured[keys[0]]
		}
	}
	return out
}
