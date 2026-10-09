package agent

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/kubedge/kubesim_blinkt/go-blinkt/pkg/ledstate"
	"github.com/kubedge/kubesim_blinkt/go-blinkt/pkg/periBlink"
)

// Frame is the 8-pixel strip state the agent draws.
type Frame = ledstate.Frame

// Writer puts frames on the strip.
type Writer interface {
	// Draw shows the frame.
	Draw(f Frame) error
	// Close leaves the strip dark (or, in compat mode, withdraws this
	// owner's pixels) and releases the GPIO lines.
	Close() error
	// Describe names where frames go, for the start-up log.
	Describe() string
}

func writeFrame(f Frame) error {
	for i, p := range f {
		periBlink.SetPixel(i, p.R, p.G, p.B, p.L)
	}
	return periBlink.Show()
}

// exclusiveWriter holds GPIO23/24 for the agent's lifetime: the agent is the
// node's only writer.
type exclusiveWriter struct{}

// NewExclusiveWriter acquires the lines; failure means no usable Blinkt! lines.
func NewExclusiveWriter() (Writer, error) {
	if err := periBlink.Setup(); err != nil {
		return nil, err
	}
	return exclusiveWriter{}, nil
}

func (exclusiveWriter) Draw(f Frame) error { return writeFrame(f) }
func (exclusiveWriter) Describe() string   { return "exclusive " + periBlink.Lines() }

func (exclusiveWriter) Close() error {
	periBlink.Clear()
	return errors.Join(periBlink.Show(), periBlink.Release())
}

// compatWriter shares the strip with legacy blinkt sidecars: it is one owner
// in the shared-state protocol and holds the lines only per frame.
type compatWriter struct {
	board *ledstate.Board
	dir   string
	lines string
}

// CompatOwner is the agent's name in the shared state.
const CompatOwner = "blinkt-node-agent"

// NewCompatWriter checks the lines once, releases them, and joins the
// shared state in dir.
func NewCompatWriter(dir string) (Writer, error) {
	if err := periBlink.Setup(); err != nil {
		return nil, err
	}
	lines := periBlink.Lines()
	if err := periBlink.Release(); err != nil {
		return nil, err
	}
	draw := func(f ledstate.Frame) error {
		if err := periBlink.Setup(); err != nil {
			return err
		}
		return errors.Join(writeFrame(f), periBlink.Release())
	}
	// Entries outlive a few missed refreshes (the agent refreshes every second).
	board, err := ledstate.Open(dir, CompatOwner, 5*time.Second, draw)
	if err != nil {
		return nil, fmt.Errorf("shared state %s: %w", dir, err)
	}
	return &compatWriter{board: board, dir: dir, lines: lines}, nil
}

// Draw publishes the agent's lit pixels; the board merges them with the
// legacy owners' and draws the merged frame.
func (w *compatWriter) Draw(f Frame) error {
	px := map[int]ledstate.Pixel{}
	for i, p := range f {
		if p != (ledstate.Pixel{}) {
			px[i] = p
		}
	}
	return w.board.Publish(px)
}

func (w *compatWriter) Close() error {
	return errors.Join(w.board.Withdraw(), w.board.Close())
}

func (w *compatWriter) Describe() string {
	return fmt.Sprintf("legacy-compat %s state=%s", w.lines, w.dir)
}

// fakeWriter logs frames instead of driving GPIO (test clusters only).
type fakeWriter struct{ logf func(string, ...any) }

// NewFakeWriter logs each drawn frame.
func NewFakeWriter() Writer { return fakeWriter{logf: log.Printf} }

func (w fakeWriter) Draw(f Frame) error {
	w.logf("blinkt-agent: frame %s", FormatFrame(f))
	return nil
}
func (w fakeWriter) Close() error {
	w.logf("blinkt-agent: frame %s", FormatFrame(Frame{}))
	return nil
}
func (fakeWriter) Describe() string { return "fake (frames logged)" }

// FormatFrame renders a frame as 8 entries "r,g,b,l" or "-" for dark.
func FormatFrame(f Frame) string {
	parts := make([]string, len(f))
	for i, p := range f {
		if p == (ledstate.Pixel{}) {
			parts[i] = "-"
		} else {
			parts[i] = fmt.Sprintf("%d,%d,%d,%d", p.R, p.G, p.B, p.L)
		}
	}
	return "[" + strings.Join(parts, " ") + "]"
}
