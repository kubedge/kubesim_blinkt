package pixelconfig

import (
	"strings"
	"testing"
	"time"

	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func cfg(source resourceapi.AllocationConfigSource, params string, requests ...string) resourceapi.DeviceAllocationConfiguration {
	return resourceapi.DeviceAllocationConfiguration{
		Source:   source,
		Requests: requests,
		DeviceConfiguration: resourceapi.DeviceConfiguration{Opaque: &resourceapi.OpaqueDeviceConfiguration{
			Driver: DriverName, Parameters: runtime.RawExtension{Raw: []byte(params)},
		}},
	}
}

const hdr = `"apiVersion":"blinkt.kubedge.io/v1alpha1","kind":"PixelConfig"`

func TestClaimWithColour(t *testing.T) {
	e, err := ForRequest([]resourceapi.DeviceAllocationConfiguration{
		cfg(resourceapi.AllocationConfigSourceClaim, `{`+hdr+`,"color":[0,0,255]}`, "led"),
	}, "led")
	if err != nil || e.R != 0 || e.G != 0 || e.B != 255 {
		t.Fatalf("e = %+v, %v", e, err)
	}
}

func TestDefaults(t *testing.T) {
	e, err := ForRequest([]resourceapi.DeviceAllocationConfiguration{
		cfg(resourceapi.AllocationConfigSourceClaim, `{`+hdr+`,"color":[0,255,0]}`),
	}, "led")
	want := Effective{R: 0, G: 255, B: 0, Intensity: 5, Algorithm: "fixed5", Frequency: time.Second}
	if err != nil || e != want {
		t.Fatalf("e = %+v, want %+v (%v)", e, want, err)
	}
	if e, _ := ForRequest(nil, "led"); e != Defaults {
		t.Errorf("no config = %+v", e)
	}
}

func TestClassThenClaimFieldByField(t *testing.T) {
	// Claim config listed first still wins: precedence is by source.
	e, err := ForRequest([]resourceapi.DeviceAllocationConfiguration{
		cfg(resourceapi.AllocationConfigSourceClaim, `{`+hdr+`,"intensity":10}`),
		cfg(resourceapi.AllocationConfigSourceClass, `{`+hdr+`,"intensity":3,"algorithm":"steady"}`),
	}, "led")
	if err != nil || e.Intensity != 10 || e.Algorithm != "steady" {
		t.Fatalf("e = %+v, %v", e, err)
	}
}

func TestRequestFilterAndOtherDrivers(t *testing.T) {
	other := cfg(resourceapi.AllocationConfigSourceClaim, `{"anything":true}`)
	other.Opaque.Driver = "gpu.example.com"
	e, err := ForRequest([]resourceapi.DeviceAllocationConfiguration{
		other,
		cfg(resourceapi.AllocationConfigSourceClaim, `{`+hdr+`,"color":[1,2,3]}`, "status"),
		cfg(resourceapi.AllocationConfigSourceClaim, `{`+hdr+`,"color":[9,9,9]}`, "led"),
	}, "led/sub")
	if err != nil || e.R != 9 {
		t.Fatalf("e = %+v, %v", e, err)
	}
}

func TestInvalidConfigsNameTheField(t *testing.T) {
	for in, field := range map[string]string{
		`{` + hdr + `,"color":[0,0,300]}`:                                "color[2]",
		`{` + hdr + `,"color":[0,0]}`:                                    "color:",
		`{` + hdr + `,"intensity":32}`:                                   "intensity",
		`{` + hdr + `,"algorithm":"strobe"}`:                             "algorithm",
		`{` + hdr + `,"frequency":0}`:                                    "frequency",
		`{` + hdr + `,"colour":[1,2,3]}`:                                 `unknown field "colour"`,
		`{"apiVersion":"blinkt.kubedge.io/v1alpha1","kind":"GPUConfig"}`: `kind "GPUConfig"`,
		`{"apiVersion":"v1","kind":"PixelConfig"}`:                       `apiVersion "v1"`,
	} {
		_, err := ForRequest([]resourceapi.DeviceAllocationConfiguration{cfg(resourceapi.AllocationConfigSourceClaim, in)}, "led")
		if err == nil || !strings.Contains(err.Error(), field) {
			t.Errorf("%s: err = %v, want it to name %s", in, err, field)
		}
	}
}

func TestPatterns(t *testing.T) {
	ms := time.Millisecond
	steady := Effective{Algorithm: "steady", Frequency: time.Second}
	fixed5 := Effective{Algorithm: "fixed5", Frequency: time.Second}
	fixed := Effective{Algorithm: "fixed", Frequency: 500 * ms}
	for _, c := range []struct {
		e    Effective
		at   time.Duration
		want bool
	}{
		{steady, 123456 * ms, true},
		{fixed5, 0, true}, {fixed5, 999 * ms, true}, {fixed5, 1005 * ms, false}, {fixed5, 1010 * ms, true},
		{fixed, 499 * ms, true}, {fixed, 500 * ms, false}, {fixed, 999 * ms, false}, {fixed, 1000 * ms, true},
	} {
		if got := c.e.Lit(c.at); got != c.want {
			t.Errorf("%s at %v = %v, want %v", c.e.Algorithm, c.at, got, c.want)
		}
	}
}
