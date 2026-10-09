package controller

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	blinktv1 "github.com/kubedge/kubesim_blinkt/blinkt-operator/api/v1alpha1"
)

const ns = "blinkt-system"

var (
	k8s    client.Client
	scheme = runtime.NewScheme()
)

func TestMain(m *testing.M) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		// Run `make operator-test`, which points KUBEBUILDER_ASSETS at the
		// envtest binaries.
		os.Stderr.WriteString("skipping controller tests: KUBEBUILDER_ASSETS not set\n")
		os.Exit(0)
	}
	_ = clientgoscheme.AddToScheme(scheme)
	_ = blinktv1.AddToScheme(scheme)
	env := &envtest.Environment{CRDDirectoryPaths: []string{filepath.Join("..", "..", "config", "crd", "bases")}, ErrorIfCRDPathMissing: true}
	cfg, err := env.Start()
	if err != nil {
		panic(err)
	}
	k8s, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		panic(err)
	}
	if err := k8s.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = env.Stop()
	os.Exit(code)
}

func reconciler() *BlinktConfigReconciler {
	return &BlinktConfigReconciler{Client: k8s, Scheme: scheme, Namespace: ns, AgentImage: "kubedge1/blinkt-operator:test"}
}

func reconcileOnce(t *testing.T, name string) ctrl.Result {
	t.Helper()
	res, err := reconciler().Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: name}})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	return res
}

func getCfg(t *testing.T, name string) *blinktv1.BlinktConfig {
	t.Helper()
	cfg := &blinktv1.BlinktConfig{}
	if err := k8s.Get(context.Background(), types.NamespacedName{Name: name}, cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func ready(t *testing.T, name string) *metav1.Condition {
	t.Helper()
	return meta.FindStatusCondition(getCfg(t, name).Status.Conditions, condReady)
}

func exists(obj client.Object, key types.NamespacedName) bool {
	err := k8s.Get(context.Background(), key, obj)
	return err == nil
}

func dsKey(name string) types.NamespacedName { return types.NamespacedName{Namespace: ns, Name: name} }

func setMode(t *testing.T, mode blinktv1.Mode, mutate ...func(*blinktv1.BlinktConfig)) {
	t.Helper()
	cfg := &blinktv1.BlinktConfig{}
	err := k8s.Get(context.Background(), types.NamespacedName{Name: blinktv1.SingletonName}, cfg)
	if apierrors.IsNotFound(err) {
		cfg = &blinktv1.BlinktConfig{ObjectMeta: metav1.ObjectMeta{Name: blinktv1.SingletonName}}
		cfg.Spec.Mode = mode
		for _, m := range mutate {
			m(cfg)
		}
		if err := k8s.Create(context.Background(), cfg); err != nil {
			t.Fatal(err)
		}
		return
	}
	cfg.Spec.Mode = mode
	for _, m := range mutate {
		m(cfg)
	}
	if err := k8s.Update(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
}

// claim creates a ResourceClaim with a blinkt allocation (status set like the scheduler would).
func claim(t *testing.T, name string) *resourceapi.ResourceClaim {
	t.Helper()
	c := &resourceapi.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: name},
		Spec: resourceapi.ResourceClaimSpec{Devices: resourceapi.DeviceClaim{Requests: []resourceapi.DeviceRequest{{
			Name: "led", Exactly: &resourceapi.ExactDeviceRequest{DeviceClassName: DeviceClassName},
		}}}},
	}
	if err := k8s.Create(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	c.Status.Allocation = &resourceapi.AllocationResult{Devices: resourceapi.DeviceAllocationResult{Results: []resourceapi.DeviceRequestAllocationResult{{
		Request: "led", Driver: DriverName, Pool: "home-pi", Device: "pixel-6",
	}}}}
	if err := k8s.Status().Update(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	return c
}

func pod(t *testing.T, component string) *corev1.Pod {
	t.Helper()
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: component + "-x", Labels: managedLabels(component)},
		Spec:       corev1.PodSpec{NodeName: "home-pi", Containers: []corev1.Container{{Name: "c", Image: "i"}}},
	}
	if err := k8s.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	return p
}

// The steps share one API server, so they run in order as one test.
func TestModesSwitchingAndGuards(t *testing.T) {
	ctx := context.Background()

	t.Run("other instances are ignored", func(t *testing.T) {
		other := &blinktv1.BlinktConfig{ObjectMeta: metav1.ObjectMeta{Name: "test"}, Spec: blinktv1.BlinktConfigSpec{Mode: blinktv1.ModeAgent}}
		if err := k8s.Create(ctx, other); err != nil {
			t.Fatal(err)
		}
		reconcileOnce(t, "test")
		if c := ready(t, "test"); c == nil || c.Status != metav1.ConditionFalse || c.Reason != ReasonIgnored {
			t.Fatalf("condition = %+v", c)
		}
		if exists(&appsv1.DaemonSet{}, dsKey(AgentDaemonSet)) {
			t.Error("ignored instance created a DaemonSet")
		}
	})

	t.Run("legacy deploys nothing", func(t *testing.T) {
		setMode(t, blinktv1.ModeLegacy)
		reconcileOnce(t, blinktv1.SingletonName)
		cfg := getCfg(t, blinktv1.SingletonName)
		if c := ready(t, blinktv1.SingletonName); cfg.Status.Mode != blinktv1.ModeLegacy || c.Status != metav1.ConditionTrue {
			t.Fatalf("status = %+v", cfg.Status)
		}
		if exists(&appsv1.DaemonSet{}, dsKey(DriverDaemonSet)) || exists(&resourceapi.DeviceClass{}, types.NamespacedName{Name: DeviceClassName}) {
			t.Error("legacy created components")
		}
		if len(cfg.Finalizers) != 1 || cfg.Finalizers[0] != Finalizer {
			t.Errorf("finalizers = %v", cfg.Finalizers)
		}
	})

	t.Run("cdi runs dra-driver and the DeviceClass with defaults", func(t *testing.T) {
		intensity := int32(3)
		setMode(t, blinktv1.ModeCDI, func(c *blinktv1.BlinktConfig) {
			c.Spec.Defaults = &blinktv1.PixelConfigSpec{Intensity: &intensity}
		})
		reconcileOnce(t, blinktv1.SingletonName)
		ds := &appsv1.DaemonSet{}
		if !exists(ds, dsKey(DriverDaemonSet)) {
			t.Fatal("no driver DaemonSet")
		}
		if img := ds.Spec.Template.Spec.Containers[0].Image; img != DefaultDriverImage || ds.Labels[labelManagedBy] != managedBy || len(ds.OwnerReferences) != 1 {
			t.Errorf("driver ds image=%s labels=%v owners=%v", img, ds.Labels, ds.OwnerReferences)
		}
		dc := &resourceapi.DeviceClass{}
		if !exists(dc, types.NamespacedName{Name: DeviceClassName}) {
			t.Fatal("no DeviceClass")
		}
		if len(dc.Spec.Config) != 1 || !strings.Contains(string(dc.Spec.Config[0].Opaque.Parameters.Raw), `"intensity":3`) {
			t.Errorf("class config = %+v", dc.Spec.Config)
		}
		if exists(&appsv1.DaemonSet{}, dsKey(AgentDaemonSet)) {
			t.Error("agent DaemonSet in cdi mode")
		}
	})

	t.Run("drift is repaired", func(t *testing.T) {
		if err := k8s.Delete(ctx, &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: DriverDaemonSet}}); err != nil {
			t.Fatal(err)
		}
		reconcileOnce(t, blinktv1.SingletonName)
		if !exists(&appsv1.DaemonSet{}, dsKey(DriverDaemonSet)) {
			t.Error("deleted DaemonSet not re-created")
		}
	})

	t.Run("cdi to agent waits for the driver pods", func(t *testing.T) {
		p := pod(t, DriverDaemonSet) // a driver pod still terminating
		setMode(t, blinktv1.ModeAgent, func(c *blinktv1.BlinktConfig) { c.Spec.LegacyCompat = true })
		res := reconcileOnce(t, blinktv1.SingletonName)
		if c := ready(t, blinktv1.SingletonName); c.Reason != ReasonSwitching || res.RequeueAfter == 0 {
			t.Fatalf("condition = %+v requeue=%v", c, res.RequeueAfter)
		}
		if exists(&appsv1.DaemonSet{}, dsKey(DriverDaemonSet)) {
			t.Error("driver DaemonSet not deleted")
		}
		if exists(&appsv1.DaemonSet{}, dsKey(AgentDaemonSet)) {
			t.Fatal("agent created while driver pods remain")
		}
		if err := k8s.Delete(ctx, p, client.GracePeriodSeconds(0)); err != nil {
			t.Fatal(err)
		}
		reconcileOnce(t, blinktv1.SingletonName)
		ds := &appsv1.DaemonSet{}
		if !exists(ds, dsKey(AgentDaemonSet)) {
			t.Fatal("agent DaemonSet not created after driver pods left")
		}
		c := ds.Spec.Template.Spec.Containers[0]
		if c.Image != "kubedge1/blinkt-operator:test" || c.Command[0] != "/node-agent" || !strings.Contains(strings.Join(c.Args, " "), "--legacy-compat") {
			t.Errorf("agent container = %+v", c)
		}
		if ds.Spec.UpdateStrategy.RollingUpdate.MaxSurge.IntValue() != 0 {
			t.Error("agent rolling update may surge: two plugins on one node")
		}
		if getCfg(t, blinktv1.SingletonName).Status.Mode != blinktv1.ModeAgent {
			t.Error("status.mode not agent")
		}
	})

	t.Run("status counts nodes and devices", func(t *testing.T) {
		ds := &appsv1.DaemonSet{}
		exists(ds, dsKey(AgentDaemonSet))
		ds.Status = appsv1.DaemonSetStatus{DesiredNumberScheduled: 3, NumberReady: 3, UpdatedNumberScheduled: 3, ObservedGeneration: ds.Generation}
		if err := k8s.Status().Update(ctx, ds); err != nil {
			t.Fatal(err)
		}
		node := "home-pi"
		slice := &resourceapi.ResourceSlice{
			ObjectMeta: metav1.ObjectMeta{Name: "home-pi-blinkt"},
			Spec:       resourceapi.ResourceSliceSpec{Driver: DriverName, NodeName: &node, Pool: resourceapi.ResourcePool{Name: node, ResourceSliceCount: 1}},
		}
		for i := 0; i < 8; i++ {
			slice.Spec.Devices = append(slice.Spec.Devices, resourceapi.Device{Name: "pixel-" + string(rune('0'+i))})
		}
		if err := k8s.Create(ctx, slice); err != nil {
			t.Fatal(err)
		}
		reconcileOnce(t, blinktv1.SingletonName)
		s := getCfg(t, blinktv1.SingletonName).Status
		if s.Nodes != (blinktv1.NodeCounts{Desired: 3, Ready: 3}) || s.Devices != 8 || ready(t, blinktv1.SingletonName).Status != metav1.ConditionTrue {
			t.Fatalf("status = %+v", s)
		}
	})

	t.Run("legacy refused while claims exist", func(t *testing.T) {
		c := claim(t, "lte-led")
		setMode(t, blinktv1.ModeLegacy)
		reconcileOnce(t, blinktv1.SingletonName)
		if cond := ready(t, blinktv1.SingletonName); cond.Reason != ReasonClaimsInUse || !strings.Contains(cond.Message, "default/lte-led") {
			t.Fatalf("condition = %+v", cond)
		}
		if !exists(&appsv1.DaemonSet{}, dsKey(AgentDaemonSet)) {
			t.Fatal("components removed under a live claim")
		}
		if err := k8s.Delete(ctx, c); err != nil {
			t.Fatal(err)
		}
		reconcileOnce(t, blinktv1.SingletonName)
		if exists(&appsv1.DaemonSet{}, dsKey(AgentDaemonSet)) || exists(&resourceapi.DeviceClass{}, types.NamespacedName{Name: DeviceClassName}) {
			t.Error("components kept after the claims left")
		}
	})

	t.Run("foreign driver blocks deployment and removes the operator's own", func(t *testing.T) {
		setMode(t, blinktv1.ModeCDI)
		reconcileOnce(t, blinktv1.SingletonName)
		if !exists(&appsv1.DaemonSet{}, dsKey(DriverDaemonSet)) {
			t.Fatal("cdi DaemonSet not deployed before the foreign driver appears")
		}
		foreign := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Namespace: "blinkt-dra", Name: "blinkt-dra-driver", Labels: map[string]string{labelName: DriverDaemonSet}}}
		if err := k8s.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "blinkt-dra"}}); err != nil {
			t.Fatal(err)
		}
		MutateDriverDaemonSet(foreign, &blinktv1.BlinktConfig{}, nil)
		foreign.Labels = map[string]string{labelName: DriverDaemonSet} // hand-applied: no managed-by
		foreign.Spec.Template.Labels = map[string]string{labelName: DriverDaemonSet}
		if err := k8s.Create(ctx, foreign); err != nil {
			t.Fatal(err)
		}
		setMode(t, blinktv1.ModeCDI)
		reconcileOnce(t, blinktv1.SingletonName)
		if c := ready(t, blinktv1.SingletonName); c.Reason != ReasonForeignDriver || !strings.Contains(c.Message, "blinkt-dra/blinkt-dra-driver") {
			t.Fatalf("condition = %+v", c)
		}
		if exists(&appsv1.DaemonSet{}, dsKey(DriverDaemonSet)) {
			t.Error("operator deployed next to a foreign driver")
		}
		if err := k8s.Delete(ctx, foreign); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("deletion waits for claims, then cleans up", func(t *testing.T) {
		reconcileOnce(t, blinktv1.SingletonName) // cdi deploys now that the foreign driver is gone
		c := claim(t, "elte-led")
		cfg := getCfg(t, blinktv1.SingletonName)
		if err := k8s.Delete(ctx, cfg); err != nil {
			t.Fatal(err)
		}
		reconcileOnce(t, blinktv1.SingletonName)
		if cond := ready(t, blinktv1.SingletonName); cond.Reason != ReasonClaimsInUse {
			t.Fatalf("condition = %+v", cond)
		}
		if err := k8s.Delete(ctx, c); err != nil {
			t.Fatal(err)
		}
		reconcileOnce(t, blinktv1.SingletonName)
		if exists(&blinktv1.BlinktConfig{}, types.NamespacedName{Name: blinktv1.SingletonName}) {
			t.Error("BlinktConfig not released")
		}
		if exists(&appsv1.DaemonSet{}, dsKey(DriverDaemonSet)) || exists(&resourceapi.DeviceClass{}, types.NamespacedName{Name: DeviceClassName}) {
			t.Error("components left after deletion")
		}
	})
}
