package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathDefault(t *testing.T) {
	t.Setenv("BLINKT_CONFIG", "")
	if got := Path(); got != DefaultPath {
		t.Errorf("Path() = %q, want %q", got, DefaultPath)
	}
}

func TestConfigFromEnvPath(t *testing.T) {
	f := filepath.Join(t.TempDir(), "blinkt.yaml")
	data := "algorithm: fixed5\nintensity: 3\nfrequency: 500\npixel0: [255, 0, 0]\npixel1: []\n"
	if err := os.WriteFile(f, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BLINKT_CONFIG", f)

	var c BlinktConfigData
	c.Config()
	if c.Algorithm != "fixed5" || c.Intensity != 3 || c.Frequency != 500 {
		t.Errorf("got %+v", c)
	}
	if len(c.Pixel0) != 3 || c.Pixel0[0] != 255 || len(c.Pixel1) != 0 {
		t.Errorf("pixels: %v %v", c.Pixel0, c.Pixel1)
	}
}
