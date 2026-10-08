package ledstate

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

var (
	red   = Pixel{R: 255, L: 5}
	green = Pixel{G: 255, L: 5}
	blue  = Pixel{B: 255, L: 5}
)

// recorder keeps the last frame drawn by any board.
type recorder struct {
	mu     sync.Mutex
	frames []Frame
}

func (r *recorder) draw(f Frame) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frames = append(r.frames, f)
	return nil
}

func (r *recorder) last(t *testing.T) Frame {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.frames) == 0 {
		t.Fatal("nothing drawn")
	}
	return r.frames[len(r.frames)-1]
}

func open(t *testing.T, dir, owner string, r *recorder) *Board {
	t.Helper()
	b, err := Open(dir, owner, time.Minute, r.draw)
	if err != nil {
		t.Fatalf("Open(%s): %v", owner, err)
	}
	t.Cleanup(func() { b.Close() })
	return b
}

func TestCoLocatedOwnersKeepTheirOwnPixels(t *testing.T) {
	dir := t.TempDir()
	r := &recorder{}
	lte := open(t, dir, "kubesim-lte-abc", r)
	elte := open(t, dir, "kubesim-elte-def", r)

	if err := lte.Publish(map[int]Pixel{6: blue}); err != nil {
		t.Fatal(err)
	}
	if err := elte.Publish(map[int]Pixel{4: green}); err != nil {
		t.Fatal(err)
	}
	f := r.last(t)
	if f[6] != blue || f[4] != green {
		t.Fatalf("frame = %+v, want pixel4 green and pixel6 blue", f)
	}

	// elte blinking off must not turn lte's LED off.
	if err := elte.Publish(map[int]Pixel{}); err != nil {
		t.Fatal(err)
	}
	if f := r.last(t); f[6] != blue || f[4] != (Pixel{}) {
		t.Fatalf("after elte off: frame = %+v", f)
	}
}

func TestWithdrawLeavesOthersLit(t *testing.T) {
	dir := t.TempDir()
	r := &recorder{}
	a := open(t, dir, "a", r)
	b := open(t, dir, "b", r)
	a.Publish(map[int]Pixel{0: red})
	b.Publish(map[int]Pixel{7: blue})

	if err := b.Withdraw(); err != nil {
		t.Fatal(err)
	}
	if f := r.last(t); f[0] != red || f[7] != (Pixel{}) {
		t.Fatalf("frame = %+v, want only pixel0 red", f)
	}
}

func TestExpiredOwnerIsNotDrawn(t *testing.T) {
	dir := t.TempDir()
	r := &recorder{}
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

	crashed, _ := Open(dir, "crashed", 5*time.Second, r.draw)
	crashed.now = func() time.Time { return now }
	crashed.Publish(map[int]Pixel{3: red})
	crashed.Close() // dies without Withdraw

	alive := open(t, dir, "alive", r)
	alive.now = func() time.Time { return now.Add(6 * time.Second) }
	alive.Publish(map[int]Pixel{5: green})

	if f := r.last(t); f[3] != (Pixel{}) || f[5] != green {
		t.Fatalf("frame = %+v, want expired pixel3 dropped", f)
	}
}

func TestConflictingClaimsResolveByOwnerName(t *testing.T) {
	s := State{Owners: map[string]Entry{
		"kubesim-nr":  {Pixels: map[int]Pixel{7: red}},
		"kubesim-5gc": {Pixels: map[int]Pixel{7: blue}},
	}}
	if f := s.Frame(); f[7] != red {
		t.Fatalf("pixel7 = %+v, want the owner sorting last (kubesim-nr) to win", f[7])
	}
}

func TestOutOfRangePixelsIgnored(t *testing.T) {
	s := State{Owners: map[string]Entry{"x": {Pixels: map[int]Pixel{-1: red, 8: red, 2: green}}}}
	f := s.Frame()
	if f[2] != green {
		t.Fatalf("frame = %+v", f)
	}
}

func TestConcurrentPublishersNeverLoseEachOther(t *testing.T) {
	dir := t.TempDir()
	r := &recorder{}
	const n = 4
	boards := make([]*Board, n)
	for i := range boards {
		boards[i] = open(t, dir, string(rune('a'+i)), r)
	}
	var wg sync.WaitGroup
	for i, b := range boards {
		wg.Add(1)
		go func(i int, b *Board) {
			defer wg.Done()
			for k := 0; k < 25; k++ {
				if err := b.Publish(map[int]Pixel{i: red}); err != nil {
					t.Error(err)
					return
				}
			}
		}(i, b)
	}
	wg.Wait()
	// One more draw after everyone has published at least once.
	boards[0].Publish(map[int]Pixel{0: red})
	f := r.last(t)
	for i := 0; i < n; i++ {
		if f[i] != red {
			t.Fatalf("pixel%d lost: frame = %+v", i, f)
		}
	}
}

func TestCorruptStateFileStartsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, stateName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &recorder{}
	b := open(t, dir, "a", r)
	if err := b.Publish(map[int]Pixel{1: green}); err != nil {
		t.Fatal(err)
	}
	if f := r.last(t); f[1] != green {
		t.Fatalf("frame = %+v", f)
	}
}

func TestSoloWhenDirectoryUnusable(t *testing.T) {
	r := &recorder{}
	b, err := Open(filepath.Join(t.TempDir(), "missing"), "a", time.Minute, r.draw)
	if err == nil {
		t.Fatal("Open succeeded on a missing directory, want a solo-mode reason")
	}
	if b.Shared() {
		t.Fatal("Shared() = true in solo mode")
	}
	if err := b.Publish(map[int]Pixel{2: blue}); err != nil {
		t.Fatal(err)
	}
	if f := r.last(t); f[2] != blue {
		t.Fatalf("frame = %+v", f)
	}
}

func TestRenderErrorIsReturned(t *testing.T) {
	boom := errors.New("gpio gone")
	b, err := Open(t.TempDir(), "a", time.Minute, func(Frame) error { return boom })
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if err := b.Publish(map[int]Pixel{0: red}); !errors.Is(err, boom) {
		t.Fatalf("Publish error = %v, want %v", err, boom)
	}
}
