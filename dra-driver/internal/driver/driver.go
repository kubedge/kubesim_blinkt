// Package driver is the blinkt.kubedge.io DRA kubelet plugin: it advertises
// a node's eight Blinkt! LEDs as devices pixel-0..pixel-7 and prepares
// allocated claims through CDI. It never draws: the workload's blinkt
// process does, through the shared LED state the CDI spec mounts.
package driver

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
	"k8s.io/dynamic-resource-allocation/resourceslice"
	drahealth "k8s.io/kubelet/pkg/apis/dra-health/v1alpha1"

	"github.com/kubedge/kubesim_blinkt/dra-driver/internal/cdi"
)

const (
	// DriverName is what claims and the DeviceClass select on.
	DriverName = "blinkt.kubedge.io"
	// NumPixels is the number of LEDs on a Blinkt!.
	NumPixels    = 8
	devicePrefix = "pixel-"
	model        = "pimoroni-blinkt"
)

// Devices returns pixel-0..pixel-7 with their index and model attributes.
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

// Resources is what the node publishes: one pool named after the node with
// the eight pixels, or nothing when the node has no Blinkt! lines.
func Resources(node string, found bool) resourceslice.DriverResources {
	if !found {
		return resourceslice.DriverResources{}
	}
	return resourceslice.DriverResources{
		Pools: map[string]resourceslice.Pool{
			node: {Slices: []resourceslice.Slice{{Devices: Devices()}}},
		},
	}
}

// Driver implements kubeletplugin.DRAPlugin and the kubelet's device health
// stream.
type Driver struct {
	drahealth.UnimplementedDRAResourceHealthServer
	Node      string
	CDI       cdi.Writer
	Published bool // the node's pixels are published
}

var (
	_ kubeletplugin.DRAPlugin           = (*Driver)(nil)
	_ drahealth.DRAResourceHealthServer = (*Driver)(nil)
)

// pixelIndex parses "pixel-N".
func pixelIndex(device string) (int, error) {
	n, err := strconv.Atoi(strings.TrimPrefix(device, devicePrefix))
	if err != nil || !strings.HasPrefix(device, devicePrefix) || n < 0 || n >= NumPixels {
		return 0, fmt.Errorf("unknown device %q", device)
	}
	return n, nil
}

// prepare handles one claim: the pixels allocated to it on this node become
// one CDI device attached to the claim's first device result.
func (d *Driver) prepare(claim *resourceapi.ResourceClaim) kubeletplugin.PrepareResult {
	if claim.Status.Allocation == nil {
		return kubeletplugin.PrepareResult{Err: fmt.Errorf("claim %s/%s is not allocated", claim.Namespace, claim.Name)}
	}
	var devices []kubeletplugin.Device
	var pixels []int
	for _, r := range claim.Status.Allocation.Devices.Results {
		if r.Driver != DriverName || r.Pool != d.Node {
			continue
		}
		idx, err := pixelIndex(r.Device)
		if err != nil {
			return kubeletplugin.PrepareResult{Err: err}
		}
		pixels = append(pixels, idx)
		devices = append(devices, kubeletplugin.Device{
			Requests:   []string{r.Request},
			PoolName:   r.Pool,
			DeviceName: r.Device,
		})
	}
	if len(devices) == 0 {
		return kubeletplugin.PrepareResult{Err: fmt.Errorf("claim %s/%s has no %s devices on node %s", claim.Namespace, claim.Name, DriverName, d.Node)}
	}
	sort.Ints(pixels)
	uid := string(claim.UID)
	if err := d.CDI.Write(uid, pixels); err != nil {
		return kubeletplugin.PrepareResult{Err: err}
	}
	// One CDI device carries all the claim's pixels; listing it once keeps
	// the runtime from applying the same edits twice.
	devices[0].CDIDeviceIDs = []string{cdi.DeviceID(uid)}
	log.Printf("blinkt-dra: prepared claim=%s/%s uid=%s pixels=%v", claim.Namespace, claim.Name, uid, pixels)
	return kubeletplugin.PrepareResult{Devices: devices}
}

func (d *Driver) PrepareResourceClaims(_ context.Context, claims []*resourceapi.ResourceClaim) (map[types.UID]kubeletplugin.PrepareResult, error) {
	out := make(map[types.UID]kubeletplugin.PrepareResult, len(claims))
	for _, c := range claims {
		out[c.UID] = d.prepare(c)
	}
	return out, nil
}

func (d *Driver) UnprepareResourceClaims(_ context.Context, claims []kubeletplugin.NamespacedObject) (map[types.UID]error, error) {
	out := make(map[types.UID]error, len(claims))
	for _, c := range claims {
		out[c.UID] = d.CDI.Remove(string(c.UID))
		if out[c.UID] == nil {
			log.Printf("blinkt-dra: unprepared claim=%s", c)
		}
	}
	return out, nil
}

func (d *Driver) HandleError(_ context.Context, err error, msg string) {
	log.Printf("blinkt-dra: %s: %v", msg, err)
}

// ResourceAPI is the DRA API version the driver needs (Kubernetes >= 1.34).
const ResourceAPI = "resource.k8s.io/v1"

// CheckAPI fails unless the API server serves resource.k8s.io/v1.
func CheckAPI(disc discovery.DiscoveryInterface) error {
	if _, err := disc.ServerResourcesForGroupVersion(ResourceAPI); err != nil {
		return fmt.Errorf("API server does not serve %s (DRA needs Kubernetes >= 1.34): %w", ResourceAPI, err)
	}
	return nil
}
