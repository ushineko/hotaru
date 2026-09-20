package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"os"
	"time"
)

// frame draws something obviously different each time: a coloured field with a
// bar that marches across it, so a stalled screen is visible at a glance.
func frame(n int) []byte {
	pal := color.Palette{
		color.RGBA{10, 10, 10, 255},
		color.RGBA{uint8(40 + (n*37)%200), uint8(30 + (n*61)%200), uint8(60 + (n*23)%180), 255},
		color.RGBA{250, 250, 250, 255},
	}
	img := image.NewPaletted(image.Rect(0, 0, lcdSide, lcdSide), pal)
	barX := (n * 40) % lcdSide
	for y := range lcdSide {
		for x := range lcdSide {
			i := uint8(1)
			if x >= barX && x < barX+60 {
				i = 2
			}
			if y < 40 {
				i = 0
			}
			img.SetColorIndex(x, y, i)
		}
	}
	var buf bytes.Buffer
	_ = gif.EncodeAll(&buf, &gif.GIF{
		Image:     []*image.Paletted{img},
		Delay:     []int{10},
		LoopCount: 0,
		Config:    image.Config{ColorModel: pal, Width: lcdSide, Height: lcdSide},
	})
	return buf.Bytes()
}

type tier struct {
	interval time.Duration
	count    int
}

var pingpong = false

func stressMain(pinned bool) {
	hid, err := os.OpenFile(device, os.O_RDWR, 0)
	if err != nil {
		fmt.Println("open hid:", err)
		os.Exit(1)
	}
	defer func() { _ = hid.Close() }()

	usb, err := openUSB("/dev/bus/usb/001/013")
	if err != nil {
		fmt.Println("open usb:", err)
		os.Exit(1)
	}
	defer func() { _ = usb.Close() }()
	if err := usb.claim(0); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer func() { _ = usb.release(0) }()

	tiers := []tier{
		{2 * time.Second, 4},
		{time.Second, 5},
		{500 * time.Millisecond, 8},
		{250 * time.Millisecond, 10},
		{100 * time.Millisecond, 15},
		{0, 20}, // as fast as it will go
	}

	n := 0
	for _, t := range tiers {
		var slowest, total time.Duration
		fails := 0
		var firstErr error
		for range t.count {
			n++
			start := time.Now()
			var err error
			if pingpong {
				err = pushPingPong(hid, usb, frame(n))
			} else if pinned {
				err = pushPinned(hid, usb, frame(n), 0)
			} else {
				err = pushGIF(hid, usb, frame(n))
			}
			if err != nil {
				fails++
				if firstErr == nil {
					firstErr = err
				}
			}
			d := time.Since(start)
			total += d
			if d > slowest {
				slowest = d
			}
			if t.interval > 0 {
				time.Sleep(t.interval)
			}
		}
		label := "flat out"
		if t.interval > 0 {
			label = t.interval.String()
		}
		fmt.Printf("%-9s %2d pushes  avg %-8v max %-8v failures %d",
			label, t.count, total/time.Duration(t.count), slowest, fails)
		if firstErr != nil {
			fmt.Printf("  first error: %v", firstErr)
		}
		fmt.Println()
	}
	fmt.Printf("\n%d pushes total\n", n)
}
