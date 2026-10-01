/*
Package canvastest is a canvas device that exists only in memory: a
sanshoku.Device satisfying lighting.Canvas, so a test reaches the adapter, the
animator and the service without a device node.

It carries only what was measured on the first canvas device (sanshoku spec
010): a frame is acknowledged, the device holds the last frame it was sent, a
frame can go unacknowledged (hidraw.ErrSilent), and an unplugged or rebooted
device answers sanshoku.ErrGone. Its lights are numbered, not named after any
product's keys.
*/
package canvastest

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/ushineko/hotaru/internal/canvas"
	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/hidraw"
	"github.com/ushineko/sanshoku/lighting"
)

/*
Device is a canvas of Lights lights.

SilentNext makes that many coming frames unacknowledged. GoneNext makes the
next frame, and every call after it, sanshoku.ErrGone, as a device that has
rebooted does.
*/
type Device struct {
	mu sync.Mutex

	Name   string
	Path   string
	Lights int
	// FloorAt is the device's floor; zero is 16 ms.
	FloorAt time.Duration

	SilentNext int
	GoneNext   bool

	frames   [][]lighting.Pixel
	held     map[byte]lighting.Pixel
	released int
	closed   bool
	gone     bool
}

// New is a canvas of n lights.
func New(n int) *Device {
	return &Device{Name: "fake canvas", Path: "fake:canvas", Lights: n}
}

// Identity is the fake's name and path under made-up IDs.
func (d *Device) Identity() sanshoku.Identity {
	return sanshoku.Identity{Vendor: 0xFFFF, Product: 0x0001, Bus: sanshoku.BusUSB, Name: d.Name, Path: d.Path}
}

// Keys are the lights, numbered from 1 and named by number.
func (d *Device) Keys() []lighting.Key {
	out := make([]lighting.Key, d.Lights)
	for i := range out {
		out[i] = lighting.Key{ID: byte(i + 1), Name: fmt.Sprintf("light %d", i+1)}
	}
	return out
}

// Floor is FloorAt, or 16 ms.
func (d *Device) Floor() time.Duration {
	if d.FloorAt > 0 {
		return d.FloorAt
	}
	return 16 * time.Millisecond
}

// Frame records and shows a frame, or misbehaves as it was told to.
func (d *Device) Frame(_ context.Context, px []lighting.Pixel) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch {
	case d.gone || d.GoneNext:
		d.gone = true
		return fmt.Errorf("a frame to %s: %w", d.Path, sanshoku.ErrGone)
	case d.closed:
		return fmt.Errorf("a frame to %s: closed", d.Path)
	case d.SilentNext > 0:
		d.SilentNext--
		return fmt.Errorf("a frame to %s: %w", d.Path, hidraw.ErrSilent)
	}
	d.frames = append(d.frames, slices.Clone(px))
	if d.held == nil {
		d.held = map[byte]lighting.Pixel{}
	}
	for _, p := range px {
		d.held[p.ID] = p // a light the frame does not name keeps its colour
	}
	return nil
}

// Release counts the release, and the device is gone afterwards.
func (d *Device) Release(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.gone {
		return fmt.Errorf("releasing %s: %w", d.Path, sanshoku.ErrGone)
	}
	d.released++
	d.gone = true
	return nil
}

// Close marks the device closed. Closing twice is not an error.
func (d *Device) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	return nil
}

// Frames is how many frames were acknowledged.
func (d *Device) Frames() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.frames)
}

// Last is the last frame acknowledged, or nil.
func (d *Device) Last() []lighting.Pixel {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.frames) == 0 {
		return nil
	}
	return slices.Clone(d.frames[len(d.frames)-1])
}

// Showing is what one light shows: the colour of the last frame that named it.
func (d *Device) Showing(id byte) (lighting.Pixel, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	p, ok := d.held[id]
	return p, ok
}

// Released is how many times the lighting was handed back.
func (d *Device) Released() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.released
}

// Closed reports whether the device was closed.
func (d *Device) Closed() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closed
}

/*
Clock is a clock a test moves by hand, with tickers that tick only when told.

Every ticker it made records the interval it was made with, which is how a
test asks what pace an animator chose without timing it.
*/
type Clock struct {
	mu      sync.Mutex
	now     time.Time
	tickers []*Ticker
}

// NewClock is a clock at an arbitrary fixed instant.
func NewClock() *Clock { return &Clock{now: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)} }

// Now is the clock's time.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock on.
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// NewTicker is a ticker that ticks when Tick is called.
func (c *Clock) NewTicker(d time.Duration) canvas.Ticker {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &Ticker{Every: d, c: make(chan time.Time)}
	c.tickers = append(c.tickers, t)
	return t
}

// Tickers is every ticker made, oldest first.
func (c *Clock) Tickers() []*Ticker {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.tickers)
}

// Running is how many tickers are running.
func (c *Clock) Running() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, t := range c.tickers {
		if !t.Stopped() {
			n++
		}
	}
	return n
}

/*
Tick advances the clock by the newest running ticker's interval and delivers
one tick, and reports whether a ticker was running. It waits for the tick to
be taken, so a test that ticks and then looks sees what the tick did once the
next one is taken.
*/
func (c *Clock) Tick(ctx context.Context) bool {
	c.mu.Lock()
	var running *Ticker
	for i := len(c.tickers) - 1; i >= 0; i-- {
		if !c.tickers[i].Stopped() {
			running = c.tickers[i]
			break
		}
	}
	c.mu.Unlock()
	if running == nil {
		return false
	}
	c.Advance(running.Every)
	select {
	case running.c <- c.Now():
		return true
	case <-ctx.Done():
		return false
	case <-time.After(time.Second): // stopped while this was ticking it
		return false
	}
}

// Ticker is one of Clock's tickers.
type Ticker struct {
	Every time.Duration
	c     chan time.Time

	mu      sync.Mutex
	stopped bool
}

// C is the channel ticks arrive on.
func (t *Ticker) C() <-chan time.Time { return t.c }

// Stop stops the ticker.
func (t *Ticker) Stop() {
	t.mu.Lock()
	t.stopped = true
	t.mu.Unlock()
}

// Stopped reports whether the ticker was stopped.
func (t *Ticker) Stopped() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stopped
}
