package periBlink

import (
	"errors"
	"testing"
)

// fakeBus records the data level latched on each rising clock edge.
type fakeBus struct {
	data   int
	clock  int
	bits   []int
	failAt int // fail the Nth SetValue call on the data pin (1-based); 0 = never
	calls  int
	closed int
}

type fakeData struct{ b *fakeBus }
type fakeClock struct{ b *fakeBus }

func (d fakeData) SetValue(v int) error {
	d.b.calls++
	if d.b.failAt != 0 && d.b.calls == d.b.failAt {
		return errors.New("boom")
	}
	d.b.data = v
	return nil
}
func (d fakeData) Close() error { d.b.closed++; return nil }

func (c fakeClock) SetValue(v int) error {
	if v == 1 && c.b.clock == 0 {
		c.b.bits = append(c.b.bits, c.b.data)
	}
	c.b.clock = v
	return nil
}
func (c fakeClock) Close() error { c.b.closed++; return nil }

func install(t *testing.T) *fakeBus {
	t.Helper()
	b := &fakeBus{}
	dat, clk = fakeData{b}, fakeClock{b}
	gpioSetUp = true
	blinkt = [numPx]Blinkt{}
	t.Cleanup(func() { dat, clk, gpioSetUp = nil, nil, false })
	return b
}

func bytesOf(bits []int) []int {
	out := make([]int, 0, len(bits)/8)
	for i := 0; i+8 <= len(bits); i += 8 {
		v := 0
		for _, b := range bits[i : i+8] {
			v = v<<1 | b
		}
		out = append(out, v)
	}
	return out
}

func TestShowWritesAPA102Frame(t *testing.T) {
	b := install(t)
	SetPixel(0, 0x12, 0x34, 0x56, 7)
	SetPixel(7, 255, 0, 128, 31)

	if err := Show(); err != nil {
		t.Fatalf("Show: %v", err)
	}

	// 32 start bits + 8 pixels * 32 bits + 36 end bits.
	if got, want := len(b.bits), 32+numPx*32+36; got != want {
		t.Fatalf("clocked %d bits, want %d", got, want)
	}
	for i, v := range b.bits[:32] {
		if v != 0 {
			t.Fatalf("start frame bit %d = %d, want 0", i, v)
		}
	}
	px := bytesOf(b.bits[32 : 32+numPx*32])
	// Each pixel: 0xE0|lum, blue, green, red.
	if got := px[0:4]; got[0] != 0xE7 || got[1] != 0x56 || got[2] != 0x34 || got[3] != 0x12 {
		t.Errorf("pixel 0 = %#x, want [0xe7 0x56 0x34 0x12]", got)
	}
	if got := px[4:8]; got[0] != 0xE0 || got[1] != 0 || got[2] != 0 || got[3] != 0 {
		t.Errorf("pixel 1 = %#x, want [0xe0 0 0 0]", got)
	}
	if got := px[28:32]; got[0] != 0xFF || got[1] != 128 || got[2] != 0 || got[3] != 255 {
		t.Errorf("pixel 7 = %#x, want [0xff 0x80 0 0xff]", got)
	}
	for i, v := range b.bits[32+numPx*32:] {
		if v != 0 {
			t.Fatalf("end frame bit %d = %d, want 0", i, v)
		}
	}
}

func TestShowReturnsPinError(t *testing.T) {
	b := install(t)
	b.failAt = 5
	if err := Show(); err == nil {
		t.Fatal("Show succeeded, want error from data pin")
	}
}

func TestExitClearsAndReleasesLines(t *testing.T) {
	b := install(t)
	SetPixel(3, 1, 2, 3, 4)
	if err := Exit(); err != nil {
		t.Fatalf("Exit: %v", err)
	}
	if r, g, bl, _ := GetPixel(3); r|g|bl != 0 {
		t.Errorf("pixel 3 = %d,%d,%d after Exit, want cleared", r, g, bl)
	}
	if b.closed != 2 {
		t.Errorf("closed %d lines, want 2", b.closed)
	}
	if gpioSetUp || dat != nil || clk != nil {
		t.Error("Exit left GPIO marked as set up")
	}
}

func TestSetPixelMasksValues(t *testing.T) {
	blinkt = [numPx]Blinkt{}
	SetPixel(2, 256+5, -1, 300, 40)
	r, g, b, l := GetPixel(2)
	if r != 5 || g != 255 || b != 300&255 || l != 40&31 {
		t.Errorf("GetPixel = %d,%d,%d,%d", r, g, b, l)
	}
}
