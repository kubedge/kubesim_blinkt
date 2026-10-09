// Package agent is the node component of the operator's agent mode: a DRA
// kubelet plugin for blinkt.kubedge.io that is the only GPIO writer on its
// node and keeps the strip equal to what the ResourceClaims describe.
package agent

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
	"k8s.io/dynamic-resource-allocation/resourceslice"
	drahealth "k8s.io/kubelet/pkg/apis/dra-health/v1alpha1"

	"github.com/kubedge/kubesim_blinkt/blinkt-operator/internal/pixelconfig"
	"github.com/kubedge/kubesim_blinkt/go-blinkt/pkg/ledstate"
)

const (
	// DriverName is shared with dra-driver so claims work in either mode.
	DriverName   = pixelconfig.DriverName
	NumPixels    = 8
	devicePrefix = "pixel-"
	model        = "pimoroni-blinkt"
	// refresh redraws an unchanged frame this often (keeps compat-mode
	// entries alive and repairs any glitch on the strip).
	refresh = time.Second
)

// Devices returns pixel-0..pixel-7 with the same attributes as dra-driver.
func Devices() []resourceapi.Device {
	devs := make([]resourceapi.Device, NumPixels)
	m := model
	for i := range devs {
		idx := int64(i)
		devs[i] = resourceapi.Device{
			Name: devicePrefix + strconv.Itoa(i),
			Attributes: map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{
				"index": {IntValue: &idx},
				"model": {StringValue: &m},
			},
		}
	}
	return devs
}

// Resources is the node's pool: the eight pixels, or nothing without lines.
func Resources(node string, found bool) resourceslice.DriverResources {
	if !found {
		return resourceslice.DriverResources{}
	}
	return resourceslice.DriverResources{Pools: map[string]resourceslice.Pool{
		node: {Slices: []resourceslice.Slice{{Devices: Devices()}}},
	}}
}

func pixelIndex(device string) (int, error) {
	n, err := strconv.Atoi(strings.TrimPrefix(device, devicePrefix))
	if err != nil || !strings.HasPrefix(device, devicePrefix) || n < 0 || n >= NumPixels {
		return 0, fmt.Errorf("unknown device %q", device)
	}
	return n, nil
}

// litPixel is one pixel a claim lights.
type litPixel struct {
	index int
	cfg   pixelconfig.Effective
}

// claimPixels returns the pixels a claim allocates on node, with their
// effective configuration. ok is false when the claim has nothing here.
func claimPixels(node string, c *resourceapi.ResourceClaim) (px []litPixel, ok bool, err error) {
	if c.Status.Allocation == nil {
		return nil, false, nil
	}
	for _, r := range c.Status.Allocation.Devices.Results {
		if r.Driver != DriverName || r.Pool != node {
			continue
		}
		idx, err := pixelIndex(r.Device)
		if err != nil {
			return nil, true, err
		}
		cfg, err := pixelconfig.ForRequest(c.Status.Allocation.Devices.Config, r.Request)
		if err != nil {
			return nil, true, err
		}
		px = append(px, litPixel{index: idx, cfg: cfg})
	}
	return px, len(px) > 0, nil
}

// Agent reconciles the strip from ResourceClaims and serves the kubelet.
type Agent struct {
	drahealth.UnimplementedDRAResourceHealthServer

	Node      string
	Writer    Writer
	Published bool
	Now       func() time.Time

	mu       sync.Mutex
	desired  map[types.UID][]litPixel
	since    map[types.UID]time.Time
	last     *Frame
	lastDraw time.Time
}

var (
	_ kubeletplugin.DRAPlugin           = (*Agent)(nil)
	_ drahealth.DRAResourceHealthServer = (*Agent)(nil)
)

// New returns an agent drawing through w.
func New(node string, w Writer, published bool) *Agent {
	return &Agent{
		Node: node, Writer: w, Published: published, Now: time.Now,
		desired: map[types.UID][]litPixel{}, since: map[types.UID]time.Time{},
	}
}

// Sync replaces the desired state with the claims that are allocated on this
// node and reserved by a pod. Claims with invalid configuration are skipped
// (their prepare already failed).
func (a *Agent) Sync(claims []*resourceapi.ResourceClaim) {
	desired := map[types.UID][]litPixel{}
	for _, c := range claims {
		if len(c.Status.ReservedFor) == 0 {
			continue
		}
		px, ok, err := claimPixels(a.Node, c)
		if !ok {
			continue
		}
		if err != nil {
			log.Printf("blinkt-agent: claim %s/%s not drawn: %v", c.Namespace, c.Name, err)
			continue
		}
		desired[c.UID] = px
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.desired = desired
	a.pruneSince()
}

// pruneSince forgets start times of claims no longer desired; new claims
// start their pattern now. Caller holds mu.
func (a *Agent) pruneSince() {
	for uid := range a.since {
		if _, ok := a.desired[uid]; !ok {
			delete(a.since, uid)
		}
	}
	for uid := range a.desired {
		if _, ok := a.since[uid]; !ok {
			a.since[uid] = a.Now()
		}
	}
}

// Frame computes the strip at now. Claims apply in UID order so a pixel
// claimed twice (which the scheduler prevents) still renders deterministically.
func (a *Agent) Frame(now time.Time) Frame {
	a.mu.Lock()
	defer a.mu.Unlock()
	uids := make([]string, 0, len(a.desired))
	for uid := range a.desired {
		uids = append(uids, string(uid))
	}
	sort.Strings(uids)
	var f Frame
	for _, uid := range uids {
		elapsed := now.Sub(a.since[types.UID(uid)])
		for _, p := range a.desired[types.UID(uid)] {
			if p.cfg.Lit(elapsed) {
				f[p.index] = ledstate.Pixel{R: p.cfg.R, G: p.cfg.G, B: p.cfg.B, L: p.cfg.Intensity}
			}
		}
	}
	return f
}

// Tick draws the frame for now if it changed, or if refresh elapsed.
func (a *Agent) Tick(now time.Time) error {
	f := a.Frame(now)
	a.mu.Lock()
	unchanged := a.last != nil && *a.last == f && now.Sub(a.lastDraw) < refresh
	a.mu.Unlock()
	if unchanged || a.Writer == nil {
		return nil
	}
	if err := a.Writer.Draw(f); err != nil {
		return err
	}
	a.mu.Lock()
	a.last, a.lastDraw = &f, now
	a.mu.Unlock()
	return nil
}

// Run draws every tick until ctx is done, then closes the writer, which
// leaves the strip dark.
func (a *Agent) Run(ctx context.Context, tick time.Duration) error {
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			if a.Writer != nil {
				return a.Writer.Close()
			}
			return nil
		case <-t.C:
			if err := a.Tick(a.Now()); err != nil {
				log.Printf("blinkt-agent: draw: %v", err)
			}
		}
	}
}

// PrepareResourceClaims validates each claim's PixelConfig and starts
// drawing it at once (the informer will agree). No CDI devices: the pod
// gets nothing from the agent.
func (a *Agent) PrepareResourceClaims(_ context.Context, claims []*resourceapi.ResourceClaim) (map[types.UID]kubeletplugin.PrepareResult, error) {
	out := make(map[types.UID]kubeletplugin.PrepareResult, len(claims))
	for _, c := range claims {
		px, ok, err := claimPixels(a.Node, c)
		switch {
		case err != nil:
			out[c.UID] = kubeletplugin.PrepareResult{Err: fmt.Errorf("claim %s/%s: %w", c.Namespace, c.Name, err)}
			continue
		case !ok:
			out[c.UID] = kubeletplugin.PrepareResult{Err: fmt.Errorf("claim %s/%s has no %s devices on node %s", c.Namespace, c.Name, DriverName, a.Node)}
			continue
		}
		var devices []kubeletplugin.Device
		for _, r := range c.Status.Allocation.Devices.Results {
			if r.Driver == DriverName && r.Pool == a.Node {
				devices = append(devices, kubeletplugin.Device{Requests: []string{r.Request}, PoolName: r.Pool, DeviceName: r.Device})
			}
		}
		a.mu.Lock()
		a.desired[c.UID] = px
		a.pruneSince()
		a.mu.Unlock()
		out[c.UID] = kubeletplugin.PrepareResult{Devices: devices}
		log.Printf("blinkt-agent: prepared claim=%s/%s uid=%s pixels=%v", c.Namespace, c.Name, c.UID, indices(px))
	}
	return out, nil
}

// UnprepareResourceClaims stops drawing the claims at once.
func (a *Agent) UnprepareResourceClaims(_ context.Context, claims []kubeletplugin.NamespacedObject) (map[types.UID]error, error) {
	out := make(map[types.UID]error, len(claims))
	a.mu.Lock()
	for _, c := range claims {
		delete(a.desired, c.UID)
		out[c.UID] = nil
	}
	a.pruneSince()
	a.mu.Unlock()
	for _, c := range claims {
		log.Printf("blinkt-agent: unprepared claim=%s", c)
	}
	return out, nil
}

func (a *Agent) HandleError(_ context.Context, err error, msg string) {
	log.Printf("blinkt-agent: %s: %v", msg, err)
}

// NodeWatchResources reports the published pixels healthy once and keeps
// the stream open (the strip has no health signal).
func (a *Agent) NodeWatchResources(_ *drahealth.NodeWatchResourcesRequest, stream grpc.ServerStreamingServer[drahealth.NodeWatchResourcesResponse]) error {
	resp := &drahealth.NodeWatchResourcesResponse{}
	if a.Published {
		now := time.Now().Unix()
		for i := 0; i < NumPixels; i++ {
			resp.Devices = append(resp.Devices, &drahealth.DeviceHealth{
				Device:          &drahealth.DeviceIdentifier{PoolName: a.Node, DeviceName: devicePrefix + strconv.Itoa(i)},
				Health:          drahealth.HealthStatus_HEALTHY,
				LastUpdatedTime: now,
			})
		}
	}
	if err := stream.Send(resp); err != nil {
		return err
	}
	<-stream.Context().Done()
	return nil
}

func indices(px []litPixel) []int {
	out := make([]int, len(px))
	for i, p := range px {
		out[i] = p.index
	}
	sort.Ints(out)
	return out
}
