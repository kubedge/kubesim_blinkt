/*
Copyright 2026 Kubedge

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package ledstate lets several blinkt processes on one node share the eight
// Blinkt! LEDs. Each process (typically a sidecar in a kubesim pod) publishes
// only the pixels it owns into a JSON file in a shared host directory. Under
// an exclusive flock it merges every owner's pixels into one frame and draws
// it, so co-located simulators each keep their own LED lit.
//
// Entries carry a TTL: a process that dies without withdrawing stops being
// drawn once its entry expires and another process redraws.
package ledstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"
)

// NumPixels is the number of LEDs on a Blinkt!.
const NumPixels = 8

const (
	lockName  = "blinkt.lock"
	stateName = "blinkt_state.json"
)

// Pixel is one LED: colour 0-255 and luminance 0-31.
type Pixel struct {
	R int `json:"r"`
	G int `json:"g"`
	B int `json:"b"`
	L int `json:"l"`
}

// Frame is what gets drawn: one Pixel per LED, zero means dark.
type Frame [NumPixels]Pixel

// Entry is one owner's contribution.
type Entry struct {
	Pixels  map[int]Pixel `json:"pixels"`
	Updated time.Time     `json:"updated"`
	TTL     time.Duration `json:"ttl"`
}

// State is the content of the shared file.
type State struct {
	Owners map[string]Entry `json:"owners"`
}

// Prune drops entries that were not refreshed within their TTL.
func (s *State) Prune(now time.Time) {
	for name, e := range s.Owners {
		if now.Sub(e.Updated) > e.TTL {
			delete(s.Owners, name)
		}
	}
}

// Frame merges all owners. When two owners claim the same LED, the owner
// whose name sorts last wins, so every process draws the same frame.
func (s *State) Frame() Frame {
	names := make([]string, 0, len(s.Owners))
	for name := range s.Owners {
		names = append(names, name)
	}
	sort.Strings(names)
	var f Frame
	for _, name := range names {
		for i, p := range s.Owners[name].Pixels {
			if i >= 0 && i < NumPixels {
				f[i] = p
			}
		}
	}
	return f
}

// Board publishes one owner's pixels and draws the merged frame.
type Board struct {
	dir    string
	owner  string
	ttl    time.Duration
	render func(Frame) error
	now    func() time.Time
	lock   *os.File // nil in solo mode
	solo   State
}

// Open joins the shared state in dir. When dir is empty or not writable, the
// board runs solo: it draws only its own pixels, and Shared reports false
// with the reason.
func Open(dir, owner string, ttl time.Duration, render func(Frame) error) (*Board, error) {
	b := &Board{
		dir:    dir,
		owner:  owner,
		ttl:    ttl,
		render: render,
		now:    time.Now,
		solo:   State{Owners: map[string]Entry{}},
	}
	if dir == "" {
		return b, errors.New("no shared state directory configured")
	}
	f, err := os.OpenFile(filepath.Join(dir, lockName), os.O_CREATE|os.O_RDWR, 0o666)
	if err != nil {
		return b, err
	}
	b.lock = f
	return b, nil
}

// Shared reports whether the board coordinates with other processes.
func (b *Board) Shared() bool {
	return b.lock != nil
}

// Publish replaces this owner's pixels and redraws.
func (b *Board) Publish(pixels map[int]Pixel) error {
	return b.update(func(s *State) {
		s.Owners[b.owner] = Entry{Pixels: pixels, Updated: b.now(), TTL: b.ttl}
	})
}

// Withdraw removes this owner's pixels and redraws whatever the others own.
func (b *Board) Withdraw() error {
	return b.update(func(s *State) {
		delete(s.Owners, b.owner)
	})
}

// Close releases the lock file.
func (b *Board) Close() error {
	if b.lock == nil {
		return nil
	}
	err := b.lock.Close()
	b.lock = nil
	return err
}

func (b *Board) update(change func(*State)) error {
	if b.lock == nil {
		change(&b.solo)
		return b.render(b.solo.Frame())
	}
	if err := syscall.Flock(int(b.lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("ledstate: lock: %w", err)
	}
	defer syscall.Flock(int(b.lock.Fd()), syscall.LOCK_UN)

	s := b.load()
	change(&s)
	s.Prune(b.now())
	err := b.render(s.Frame())
	return errors.Join(err, b.save(s))
}

// load reads the shared state; a missing or corrupt file starts empty.
func (b *Board) load() State {
	s := State{}
	if data, err := os.ReadFile(filepath.Join(b.dir, stateName)); err == nil {
		_ = json.Unmarshal(data, &s)
	}
	if s.Owners == nil {
		s.Owners = map[string]Entry{}
	}
	return s
}

func (b *Board) save(s State) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := filepath.Join(b.dir, stateName+".tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("ledstate: save: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(b.dir, stateName)); err != nil {
		return fmt.Errorf("ledstate: save: %w", err)
	}
	return nil
}
