package periBlink

import (
	"os"
	"strings"
	"testing"
)

// The Rust port (src/led_output.rs) checks the same fixtures, so both
// implementations clock out identical frames.
func TestFramesMatchSharedFixtures(t *testing.T) {
	cases := map[string]func(){
		"frame_one_pixel.bits":     func() { SetPixel(0, 0x12, 0x34, 0x56, 7) },
		"frame_masked_pixel2.bits": func() { SetPixel(2, 261, -1, 300, 40) },
		"frame_dark.bits":          func() {},
		"frame_lte_elte.bits":      func() { SetPixel(4, 0, 255, 0, 5); SetPixel(6, 0, 0, 255, 5) },
	}
	for name, set := range cases {
		t.Run(name, func(t *testing.T) {
			want, err := os.ReadFile("../../tests/fixtures/" + name)
			if err != nil {
				t.Fatal(err)
			}
			b := install(t)
			set()
			if err := Show(); err != nil {
				t.Fatal(err)
			}
			var got strings.Builder
			for _, v := range b.bits {
				got.WriteByte(byte('0' + v))
			}
			if got.String() != strings.TrimSpace(string(want)) {
				t.Errorf("frame differs from %s", name)
			}
		})
	}
}
