package driver

import (
	"strconv"
	"time"

	"google.golang.org/grpc"
	drahealth "k8s.io/kubelet/pkg/apis/dra-health/v1alpha1"
)

// NodeWatchResources reports the published pixels as healthy once, then
// keeps the stream open. The Blinkt! is write-only, so there is no health
// signal to watch; answering stops the kubelet from logging an
// "unknown service DRAResourceHealth" error every few seconds.
func (d *Driver) NodeWatchResources(_ *drahealth.NodeWatchResourcesRequest, stream grpc.ServerStreamingServer[drahealth.NodeWatchResourcesResponse]) error {
	resp := &drahealth.NodeWatchResourcesResponse{}
	if d.Published {
		now := time.Now().Unix()
		for i := 0; i < NumPixels; i++ {
			resp.Devices = append(resp.Devices, &drahealth.DeviceHealth{
				Device:          &drahealth.DeviceIdentifier{PoolName: d.Node, DeviceName: devicePrefix + strconv.Itoa(i)},
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
