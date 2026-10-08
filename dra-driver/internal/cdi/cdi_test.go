package cdi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writer(t *testing.T) Writer {
	return Writer{Dir: t.TempDir(), GPIODevice: "/dev/gpiochip0", StateDir: "/etc/kubedge"}
}

func read(t *testing.T, w Writer, uid string) Spec {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(w.Dir, "blinkt.kubedge.io-"+uid+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var s Spec
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestOnePixelClaim(t *testing.T) {
	w := writer(t)
	if err := w.Write("uid-1", []int{6}); err != nil {
		t.Fatal(err)
	}
	s := read(t, w, "uid-1")
	if s.Kind != "blinkt.kubedge.io/claim" || s.Version != "0.6.0" || len(s.Devices) != 1 || s.Devices[0].Name != "uid-1" {
		t.Fatalf("spec = %+v", s)
	}
	e := s.Devices[0].ContainerEdits
	if !reflect.DeepEqual(e.Env, []string{"BLINKT_STATE_DIR=/run/blinkt", "BLINKT_PIXELS=6"}) {
		t.Errorf("env = %v", e.Env)
	}
	if !reflect.DeepEqual(e.DeviceNodes, []DeviceNode{{Path: "/dev/gpiochip0"}}) {
		t.Errorf("deviceNodes = %v", e.DeviceNodes)
	}
	if !reflect.DeepEqual(e.Mounts, []Mount{{HostPath: "/etc/kubedge", ContainerPath: "/run/blinkt", Options: []string{"rbind", "rw"}}}) {
		t.Errorf("mounts = %v", e.Mounts)
	}
	if DeviceID("uid-1") != "blinkt.kubedge.io/claim=uid-1" {
		t.Errorf("DeviceID = %s", DeviceID("uid-1"))
	}
}

func TestAllPixelsClaim(t *testing.T) {
	w := writer(t)
	w.Write("uid-all", []int{0, 1, 2, 3, 4, 5, 6, 7})
	if env := read(t, w, "uid-all").Devices[0].ContainerEdits.Env; env[1] != "BLINKT_PIXELS=0,1,2,3,4,5,6,7" {
		t.Errorf("env = %v", env)
	}
}

func TestStateDirFollowsSetting(t *testing.T) {
	w := writer(t)
	w.StateDir = "/var/lib/blinkt"
	w.Write("u", []int{1})
	if m := read(t, w, "u").Devices[0].ContainerEdits.Mounts[0]; m.HostPath != "/var/lib/blinkt" {
		t.Errorf("mount = %+v", m)
	}
}

func TestWriteAndRemoveAreIdempotent(t *testing.T) {
	w := writer(t)
	for i := 0; i < 2; i++ {
		if err := w.Write("u", []int{2}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := w.Remove("u"); err != nil {
			t.Fatalf("remove %d: %v", i, err)
		}
	}
	if left, _ := os.ReadDir(w.Dir); len(left) != 0 {
		t.Errorf("leaked files: %v", left)
	}
}
