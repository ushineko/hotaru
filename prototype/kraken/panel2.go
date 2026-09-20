package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"math"
	"os"
	"runtime"
	"syscall"
	"time"
)

// snapshot is what the screen draws. Every field can be absent, and an absent
// one draws a placeholder: the panel is decorative and a render fault must not
// disturb telemetry.
type snapshot struct {
	coolantC     float64
	coolantOK    bool
	cpuC, gpuC   int
	cpuOK, gpuOK bool
	pumpRPM      int
	pumpOK       bool
	tick         int
}

func fmtTemp(v float64, ok bool) string {
	if !ok {
		return "--"
	}
	return fmt.Sprintf("%.1f", v)
}

func fmtInt(v int, ok bool) string {
	if !ok {
		return "--"
	}
	return fmt.Sprintf("%d", v)
}

// metric draws one of the bottom columns. The value font is smaller at three
// columns because a four-digit pump reading at the two-column size overflows
// its width and collides with its neighbour.
func metric(dst *image.RGBA, slot int, label, value, unit string, c color.RGBA) {
	width := (panelSize - 2*metricInset) / metricSlots
	x := metricInset + slot*width
	centred(dst, label, x, 416, width, 28, 16, false, colMuted)
	centred(dst, value, x, 446, width, 66, 34, true, c)
	centred(dst, unit, x, 516, width, 26, 14, false, colMuted)
}

// drawPanel renders the monitor's dashboard.
func drawPanel(s snapshot) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, panelSize, panelSize))
	draw.Draw(img, img.Bounds(), background(), image.Point{}, draw.Src)

	cc := coolantColour(s.coolantC, s.coolantOK)
	cx, cy := float64(panelSize)/2, float64(panelSize)/2
	radius := float64(panelSize-2*panelMargin) / 2

	// The track, then the value arc over it: a single arc carrying the
	// headline value's severity, legible from across a room without reading
	// the number.
	arc(img, cx, cy, radius, ringWidth, 0, 2*math.Pi, colTrack)
	if s.coolantOK {
		frac := math.Max(0, math.Min(1, (s.coolantC-20)/50))
		arc(img, cx, cy, radius, ringWidth, math.Pi/2, -2*math.Pi*frac, cc)
	}

	centred(img, "COOLANT", 0, 132, panelSize, 36, 22, false, colMuted)
	centred(img, fmtTemp(s.coolantC, s.coolantOK), 0, 170, panelSize, 158, 118, true, cc)
	centred(img, "°C", 0, 330, panelSize, 38, 27, false, colMuted)

	// CPU is never colour-graded: an i9 boosting to 100 C is normal here and
	// reddening it would cry wolf on every compile. A pump reading zero is the
	// opposite -- the most alarming number this screen can show.
	pumpCol := colAccent
	if s.pumpOK && s.pumpRPM == 0 {
		pumpCol = colCrit
	}
	metric(img, 0, "CPU", fmtInt(s.cpuC, s.cpuOK), "°C", colAccent)
	metric(img, 1, "GPU", fmtInt(s.gpuC, s.gpuOK), "°C", colAccent)
	metric(img, 2, "PUMP", fmtInt(s.pumpRPM, s.pumpOK), "RPM", pumpCol)

	// Not in the Python: a pip that advances every update, so a screen that
	// has stopped being written cannot be mistaken for an idle machine.
	for i := range 12 {
		c := colMuted
		if i == s.tick%12 {
			c = color.RGBA{236, 239, 244, 255}
		}
		for y := 600; y < 608; y++ {
			for x := 236 + i*14; x < 236+i*14+9; x++ {
				img.SetRGBA(x, y, c)
			}
		}
	}
	return img
}

/*
palettedBackground is the starfield, quantised once.

The background never changes -- that is the whole point of caching the
starfield -- so re-quantising four hundred thousand pixels every frame was
paying for a constant. Quantised once and copied per frame, the only pixels
that need converting are the ring and the glyphs.
*/
var bgPaletted *image.Paletted

/*
panelPalette is eight colours, which is a size decision as much as a visual one.

GIF codes are as wide as the palette: 256 entries means eight-bit codes and a
larger stream even where the image is flat, and every anti-aliased glyph edge
finds a different entry to blend into. Eight entries means three-bit codes and
glyph edges snapping to one of two greys, which is both smaller and sharper --
and sharp suits a panel viewed through a case window.
*/
func panelPalette() color.Palette {
	pal := color.Palette{
		colBG,
		colMuted,
		colAccent,
		colOK,
		colWarn,
		colCrit,
		color.RGBA{236, 239, 244, 255},
		color.RGBA{60, 68, 82, 255}, // one mid grey, for glyph edges
	}
	// A ramp for the nebulae. Longer than eight makes the sky smoother and
	// the frame larger, and frame size buys settling time on this panel
	// rather than nothing -- so it is a real trade and not free.
	for i := range 24 {
		f := float64(i) / 23
		pal = append(pal, color.RGBA{
			R: clamp8(12 + f*100),
			G: clamp8(14 + f*80),
			B: clamp8(18 + f*140),
			A: 255,
		})
	}
	return pal
}

// drawPanelFast composes onto a pre-quantised background.
func drawPanelFast(s snapshot) *image.Paletted {
	if bgPaletted == nil {
		pal := panelPalette()
		bgPaletted = image.NewPaletted(image.Rect(0, 0, panelSize, panelSize), pal)
		draw.Draw(bgPaletted, bgPaletted.Bounds(), background(), image.Point{}, draw.Src)
	}
	p := image.NewPaletted(bgPaletted.Bounds(), bgPaletted.Palette)
	copy(p.Pix, bgPaletted.Pix)

	cc := coolantColour(s.coolantC, s.coolantOK)
	cx, cy := float64(panelSize)/2, float64(panelSize)/2
	radius := float64(panelSize-2*panelMargin) / 2
	arcP(p, cx, cy, radius, ringWidth, 0, 2*math.Pi, colMuted)
	if s.coolantOK {
		frac := math.Max(0, math.Min(1, (s.coolantC-20)/50))
		arcP(p, cx, cy, radius, ringWidth, math.Pi/2, -2*math.Pi*frac, cc)
	}

	centredP(p, "COOLANT", 0, 132, panelSize, 36, 22, false, colMuted)
	centredP(p, fmtTemp(s.coolantC, s.coolantOK), 0, 170, panelSize, 158, 118, true, cc)
	centredP(p, "°C", 0, 330, panelSize, 38, 27, false, colMuted)

	pumpCol := colAccent
	if s.pumpOK && s.pumpRPM == 0 {
		pumpCol = colCrit
	}
	metricP(p, 0, "CPU", fmtInt(s.cpuC, s.cpuOK), "°C", colAccent)
	metricP(p, 1, "GPU", fmtInt(s.gpuC, s.gpuOK), "°C", colAccent)
	metricP(p, 2, "PUMP", fmtInt(s.pumpRPM, s.pumpOK), "RPM", pumpCol)

	for i := range 12 {
		c := colMuted
		if i == s.tick%12 {
			c = color.RGBA{236, 239, 244, 255}
		}
		for y := 600; y < 608; y++ {
			for x := 236 + i*14; x < 236+i*14+9; x++ {
				p.Set(x, y, c)
			}
		}
	}
	return p
}

func encodePaletted(p *image.Paletted) []byte {
	var buf bytes.Buffer
	_ = gif.EncodeAll(&buf, &gif.GIF{
		Image: []*image.Paletted{p}, Delay: []int{100}, LoopCount: 0,
		Config: image.Config{ColorModel: p.Palette, Width: panelSize, Height: panelSize},
	})
	return buf.Bytes()
}

// encodePanel quantises to a palette and writes a single-frame GIF, which is
// what the firmware retains.
func encodePanel(img *image.RGBA) []byte {
	pal := make(color.Palette, 0, 256)
	for _, c := range []color.RGBA{colBG, colMuted, colAccent, colOK, colWarn, colCrit,
		{236, 239, 244, 255}} {
		pal = append(pal, c)
	}
	// A ramp through the nebula hues, so the background does not band.
	for i := range 249 {
		f := float64(i) / 248
		pal = append(pal, color.RGBA{
			R: clamp8(12 + f*110),
			G: clamp8(14 + f*90),
			B: clamp8(18 + f*150),
			A: 255,
		})
	}
	/*
		No dithering.

		Floyd-Steinberg across a starfield turns smooth gradients into noise,
		and noise does not compress: the dithered frame was 105 KB against a
		few KB for a flat one, which is 25x the transfer for a decorative
		background. Banding in a dim nebula is invisible at arm's length;
		a 105 KB push is not.
	*/
	p := image.NewPaletted(img.Bounds(), pal)
	draw.Draw(p, img.Bounds(), img, image.Point{}, draw.Src)

	var buf bytes.Buffer
	_ = gif.EncodeAll(&buf, &gif.GIF{
		Image: []*image.Paletted{p}, Delay: []int{100}, LoopCount: 0,
		Config: image.Config{ColorModel: pal, Width: panelSize, Height: panelSize},
	})
	return buf.Bytes()
}

// panelMain runs the ported dashboard, timing render and push separately.
func panelMain(seconds int, every time.Duration) {
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

	if err := clearAllBuckets(hid); err != nil {
		fmt.Println("clear:", err)
		return
	}

	warm := time.Now()
	background()
	_ = encodePaletted(drawPanelFast(snapshot{}))
	fmt.Printf("first render, including the starfield and font setup: %v\n", time.Since(warm))

	started := time.Now()
	gpu, gpuOK := gpuTemp()
	var renderTotal, pushTotal time.Duration
	var bytesTotal, pushed, failed int
	tick := time.NewTicker(every)
	defer tick.Stop()
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)

	for n := 0; time.Now().Before(deadline); n++ {
		st, err := read(hid)
		if err != nil {
			fmt.Println("status:", err)
			break
		}
		cpu, cpuOK := hwmonTemp("coretemp", "Package id 0")
		if n%5 == 0 {
			gpu, gpuOK = gpuTemp()
		}

		r0 := time.Now()
		frame := encodePaletted(drawPanelFast(snapshot{
			coolantC: st.Coolant, coolantOK: true,
			cpuC: cpu, cpuOK: cpuOK, gpuC: gpu, gpuOK: gpuOK,
			pumpRPM: st.PumpRPM, pumpOK: true, tick: n,
		}))
		renderTotal += time.Since(r0)
		bytesTotal += len(frame)

		p0 := time.Now()
		if err := pushGIF(hid, usb, frame); err != nil {
			failed++
			if failed == 1 {
				fmt.Println("push:", err)
			}
		} else {
			pushed++
		}
		pushTotal += time.Since(p0)
		<-tick.C
	}

	n := pushed + failed
	if n == 0 {
		return
	}
	fmt.Printf("\n%d updates at one every %v: %d landed, %d refused\n", n, every, pushed, failed)
	fmt.Printf("render: avg %v   push: avg %v   frame: avg %d KB\n",
		renderTotal/time.Duration(n), pushTotal/time.Duration(n), bytesTotal/n/1024)
	cost(time.Since(started), n)
}

// sizesMain prints the weight of each frame this prototype can draw, so the
// budget can be compared against what the panel actually accepted.
func sizesMain() {
	simple := renderDash(reading{coolant: 37.5, pumpRPM: 2600, fanRPM: 1190,
		cpuC: 52, cpuOK: true, gpuC: 38, gpuOK: true, tick: 3})
	fmt.Printf("seven-segment dashboard (worked):  %5d bytes\n", len(simple))

	panel := encodePaletted(drawPanelFast(snapshot{
		coolantC: 37.5, coolantOK: true, cpuC: 52, cpuOK: true,
		gpuC: 38, gpuOK: true, pumpRPM: 2600, pumpOK: true, tick: 3}))
	fmt.Printf("ported panel, flat, 8 colours:     %5d bytes\n", len(panel))

	_ = os.WriteFile("/tmp/claude-1000/-home-nverenin-git-fynedesygn/73983bf0-0c84-4ffa-b25b-374694eda4dc/scratchpad/panel.gif", panel, 0o644)

	card := testCard()
	fmt.Printf("test card (worked):                %5d bytes\n", len(card))
	anim := animatedCard(12)
	fmt.Printf("12-frame animation (worked):       %5d bytes\n", len(anim))
}

// cost reports what the process actually consumed, for a service that will
// run for months: CPU time against wall time, and peak resident memory.
func cost(wall time.Duration, updates int) {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return
	}
	cpu := time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	fmt.Printf("\ncpu: %v over %v wall (%.2f%% of one core)\n",
		cpu.Round(time.Millisecond), wall.Round(time.Millisecond),
		100*float64(cpu)/float64(wall))
	if updates > 0 {
		fmt.Printf("     %v of CPU per update\n", (cpu / time.Duration(updates)).Round(time.Microsecond))
	}
	fmt.Printf("peak rss: %d KB   go heap in use: %d KB   total allocated: %d KB\n",
		ru.Maxrss, ms.HeapInuse/1024, ms.TotalAlloc/1024)
}
