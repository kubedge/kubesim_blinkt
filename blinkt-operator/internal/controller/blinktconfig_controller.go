// Package controller reconciles BlinktConfig: it runs the node component of
// the selected mode (dra-driver in cdi mode, the node agent in agent mode, or
// none in legacy mode) together with the DeviceClass, switches modes without
// ever running two blinkt.kubedge.io drivers on a node, and refuses to remove
// DRA components while blinkt claims exist.
package controller

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	blinktv1 "github.com/kubedge/kubesim_blinkt/blinkt-operator/api/v1alpha1"
)

const (
	// Finalizer keeps a BlinktConfig until its DRA components can go.
	Finalizer = "blinkt.kubedge.io/claims-guard"

	condReady = "Ready"

	ReasonReady           = "Ready"
	ReasonIgnored         = "IgnoredNotSingleton"
	ReasonForeignDriver   = "ForeignDriver"
	ReasonClaimsInUse     = "ClaimsInUse"
	ReasonSwitching       = "Switching"
	ReasonRollingOut      = "RollingOut"
	ReasonNodeClaimsInUse = "NodeClaimsInUse"
	switchRequeue         = 2 * time.Second
	clearRequeue          = 5 * time.Second
	maxNamesInStatusLines = 5
)

// BlinktConfigReconciler reconciles the BlinktConfig named "cluster".
type BlinktConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// Namespace holds the node component DaemonSets.
	Namespace string
	// AgentImage is the image with /node-agent (the operator's own image).
	AgentImage string
	// AgentExtraArgs are appended to the agent's args (e.g. --fake-gpio on kind).
	AgentExtraArgs []string
	// DriverExtraArgs are appended to dra-driver's args (e.g. --assume-gpio on kind).
	DriverExtraArgs []string
}

// +kubebuilder:rbac:groups=blinkt.kubedge.io,resources=blinktconfigs,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=blinkt.kubedge.io,resources=blinktconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=blinkt.kubedge.io,resources=blinktconfigs/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=resource.k8s.io,resources=deviceclasses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=resource.k8s.io,resources=resourceclaims;resourceslices,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *BlinktConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	cfg := &blinktv1.BlinktConfig{}
	if err := r.Get(ctx, req.NamespacedName, cfg); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if cfg.Name != blinktv1.SingletonName {
		return ctrl.Result{}, r.setStatus(ctx, cfg, func(s *blinktv1.BlinktConfigStatus) {
			setReady(s, cfg, false, ReasonIgnored, fmt.Sprintf("only the BlinktConfig named %q is acted on", blinktv1.SingletonName))
		})
	}

	claims, claimsByNode, err := r.blinktClaims(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Deletion: remove the components unless claims still use them.
	if !cfg.DeletionTimestamp.IsZero() {
		if len(claims) > 0 {
			return ctrl.Result{}, r.setStatus(ctx, cfg, func(s *blinktv1.BlinktConfigStatus) {
				setReady(s, cfg, false, ReasonClaimsInUse, "deletion waits for blinkt claims: "+names(claims))
			})
		}
		if err := r.removeComponents(ctx); err != nil {
			return ctrl.Result{}, err
		}
		if controllerutil.RemoveFinalizer(cfg, Finalizer) {
			return ctrl.Result{}, r.Update(ctx, cfg)
		}
		return ctrl.Result{}, nil
	}
	if controllerutil.AddFinalizer(cfg, Finalizer) {
		if err := r.Update(ctx, cfg); err != nil {
			return ctrl.Result{}, err
		}
	}

	mode := cfg.Spec.Mode
	if mode == "" {
		mode = blinktv1.ModeLegacy
	}

	if mode == blinktv1.ModeLegacy {
		if len(claims) > 0 && r.anyComponent(ctx) {
			return ctrl.Result{}, r.setStatus(ctx, cfg, func(s *blinktv1.BlinktConfigStatus) {
				setReady(s, cfg, false, ReasonClaimsInUse, "DRA components kept while blinkt claims exist: "+names(claims))
			})
		}
		if err := r.removeComponents(ctx); err != nil {
			return ctrl.Result{}, err
		}
		agentNodes, clearing, err := r.clearLeavingNodes(ctx, cfg, nil)
		if err != nil {
			return ctrl.Result{}, err
		}
		return requeueIf(clearing), r.setStatus(ctx, cfg, func(s *blinktv1.BlinktConfigStatus) {
			s.Mode, s.Nodes, s.Devices, s.AgentNodes = blinktv1.ModeLegacy, blinktv1.NodeCounts{}, 0, agentNodes
			setReady(s, cfg, true, ReasonReady, "legacy mode: no DRA components"+clearingNote(agentNodes, nil))
		})
	}

	// cdi or agent.
	foreign, err := r.foreignDrivers(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	if len(foreign) > 0 {
		// One driver per node: step aside for the hand-applied one.
		for _, name := range []string{DriverDaemonSet, AgentDaemonSet} {
			if err := r.deleteDaemonSet(ctx, name); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, r.setStatus(ctx, cfg, func(s *blinktv1.BlinktConfigStatus) {
			setReady(s, cfg, false, ReasonForeignDriver, "a blinkt driver DaemonSet not managed by the operator exists: "+strings.Join(foreign, ", "))
		})
	}

	if err := r.applyDeviceClass(ctx, cfg); err != nil {
		return ctrl.Result{}, err
	}

	want, other := DriverDaemonSet, AgentDaemonSet
	if mode == blinktv1.ModeAgent {
		want, other = AgentDaemonSet, DriverDaemonSet
	}
	// One driver per node: the other mode's pods must be gone first.
	if err := r.deleteDaemonSet(ctx, other); err != nil {
		return ctrl.Result{}, err
	}
	if n, err := r.podsOf(ctx, other); err != nil {
		return ctrl.Result{}, err
	} else if n > 0 {
		logger.Info("waiting for the previous mode's pods", "daemonset", other, "pods", n)
		return ctrl.Result{RequeueAfter: switchRequeue}, r.setStatus(ctx, cfg, func(s *blinktv1.BlinktConfigStatus) {
			setReady(s, cfg, false, ReasonSwitching, fmt.Sprintf("switching to %s: waiting for %d %s pod(s) to terminate", mode, n, other))
		})
	}

	keepSel, blockedBy, err := r.selectorGuard(ctx, cfg, want, claimsByNode)
	if err != nil {
		return ctrl.Result{}, err
	}
	ds, err := r.applyDaemonSet(ctx, cfg, want, keepSel)
	if err != nil {
		return ctrl.Result{}, err
	}
	var covered []string
	if mode == blinktv1.ModeAgent {
		if covered, err = r.nodesMatching(ctx, ds.Spec.Template.Spec.NodeSelector); err != nil {
			return ctrl.Result{}, err
		}
	}
	agentNodes, clearing, err := r.clearLeavingNodes(ctx, cfg, covered)
	if err != nil {
		return ctrl.Result{}, err
	}
	devices, err := r.publishedDevices(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	desired, ready := ds.Status.DesiredNumberScheduled, ds.Status.NumberReady
	rolledOut := ds.Status.ObservedGeneration >= ds.Generation && ds.Status.UpdatedNumberScheduled == desired && ready == desired
	return requeueIf(clearing), r.setStatus(ctx, cfg, func(s *blinktv1.BlinktConfigStatus) {
		s.Mode = mode
		s.Nodes = blinktv1.NodeCounts{Desired: desired, Ready: ready}
		s.Devices = devices
		s.AgentNodes = agentNodes
		switch {
		case blockedBy != "":
			setReady(s, cfg, false, ReasonNodeClaimsInUse, "nodeSelector change waits for blinkt claims on the nodes it would drop: "+blockedBy)
		case rolledOut:
			setReady(s, cfg, true, ReasonReady, fmt.Sprintf("%s mode: %d/%d nodes ready, %d devices", mode, ready, desired, devices)+clearingNote(agentNodes, covered))
		default:
			setReady(s, cfg, false, ReasonRollingOut, fmt.Sprintf("%s mode: %d/%d nodes ready", mode, ready, desired)+clearingNote(agentNodes, covered))
		}
	})
}

func requeueIf(pending bool) ctrl.Result {
	if pending {
		return ctrl.Result{RequeueAfter: clearRequeue}
	}
	return ctrl.Result{}
}

// clearingNote mentions nodes still being cleared after leaving the agent.
func clearingNote(agentNodes, covered []string) string {
	var leaving []string
	for _, n := range agentNodes {
		if !slices.Contains(covered, n) {
			leaving = append(leaving, n)
		}
	}
	if len(leaving) == 0 {
		return ""
	}
	return "; clearing LEDs on " + strings.Join(leaving, ", ")
}

// blinktClaims lists ResourceClaims with a blinkt.kubedge.io allocation,
// and the same claims by node (allocation pool).
func (r *BlinktConfigReconciler) blinktClaims(ctx context.Context) ([]string, map[string][]string, error) {
	var list resourceapi.ResourceClaimList
	if err := r.List(ctx, &list); err != nil {
		return nil, nil, err
	}
	var out []string
	byNode := map[string][]string{}
	for _, c := range list.Items {
		if c.Status.Allocation == nil {
			continue
		}
		name := c.Namespace + "/" + c.Name
		seen := false
		for _, res := range c.Status.Allocation.Devices.Results {
			if res.Driver != DriverName {
				continue
			}
			if !seen {
				out = append(out, name)
				seen = true
			}
			byNode[res.Pool] = append(byNode[res.Pool], name)
		}
	}
	sort.Strings(out)
	return out, byNode, nil
}

// nodesMatching lists the names of nodes whose labels match sel.
func (r *BlinktConfigReconciler) nodesMatching(ctx context.Context, sel map[string]string) ([]string, error) {
	var nodes corev1.NodeList
	if err := r.List(ctx, &nodes); err != nil {
		return nil, err
	}
	match := labels.SelectorFromSet(sel)
	var out []string
	for _, n := range nodes.Items {
		if match.Matches(labels.Set(n.Labels)) {
			out = append(out, n.Name)
		}
	}
	sort.Strings(out)
	return out, nil
}

// selectorGuard returns the node selector the DaemonSet must keep (nil: the
// desired one may be applied) and, when kept, which nodes/claims block it:
// narrowing must not drop nodes that still hold blinkt claims.
func (r *BlinktConfigReconciler) selectorGuard(ctx context.Context, cfg *blinktv1.BlinktConfig, name string, byNode map[string][]string) (map[string]string, string, error) {
	existing := &appsv1.DaemonSet{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: name}, existing); err != nil {
		return nil, "", client.IgnoreNotFound(err)
	}
	oldSel, newSel := existing.Spec.Template.Spec.NodeSelector, nodeSelector(cfg)
	if maps.Equal(oldSel, newSel) {
		return nil, "", nil
	}
	before, err := r.nodesMatching(ctx, oldSel)
	if err != nil {
		return nil, "", err
	}
	after, err := r.nodesMatching(ctx, newSel)
	if err != nil {
		return nil, "", err
	}
	var holds []string
	for _, n := range before {
		if !slices.Contains(after, n) {
			for _, c := range byNode[n] {
				holds = append(holds, n+": "+c)
			}
		}
	}
	if len(holds) == 0 {
		return nil, "", nil
	}
	return oldSel, names(holds), nil
}

// clearLeavingNodes runs a clear Job on each recorded agent node that the
// agent no longer covers, once no agent pod is left there. It returns the new
// status.agentNodes (covered nodes plus nodes still being cleared) and
// whether clearing is still in progress.
func (r *BlinktConfigReconciler) clearLeavingNodes(ctx context.Context, cfg *blinktv1.BlinktConfig, covered []string) ([]string, bool, error) {
	var agentPods corev1.PodList
	if err := r.List(ctx, &agentPods, client.InNamespace(r.Namespace), client.MatchingLabels{labelName: AgentDaemonSet, labelManagedBy: managedBy}); err != nil {
		return nil, false, err
	}
	podOn := map[string]bool{}
	for _, p := range agentPods.Items {
		podOn[p.Spec.NodeName] = true
	}
	keep := slices.Clone(covered)
	pending := false
	for _, n := range covered {
		// A node that came back must not race a pending clear with its new agent.
		job := &batchv1.Job{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: ClearJobName(n)}, job); err == nil && !jobFinished(job) {
			if err := client.IgnoreNotFound(r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground))); err != nil {
				return nil, false, err
			}
		}
	}
	for _, n := range cfg.Status.AgentNodes {
		if slices.Contains(covered, n) {
			continue
		}
		if podOn[n] {
			keep, pending = append(keep, n), true // wait for the agent to stop first
			continue
		}
		job := &batchv1.Job{}
		err := r.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: ClearJobName(n)}, job)
		switch {
		case apierrors.IsNotFound(err):
			job = &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Namespace: r.Namespace, Name: ClearJobName(n)}}
			MutateClearJob(job, cfg, n, r.AgentImage, r.AgentExtraArgs)
			if err := controllerutil.SetControllerReference(cfg, job, r.Scheme); err != nil {
				return nil, false, err
			}
			if err := r.Create(ctx, job); err != nil && !apierrors.IsAlreadyExists(err) {
				return nil, false, err
			}
			log.FromContext(ctx).Info("clearing node that left the agent", "node", n)
			keep, pending = append(keep, n), true
		case err != nil:
			return nil, false, err
		case job.Status.Succeeded > 0:
			// cleared: the node leaves status.agentNodes
		case jobFailed(job):
			if err := client.IgnoreNotFound(r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground))); err != nil {
				return nil, false, err
			}
			keep, pending = append(keep, n), true // re-created on the next reconcile
		default:
			keep, pending = append(keep, n), true
		}
	}
	sort.Strings(keep)
	return slices.Compact(keep), pending, nil
}

func jobFailed(j *batchv1.Job) bool {
	for _, c := range j.Status.Conditions {
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func jobFinished(j *batchv1.Job) bool { return j.Status.Succeeded > 0 || jobFailed(j) }

// foreignDrivers lists blinkt driver DaemonSets the operator does not manage.
func (r *BlinktConfigReconciler) foreignDrivers(ctx context.Context) ([]string, error) {
	var out []string
	for _, name := range []string{DriverDaemonSet, AgentDaemonSet} {
		var list appsv1.DaemonSetList
		if err := r.List(ctx, &list, client.MatchingLabels{labelName: name}); err != nil {
			return nil, err
		}
		for _, ds := range list.Items {
			if ds.Labels[labelManagedBy] != managedBy {
				out = append(out, ds.Namespace+"/"+ds.Name)
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

func (r *BlinktConfigReconciler) applyDeviceClass(ctx context.Context, cfg *blinktv1.BlinktConfig) error {
	dc := &resourceapi.DeviceClass{ObjectMeta: metav1.ObjectMeta{Name: DeviceClassName}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, dc, func() error {
		if dc.CreationTimestamp.IsZero() || dc.Labels[labelManagedBy] == managedBy {
			if err := MutateDeviceClass(dc, cfg); err != nil {
				return err
			}
			return controllerutil.SetControllerReference(cfg, dc, r.Scheme)
		}
		return nil // a hand-applied DeviceClass is left as is
	})
	return err
}

// applyDaemonSet creates or updates the mode's DaemonSet. keepSel, when not
// nil, keeps the currently applied node selector (see selectorGuard).
func (r *BlinktConfigReconciler) applyDaemonSet(ctx context.Context, cfg *blinktv1.BlinktConfig, name string, keepSel map[string]string) (*appsv1.DaemonSet, error) {
	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: r.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, ds, func() error {
		if name == AgentDaemonSet {
			MutateAgentDaemonSet(ds, cfg, r.AgentImage, r.AgentExtraArgs)
		} else {
			MutateDriverDaemonSet(ds, cfg, r.DriverExtraArgs)
		}
		if keepSel != nil {
			ds.Spec.Template.Spec.NodeSelector = keepSel
		}
		return controllerutil.SetControllerReference(cfg, ds, r.Scheme)
	})
	return ds, err
}

func (r *BlinktConfigReconciler) deleteDaemonSet(ctx context.Context, name string) error {
	ds := &appsv1.DaemonSet{}
	err := r.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: name}, ds)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if ds.Labels[labelManagedBy] != managedBy {
		return nil
	}
	return client.IgnoreNotFound(r.Delete(ctx, ds, client.PropagationPolicy(metav1.DeletePropagationBackground)))
}

func (r *BlinktConfigReconciler) podsOf(ctx context.Context, name string) (int, error) {
	var pods corev1.PodList
	if err := r.List(ctx, &pods, client.InNamespace(r.Namespace), client.MatchingLabels{labelName: name, labelManagedBy: managedBy}); err != nil {
		return 0, err
	}
	return len(pods.Items), nil
}

func (r *BlinktConfigReconciler) anyComponent(ctx context.Context) bool {
	for _, name := range []string{DriverDaemonSet, AgentDaemonSet} {
		if err := r.Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: name}, &appsv1.DaemonSet{}); err == nil {
			return true
		}
	}
	return r.Get(ctx, types.NamespacedName{Name: DeviceClassName}, &resourceapi.DeviceClass{}) == nil
}

func (r *BlinktConfigReconciler) removeComponents(ctx context.Context) error {
	for _, name := range []string{DriverDaemonSet, AgentDaemonSet} {
		if err := r.deleteDaemonSet(ctx, name); err != nil {
			return err
		}
	}
	dc := &resourceapi.DeviceClass{}
	if err := r.Get(ctx, types.NamespacedName{Name: DeviceClassName}, dc); err == nil && dc.Labels[labelManagedBy] == managedBy {
		return client.IgnoreNotFound(r.Delete(ctx, dc))
	} else if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (r *BlinktConfigReconciler) publishedDevices(ctx context.Context) (int32, error) {
	var slices resourceapi.ResourceSliceList
	if err := r.List(ctx, &slices); err != nil {
		return 0, err
	}
	var n int32
	for _, s := range slices.Items {
		if s.Spec.Driver == DriverName {
			n += int32(len(s.Spec.Devices))
		}
	}
	return n, nil
}

func (r *BlinktConfigReconciler) setStatus(ctx context.Context, cfg *blinktv1.BlinktConfig, mutate func(*blinktv1.BlinktConfigStatus)) error {
	before := cfg.Status.DeepCopy()
	mutate(&cfg.Status)
	cfg.Status.ObservedGeneration = cfg.Generation
	if equalStatus(before, &cfg.Status) {
		return nil
	}
	return r.Status().Update(ctx, cfg)
}

func equalStatus(a, b *blinktv1.BlinktConfigStatus) bool {
	if a.Mode != b.Mode || a.Nodes != b.Nodes || a.Devices != b.Devices || a.ObservedGeneration != b.ObservedGeneration || len(a.Conditions) != len(b.Conditions) || !slices.Equal(a.AgentNodes, b.AgentNodes) {
		return false
	}
	for i := range a.Conditions {
		x, y := a.Conditions[i], b.Conditions[i]
		if x.Type != y.Type || x.Status != y.Status || x.Reason != y.Reason || x.Message != y.Message || x.ObservedGeneration != y.ObservedGeneration {
			return false
		}
	}
	return true
}

func setReady(s *blinktv1.BlinktConfigStatus, cfg *blinktv1.BlinktConfig, ok bool, reason, msg string) {
	status := metav1.ConditionFalse
	if ok {
		status = metav1.ConditionTrue
	}
	meta.SetStatusCondition(&s.Conditions, metav1.Condition{
		Type: condReady, Status: status, Reason: reason, Message: msg, ObservedGeneration: cfg.Generation,
	})
}

func names(items []string) string {
	if len(items) > maxNamesInStatusLines {
		return strings.Join(items[:maxNamesInStatusLines], ", ") + fmt.Sprintf(" and %d more", len(items)-maxNamesInStatusLines)
	}
	return strings.Join(items, ", ")
}

// SetupWithManager watches the BlinktConfig, the objects it owns, and the
// cluster state that changes its decisions (claims, slices, the previous
// mode's pods, foreign driver DaemonSets).
func (r *BlinktConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	toSingleton := handler.EnqueueRequestsFromMapFunc(func(context.Context, client.Object) []reconcile.Request {
		return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: blinktv1.SingletonName}}}
	})
	return ctrl.NewControllerManagedBy(mgr).
		For(&blinktv1.BlinktConfig{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&resourceapi.DeviceClass{}).
		Owns(&batchv1.Job{}).
		Watches(&corev1.Node{}, toSingleton).
		Watches(&resourceapi.ResourceClaim{}, toSingleton).
		Watches(&resourceapi.ResourceSlice{}, toSingleton).
		Watches(&appsv1.DaemonSet{}, toSingleton).
		Named("blinktconfig").
		Complete(r)
}
