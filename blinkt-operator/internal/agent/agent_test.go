package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	resourceapi "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"

	"github.com/kubedge/kubesim_blinkt/go-blinkt/pkg/ledstate"
)

// recorder is a Writer that keeps every frame.
type recorder struct {
	frames []Frame
	closed bool
}

func (r *recorder) Draw(f Frame) error { r.frames = append(r.frames, f); return nil }
func (r *recorder) Close() error       { r.closed = true; return nil }
func (r *recorder) Describe() string   { return "recorder" }
func (r *recorder) last() Frame        { return r.frames[len(r.frames)-1] }

var t0 = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

func newAgent() (*Agent, *recorder) {
	r := &recorder{}
	a := New("home-pi", r, true)
	a.Now = func() time.Time { return t0 }
	return a, r
}

const hdr = `"apiVersion":"blinkt.kubedge.io/v1alpha1","kind":"PixelConfig"`

// claim allocated on pool with one result per device and an optional claim config.
func claim(uid, pool string, reserved bool, params string, devices ...string) *resourceapi.ResourceClaim {
	c := &resourceapi.ResourceClaim{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "c-" + uid, UID: types.UID(uid)}}
	alloc := &resourceapi.AllocationResult{}
	for _, d := range devices {
		alloc.Devices.Results = append(alloc.Devices.Results, resourceapi.DeviceRequestAllocationResult{Request: "led", Driver: DriverName, Pool: pool, Device: d})
	}
	if params != "" {
		alloc.Devices.Config = []resourceapi.DeviceAllocationConfiguration{{
			Source: resourceapi.AllocationConfigSourceClaim,
			DeviceConfiguration: resourceapi.DeviceConfiguration{Opaque: &resourceapi.OpaqueDeviceConfiguration{
				Driver: DriverName, Parameters: runtime.RawExtension{Raw: []byte(params)},
			}},
		}}
	}
	c.Status.Allocation = alloc
	if reserved {
		c.Status.ReservedFor = []resourceapi.ResourceClaimConsumerReference{{Resource: "pods", Name: "p-" + uid, UID: types.UID("pod-" + uid)}}
	}
	return c
}

var (
	green = ledstate.Pixel{G: 255, L: 5}
	blue  = ledstate.Pixel{B: 255, L: 5}
)

func TestTwoClaimsBothLit(t *testing.T) {
	a, r := newAgent()
	a.Sync([]*resourceapi.ResourceClaim{
		claim("elte", "home-pi", true, `{`+hdr+`,"color":[0,255,0],"algorithm":"steady"}`, "pixel-4"),
		claim("lte", "home-pi", true, `{`+hdr+`,"color":[0,0,255],"algorithm":"steady"}`, "pixel-6"),
	})
	if err := a.Tick(t0); err != nil {
		t.Fatal(err)
	}
	if f := r.last(); f[4] != green || f[6] != blue {
		t.Fatalf("frame = %s", FormatFrame(f))
	}
}

func TestIgnoresUnreservedOtherNodesAndDrivers(t *testing.T) {
	a, r := newAgent()
	other := claim("gpu", "home-pi", true, "", "pixel-1")
	other.Status.Allocation.Devices.Results[0].Driver = "gpu.example.com"
	a.Sync([]*resourceapi.ResourceClaim{
		claim("unreserved", "home-pi", false, "", "pixel-0"),
		claim("elsewhere", "nas-pi", true, "", "pixel-2"),
		other,
	})
	a.Tick(t0)
	if f := r.last(); f != (Frame{}) {
		t.Fatalf("frame = %s, want dark", FormatFrame(f))
	}
}

func TestPodDeletedDarkensOnlyItsPixel(t *testing.T) {
	a, r := newAgent()
	lte := claim("lte", "home-pi", true, `{`+hdr+`,"color":[0,0,255],"algorithm":"steady"}`, "pixel-6")
	elte := claim("elte", "home-pi", true, `{`+hdr+`,"color":[0,255,0],"algorithm":"steady"}`, "pixel-4")
	a.Sync([]*resourceapi.ResourceClaim{lte, elte})
	a.Tick(t0)
	lte.Status.ReservedFor = nil // pod gone
	a.Sync([]*resourceapi.ResourceClaim{lte, elte})
	a.Tick(t0.Add(10 * time.Millisecond))
	if f := r.last(); f[6] != (ledstate.Pixel{}) || f[4] != green {
		t.Fatalf("frame = %s", FormatFrame(f))
	}
}

func TestPatternAndRedrawOnlyOnChange(t *testing.T) {
	a, r := newAgent()
	a.Sync([]*resourceapi.ResourceClaim{claim("b", "home-pi", true, `{`+hdr+`,"color":[0,0,255],"algorithm":"fixed","frequency":500}`, "pixel-3")})
	for ms := 0; ms <= 1000; ms += 10 {
		a.Tick(t0.Add(time.Duration(ms) * time.Millisecond))
	}
	// on at 0, off at 500, on at 1000: three draws, not 101.
	if len(r.frames) != 3 || r.frames[0][3] != blue || r.frames[1][3] != (ledstate.Pixel{}) || r.frames[2][3] != blue {
		t.Fatalf("%d frames: %v", len(r.frames), r.frames)
	}
	a.Tick(t0.Add(2500 * time.Millisecond))
	if len(r.frames) != 4 {
		t.Error("unchanged frame not refreshed after 1s")
	}
}

func TestPrepareValidatesAndDrawsWithoutCDI(t *testing.T) {
	a, r := newAgent()
	good := claim("good", "home-pi", true, `{`+hdr+`,"color":[0,255,0],"algorithm":"steady"}`, "pixel-4")
	bad := claim("bad", "home-pi", true, `{`+hdr+`,"color":[0,0,300]}`, "pixel-5")
	none := claim("none", "nas-pi", true, "", "pixel-1")
	for i := 0; i < 2; i++ { // twice: idempotent
		res, err := a.PrepareResourceClaims(context.Background(), []*resourceapi.ResourceClaim{good, bad, none})
		if err != nil {
			t.Fatal(err)
		}
		if g := res["good"]; g.Err != nil || len(g.Devices) != 1 || g.Devices[0].DeviceName != "pixel-4" || len(g.Devices[0].CDIDeviceIDs) != 0 {
			t.Fatalf("good = %+v", g)
		}
		if res["bad"].Err == nil || !strings.Contains(res["bad"].Err.Error(), "color[2]") {
			t.Errorf("bad = %v", res["bad"].Err)
		}
		if res["none"].Err == nil {
			t.Error("claim for another node prepared")
		}
	}
	a.Tick(t0)
	if f := r.last(); f[4] != green || f[5] != (ledstate.Pixel{}) {
		t.Fatalf("frame = %s", FormatFrame(f))
	}
	obj := kubeletplugin.NamespacedObject{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "c-good"}, UID: "good"}
	if res, _ := a.UnprepareResourceClaims(context.Background(), []kubeletplugin.NamespacedObject{obj}); res["good"] != nil {
		t.Fatal(res["good"])
	}
	a.Tick(t0.Add(10 * time.Millisecond))
	if f := r.last(); f != (Frame{}) {
		t.Errorf("frame after unprepare = %s", FormatFrame(f))
	}
}

func TestRestartRebuildsFromClaims(t *testing.T) {
	claims := []*resourceapi.ResourceClaim{
		claim("lte", "home-pi", true, `{`+hdr+`,"color":[0,0,255],"algorithm":"steady"}`, "pixel-6"),
		claim("elte", "home-pi", true, `{`+hdr+`,"color":[0,255,0],"algorithm":"steady"}`, "pixel-4"),
	}
	a, r := newAgent() // a fresh agent: no state but the API listing
	a.Sync(claims)
	a.Tick(t0)
	if f := r.last(); f[4] != green || f[6] != blue {
		t.Fatalf("frame = %s", FormatFrame(f))
	}
}

func TestRunClosesWriterOnShutdown(t *testing.T) {
	a, r := newAgent()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.Run(ctx, time.Millisecond); err != nil || !r.closed {
		t.Fatalf("err=%v closed=%v", err, r.closed)
	}
}

func TestDevicesAndResources(t *testing.T) {
	if d := Devices(); len(d) != 8 || d[6].Name != "pixel-6" || *d[6].Attributes["index"].IntValue != 6 || *d[6].Attributes["model"].StringValue != "pimoroni-blinkt" {
		t.Fatalf("devices = %+v", d)
	}
	if len(Resources("home-pi", false).Pools) != 0 || len(Resources("home-pi", true).Pools["home-pi"].Slices[0].Devices) != 8 {
		t.Error("resources")
	}
}

func TestFakeWriterLogsFrames(t *testing.T) {
	var lines []string
	w := fakeWriter{logf: func(f string, a ...any) { lines = append(lines, strings.TrimSpace(fmt.Sprintf(f, a...))) }}
	var f Frame
	f[4] = green
	w.Draw(f)
	w.Close()
	want := []string{"blinkt-agent: frame [- - - - 0,255,0,5 - - -]", "blinkt-agent: frame [- - - - - - - -]"}
	if len(lines) != 2 || lines[0] != want[0] || lines[1] != want[1] {
		t.Fatalf("lines = %q", lines)
	}
}

func TestUnprepareRemovesStaleCDISpec(t *testing.T) {
	a, _ := newAgent()
	a.CDIDir = t.TempDir()
	stale := filepath.Join(a.CDIDir, "blinkt.kubedge.io-u1.json")
	other := filepath.Join(a.CDIDir, "gpu.example.com-u1.json")
	for _, p := range []string{stale, other} {
		if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	objs := []kubeletplugin.NamespacedObject{
		{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "c-u1"}, UID: "u1"},
		{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "c-u2"}, UID: "u2"}, // no file: fine
	}
	res, err := a.UnprepareResourceClaims(context.Background(), objs)
	if err != nil || res["u1"] != nil || res["u2"] != nil {
		t.Fatalf("res=%v err=%v", res, err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("stale cdi-mode spec not removed")
	}
	if _, err := os.Stat(other); err != nil {
		t.Error("another driver's CDI spec was touched")
	}
}

func TestClearClosesTheWriter(t *testing.T) {
	r := &recorder{}
	if err := Clear(r); err != nil || !r.closed {
		t.Fatalf("err=%v closed=%v", err, r.closed)
	}
	if err := Clear(nil); err != nil {
		t.Errorf("Clear(nil) = %v", err)
	}
}
