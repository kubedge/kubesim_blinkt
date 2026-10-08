// blinkt-dra-driver is the node-side DRA driver blinkt.kubedge.io: it runs as
// a DaemonSet kubelet plugin, publishes the node's Blinkt! pixels as devices
// and prepares claims through CDI.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"

	"github.com/kubedge/kubesim_blinkt/dra-driver/internal/cdi"
	"github.com/kubedge/kubesim_blinkt/dra-driver/internal/driver"
	"github.com/kubedge/kubesim_blinkt/dra-driver/internal/gpio"
)

func main() {
	nodeName := flag.String("node-name", os.Getenv("NODE_NAME"), "node this plugin serves (default $NODE_NAME)")
	kubeconfig := flag.String("kubeconfig", "", "kubeconfig file; in-cluster config when empty")
	stateDir := flag.String("state-dir", "/etc/kubedge", "host directory shared by blinkt processes, mounted at "+cdi.ContainerStateDir)
	cdiDir := flag.String("cdi-dir", "/var/run/cdi", "directory for per-claim CDI spec files")
	gpioDevice := flag.String("gpio-device", "/dev/gpiochip0", "device node injected into claiming containers")
	assumeGPIO := flag.Bool("assume-gpio", false, "publish pixels without checking for GPIO23/24 (test clusters only)")
	flag.Parse()
	log.SetFlags(log.LstdFlags)

	if *nodeName == "" {
		log.Fatal("blinkt-dra: --node-name or NODE_NAME is required")
	}
	client, err := newClient(*kubeconfig)
	if err != nil {
		log.Fatalf("blinkt-dra: kubernetes client: %v", err)
	}
	if err := driver.CheckAPI(client.Discovery()); err != nil {
		log.Fatalf("blinkt-dra: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	node, err := client.CoreV1().Nodes().Get(ctx, *nodeName, metav1.GetOptions{})
	if err != nil {
		log.Fatalf("blinkt-dra: get node %s: %v", *nodeName, err)
	}

	found := gpio.Result{Found: true, Chip: "assumed (--assume-gpio)"}
	if !*assumeGPIO {
		found = gpio.Discover(gpio.System{})
	}

	// The helper listens on <plugins dir>/<driver>/dra.sock but does not
	// create the directory.
	pluginDir := filepath.Join(kubeletplugin.KubeletPluginsDir, driver.DriverName)
	if err := os.MkdirAll(pluginDir, 0o750); err != nil {
		log.Fatalf("blinkt-dra: %v", err)
	}

	drv := &driver.Driver{
		Node:      *nodeName,
		CDI:       cdi.Writer{Dir: *cdiDir, GPIODevice: *gpioDevice, StateDir: *stateDir},
		Published: found.Found,
	}
	helper, err := kubeletplugin.Start(ctx, drv,
		kubeletplugin.DriverName(driver.DriverName),
		kubeletplugin.KubeClient(client),
		kubeletplugin.NodeName(*nodeName),
		kubeletplugin.NodeUID(node.UID),
	)
	if err != nil {
		log.Fatalf("blinkt-dra: start kubelet plugin: %v", err)
	}
	defer helper.Stop()

	if err := waitRegistered(ctx, helper); err != nil {
		log.Fatalf("blinkt-dra: %v", err)
	}
	log.Printf("blinkt-dra: registered driver=%s node=%s", driver.DriverName, *nodeName)

	if err := helper.PublishResources(ctx, driver.Resources(*nodeName, found.Found)); err != nil {
		log.Fatalf("blinkt-dra: publish resources: %v", err)
	}
	if found.Found {
		log.Printf("blinkt-dra: published devices=%d node=%s chip=%s", driver.NumPixels, *nodeName, found.Chip)
	} else {
		log.Printf("blinkt-dra: published devices=0 node=%s reason=%s", *nodeName, found.Reason)
	}

	<-ctx.Done()
	log.Printf("blinkt-dra: stopping")
}

func newClient(kubeconfig string) (kubernetes.Interface, error) {
	var cfg *rest.Config
	var err error
	if kubeconfig != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		cfg, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}

// waitRegistered blocks until the kubelet has accepted the plugin.
func waitRegistered(ctx context.Context, h *kubeletplugin.Helper) error {
	deadline := time.After(2 * time.Minute)
	for {
		if s := h.RegistrationStatus(); s != nil && s.PluginRegistered {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("kubelet did not register plugin %s within 2m (is %s mounted?)", driver.DriverName, kubeletplugin.KubeletRegistryDir)
		case <-time.After(time.Second):
		}
	}
}
