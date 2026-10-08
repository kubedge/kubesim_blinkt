// Package cdi writes and removes the per-claim CDI spec files through which
// the container runtime hands a claimed Blinkt! to a container: the GPIO
// device node, the shared LED state directory and the allocated pixels.
package cdi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// Vendor/class of the CDI devices this driver creates.
	Kind = "blinkt.kubedge.io/claim"
	// Version of the CDI spec format written.
	Version = "0.6.0"
	// ContainerStateDir is where the shared state directory appears.
	ContainerStateDir = "/run/blinkt"
	filePrefix        = "blinkt.kubedge.io-"
)

// Spec is the subset of the CDI spec format this driver writes.
type Spec struct {
	Version string   `json:"cdiVersion"`
	Kind    string   `json:"kind"`
	Devices []Device `json:"devices"`
}

type Device struct {
	Name           string `json:"name"`
	ContainerEdits Edits  `json:"containerEdits"`
}

type Edits struct {
	Env         []string     `json:"env,omitempty"`
	DeviceNodes []DeviceNode `json:"deviceNodes,omitempty"`
	Mounts      []Mount      `json:"mounts,omitempty"`
}

type DeviceNode struct {
	Path string `json:"path"`
}

type Mount struct {
	HostPath      string   `json:"hostPath"`
	ContainerPath string   `json:"containerPath"`
	Options       []string `json:"options,omitempty"`
}

// Writer manages the spec files in one CDI directory.
type Writer struct {
	Dir        string // e.g. /var/run/cdi
	GPIODevice string // e.g. /dev/gpiochip0
	StateDir   string // host directory shared by blinkt processes, e.g. /etc/kubedge
}

// DeviceID is the fully qualified CDI device name for a claim.
func DeviceID(claimUID string) string {
	return Kind + "=" + claimUID
}

// SpecFor builds the CDI spec for a claim allocated the given pixels.
func (w Writer) SpecFor(claimUID string, pixels []int) Spec {
	idx := make([]string, len(pixels))
	for i, p := range pixels {
		idx[i] = strconv.Itoa(p)
	}
	return Spec{
		Version: Version,
		Kind:    Kind,
		Devices: []Device{{
			Name: claimUID,
			ContainerEdits: Edits{
				Env: []string{
					"BLINKT_STATE_DIR=" + ContainerStateDir,
					"BLINKT_PIXELS=" + strings.Join(idx, ","),
				},
				DeviceNodes: []DeviceNode{{Path: w.GPIODevice}},
				Mounts: []Mount{{
					HostPath:      w.StateDir,
					ContainerPath: ContainerStateDir,
					Options:       []string{"rbind", "rw"},
				}},
			},
		}},
	}
}

func (w Writer) path(claimUID string) string {
	return filepath.Join(w.Dir, filePrefix+claimUID+".json")
}

// Write creates or replaces the claim's spec file atomically.
func (w Writer) Write(claimUID string, pixels []int) error {
	data, err := json.MarshalIndent(w.SpecFor(claimUID, pixels), "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(w.Dir, 0o755); err != nil {
		return fmt.Errorf("cdi: %w", err)
	}
	tmp := w.path(claimUID) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("cdi: %w", err)
	}
	if err := os.Rename(tmp, w.path(claimUID)); err != nil {
		return fmt.Errorf("cdi: %w", err)
	}
	return nil
}

// Remove deletes the claim's spec file; a missing file is not an error.
func (w Writer) Remove(claimUID string) error {
	if err := os.Remove(w.path(claimUID)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("cdi: %w", err)
	}
	return nil
}
