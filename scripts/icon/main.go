/*
Command icon draws hotaru's SVG icon as a square PNG, for the Windows
installer, which wants a PNG of at least 512 pixels (spec 072 R6.2).

	go run ./scripts/icon packaging/hotaru.svg dist/hotaru.png 512

Drawn at build time from the one SVG the window and the Linux package use,
so there is no second picture to keep in step.
*/
package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"strconv"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "icon: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("usage: icon <in.svg> <out.png> <size>")
	}
	size, err := strconv.Atoi(args[2])
	if err != nil || size < 16 || size > 4096 {
		return fmt.Errorf("size %q is not a number of pixels from 16 to 4096", args[2])
	}
	icon, err := oksvg.ReadIcon(args[0], oksvg.StrictErrorMode)
	if err != nil {
		return fmt.Errorf("read %s: %w", args[0], err)
	}
	icon.SetTarget(0, 0, float64(size), float64(size))
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	icon.Draw(rasterx.NewDasher(size, size, rasterx.NewScannerGV(size, size, img, img.Bounds())), 1)

	out, err := os.Create(args[1]) //nolint:gosec // the path the Makefile names, in a build step
	if err != nil {
		return fmt.Errorf("write %s: %w", args[1], err)
	}
	if err := png.Encode(out, img); err != nil {
		_ = out.Close()
		return fmt.Errorf("write %s: %w", args[1], err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("write %s: %w", args[1], err)
	}
	return nil
}
