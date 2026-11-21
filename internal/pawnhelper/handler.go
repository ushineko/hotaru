package pawnhelper

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/lighting"
)

/*
handler is the helper's side of a request: the devices it opened, and what a
request may do with them. It keeps them open between connections, so a
frame the service sent stays shown when the service restarts.
*/
type handler struct {
	scan func(ctx context.Context) ([]sanshoku.Device, error)
	// epoch makes this process's device IDs its own, and scans counts its
	// scans: an ID from an earlier helper or an earlier scan names nothing,
	// so a stale handle is told its device is gone rather than drawing on
	// another.
	epoch string
	now   func() time.Time

	mu       sync.Mutex
	devices  []held
	scans    int
	lastScan time.Time
	lastErr  error
}

// rescanAfter is how long a scan that found nothing is answered from
// memory. The service asks every 30 s; a machine with nothing for the helper
// is scanned at this pace instead, a few PawnIO module loads each time.
const rescanAfter = 5 * time.Minute

// held is one open device that is a canvas.
type held struct {
	id     string
	dev    sanshoku.Device
	canvas lighting.Canvas
	keys   map[byte]bool
}

// handle answers one request.
func (h *handler) handle(ctx context.Context, req request) response {
	if req.V != Protocol {
		return response{V: Protocol, Error: fmt.Sprintf("this helper speaks version %d, and was asked in %d", Protocol, req.V)}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	switch req.Op {
	case opList:
		if err := h.found(ctx); err != nil {
			return response{V: Protocol, Error: err.Error()}
		}
		out := make([]device, 0, len(h.devices))
		for _, d := range h.devices {
			out = append(out, describe(d))
		}
		return response{V: Protocol, Devices: out}
	case opFrame, opRelease:
		d, ok := h.find(req.Device)
		if !ok {
			return response{V: Protocol, Gone: true, Error: "no such device"}
		}
		var err error
		if req.Op == opFrame {
			err = h.frame(ctx, d, req.Pixels)
		} else {
			err = d.canvas.Release(ctx)
		}
		if errors.Is(err, sanshoku.ErrGone) {
			h.drop(d.id)
			return response{V: Protocol, Gone: true, Error: err.Error()}
		}
		if err != nil {
			return response{V: Protocol, Error: err.Error()}
		}
		return response{V: Protocol}
	default:
		return response{V: Protocol, Error: "no such request"}
	}
}

// found scans where nothing is held. A device that went is dropped by the
// request that found it gone, and the next list looks again. The caller
// holds the lock.
func (h *handler) found(ctx context.Context) error {
	if len(h.devices) > 0 {
		return nil
	}
	now := time.Now
	if h.now != nil {
		now = h.now
	}
	if !h.lastScan.IsZero() && now().Sub(h.lastScan) < rescanAfter {
		return h.lastErr
	}
	h.scans++
	h.lastScan = now()
	devs, err := h.scan(ctx)
	for _, dev := range devs {
		c, ok := dev.(lighting.Canvas)
		if !ok || len(h.devices) == maxDevices {
			_ = dev.Close()
			continue
		}
		keys := map[byte]bool{}
		for _, k := range c.Keys() {
			keys[k.ID] = true
		}
		id := fmt.Sprintf("%s.%d.%d", h.epoch, h.scans, len(h.devices))
		h.devices = append(h.devices, held{id: id, dev: dev, canvas: c, keys: keys})
	}
	h.lastErr = nil
	if len(h.devices) == 0 && err != nil {
		h.lastErr = err
	}
	return h.lastErr
}

// frame shows a frame, refusing one that names a light the device does not
// have or is larger than any device here.
func (h *handler) frame(ctx context.Context, d held, in [][4]uint8) error {
	if len(in) > maxPixels {
		return fmt.Errorf("a frame of %d lights", len(in)) //nolint:err113 // the count is the message
	}
	for _, p := range in {
		if !d.keys[p[0]] {
			return fmt.Errorf("light %d is not one of %s's", p[0], d.dev.Identity().Name) //nolint:err113 // said plainly
		}
	}
	return d.canvas.Frame(ctx, unpixels(in)) //nolint:wrapcheck // sanshoku names the device
}

func (h *handler) find(id string) (held, bool) {
	for _, d := range h.devices {
		if d.id == id {
			return d, true
		}
	}
	return held{}, false
}

func (h *handler) drop(id string) {
	kept := h.devices[:0]
	for _, d := range h.devices {
		if d.id == id {
			_ = d.dev.Close()
			continue
		}
		kept = append(kept, d)
	}
	h.devices = kept
}

// close closes every device, handing none back: stopping the helper leaves
// the last frame, as stopping the service does.
func (h *handler) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, d := range h.devices {
		_ = d.dev.Close()
	}
	h.devices = nil
}

func describe(d held) device {
	id := d.dev.Identity()
	out := device{ID: d.id, Name: id.Name, Vendor: id.Vendor, Product: id.Product, Path: id.Path,
		Keys: d.canvas.Keys(), FloorMS: d.canvas.Floor().Milliseconds()}
	if z, ok := d.canvas.(lighting.Zoned); ok {
		out.Zones = z.Zones()
	}
	return out
}

// printable is text from the helper as the service may show it: no control
// characters, and not longer than maxText.
func printable(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	if len(s) > maxText {
		s = s[:maxText]
	}
	return s
}

/*
serveConn answers one connection's requests in turn until it closes. A line
that is too long or not a request ends the connection: the service, the one
client, does not send one.
*/
func (h *handler) serveConn(ctx context.Context, rw io.ReadWriter) error {
	r := bufio.NewReaderSize(rw, 4096)
	for ctx.Err() == nil {
		line, err := readLine(r)
		if err != nil {
			return err
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			return fmt.Errorf("a line that is not a request: %w", err)
		}
		answer, _ := json.Marshal(h.handle(ctx, req))
		if _, err := rw.Write(append(answer, '\n')); err != nil {
			return fmt.Errorf("answering: %w", err)
		}
	}
	return ctx.Err() //nolint:wrapcheck // the context's own
}
