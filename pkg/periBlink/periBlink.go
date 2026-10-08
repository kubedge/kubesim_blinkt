// periBlink.go
// Rich Robinson
// Sept 2018
//
// Drives a Pimoroni Blinkt! (8 x APA102) by bit-banging BCM GPIO23 (data)
// and GPIO24 (clock) through the Linux GPIO character device
// (/dev/gpiochipN). Unlike memory-mapped /dev/gpiomem or sysfs GPIO numbers,
// this works unchanged on 32 and 64 bit kernels and on kernels that number
// sysfs GPIOs from 512.

package periBlink

import (
	"errors"
	"fmt"
	"time"
)

const (
	numPx = 8
	// used luminance instead of brightness
	// note values 0 to 31 instead of float
	luminance = 3
	redI      = 0
	greenI    = 1
	blueI     = 2
	lumI      = 3

	// BCM GPIO lines wired to the Blinkt! header.
	datOffset = 23
	clkOffset = 24
	// Chip used when the kernel does not publish gpio-line-names.
	defaultChip = "gpiochip0"
	consumer    = "kubesim_blinkt"
)

type Pix struct {
	red, green, blue, lum int
}

type Blinkt struct {
	pix [4]int
}

// outputPin is the subset of *gpiocdev.Line used here; tests substitute a fake.
type outputPin interface {
	SetValue(value int) error
	Close() error
}

var (
	gpioSetUp   bool = false
	clearOnExit bool = true
	pix         []Pix
	blinkt      [numPx]Blinkt
	dat, clk    outputPin
	lines       string
)

// Exit clears the LEDs (unless disabled) and releases the GPIO lines.
func Exit() error {
	var err error
	if clearOnExit && gpioSetUp {
		Clear()
		err = Show()
	}
	if dat != nil {
		err = errors.Join(err, dat.Close())
	}
	if clk != nil {
		err = errors.Join(err, clk.Close())
	}
	dat, clk = nil, nil
	gpioSetUp = false
	return err
}

func SetLuminance(lum int) {
	for i := range blinkt {
		blinkt[i].pix[lumI] = lum
	}
}

func Clear() {
	for i := range blinkt {
		blinkt[i].pix[redI] = 0
		blinkt[i].pix[greenI] = 0
		blinkt[i].pix[blueI] = 0
	}
}

func pulse() error {
	if err := clk.SetValue(1); err != nil {
		return fmt.Errorf("periBlink: clock high: %w", err)
	}
	if err := clk.SetValue(0); err != nil {
		return fmt.Errorf("periBlink: clock low: %w", err)
	}
	return nil
}

func writeByte(val int) error {
	for i := 0; i < 8; i++ {
		if err := dat.SetValue((val >> 7) & 1); err != nil {
			return fmt.Errorf("periBlink: data: %w", err)
		}
		if err := pulse(); err != nil {
			return err
		}
		val = val << 1
	}
	return nil
}

func clockZeros(n int) error {
	if err := dat.SetValue(0); err != nil {
		return fmt.Errorf("periBlink: data: %w", err)
	}
	for i := 0; i < n; i++ {
		if err := pulse(); err != nil {
			return err
		}
	}
	return nil
}

func eof() error {
	return clockZeros(36)
}

func sof() error {
	return clockZeros(32)
}

// Show writes the current pixel buffer to the LEDs.
func Show() error {
	if !gpioSetUp {
		if err := Setup(); err != nil {
			return err
		}
	}
	if err := sof(); err != nil {
		return err
	}
	for i := range blinkt {
		r := blinkt[i].pix[redI]
		g := blinkt[i].pix[greenI]
		b := blinkt[i].pix[blueI]
		l := blinkt[i].pix[lumI]
		bitwise := 224
		for _, v := range []int{bitwise | l, b, g, r} {
			if err := writeByte(v); err != nil {
				return err
			}
		}
	}
	return eof()
}

func SetAll(r int, g int, b int, l int) {
	for i := 0; i < numPx; i++ {
		SetPixel(i, r&255, g&255, b&255, l&31)
	}
}

func SetPixel(p int, r int, g int, b int, l int) {
	blinkt[p].pix[redI] = r & 255
	blinkt[p].pix[greenI] = g & 255
	blinkt[p].pix[blueI] = b & 255
	blinkt[p].pix[lumI] = l & 31
}

func GetPixel(p int) (r int, g int, b int, l int) {
	r = blinkt[p].pix[redI]
	g = blinkt[p].pix[greenI]
	b = blinkt[p].pix[blueI]
	l = blinkt[p].pix[lumI]
	return r, g, b, l
}

func SetclearOnExit(ce bool) {
	clearOnExit = ce
}

func delay(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

// Setup requests the data and clock lines as outputs.
func Setup() error {
	if gpioSetUp {
		return nil
	}
	d, dDesc, err := requestOutput(datOffset)
	if err != nil {
		return err
	}
	c, cDesc, err := requestOutput(clkOffset)
	if err != nil {
		d.Close()
		return err
	}
	dat, clk = d, c
	lines = fmt.Sprintf("data=%s clock=%s", dDesc, cDesc)
	gpioSetUp = true
	return nil
}

// Lines describes the GPIO lines held since Setup, e.g. "data=gpiochip0:23 clock=gpiochip0:24".
func Lines() string {
	return lines
}
