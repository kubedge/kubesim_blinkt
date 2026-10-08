package gpio

import (
	"errors"
	"strings"
	"testing"
)

type fake struct {
	names map[string]string // line name -> chip
	lines map[string]int    // chip -> number of lines
}

func (f fake) FindLine(name string) (string, int, error) {
	if c, ok := f.names[name]; ok {
		return c, 0, nil
	}
	return "", 0, errors.New("not found")
}

func (f fake) NumLines(chip string) (int, error) {
	if n, ok := f.lines[chip]; ok {
		return n, nil
	}
	return 0, errors.New("no such chip")
}

func TestNamedLines(t *testing.T) {
	r := Discover(fake{names: map[string]string{"GPIO23": "gpiochip0", "GPIO24": "gpiochip0"}})
	if !r.Found || r.Chip != "gpiochip0" {
		t.Fatalf("got %+v", r)
	}
}

func TestFallbackToGpiochip0(t *testing.T) {
	r := Discover(fake{lines: map[string]int{"gpiochip0": 54}})
	if !r.Found || r.Chip != "gpiochip0" {
		t.Fatalf("got %+v", r)
	}
}

func TestNoLines(t *testing.T) {
	for name, f := range map[string]fake{
		"no chip":     {},
		"short chip":  {lines: map[string]int{"gpiochip0": 8}},
		"split chips": {names: map[string]string{"GPIO23": "gpiochip0", "GPIO24": "gpiochip1"}},
	} {
		r := Discover(f)
		if r.Found || r.Reason == "" {
			t.Errorf("%s: got %+v, want not found with a reason", name, r)
		}
	}
	if r := Discover(fake{}); !strings.Contains(r.Reason, "gpiochip0 unavailable") {
		t.Errorf("reason = %q", r.Reason)
	}
}
