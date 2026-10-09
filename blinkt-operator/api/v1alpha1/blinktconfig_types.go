package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Mode selects how the cluster runs its Blinkt! LEDs.
// +kubebuilder:validation:Enum=legacy;cdi;agent
type Mode string

const (
	// ModeLegacy runs no DRA components: simulators use privileged sidecars.
	ModeLegacy Mode = "legacy"
	// ModeCDI runs the dra-driver DaemonSet: claims hand the device to the
	// pod's own blinkt sidecar through CDI (the DaemonSet approach).
	ModeCDI Mode = "cdi"
	// ModeAgent runs the node agent, the sole GPIO writer, which draws the
	// strip from the ResourceClaims and their PixelConfig.
	ModeAgent Mode = "agent"
)

// SingletonName is the only BlinktConfig the operator acts on.
const SingletonName = "cluster"

// Images overrides the node component images.
type Images struct {
	// Driver is the dra-driver image used in cdi mode.
	// +optional
	Driver string `json:"driver,omitempty"`
	// Agent is the image holding /node-agent, used in agent mode.
	// +optional
	Agent string `json:"agent,omitempty"`
}

// BlinktConfigSpec is the desired Blinkt! setup of the cluster.
type BlinktConfigSpec struct {
	// Mode selects legacy, cdi or agent.
	// +kubebuilder:default=legacy
	// +optional
	Mode Mode `json:"mode,omitempty"`

	// NodeSelector for the node components. Defaults to arm64 nodes.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations for the node components. Defaults to tolerating the
	// control-plane taint.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Images overrides the node component images.
	// +optional
	Images Images `json:"images,omitempty"`

	// Defaults is the PixelConfig carried by the DeviceClass; claims
	// override it field by field.
	// +optional
	Defaults *PixelConfigSpec `json:"defaults,omitempty"`

	// LegacyCompat makes the agent share the strip with legacy blinkt
	// sidecars through the shared-state file, for nodes being migrated.
	// +optional
	LegacyCompat bool `json:"legacyCompat,omitempty"`
}

// NodeCounts reports the active DaemonSet's rollout.
type NodeCounts struct {
	Desired int32 `json:"desired"`
	Ready   int32 `json:"ready"`
}

// BlinktConfigStatus is the observed Blinkt! setup.
type BlinktConfigStatus struct {
	// Mode in effect.
	// +optional
	Mode Mode `json:"mode,omitempty"`
	// Nodes of the active DaemonSet.
	// +optional
	Nodes NodeCounts `json:"nodes,omitempty"`
	// Devices is the number of published blinkt.kubedge.io devices.
	// +optional
	Devices int32 `json:"devices,omitempty"`
	// ObservedGeneration of the spec this status reflects.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Conditions: Ready.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// BlinktConfig selects how the cluster runs its Blinkt! LEDs. The operator
// acts only on the instance named "cluster".
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=blinkt
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.mode`
// +kubebuilder:printcolumn:name="Active",type=string,JSONPath=`.status.mode`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Nodes",type=string,JSONPath=`.status.nodes.ready`
// +kubebuilder:printcolumn:name="Devices",type=integer,JSONPath=`.status.devices`
type BlinktConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BlinktConfigSpec   `json:"spec,omitempty"`
	Status BlinktConfigStatus `json:"status,omitempty"`
}

// BlinktConfigList is a list of BlinktConfig.
// +kubebuilder:object:root=true
type BlinktConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BlinktConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BlinktConfig{}, &BlinktConfigList{})
}
