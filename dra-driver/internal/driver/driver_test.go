package driver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/grpc"
	resourceapi "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
	drahealth "k8s.io/kubelet/pkg/apis/dra-health/v1alpha1"

	"github.com/kubedge/kubesim_blinkt/dra-driver/internal/cdi"
)

func newDriver(t *testing.T) *Driver {
	return &Driver{Node: "home-pi", CDI: cdi.Writer{Dir: t.TempDir(), GPIODevice: "/dev/gpiochip0", StateDir: "/etc/kubedge"}}
}

func claim(uid string, results ...resourceapi.DeviceRequestAllocationResult) *resourceapi.ResourceClaim {
	c := &resourceapi.ResourceClaim{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "c-" + uid, UID: types.UID(uid)}}
	c.Status.Allocation = &resourceapi.AllocationResult{Devices: resourceapi.DeviceAllocationResult{Results: results}}
	return c
}

func result(req, pool, dev string) resourceapi.DeviceRequestAllocationResult {
	return resourceapi.DeviceRequestAllocationResult{Request: req, Driver: DriverName, Pool: pool, Device: dev}
}

func env(t *testing.T, d *Driver, uid string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(d.CDI.Dir, "blinkt.kubedge.io-"+uid+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestDevices(t *testing.T) {
	devs := Devices()
	if len(devs) != 8 {
		t.Fatalf("%d devices", len(devs))
	}
	for i, d := range devs {
		if d.Name != "pixel-"+string(rune('0'+i)) || *d.Attributes["index"].IntValue != int64(i) || *d.Attributes["model"].StringValue != "pimoroni-blinkt" {
			t.Errorf("device %d = %+v", i, d)
		}
	}
	if got := Resources("home-pi", true).Pools["home-pi"].Slices[0].Devices; len(got) != 8 {
		t.Errorf("pool devices = %d", len(got))
	}
	if got := Resources("home-pi", false); len(got.Pools) != 0 {
		t.Errorf("no-GPIO node publishes %v", got.Pools)
	}
}

func TestPrepareOnePixel(t *testing.T) {
	d := newDriver(t)
	res, err := d.PrepareResourceClaims(context.Background(), []*resourceapi.ResourceClaim{claim("u1", result("led", "home-pi", "pixel-6"))})
	if err != nil {
		t.Fatal(err)
	}
	r := res["u1"]
	if r.Err != nil || len(r.Devices) != 1 {
		t.Fatalf("result = %+v", r)
	}
	if got := r.Devices[0]; got.DeviceName != "pixel-6" || got.PoolName != "home-pi" || got.Requests[0] != "led" || got.CDIDeviceIDs[0] != "blinkt.kubedge.io/claim=u1" {
		t.Errorf("device = %+v", got)
	}
	if !strings.Contains(env(t, d, "u1"), `"BLINKT_PIXELS=6"`) {
		t.Error("CDI spec lacks BLINKT_PIXELS=6")
	}
}

func TestPrepareAllPixelsSortedWithOneCDIDevice(t *testing.T) {
	d := newDriver(t)
	var rs []resourceapi.DeviceRequestAllocationResult
	for _, i := range []string{"7", "0", "3", "1", "2", "6", "5", "4"} {
		rs = append(rs, result("all", "home-pi", "pixel-"+i))
	}
	res, _ := d.PrepareResourceClaims(context.Background(), []*resourceapi.ResourceClaim{claim("u8", rs...)})
	r := res["u8"]
	if r.Err != nil || len(r.Devices) != 8 {
		t.Fatalf("result = %+v", r)
	}
	ids := 0
	for _, dev := range r.Devices {
		ids += len(dev.CDIDeviceIDs)
	}
	if ids != 1 {
		t.Errorf("%d CDI ids across devices, want 1", ids)
	}
	if !strings.Contains(env(t, d, "u8"), `"BLINKT_PIXELS=0,1,2,3,4,5,6,7"`) {
		t.Error("pixels not sorted in CDI spec")
	}
}

func TestPrepareIgnoresOtherDriversAndNodes(t *testing.T) {
	d := newDriver(t)
	other := result("x", "home-pi", "gpu-0")
	other.Driver = "gpu.example.com"
	res, _ := d.PrepareResourceClaims(context.Background(), []*resourceapi.ResourceClaim{
		claim("mixed", other, result("led", "home-pi", "pixel-2")),
		claim("elsewhere", result("led", "nas-pi", "pixel-2")),
	})
	if r := res["mixed"]; r.Err != nil || len(r.Devices) != 1 || r.Devices[0].DeviceName != "pixel-2" {
		t.Errorf("mixed = %+v", r)
	}
	if res["elsewhere"].Err == nil {
		t.Error("claim for another node prepared")
	}
}

func TestPrepareErrors(t *testing.T) {
	d := newDriver(t)
	unalloc := claim("none")
	unalloc.Status.Allocation = nil
	res, _ := d.PrepareResourceClaims(context.Background(), []*resourceapi.ResourceClaim{
		unalloc, claim("bad", result("led", "home-pi", "pixel-9")),
	})
	if res["none"].Err == nil || res["bad"].Err == nil {
		t.Errorf("errors = %v / %v", res["none"].Err, res["bad"].Err)
	}
}

func TestPrepareTwiceAndUnprepareTwice(t *testing.T) {
	d := newDriver(t)
	c := claim("u", result("led", "home-pi", "pixel-1"))
	for i := 0; i < 2; i++ {
		if res, _ := d.PrepareResourceClaims(context.Background(), []*resourceapi.ResourceClaim{c}); res["u"].Err != nil {
			t.Fatal(res["u"].Err)
		}
	}
	// A restarted driver has no memory of prepared claims: unprepare works
	// from the claim UID alone.
	restarted := &Driver{Node: d.Node, CDI: d.CDI}
	obj := kubeletplugin.NamespacedObject{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "c-u"}, UID: "u"}
	for i := 0; i < 2; i++ {
		res, err := restarted.UnprepareResourceClaims(context.Background(), []kubeletplugin.NamespacedObject{obj})
		if err != nil || res["u"] != nil {
			t.Fatalf("unprepare %d: %v %v", i, err, res["u"])
		}
	}
	if left, _ := os.ReadDir(d.CDI.Dir); len(left) != 0 {
		t.Errorf("leaked CDI files: %v", left)
	}
}

type healthStream struct {
	grpc.ServerStream
	ctx  context.Context
	sent []*drahealth.NodeWatchResourcesResponse
}

func (s *healthStream) Send(r *drahealth.NodeWatchResourcesResponse) error {
	s.sent = append(s.sent, r)
	return nil
}
func (s *healthStream) Context() context.Context { return s.ctx }

func TestHealthReportsPublishedPixelsHealthy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // stream already closed: NodeWatchResources returns after the first send
	for _, published := range []bool{true, false} {
		d := newDriver(t)
		d.Published = published
		s := &healthStream{ctx: ctx}
		if err := d.NodeWatchResources(&drahealth.NodeWatchResourcesRequest{}, s); err != nil {
			t.Fatal(err)
		}
		devs := s.sent[0].Devices
		if !published {
			if len(devs) != 0 {
				t.Errorf("unpublished node reports %d devices", len(devs))
			}
			continue
		}
		if len(devs) != 8 || devs[6].Device.DeviceName != "pixel-6" || devs[6].Device.PoolName != "home-pi" || devs[6].Health != drahealth.HealthStatus_HEALTHY {
			t.Errorf("health = %v", devs)
		}
	}
}

func TestCheckAPI(t *testing.T) {
	old := fake.NewSimpleClientset()
	old.Resources = []*metav1.APIResourceList{{GroupVersion: "resource.k8s.io/v1beta1"}}
	if err := CheckAPI(old.Discovery()); err == nil || !strings.Contains(err.Error(), "resource.k8s.io/v1") {
		t.Errorf("cluster without v1: err = %v", err)
	}
	cur := fake.NewSimpleClientset()
	cur.Resources = []*metav1.APIResourceList{{GroupVersion: "resource.k8s.io/v1", APIResources: []metav1.APIResource{{Name: "resourceslices"}}}}
	if err := CheckAPI(cur.Discovery()); err != nil {
		t.Errorf("cluster with v1: %v", err)
	}
}
