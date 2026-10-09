// manager is the blinkt-operator: it reconciles the BlinktConfig named
// "cluster" and runs the node component of the selected mode.
package main

import (
	"flag"
	"os"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	blinktv1 "github.com/kubedge/kubesim_blinkt/blinkt-operator/api/v1alpha1"
	"github.com/kubedge/kubesim_blinkt/blinkt-operator/internal/controller"
)

func main() {
	var namespace, agentImage, agentExtraArgs, driverExtraArgs, metricsAddr, probeAddr string
	var leaderElect bool
	flag.StringVar(&namespace, "namespace", envOr("POD_NAMESPACE", "blinkt-system"), "namespace of the node component DaemonSets")
	flag.StringVar(&agentImage, "agent-image", os.Getenv("AGENT_IMAGE"), "image with /node-agent (default: $AGENT_IMAGE, normally the operator's own image)")
	flag.StringVar(&agentExtraArgs, "agent-extra-args", "", "comma-separated extra node-agent args, e.g. --fake-gpio on test clusters")
	flag.StringVar(&driverExtraArgs, "driver-extra-args", "", "comma-separated extra dra-driver args, e.g. --assume-gpio,--gpio-device=/dev/null on test clusters")
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "metrics address, \"0\" disables")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "health probe address")
	flag.BoolVar(&leaderElect, "leader-elect", true, "leader election, so only one manager reconciles")
	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	setupLog := ctrl.Log.WithName("setup")

	if agentImage == "" {
		setupLog.Error(nil, "--agent-image or AGENT_IMAGE is required")
		os.Exit(1)
	}

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		panic(err)
	}
	if err := blinktv1.AddToScheme(scheme); err != nil {
		panic(err)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                  scheme,
		Metrics:                 metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress:  probeAddr,
		LeaderElection:          leaderElect,
		LeaderElectionID:        "blinkt-operator.blinkt.kubedge.io",
		LeaderElectionNamespace: namespace,
	})
	if err != nil {
		setupLog.Error(err, "create manager")
		os.Exit(1)
	}

	if err := (&controller.BlinktConfigReconciler{
		Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
		Namespace: namespace, AgentImage: agentImage,
		AgentExtraArgs: splitArgs(agentExtraArgs), DriverExtraArgs: splitArgs(driverExtraArgs),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "set up controller")
		os.Exit(1)
	}
	_ = mgr.AddHealthzCheck("healthz", healthz.Ping)
	_ = mgr.AddReadyzCheck("readyz", healthz.Ping)

	setupLog.Info("blinkt-operator: starting", "namespace", namespace, "agentImage", agentImage)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "run manager")
		os.Exit(1)
	}
}

func splitArgs(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
