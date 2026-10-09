// Package pixelconfig decodes, validates and resolves the PixelConfig a
// blinkt.kubedge.io claim (or its DeviceClass) carries as opaque device
// configuration, and computes when a configured pixel is lit.
package pixelconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	resourceapi "k8s.io/api/resource/v1"

	blinktv1 "github.com/kubedge/kubesim_blinkt/blinkt-operator/api/v1alpha1"
)

// DriverName is the DRA driver PixelConfig is addressed to.
const DriverName = "blinkt.kubedge.io"

// Defaults apply to every field a configuration omits.
var Defaults = Effective{R: 255, G: 255, B: 255, Intensity: 5, Algorithm: "fixed5", Frequency: time.Second}

// darkFixed5 is how long fixed5 leaves the pixel dark between blinks.
const darkFixed5 = 10 * time.Millisecond

// Effective is a fully resolved pixel configuration.
type Effective struct {
	R, G, B   int
	Intensity int
	Algorithm string
	Frequency time.Duration
}

// Decode parses and validates opaque parameters. Unknown fields, a wrong
// apiVersion/kind and out-of-range values are errors naming the field.
func Decode(raw []byte) (blinktv1.PixelConfigSpec, error) {
	var pc blinktv1.PixelConfig
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&pc); err != nil {
		return blinktv1.PixelConfigSpec{}, fmt.Errorf("PixelConfig: %w", err)
	}
	if pc.APIVersion != blinktv1.GroupVersion.String() {
		return blinktv1.PixelConfigSpec{}, fmt.Errorf("PixelConfig: unsupported apiVersion %q, want %q", pc.APIVersion, blinktv1.GroupVersion.String())
	}
	if pc.Kind != blinktv1.PixelConfigKind {
		return blinktv1.PixelConfigSpec{}, fmt.Errorf("PixelConfig: unsupported kind %q, want %q", pc.Kind, blinktv1.PixelConfigKind)
	}
	if err := Validate(pc.PixelConfigSpec); err != nil {
		return blinktv1.PixelConfigSpec{}, err
	}
	return pc.PixelConfigSpec, nil
}

// Validate checks field ranges.
func Validate(s blinktv1.PixelConfigSpec) error {
	if s.Color != nil {
		if len(s.Color) != 3 {
			return fmt.Errorf("PixelConfig: color: want 3 values [r, g, b], got %d", len(s.Color))
		}
		for i, v := range s.Color {
			if v < 0 || v > 255 {
				return fmt.Errorf("PixelConfig: color[%d]: value %d out of range 0-255", i, v)
			}
		}
	}
	if s.Intensity != nil && (*s.Intensity < 0 || *s.Intensity > 31) {
		return fmt.Errorf("PixelConfig: intensity: value %d out of range 0-31", *s.Intensity)
	}
	switch s.Algorithm {
	case "", "steady", "fixed5", "fixed":
	default:
		return fmt.Errorf("PixelConfig: algorithm: %q is not steady, fixed5 or fixed", s.Algorithm)
	}
	if s.Frequency != nil && *s.Frequency <= 0 {
		return fmt.Errorf("PixelConfig: frequency: value %d must be > 0 ms", *s.Frequency)
	}
	return nil
}

// Apply overlays the fields s sets onto e.
func (e Effective) Apply(s blinktv1.PixelConfigSpec) Effective {
	if s.Color != nil {
		e.R, e.G, e.B = int(s.Color[0]), int(s.Color[1]), int(s.Color[2])
	}
	if s.Intensity != nil {
		e.Intensity = int(*s.Intensity)
	}
	if s.Algorithm != "" {
		e.Algorithm = s.Algorithm
	}
	if s.Frequency != nil {
		e.Frequency = time.Duration(*s.Frequency) * time.Millisecond
	}
	return e
}

// requestMatches reports whether a config listing requests applies to an
// allocation result for request (which may name a subrequest "req/sub").
func requestMatches(requests []string, request string) bool {
	if len(requests) == 0 {
		return true
	}
	base, _, _ := strings.Cut(request, "/")
	for _, r := range requests {
		if r == request || r == base {
			return true
		}
	}
	return false
}

// ForRequest resolves the configuration for one allocated request: defaults,
// then the DeviceClass configs, then the claim's configs.
func ForRequest(configs []resourceapi.DeviceAllocationConfiguration, request string) (Effective, error) {
	e := Defaults
	for _, source := range []resourceapi.AllocationConfigSource{resourceapi.AllocationConfigSourceClass, resourceapi.AllocationConfigSourceClaim} {
		for _, c := range configs {
			if c.Source != source || c.Opaque == nil || c.Opaque.Driver != DriverName || !requestMatches(c.Requests, request) {
				continue
			}
			s, err := Decode(c.Opaque.Parameters.Raw)
			if err != nil {
				return Effective{}, err
			}
			e = e.Apply(s)
		}
	}
	return e, nil
}

// Lit reports whether a pixel with this configuration is on, elapsed after
// it was first lit.
func (e Effective) Lit(elapsed time.Duration) bool {
	switch e.Algorithm {
	case "fixed5":
		return elapsed%(e.Frequency+darkFixed5) < e.Frequency
	case "fixed":
		return elapsed%(2*e.Frequency) < e.Frequency
	default: // steady
		return true
	}
}
