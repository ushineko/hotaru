package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Seven-segment digits: no font, no dependency, and it suits the panel.
// Segments are a b c d e f g, clockwise from the top then the middle.
var segments = map[rune][7]bool{
	'0': {true, true, true, true, true, true, false},
	'1': {false, true, true, false, false, false, false},
	'2': {true, true, false, true, true, false, true},
	'3': {true, true, true, true, false, false, true},
	'4': {false, true, true, false, false, true, true},
	'5': {true, false, true, true, false, true, true},
	'6': {true, false, true, true, true, true, true},
	'7': {true, true, true, false, false, false, false},
	'8': {true, true, true, true, true, true, true},
	'9': {true, true, true, true, false, true, true},
	'-': {false, false, false, false, false, false, true},
	' ': {},
}

func fill(img *image.Paletted, x0, y0, w, h int, c uint8) {
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			if x >= 0 && y >= 0 && x < lcdSide && y < lcdSide {
				img.SetColorIndex(x, y, c)
			}
		}
	}
}

// digit draws one seven-segment glyph with its top-left at (x, y).
func digit(img *image.Paletted, r rune, x, y, w, h, t int, c uint8) {
	s, ok := segments[r]
	if !ok {
		return
	}
	mid := y + h/2
	if s[0] {
		fill(img, x+t, y, w-2*t, t, c)
	}
	if s[1] {
		fill(img, x+w-t, y+t, t, h/2-t, c)
	}
	if s[2] {
		fill(img, x+w-t, mid, t, h/2-t, c)
	}
	if s[3] {
		fill(img, x+t, y+h-t, w-2*t, t, c)
	}
	if s[4] {
		fill(img, x, mid, t, h/2-t, c)
	}
	if s[5] {
		fill(img, x, y+t, t, h/2-t, c)
	}
	if s[6] {
		fill(img, x+t, mid-t/2, w-2*t, t, c)
	}
}

// label draws a string of digits, honouring '.' as a trailing dot.
func label(img *image.Paletted, s string, x, y, w, h, t, gap int, c uint8) int {
	for _, r := range s {
		if r == '.' {
			fill(img, x+2, y+h-t, t, t, c)
			x += t + gap
			continue
		}
		digit(img, r, x, y, w, h, t, c)
		x += w + gap
	}
	return x
}

type reading struct {
	coolant      float64
	pumpRPM      int
	fanRPM       int
	cpuC, gpuC   int
	cpuOK, gpuOK bool
	tick         int // counts updates, so a frozen screen is obvious
}

// hwmonTemp finds a labelled temperature under a named hwmon device. Read by
// label, never by hwmon index: the indices move between boots.
func hwmonTemp(chip, want string) (int, bool) {
	dirs, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	for _, d := range dirs {
		name, err := os.ReadFile(filepath.Join(d, "name"))
		if err != nil || strings.TrimSpace(string(name)) != chip {
			continue
		}
		labels, _ := filepath.Glob(filepath.Join(d, "temp*_label"))
		for _, l := range labels {
			b, err := os.ReadFile(l)
			if err != nil || strings.TrimSpace(string(b)) != want {
				continue
			}
			v, err := os.ReadFile(strings.TrimSuffix(l, "_label") + "_input")
			if err != nil {
				return 0, false
			}
			n, err := strconv.Atoi(strings.TrimSpace(string(v)))
			if err != nil {
				return 0, false
			}
			return n / 1000, true
		}
	}
	return 0, false
}

func gpuTemp() (int, bool) {
	out, err := exec.Command("nvidia-smi", "--query-gpu=temperature.gpu", "--format=csv,noheader").Output()
	if err != nil {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	return n, err == nil
}

// renderDash draws the frame and encodes it. It never fails: a missing metric
// draws dashes, because the screen is decorative and a render fault must not
// take telemetry down with it.
func renderDash(r reading) []byte {
	pal := color.Palette{
		color.RGBA{8, 10, 16, 255},     // 0 background
		color.RGBA{90, 200, 255, 255},  // 1 coolant
		color.RGBA{120, 130, 150, 255}, // 2 dim
		color.RGBA{240, 240, 245, 255}, // 3 bright
		color.RGBA{255, 150, 60, 255},  // 4 warm
	}
	img := image.NewPaletted(image.Rect(0, 0, lcdSide, lcdSide), pal)

	// Coolant, big, across the top half.
	txt := fmt.Sprintf("%.1f", r.coolant)
	label(img, txt, 110, 90, 90, 190, 18, 26, 1)
	fill(img, 470, 96, 34, 34, 2) // degree mark

	// A divider, then the smaller rows.
	fill(img, 90, 320, lcdSide-180, 4, 2)

	small := func(v int, ok bool, x, y int, c uint8) {
		s := "---"
		if ok {
			s = strconv.Itoa(v)
		}
		label(img, s, x, y, 52, 100, 11, 18, c)
	}
	small(r.cpuC, r.cpuOK, 110, 355, 4)
	small(r.gpuC, r.gpuOK, 400, 355, 3)

	// pump and fan, bottom row, in hundreds of rpm to keep the digits large
	label(img, strconv.Itoa(r.pumpRPM/100), 130, 480, 46, 90, 10, 16, 2)
	label(img, strconv.Itoa(r.fanRPM/100), 400, 480, 46, 90, 10, 16, 2)

	// A pip that advances with every update. Without it a screen that has
	// stopped being written looks identical to an idle machine, which is how
	// the Python's dashboard hid an expiry bug for weeks.
	pip := r.tick % 12
	for i := range 12 {
		c := uint8(2)
		if i == pip {
			c = 3
		}
		fill(img, 60+i*44, 600, 30, 10, c)
	}

	var buf bytes.Buffer
	_ = gif.EncodeAll(&buf, &gif.GIF{
		Image:     []*image.Paletted{img},
		Delay:     []int{100},
		LoopCount: 0,
		Config:    image.Config{ColorModel: pal, Width: lcdSide, Height: lcdSide},
	})
	return buf.Bytes()
}

// dashMain renders and pushes once a second, and reports what the device did.
func dashMain(seconds int, every time.Duration) {
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
		os.Exit(1)
	}
	fmt.Println("panel memory cleared")
	gpu, gpuOK := gpuTemp()

	var pushed, failed int
	var worst, total time.Duration
	firstFail := ""
	tick := time.NewTicker(every)
	defer tick.Stop()
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)

	for n := 0; time.Now().Before(deadline); n++ {
		s, err := read(hid)
		if err != nil {
			fmt.Println("status:", err)
			break
		}
		cpu, cpuOK := hwmonTemp("coretemp", "Package id 0")
		if n%5 == 0 { // nvidia-smi is a process; not every second
			gpu, gpuOK = gpuTemp()
		}
		frame := renderDash(reading{
			coolant: s.Coolant, pumpRPM: s.PumpRPM, fanRPM: s.FanRPM,
			cpuC: cpu, cpuOK: cpuOK, gpuC: gpu, gpuOK: gpuOK, tick: n,
		})

		start := time.Now()
		err = pushGIF(hid, usb, frame)
		d := time.Since(start)
		total += d
		if d > worst {
			worst = d
		}
		if err != nil {
			failed++
			if firstFail == "" {
				firstFail = err.Error()
			}
		} else {
			pushed++
		}
		<-tick.C
	}

	n := pushed + failed
	if n == 0 {
		return
	}
	fmt.Printf("\n%d updates over %ds at one every %v: %d landed, %d refused (%.0f%%)\n",
		n, seconds, every, pushed, failed, 100*float64(failed)/float64(n))
	fmt.Printf("push time: avg %v, worst %v\n", total/time.Duration(n), worst)
	if firstFail != "" {
		fmt.Println("first refusal:", firstFail)
	}
}

// probeMain asks the device what it will accept, one variable at a time.
func probeMain() {
	hid, err := os.OpenFile(device, os.O_RDWR, 0)
	if err != nil {
		fmt.Println("open hid:", err)
		os.Exit(1)
	}
	defer func() { _ = hid.Close() }()

	if err := clearAllBuckets(hid); err != nil {
		fmt.Println("clear:", err)
		return
	}
	fmt.Println("cleared. what does deleting an already-empty bucket say?")
	for i := range 3 {
		ok, err := deleteBucket(hid, i)
		fmt.Printf("  delete bucket %d -> ok=%v err=%v\n", i, ok, err)
	}

	fmt.Println("\nsetup at address 0, varying size:")
	for _, packets := range []int{1, 2, 3, 4, 5, 6, 8} {
		var addr, size [2]byte
		size[0] = byte(packets)
		ok, resp, err := setupBucket(hid, 0, 1, addr, size)
		fmt.Printf("  bucket 0, %d packets -> ok=%v reply[14]=%#02x err=%v\n",
			packets, ok, resp[14], err)
	}

	fmt.Println("\nsetup of 5 packets, varying bucket:")
	for _, b := range []int{0, 1, 2} {
		var addr, size [2]byte
		size[0] = 5
		ok, resp, err := setupBucket(hid, b, b+1, addr, size)
		fmt.Printf("  bucket %d, addr 0 -> ok=%v reply[14]=%#02x err=%v\n",
			b, ok, resp[14], err)
	}
}

// counterMain puts one huge incrementing number on the panel. Nothing else.
// A screen that is receiving updates counts; one that is not, does not.
func counterMain(n int, every time.Duration, recycleOn bool) {
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

	recycle = recycleOn
	if err := clearAllBuckets(hid); err != nil {
		fmt.Println("clear:", err)
		return
	}
	fmt.Printf("counting to %d, one every %v, recycle=%v\n", n, every, recycleOn)

	pal := color.Palette{
		color.RGBA{0, 0, 0, 255},
		color.RGBA{255, 240, 80, 255},
	}
	for i := 1; i <= n; i++ {
		img := image.NewPaletted(image.Rect(0, 0, lcdSide, lcdSide), pal)
		label(img, strconv.Itoa(i), 140, 180, 120, 280, 26, 40, 1)
		var buf bytes.Buffer
		_ = gif.EncodeAll(&buf, &gif.GIF{
			Image: []*image.Paletted{img}, Delay: []int{100}, LoopCount: 0,
			Config: image.Config{ColorModel: pal, Width: lcdSide, Height: lcdSide},
		})
		err := pushGIF(hid, usb, buf.Bytes())
		fmt.Printf("  %2d -> bucket %d  %v\n", i, lastShown, err)
		time.Sleep(every)
	}
}
