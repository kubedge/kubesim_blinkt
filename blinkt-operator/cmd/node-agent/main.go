// node-agent is the blinkt-operator's agent-mode node component: the DRA
// kubelet plugin blinkt.kubedge.io that is the only GPIO writer on its node
// and draws the strip from the ResourceClaims allocated there.
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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"

	"github.com/kubedge/kubesim_blinkt/blinkt-operator/internal/agent"
)

func main() {
	nodeName := flag.String("node-name", os.Getenv("NODE_NAME"), "node this agent serves (default $NODE_NAME)")
	kubeconfig := flag.String("kubeconfig", "", "kubeconfig file; in-cluster config when empty")
	legacyCompat := flag.Bool("legacy-compat", false, "share the strip with legacy blinkt sidecars through the shared-state file")
	stateDir := flag.String("state-dir", "/etc/kubedge", "shared-state directory used with --legacy-compat")
	fakeGPIO := flag.Bool("fake-gpio", false, "log frames instead of driving GPIO (test clusters only)")
	cdiDir := flag.String("cdi-dir", "/var/run/cdi", "where dra-driver (cdi mode) wrote claim specs; leftovers are removed on unprepare")
	clearOnly := flag.Bool("clear", false, "clear the agent's pixels (dark frame, or withdraw with --legacy-compat), then exit; used by the operator when a node leaves the agent")
	flag.Parse()
	log.SetFlags(log.LstdFlags)

	if *clearOnly {
		w, err := newWriter(*fakeGPIO, *legacyCompat, *stateDir)
		if err != nil {
			log.Fatalf("blinkt-agent: clear: %v", err)
		}
		if err := agent.Clear(w); err != nil {
			log.Fatalf("blinkt-agent: clear: %v", err)
		}
		log.Printf("blinkt-agent: cleared strip (%s)", w.Describe())
		return
	}

	if *nodeName == "" {
		log.Fatal("blinkt-agent: --node-name or NODE_NAME is required")
	}
	client, err := newClient(*kubeconfig)
	if err != nil {
		log.Fatalf("blinkt-agent: kubernetes client: %v", err)
	}
	if _, err := client.Discovery().ServerResourcesForGroupVersion("resource.k8s.io/v1"); err != nil {
		log.Fatalf("blinkt-agent: API server does not serve resource.k8s.io/v1 (DRA needs Kubernetes >= 1.34): %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	node, err := client.CoreV1().Nodes().Get(ctx, *nodeName, metav1.GetOptions{})
	if err != nil {
		log.Fatalf("blinkt-agent: get node %s: %v", *nodeName, err)
	}

	// Acquiring the lines is the hardware discovery.
	var reason string
	writer, err := newWriter(*fakeGPIO, *legacyCompat, *stateDir)
	if err != nil {
		reason = err.Error()
		writer = nil
	}

	a := agent.New(*nodeName, writer, writer != nil)
	a.CDIDir = *cdiDir

	pluginDir := filepath.Join(kubeletplugin.KubeletPluginsDir, agent.DriverName)
	if err := os.MkdirAll(pluginDir, 0o750); err != nil {
		log.Fatalf("blinkt-agent: %v", err)
	}
	helper, err := kubeletplugin.Start(ctx, a,
		kubeletplugin.DriverName(agent.DriverName),
		kubeletplugin.KubeClient(client),
		kubeletplugin.NodeName(*nodeName),
		kubeletplugin.NodeUID(node.UID),
	)
	if err != nil {
		log.Fatalf("blinkt-agent: start kubelet plugin: %v", err)
	}
	defer helper.Stop()
	if err := waitRegistered(ctx, helper); err != nil {
		log.Fatalf("blinkt-agent: %v", err)
	}
	log.Printf("blinkt-agent: registered driver=%s node=%s", agent.DriverName, *nodeName)

	// Desired state: every ResourceClaim; the agent keeps the ones
	// allocated on this node and reserved by a pod.
	factory := informers.NewSharedInformerFactory(client, 10*time.Minute)
	claims := factory.Resource().V1().ResourceClaims()
	resync := func() {
		list, err := claims.Lister().List(labels.Everything())
		if err != nil {
			log.Printf("blinkt-agent: list claims: %v", err)
			return
		}
		a.Sync(list)
	}
	if _, err := claims.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(any) { resync() },
		UpdateFunc: func(any, any) { resync() },
		DeleteFunc: func(any) { resync() },
	}); err != nil {
		log.Fatalf("blinkt-agent: claim informer: %v", err)
	}
	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), claims.Informer().HasSynced) {
		log.Fatal("blinkt-agent: claim informer did not sync")
	}
	resync()

	if err := helper.PublishResources(ctx, agent.Resources(*nodeName, writer != nil)); err != nil {
		log.Fatalf("blinkt-agent: publish resources: %v", err)
	}
	if writer != nil {
		log.Printf("blinkt-agent: published devices=%d node=%s writer=%s", agent.NumPixels, *nodeName, writer.Describe())
	} else {
		log.Printf("blinkt-agent: published devices=0 node=%s reason=%s", *nodeName, reason)
	}

	if err := a.Run(ctx, 10*time.Millisecond); err != nil {
		log.Printf("blinkt-agent: shutdown: %v", err)
	}
	log.Printf("blinkt-agent: stopped, strip cleared")
}

// newWriter acquires the lines; acquiring them is the hardware discovery.
func newWriter(fake, compat bool, stateDir string) (agent.Writer, error) {
	switch {
	case fake:
		return agent.NewFakeWriter(), nil
	case compat:
		return agent.NewCompatWriter(stateDir)
	default:
		return agent.NewExclusiveWriter()
	}
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
			return fmt.Errorf("kubelet did not register plugin %s within 2m (is %s mounted?)", agent.DriverName, kubeletplugin.KubeletRegistryDir)
		case <-time.After(time.Second):
		}
	}
}
