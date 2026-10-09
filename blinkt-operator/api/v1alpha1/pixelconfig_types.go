package v1alpha1

// PixelConfigKind is the kind of the opaque device configuration a claim (or
// the DeviceClass) passes to driver blinkt.kubedge.io.
const PixelConfigKind = "PixelConfig"

// PixelConfigSpec is the colour, intensity and pattern of claimed pixels.
// Omitted fields take defaults; claim values override class values.
type PixelConfigSpec struct {
	// Color is red, green, blue, each 0-255.
	// +kubebuilder:validation:MinItems=3
	// +kubebuilder:validation:MaxItems=3
	// +optional
	Color []int32 `json:"color,omitempty"`
	// Intensity is the APA102 luminance, 0-31.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=31
	// +optional
	Intensity *int32 `json:"intensity,omitempty"`
	// Algorithm: steady, fixed5 (lit frequency ms, dark 10 ms) or fixed
	// (lit and dark frequency ms each).
	// +kubebuilder:validation:Enum=steady;fixed5;fixed
	// +optional
	Algorithm string `json:"algorithm,omitempty"`
	// Frequency in milliseconds.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Frequency *int32 `json:"frequency,omitempty"`
}

// PixelConfig is the opaque parameters object:
// {apiVersion: blinkt.kubedge.io/v1alpha1, kind: PixelConfig, ...}.
// It is decoded by the node agent, not served by the API server.
type PixelConfig struct {
	APIVersion      string `json:"apiVersion"`
	Kind            string `json:"kind"`
	PixelConfigSpec `json:",inline"`
}
