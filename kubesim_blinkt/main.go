package main

import (
	"fmt"
	"github.com/kubedge/kubesim_blinkt/config"
	"github.com/kubedge/kubesim_blinkt/periBlink"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func delay(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

func blinkt5(running *bool, conf config.BlinktConfigData) {
	for *running {
		pixel := rand.Intn(8)
		periBlink.SetPixel(pixel, rand.Intn(255), rand.Intn(255), rand.Intn(255), rand.Intn(3))
		periBlink.Show()
		delay(60)
	}
}

func fixed5(running *bool, conf config.BlinktConfigData) {
	for *running {
		if len(conf.Pixel0) != 0 {
			periBlink.SetPixel(0, conf.Pixel0[0], conf.Pixel0[1], conf.Pixel0[2], conf.Intensity)
		}
		if len(conf.Pixel1) != 0 {
			periBlink.SetPixel(1, conf.Pixel1[0], conf.Pixel1[1], conf.Pixel1[2], conf.Intensity)
		}
		if len(conf.Pixel2) != 0 {
			periBlink.SetPixel(2, conf.Pixel2[0], conf.Pixel2[1], conf.Pixel2[2], conf.Intensity)
		}
		if len(conf.Pixel3) != 0 {
			periBlink.SetPixel(3, conf.Pixel3[0], conf.Pixel3[1], conf.Pixel3[2], conf.Intensity)
		}
		if len(conf.Pixel4) != 0 {
			periBlink.SetPixel(4, conf.Pixel4[0], conf.Pixel4[1], conf.Pixel4[2], conf.Intensity)
		}
		if len(conf.Pixel5) != 0 {
			periBlink.SetPixel(5, conf.Pixel5[0], conf.Pixel5[1], conf.Pixel5[2], conf.Intensity)
		}
		if len(conf.Pixel6) != 0 {
			periBlink.SetPixel(6, conf.Pixel6[0], conf.Pixel6[1], conf.Pixel6[2], conf.Intensity)
		}
		if len(conf.Pixel7) != 0 {
			periBlink.SetPixel(7, conf.Pixel7[0], conf.Pixel7[1], conf.Pixel7[2], conf.Intensity)
		}

		periBlink.Show()
		delay(conf.Frequency)

		if len(conf.Pixel0) != 0 {
			periBlink.SetPixel(0, 0, 0, 0, 0)
		}
		if len(conf.Pixel1) != 0 {
			periBlink.SetPixel(1, 0, 0, 0, 0)
		}
		if len(conf.Pixel2) != 0 {
			periBlink.SetPixel(2, 0, 0, 0, 0)
		}
		if len(conf.Pixel3) != 0 {
			periBlink.SetPixel(3, 0, 0, 0, 0)
		}
		if len(conf.Pixel4) != 0 {
			periBlink.SetPixel(4, 0, 0, 0, 0)
		}
		if len(conf.Pixel5) != 0 {
			periBlink.SetPixel(5, 0, 0, 0, 0)
		}
		if len(conf.Pixel6) != 0 {
			periBlink.SetPixel(6, 0, 0, 0, 0)
		}
		if len(conf.Pixel7) != 0 {
			periBlink.SetPixel(7, 0, 0, 0, 0)
		}

		periBlink.Show()
		if conf.Algorithm == "fixed5" {
			// We only leave the led dark for
			// a couple of milliseconds
			delay(10)
		} else {
			delay(conf.Frequency)
		}

	}
}

func main() {
	running := true
	// initialise getout
	signalChannel := make(chan os.Signal, 2)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-signalChannel
		switch sig {
		case os.Interrupt:
			fmt.Println("Stopping on Interrupt")
			running = false
			return
		case syscall.SIGTERM:
			fmt.Println("Stopping on Terminate")
			running = false
			return
		}
	}()

	periBlink.Setup()
	periBlink.SetLuminance(1)
	periBlink.Clear()
	periBlink.Show()

	var conf config.BlinktConfigData
	conf.Config()

	if conf.Algorithm == "blinkt5" {
		blinkt5(&running, conf)
	} else {
		fixed5(&running, conf)
	}
	fmt.Println("Stopping")
	periBlink.Exit()
}
