// Prototype: read an NZXT Kraken Elite V2's status straight from /dev/hidraw,
// with no liquidctl, no cgo and no dependencies.
//
// Protocol from liquidctl's kraken3 driver: write a 64-byte report starting
// 0x74 0x01, read a 64-byte report back. Offsets are into the raw hidraw
// buffer, report number included.
package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	reportLen = 64
	device    = "/dev/hidraw7"
)

type status struct {
	Coolant  float64
	PumpRPM  int
	PumpDuty int
	FanRPM   int
	FanDuty  int
}

func parse(b []byte) status {
	return status{
		Coolant:  float64(b[15]) + float64(b[16])/10,
		PumpRPM:  int(b[18])<<8 | int(b[17]),
		PumpDuty: int(b[19]),
		FanRPM:   int(b[24])<<8 | int(b[23]),
		FanDuty:  int(b[25]),
	}
}

func read(f *os.File) (status, error) {
	req := make([]byte, reportLen)
	req[0], req[1] = 0x74, 0x01
	if _, err := f.Write(req); err != nil {
		return status{}, fmt.Errorf("write request: %w", err)
	}
	buf := make([]byte, reportLen)
	for range 12 { // the device streams other reports too; find the reply
		n, err := f.Read(buf)
		if err != nil {
			return status{}, fmt.Errorf("read: %w", err)
		}
		if n >= 26 && buf[0] == 0x75 {
			return parse(buf), nil
		}
	}
	return status{}, fmt.Errorf("no status report in 12 reads")
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "sizes" {
		sizesMain()
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "panel" {
		secs, every := 60, time.Second
		if len(os.Args) > 2 {
			if n, err := strconv.Atoi(os.Args[2]); err == nil {
				secs = n
			}
		}
		if len(os.Args) > 3 {
			if d, err := time.ParseDuration(os.Args[3]); err == nil {
				every = d
			}
		}
		verbose = false
		panelMain(secs, every)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "count" {
		counterMain(10, 2*time.Second, len(os.Args) > 2 && os.Args[2] == "recycle")
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "probe" {
		probeMain()
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "dash" {
		secs := 30
		if len(os.Args) > 2 {
			if n, err := strconv.Atoi(os.Args[2]); err == nil {
				secs = n
			}
		}
		every := time.Second
		if len(os.Args) > 3 {
			if d, err := time.ParseDuration(os.Args[3]); err == nil {
				every = d
			}
		}
		recycle = len(os.Args) > 4 && os.Args[4] == "recycle"
		fmt.Printf("dashboard for %ds, one update every %v, recycle=%v\n", secs, every, recycle)
		dashMain(secs, every)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "anim" {
		data := animatedCard(12)
		fmt.Printf("12-frame animated GIF: %d bytes\n", len(data))
		lcdMain(data)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "fb" {
		fbMain()
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "stress" {
		recycle = len(os.Args) > 2 && os.Args[2] == "recycle"
		pingpong = len(os.Args) > 2 && os.Args[2] == "pingpong"
		stressMain(false)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "dump" {
		data := testCard()
		_ = os.WriteFile("/tmp/claude-1000/-home-nverenin-git-fynedesygn/73983bf0-0c84-4ffa-b25b-374694eda4dc/scratchpad/go.gif", data, 0o644)
		fmt.Println("wrote", len(data), "bytes")
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "lcd" {
		var data []byte
		if len(os.Args) > 2 {
			b, err := os.ReadFile(os.Args[2])
			if err != nil {
				fmt.Println("read:", err)
				os.Exit(1)
			}
			data = b
			fmt.Println("pushing", os.Args[2], len(data), "bytes")
		} else {
			data = testCard()
			fmt.Println("pushing the Go-encoded test card,", len(data), "bytes")
		}
		lcdMain(data)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "liquid" {
		hid, err := os.OpenFile(device, os.O_RDWR, 0)
		if err != nil {
			fmt.Println("open hid:", err)
			os.Exit(1)
		}
		defer func() { _ = hid.Close() }()
		ok, err := switchBucket(hid, 0, 0x02)
		fmt.Println("back to the firmware readout:", ok, err)
		return
	}
	f, err := os.OpenFile(device, os.O_RDWR, 0)
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	defer func() { _ = f.Close() }()

	open := time.Now()
	s, err := read(f)
	if err != nil {
		fmt.Println("first read:", err)
		os.Exit(1)
	}
	fmt.Printf("coolant %.1f C  pump %d rpm (%d%%)  fan %d rpm (%d%%)\n",
		s.Coolant, s.PumpRPM, s.PumpDuty, s.FanRPM, s.FanDuty)
	fmt.Printf("first read (incl. open): %v\n", time.Since(open))

	// Steady-state cost: the handle stays open in a service.
	const n = 20
	start := time.Now()
	for range n {
		if _, err := read(f); err != nil {
			fmt.Println("read:", err)
			os.Exit(1)
		}
	}
	fmt.Printf("%d reads on an open handle: %v total, %v each\n",
		n, time.Since(start), time.Since(start)/n)
}

// lcdMain pushes a test card to the screen over the bulk endpoint.
func lcdMain(data []byte) {
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
	fmt.Println("claimed interface 0 (bulk); hidraw still open")

	start := time.Now()
	if err := pushGIF(hid, usb, data); err != nil {
		fmt.Println("push:", err)
		os.Exit(1)
	}
	fmt.Printf("pushed in %v\n", time.Since(start))
}

// fbMain streams raw frames and reports the rate achieved.
func fbMain() {
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

	frames := rgb565Frame(1)
	fmt.Printf("frame is %d bytes of RGB565\n", len(frames))

	start := time.Now()
	if err := pushFramebuffer(hid, usb, frames, 0); err != nil {
		fmt.Println("first frame:", err)
		os.Exit(1)
	}
	fmt.Printf("first frame: %v\n", time.Since(start))

	if len(os.Args) > 2 && os.Args[2] == "once" {
		fmt.Println("one frame pushed; holding the interface open for 30s, not writing")
		time.Sleep(30 * time.Second)
		fmt.Println("releasing now")
		return
	}
	const n = 60
	fails := 0
	start = time.Now()
	for i := 2; i < n+2; i++ {
		if err := pushFramebuffer(hid, usb, rgb565Frame(i), 0); err != nil {
			fails++
			if fails == 1 {
				fmt.Println("error:", err)
			}
		}
	}
	d := time.Since(start)
	fmt.Printf("%d frames in %v: %.1f fps, %d failures\n", n, d, float64(n)/d.Seconds(), fails)
}
