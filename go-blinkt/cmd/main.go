package main

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/kubedge/kubesim_blinkt/go-blinkt/pkg/config"
	"github.com/kubedge/kubesim_blinkt/go-blinkt/pkg/ledstate"
	"github.com/kubedge/kubesim_blinkt/go-blinkt/pkg/periBlink"
)

// defaultFrequency applies when the config omits frequency (the kubesim
// charts do): without it fixed5 would redraw in a tight loop.
const defaultFrequency = 1000

func delay(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

// draw writes one merged frame. The GPIO lines are held only for the frame,
// so other blinkt processes on the node can draw theirs.
func draw(f ledstate.Frame) error {
	for i, p := range f {
		periBlink.SetPixel(i, p.R, p.G, p.B, p.L)
	}
	if err := periBlink.Setup(); err != nil {
		return err
	}
	return errors.Join(periBlink.Show(), periBlink.Release())
}

// publish shows this process's pixels and aborts on GPIO failure, so a broken
// GPIO setup shows up in the pod log instead of as dark LEDs.
func publish(board *ledstate.Board, px map[int]ledstate.Pixel) {
	if err := board.Publish(px); err != nil {
		log.Fatalf("blinkt: %v", err)
	}
}

// blinkt5 sets random colours on random pixels among those it owns.
func blinkt5(running *atomic.Bool, board *ledstate.Board, owned []int) {
	px := map[int]ledstate.Pixel{}
	for running.Load() {
		px[owned[rand.Intn(len(owned))]] = ledstate.Pixel{R: rand.Intn(256), G: rand.Intn(256), B: rand.Intn(256), L: rand.Intn(3)}
		publish(board, px)
		delay(60)
	}
}

// ownedPixels returns the pixels this process lights, at the configured
// intensity: the configured ones, or with a DRA allocation (BLINKT_PIXELS)
// exactly the allocated ones.
func ownedPixels(conf config.BlinktConfigData, alloc []int, allocated bool) map[int]ledstate.Pixel {
	colours := conf.Colours()
	if allocated {
		colours = config.Allocate(colours, alloc)
	}
	px := map[int]ledstate.Pixel{}
	for i, c := range colours {
		px[i] = ledstate.Pixel{R: c[0], G: c[1], B: c[2], L: conf.Intensity}
	}
	return px
}

// darkDelay is how long fixed5 leaves its LEDs off between blinks.
func darkDelay(conf config.BlinktConfigData) int {
	if conf.Algorithm == "fixed5" {
		// We only leave the led dark for
		// a couple of milliseconds
		return 10
	}
	return conf.Frequency
}

func fixed5(running *atomic.Bool, board *ledstate.Board, conf config.BlinktConfigData, on map[int]ledstate.Pixel) {
	off := map[int]ledstate.Pixel{}
	for running.Load() {
		publish(board, on)
		delay(conf.Frequency)
		publish(board, off)
		delay(darkDelay(conf))
	}
}

// owner names this process in the shared state: the pod name in Kubernetes.
func owner() string {
	if o := os.Getenv("BLINKT_OWNER"); o != "" {
		return o
	}
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return fmt.Sprintf("pid-%d", os.Getpid())
}

// stateDir is the host directory shared by every blinkt process on the node.
func stateDir() string {
	if d, ok := os.LookupEnv("BLINKT_STATE_DIR"); ok {
		return d
	}
	return "/etc/kubedge"
}

func main() {
	var running atomic.Bool
	running.Store(true)
	// initialise getout
	signalChannel := make(chan os.Signal, 2)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-signalChannel
		switch sig {
		case os.Interrupt:
			fmt.Println("Stopping on Interrupt")
		case syscall.SIGTERM:
			fmt.Println("Stopping on Terminate")
		}
		running.Store(false)
	}()

	// Check the GPIO lines once, then hand them back: other blinkt
	// processes on this node may be drawing.
	if err := periBlink.Setup(); err != nil {
		log.Fatalf("blinkt: GPIO setup failed: %v", err)
	}
	log.Printf("blinkt: GPIO ready (%s)", periBlink.Lines())
	if err := periBlink.Release(); err != nil {
		log.Fatalf("blinkt: GPIO release failed: %v", err)
	}

	var conf config.BlinktConfigData
	conf.Config()
	if conf.Frequency <= 0 {
		conf.Frequency = defaultFrequency
	}
	// A DRA claim says which pixels this process owns.
	alloc := []int{0, 1, 2, 3, 4, 5, 6, 7}
	envPixels, allocated := os.LookupEnv("BLINKT_PIXELS")
	if allocated {
		var err error
		if alloc, err = config.ParsePixels(envPixels); err != nil {
			log.Fatalf("blinkt: %v", err)
		}
	}

	// An entry outlives a few missed redraws before others stop drawing it.
	ttl := 3*time.Duration(conf.Frequency+darkDelay(conf))*time.Millisecond + 2*time.Second
	board, err := ledstate.Open(stateDir(), owner(), ttl, draw)
	mode := "shared state=" + stateDir()
	if err != nil {
		mode = fmt.Sprintf("solo (%v)", err)
	}
	// Deploy verification greps for this line.
	log.Printf("blinkt: running algorithm=%s frequency=%dms config=%s owner=%s %s",
		conf.Algorithm, conf.Frequency, config.Path(), owner(), mode)

	if conf.Algorithm == "blinkt5" {
		blinkt5(&running, board, alloc)
	} else {
		fixed5(&running, board, conf, ownedPixels(conf, alloc, allocated))
	}
	fmt.Println("Stopping")
	// Turn off only this process's LEDs; others on the node keep theirs.
	if err := board.Withdraw(); err != nil {
		log.Printf("blinkt: exit: %v", err)
	}
	board.Close()
}
