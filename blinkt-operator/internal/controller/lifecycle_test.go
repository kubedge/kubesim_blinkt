package controller

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	blinktv1 "github.com/kubedge/kubesim_blinkt/blinkt-operator/api/v1alpha1"
)

func node(t *testing.T, name string, l map[string]string) {
	t.Helper()
	if err := k8s.Create(context.Background(), &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: l}}); err != nil {
		t.Fatal(err)
	}
}

func claimOn(t *testing.T, name, pool string) *resourceapi.ResourceClaim {
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
		Request: "led", Driver: DriverName, Pool: pool, Device: "pixel-4",
	}}}}
	if err := k8s.Status().Update(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	return c
}

func agentPodOn(t *testing.T, nodeName string) *corev1.Pod {
	t.Helper()
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "agent-on-" + nodeName, Labels: managedLabels(AgentDaemonSet)},
		Spec:       corev1.PodSpec{NodeName: nodeName, Containers: []corev1.Container{{Name: "c", Image: "i"}}},
	}
	if err := k8s.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	return p
}

func agentSelector(t *testing.T) map[string]string {
	t.Helper()
	ds := &appsv1.DaemonSet{}
	if !exists(ds, dsKey(AgentDaemonSet)) {
		t.Fatal("no agent DaemonSet")
	}
	return ds.Spec.Template.Spec.NodeSelector
}

func jobSucceeded(t *testing.T, nodeName string) {
	t.Helper()
	job := &batchv1.Job{}
	if err := k8s.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: ClearJobName(nodeName)}, job); err != nil {
		t.Fatal(err)
	}
	now := metav1.NewTime(time.Now())
	job.Status = batchv1.JobStatus{
		StartTime: &now, CompletionTime: &now, Succeeded: 1,
		Conditions: []batchv1.JobCondition{
			{Type: batchv1.JobSuccessCriteriaMet, Status: corev1.ConditionTrue, LastTransitionTime: now},
			{Type: batchv1.JobComplete, Status: corev1.ConditionTrue, LastTransitionTime: now},
		},
	}
	if err := k8s.Status().Update(context.Background(), job); err != nil {
		t.Fatal(err)
	}
}

func agentNodes(t *testing.T) []string {
	t.Helper()
	return getCfg(t, blinktv1.SingletonName).Status.AgentNodes
}

// Runs after TestModesSwitchingAndGuards, which leaves no BlinktConfig.
func TestNodeLifecycle(t *testing.T) {
	ctx := context.Background()
	node(t, "n1", map[string]string{"blinkt": "true", "zone": "a"})
	node(t, "n2", map[string]string{"blinkt": "true"})

	t.Run("agent covers the selected nodes and mounts the CDI dir", func(t *testing.T) {
		setMode(t, blinktv1.ModeAgent, func(c *blinktv1.BlinktConfig) {
			c.Spec.NodeSelector = map[string]string{"blinkt": "true"}
		})
		reconcileOnce(t, blinktv1.SingletonName)
		if got := agentNodes(t); !slices.Equal(got, []string{"n1", "n2"}) {
			t.Fatalf("agentNodes = %v", got)
		}
		ds := &appsv1.DaemonSet{}
		exists(ds, dsKey(AgentDaemonSet))
		if !strings.Contains(strings.Join(ds.Spec.Template.Spec.Containers[0].Args, " "), "--cdi-dir=/var/run/cdi") {
			t.Error("agent does not get --cdi-dir")
		}
	})

	var c *resourceapi.ResourceClaim
	t.Run("narrowing waits while a dropped node holds a claim", func(t *testing.T) {
		c = claimOn(t, "elte-led", "n2")
		setMode(t, blinktv1.ModeAgent, func(cfg *blinktv1.BlinktConfig) {
			cfg.Spec.NodeSelector = map[string]string{"blinkt": "true", "zone": "a"}
		})
		reconcileOnce(t, blinktv1.SingletonName)
		if sel := agentSelector(t); len(sel) != 1 || sel["blinkt"] != "true" {
			t.Fatalf("selector applied despite the claim: %v", sel)
		}
		cond := ready(t, blinktv1.SingletonName)
		if cond.Reason != ReasonNodeClaimsInUse || !strings.Contains(cond.Message, "n2: default/elte-led") {
			t.Fatalf("condition = %+v", cond)
		}
	})

	t.Run("claim released: selector applied, clear Job waits for the agent pod, then runs on the node", func(t *testing.T) {
		p := agentPodOn(t, "n2") // agent still terminating on n2
		if err := k8s.Delete(ctx, c); err != nil {
			t.Fatal(err)
		}
		res := reconcileOnce(t, blinktv1.SingletonName)
		if sel := agentSelector(t); sel["zone"] != "a" {
			t.Fatalf("new selector not applied: %v", sel)
		}
		if exists(&batchv1.Job{}, types.NamespacedName{Namespace: ns, Name: ClearJobName("n2")}) {
			t.Fatal("clear Job created while an agent pod is still on n2")
		}
		if got := agentNodes(t); !slices.Equal(got, []string{"n1", "n2"}) || res.RequeueAfter == 0 {
			t.Fatalf("agentNodes = %v requeue=%v", got, res.RequeueAfter)
		}
		if err := k8s.Delete(ctx, p, client.GracePeriodSeconds(0)); err != nil {
			t.Fatal(err)
		}
		reconcileOnce(t, blinktv1.SingletonName)
		job := &batchv1.Job{}
		if !exists(job, types.NamespacedName{Namespace: ns, Name: ClearJobName("n2")}) {
			t.Fatal("no clear Job for n2")
		}
		spec := job.Spec.Template.Spec
		if spec.NodeName != "n2" || spec.Containers[0].Command[0] != "/node-agent" || spec.Containers[0].Args[0] != "--clear" || *spec.AutomountServiceAccountToken {
			t.Errorf("clear job = node %s cmd %v args %v", spec.NodeName, spec.Containers[0].Command, spec.Containers[0].Args)
		}
		if !strings.Contains(ready(t, blinktv1.SingletonName).Message, "clearing LEDs on n2") {
			t.Errorf("status message = %q", ready(t, blinktv1.SingletonName).Message)
		}
	})

	t.Run("node leaves status once cleared", func(t *testing.T) {
		jobSucceeded(t, "n2")
		reconcileOnce(t, blinktv1.SingletonName)
		if got := agentNodes(t); !slices.Equal(got, []string{"n1"}) {
			t.Fatalf("agentNodes = %v", got)
		}
	})

	t.Run("leaving agent mode clears every agent node", func(t *testing.T) {
		setMode(t, blinktv1.ModeCDI)
		reconcileOnce(t, blinktv1.SingletonName)
		if exists(&appsv1.DaemonSet{}, dsKey(AgentDaemonSet)) {
			t.Fatal("agent DaemonSet kept in cdi mode")
		}
		if !exists(&batchv1.Job{}, types.NamespacedName{Namespace: ns, Name: ClearJobName("n1")}) {
			t.Fatal("no clear Job for n1 after leaving agent mode")
		}
		jobSucceeded(t, "n1")
		reconcileOnce(t, blinktv1.SingletonName)
		if got := agentNodes(t); len(got) != 0 {
			t.Fatalf("agentNodes = %v, want empty", got)
		}
	})

	t.Run("cleanup", func(t *testing.T) {
		if err := k8s.Delete(ctx, getCfg(t, blinktv1.SingletonName)); err != nil {
			t.Fatal(err)
		}
		reconcileOnce(t, blinktv1.SingletonName)
	})
}
