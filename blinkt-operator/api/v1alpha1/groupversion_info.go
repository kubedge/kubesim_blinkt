// Package v1alpha1 contains the blinkt.kubedge.io/v1alpha1 API: the
// BlinktConfig custom resource that selects how a cluster runs its Blinkt!
// LEDs, and PixelConfig, the opaque device configuration claims carry.
// +kubebuilder:object:generate=true
// +groupName=blinkt.kubedge.io
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	// GroupVersion is the API group and version of these types.
	GroupVersion = schema.GroupVersion{Group: "blinkt.kubedge.io", Version: "v1alpha1"}

	// SchemeBuilder registers the types with a scheme.
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types to a scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)
