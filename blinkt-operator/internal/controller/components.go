package controller

import (
	"encoding/json"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"

	blinktv1 "github.com/kubedge/kubesim_blinkt/blinkt-operator/api/v1alpha1"
)

const (
	// DriverName is what both node components register.
	DriverName = "blinkt.kubedge.io"
	// DeviceClassName is the class claims use in cdi and agent modes.
	DeviceClassName = "blinkt-pixel.kubedge.io"

	// DriverDaemonSet runs dra-driver (cdi mode); AgentDaemonSet runs the
	// node agent (agent mode). The names double as app.kubernetes.io/name.
	DriverDaemonSet = "blinkt-dra-driver"
	AgentDaemonSet  = "blinkt-node-agent"

	labelName      = "app.kubernetes.io/name"
	labelNode      = "blinkt.kubedge.io/node"
	labelManagedBy = "app.kubernetes.io/managed-by"
	managedBy      = "blinkt-operator"

	// DefaultDriverImage is the published dra-driver (the DaemonSet approach).
	DefaultDriverImage = "docker.io/kubedge1/blinkt-dra-driver:0.5.2"
)

func managedLabels(name string) map[string]string {
	return map[string]string{labelName: name, labelManagedBy: managedBy}
}

func nodeSelector(cfg *blinktv1.BlinktConfig) map[string]string {
	if len(cfg.Spec.NodeSelector) > 0 {
		return cfg.Spec.NodeSelector
	}
	return map[string]string{"kubernetes.io/arch": "arm64"}
}

func tolerations(cfg *blinktv1.BlinktConfig) []corev1.Toleration {
	if len(cfg.Spec.Tolerations) > 0 {
		return cfg.Spec.Tolerations
	}
	return []corev1.Toleration{{Key: "node-role.kubernetes.io/control-plane", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}}
}

func hostPath(name, path string, t corev1.HostPathType) corev1.Volume {
	v := corev1.Volume{Name: name, VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: path}}}
	if t != "" {
		v.HostPath.Type = &t
	}
	return v
}

func memory(req, limit string) corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10m"), corev1.ResourceMemory: resource.MustParse(req)},
		Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse(limit)},
	}
}

var nodeNameEnv = corev1.EnvVar{Name: "NODE_NAME", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "spec.nodeName"}}}

// daemonSet fills the parts both node components share. Rolling updates
// remove the old pod before starting the new one: two plugins registering
// blinkt.kubedge.io on a node must never overlap.
func daemonSet(ds *appsv1.DaemonSet, cfg *blinktv1.BlinktConfig, name, sa string, c corev1.Container, vols []corev1.Volume) {
	labels := managedLabels(name)
	one := intstr.FromInt32(1)
	zero := intstr.FromInt32(0)
	grace := int64(10)
	privileged := true
	root := int64(0)
	c.Env = []corev1.EnvVar{nodeNameEnv}
	// Root as well as privileged: the image's default user (65532, for the
	// manager) cannot create the kubelet plugin sockets.
	c.SecurityContext = &corev1.SecurityContext{Privileged: &privileged, RunAsUser: &root}
	ds.Labels = labels
	ds.Spec = appsv1.DaemonSetSpec{
		Selector: &metav1.LabelSelector{MatchLabels: map[string]string{labelName: name}},
		UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
			Type:          appsv1.RollingUpdateDaemonSetStrategyType,
			RollingUpdate: &appsv1.RollingUpdateDaemonSet{MaxUnavailable: &one, MaxSurge: &zero},
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: labels},
			Spec: corev1.PodSpec{
				ServiceAccountName:            sa,
				PriorityClassName:             "system-node-critical",
				NodeSelector:                  nodeSelector(cfg),
				Tolerations:                   tolerations(cfg),
				TerminationGracePeriodSeconds: &grace,
				Containers:                    []corev1.Container{c},
				Volumes:                       vols,
			},
		},
	}
}

func mount(name, path string, ro bool) corev1.VolumeMount {
	return corev1.VolumeMount{Name: name, MountPath: path, ReadOnly: ro}
}

// MutateDriverDaemonSet sets the cdi-mode DaemonSet, equal to
// dra-driver/deploy/blinkt-dra-driver.yaml. extraArgs lets test clusters add
// --assume-gpio.
func MutateDriverDaemonSet(ds *appsv1.DaemonSet, cfg *blinktv1.BlinktConfig, extraArgs []string) {
	image := cfg.Spec.Images.Driver
	if image == "" {
		image = DefaultDriverImage
	}
	daemonSet(ds, cfg, DriverDaemonSet, "blinkt-dra-driver", corev1.Container{
		Name:            "plugin",
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args:            append([]string{"--state-dir=/etc/kubedge", "--cdi-dir=/var/run/cdi"}, extraArgs...),
		Resources:       memory("24Mi", "64Mi"),
		VolumeMounts: []corev1.VolumeMount{
			mount("plugins-registry", "/var/lib/kubelet/plugins_registry", false),
			mount("plugins", "/var/lib/kubelet/plugins", false),
			mount("cdi", "/var/run/cdi", false),
			mount("dev", "/dev", false),
			mount("state", "/etc/kubedge", true),
		},
	}, []corev1.Volume{
		hostPath("plugins-registry", "/var/lib/kubelet/plugins_registry", ""),
		hostPath("plugins", "/var/lib/kubelet/plugins", ""),
		hostPath("cdi", "/var/run/cdi", corev1.HostPathDirectoryOrCreate),
		hostPath("dev", "/dev", ""),
		hostPath("state", "/etc/kubedge", corev1.HostPathDirectoryOrCreate),
	})
}

// MutateAgentDaemonSet sets the agent-mode DaemonSet. extraArgs lets test
// clusters add --fake-gpio.
func MutateAgentDaemonSet(ds *appsv1.DaemonSet, cfg *blinktv1.BlinktConfig, defaultAgentImage string, extraArgs []string) {
	image := agentImage(cfg, defaultAgentImage)
	args := append([]string{"--cdi-dir=/var/run/cdi"}, agentArgs(cfg, extraArgs)...)
	daemonSet(ds, cfg, AgentDaemonSet, "blinkt-node-agent", corev1.Container{
		Name:            "agent",
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"/node-agent"},
		Args:            args,
		Resources:       memory("32Mi", "64Mi"),
		VolumeMounts: []corev1.VolumeMount{
			mount("plugins-registry", "/var/lib/kubelet/plugins_registry", false),
			mount("plugins", "/var/lib/kubelet/plugins", false),
			mount("cdi", "/var/run/cdi", false),
			mount("dev", "/dev", false),
			mount("state", "/etc/kubedge", false),
		},
	}, []corev1.Volume{
		hostPath("plugins-registry", "/var/lib/kubelet/plugins_registry", ""),
		hostPath("plugins", "/var/lib/kubelet/plugins", ""),
		hostPath("cdi", "/var/run/cdi", corev1.HostPathDirectoryOrCreate),
		hostPath("dev", "/dev", ""),
		hostPath("state", "/etc/kubedge", corev1.HostPathDirectoryOrCreate),
	})
}

// agentArgs are the node-agent flags shared by the DaemonSet and the clear Job.
func agentArgs(cfg *blinktv1.BlinktConfig, extraArgs []string) []string {
	args := []string{"--state-dir=/etc/kubedge"}
	if cfg.Spec.LegacyCompat {
		args = append(args, "--legacy-compat")
	}
	return append(args, extraArgs...)
}

func agentImage(cfg *blinktv1.BlinktConfig, def string) string {
	if cfg.Spec.Images.Agent != "" {
		return cfg.Spec.Images.Agent
	}
	return def
}

// ClearJobName is the one-shot Job that clears the agent's pixels on node.
func ClearJobName(node string) string { return "blinkt-clear-" + node }

// MutateClearJob sets the Job that runs `/node-agent --clear` on node after
// the agent left it (it needs no API access).
func MutateClearJob(job *batchv1.Job, cfg *blinktv1.BlinktConfig, node, image string, extraArgs []string) {
	privileged := true
	root := int64(0)
	backoff := int32(3)
	ttl := int32(600)
	noToken := false
	labels := managedLabels("blinkt-clear")
	labels[labelNode] = node
	job.Labels = labels
	job.Spec = batchv1.JobSpec{
		BackoffLimit:            &backoff,
		TTLSecondsAfterFinished: &ttl,
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: labels},
			Spec: corev1.PodSpec{
				NodeName:                     node,
				RestartPolicy:                corev1.RestartPolicyNever,
				AutomountServiceAccountToken: &noToken,
				Tolerations:                  []corev1.Toleration{{Operator: corev1.TolerationOpExists}},
				Containers: []corev1.Container{{
					Name:            "clear",
					Image:           agentImage(cfg, image),
					ImagePullPolicy: corev1.PullIfNotPresent,
					Command:         []string{"/node-agent"},
					Args:            append([]string{"--clear"}, agentArgs(cfg, extraArgs)...),
					SecurityContext: &corev1.SecurityContext{Privileged: &privileged, RunAsUser: &root},
					VolumeMounts:    []corev1.VolumeMount{mount("dev", "/dev", false), mount("state", "/etc/kubedge", false)},
				}},
				Volumes: []corev1.Volume{
					hostPath("dev", "/dev", ""),
					hostPath("state", "/etc/kubedge", corev1.HostPathDirectoryOrCreate),
				},
			},
		},
	}
}

// MutateDeviceClass sets the DeviceClass, carrying spec.defaults as its
// PixelConfig (FromClass) when given.
func MutateDeviceClass(dc *resourceapi.DeviceClass, cfg *blinktv1.BlinktConfig) error {
	dc.Labels = managedLabels(DeviceClassName)
	dc.Spec = resourceapi.DeviceClassSpec{
		Selectors: []resourceapi.DeviceSelector{{CEL: &resourceapi.CELDeviceSelector{Expression: `device.driver == "` + DriverName + `"`}}},
	}
	if cfg.Spec.Defaults == nil {
		return nil
	}
	raw, err := json.Marshal(blinktv1.PixelConfig{
		APIVersion:      blinktv1.GroupVersion.String(),
		Kind:            blinktv1.PixelConfigKind,
		PixelConfigSpec: *cfg.Spec.Defaults,
	})
	if err != nil {
		return err
	}
	dc.Spec.Config = []resourceapi.DeviceClassConfiguration{{DeviceConfiguration: resourceapi.DeviceConfiguration{
		Opaque: &resourceapi.OpaqueDeviceConfiguration{Driver: DriverName, Parameters: runtime.RawExtension{Raw: raw}},
	}}}
	return nil
}
