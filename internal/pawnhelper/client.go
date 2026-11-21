package pawnhelper

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/lighting"
)

/*
client is the service's end of the pipe: one connection, opened when first
needed, one request at a time. A connection that fails is dropped, and the
next request opens another.
*/
type client struct {
	// dial opens the pipe and checks who serves it. errNoHelper is a helper
	// that is not installed.
	dial func(ctx context.Context) (io.ReadWriteCloser, error)

	mu   sync.Mutex
	conn io.ReadWriteCloser
	r    *bufio.Reader
}

// errNoHelper is the helper not installed: an ordinary state, the machine's
// devices are then absent.
var errNoHelper = errors.New("hotaru's PawnIO helper is not installed")

// call sends one request and reads its answer.
func (c *client) call(ctx context.Context, req request) (response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		conn, err := c.dial(ctx)
		if err != nil {
			return response{}, err
		}
		c.conn, c.r = conn, bufio.NewReaderSize(conn, 4096)
	}
	req.V = Protocol
	line, err := json.Marshal(req)
	if err != nil {
		return response{}, fmt.Errorf("the helper's request: %w", err)
	}
	// The pipe's reads and writes do not end on a context, so the exchange
	// runs aside and is given up on: a helper that stops answering costs a
	// timeout, and the connection is dropped, not every device it holds.
	type result struct {
		resp response
		err  error
	}
	conn, r := c.conn, c.r
	done := make(chan result, 1)
	go func() {
		resp, err := exchange(conn, r, line)
		done <- result{resp, err}
	}()
	timer := time.NewTimer(callTimeout)
	defer timer.Stop()
	select {
	case got := <-done:
		if got.err != nil {
			c.drop()
		}
		return got.resp, got.err
	case <-ctx.Done():
		c.drop()
		return response{}, ctx.Err() //nolint:wrapcheck // the context's own
	case <-timer.C:
		c.drop()
		return response{}, fmt.Errorf("hotaru's PawnIO helper did not answer in %s", callTimeout) //nolint:err113 // said plainly
	}
}

// callTimeout is the longest a request waits. A first list scans the buses,
// each transfer waiting up to 2 s on another program's bus lock.
const callTimeout = 30 * time.Second

// exchange writes one request and reads its answer.
func exchange(conn io.Writer, r *bufio.Reader, line []byte) (response, error) {
	if _, err := conn.Write(append(line, '\n')); err != nil {
		return response{}, fmt.Errorf("writing to the helper: %w", err)
	}
	answer, err := readLine(r)
	if err != nil {
		return response{}, fmt.Errorf("reading the helper: %w", err)
	}
	var resp response
	if err := json.Unmarshal(answer, &resp); err != nil {
		return response{}, fmt.Errorf("the helper's answer: %w", err)
	}
	if resp.V != Protocol {
		return response{}, fmt.Errorf("the helper speaks version %d, and this service %d: install them together", resp.V, Protocol) //nolint:err113 // the versions are the message
	}
	return resp, nil
}

func (c *client) drop() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.conn, c.r = nil, nil
}

// readLine reads one line of at most maxLine bytes.
func readLine(r *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		part, more, err := r.ReadLine()
		if err != nil {
			return nil, err //nolint:wrapcheck // the caller says which
		}
		line = append(line, part...)
		if len(line) > maxLine {
			return nil, fmt.Errorf("a line longer than %d bytes", maxLine) //nolint:err113 // said plainly
		}
		if !more {
			return line, nil
		}
	}
}

/*
driver finds the helper's devices. Find asks the helper for its list, which
opens nothing on the service's side; each candidate opens to a device that
draws through the helper.
*/
type driver struct{ c *client }

// Name is the driver's name.
func (driver) Name() string { return "pawnio-helper" }

// Find is the helper's devices: none where it is not installed.
func (d driver) Find(ctx context.Context) ([]sanshoku.Candidate, error) {
	resp, err := d.c.call(ctx, request{Op: opList})
	if errors.Is(err, errNoHelper) {
		return nil, fmt.Errorf("%w: %w", sanshoku.ErrAbsent, err)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", sanshoku.ErrUnavailable, err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("%w: hotaru's PawnIO helper: %s", sanshoku.ErrUnavailable, printable(resp.Error)) //nolint:err113 // the helper's words
	}
	if len(resp.Devices) > maxDevices {
		resp.Devices = resp.Devices[:maxDevices]
	}
	out := make([]sanshoku.Candidate, 0, len(resp.Devices))
	for _, dev := range resp.Devices {
		// Each device its own connection: a module's slow frame on the
		// SMBus does not hold up the board's on the Super I/O chip.
		r := &remote{c: &client{dial: d.c.dial}, info: dev}
		r.info.Name = printable(dev.Name)
		var opened sanshoku.Device = r
		if len(dev.Effects) > 0 {
			// A device that runs effects of its own is offered them
			// (spec 085).
			opened = &tuned{remote: r}
		}
		out = append(out, sanshoku.Candidate{
			Identity: r.Identity(),
			Driver:   d.Name(),
			Key:      "pawnio:" + printable(dev.ID),
			Open:     func(context.Context) (sanshoku.Device, error) { return opened, nil },
		})
	}
	return out, nil
}

/*
remote is one of the helper's devices, drawn through it. It is a
lighting.Canvas, and Zoned where the device is, with zones it cannot resize.
Close lets go of nothing: the helper keeps the device, and its last frame.
*/
type remote struct {
	c    *client
	info device
}

func (r *remote) Identity() sanshoku.Identity {
	return sanshoku.Identity{Vendor: r.info.Vendor, Product: r.info.Product, Name: r.info.Name, Path: printable(r.info.Path)}
}

func (r *remote) Close() error { return nil }

func (r *remote) Keys() []lighting.Key { return r.info.Keys }

func (r *remote) Floor() time.Duration { return time.Duration(r.info.FloorMS) * time.Millisecond }

func (r *remote) Zones() []lighting.Zone { return r.info.Zones }

func (r *remote) Resize(zone string, _ int) error {
	return fmt.Errorf("%s's zone %q is the device's own length", r.info.Name, zone) //nolint:err113 // said plainly
}

func (r *remote) Frame(ctx context.Context, px []lighting.Pixel) error {
	return r.ask(ctx, request{Op: opFrame, Device: r.info.ID, Pixels: pixels(px)})
}

func (r *remote) Release(ctx context.Context) error {
	return r.ask(ctx, request{Op: opRelease, Device: r.info.ID})
}

func (r *remote) ask(ctx context.Context, req request) error {
	_, err := r.answer(ctx, req)
	return err
}

// answer is the helper's answer to req, or the error it carries, a lighting
// error by its name where it has one.
func (r *remote) answer(ctx context.Context, req request) (response, error) {
	resp, err := r.c.call(ctx, req)
	if err != nil {
		return response{}, fmt.Errorf("%s: %w: %w", r.info.Name, sanshoku.ErrGone, err)
	}
	if resp.Gone {
		return response{}, fmt.Errorf("%s: %w: %w", r.info.Name, sanshoku.ErrGone, ErrGone)
	}
	if resp.Error != "" {
		if k, ok := kinds[resp.Kind]; ok {
			return response{}, fmt.Errorf("%s: %s: %w", r.info.Name, printable(resp.Error), k)
		}
		return response{}, fmt.Errorf("%s: %s", r.info.Name, printable(resp.Error)) //nolint:err113 // the helper's words
	}
	return resp, nil
}

/*
tuned is one of the helper's devices that runs effects of its own (spec 085):
the effects its driver lists, run by the helper. A lighting.Tuned, as a
device the service opens itself would be.
*/
type tuned struct{ *remote }

var _ lighting.Tuned = (*tuned)(nil)

func (t *tuned) Effects() []string {
	out := make([]string, len(t.info.Effects))
	for i, e := range t.info.Effects {
		out[i] = e.Name
	}
	return out
}

func (t *tuned) Catalogue() []lighting.EffectInfo { return t.info.Effects }

func (t *tuned) Effect(ctx context.Context) (lighting.Effect, error) {
	resp, err := t.answer(ctx, request{Op: opRunning, Device: t.info.ID})
	if err != nil {
		return lighting.Effect{}, err
	}
	if resp.Running == nil {
		return lighting.Effect{}, fmt.Errorf("%s: %w", t.info.Name, lighting.ErrUnknownEffect)
	}
	return *resp.Running, nil
}

func (t *tuned) SetEffect(ctx context.Context, name string) (bool, error) {
	return t.SetEffectWith(ctx, name, lighting.Settings{})
}

func (t *tuned) SetEffectWith(ctx context.Context, name string, s lighting.Settings) (bool, error) {
	colours := make([][3]uint8, len(s.Colours))
	for i, c := range s.Colours {
		colours[i] = [3]uint8{c.R, c.G, c.B}
	}
	resp, err := t.answer(ctx, request{Op: opEffect, Device: t.info.ID, Effect: name, Colours: colours, Speed: s.Speed, Restart: s.Restart})
	return resp.Wrote, err
}

func (t *tuned) SetColour(ctx context.Context, c lighting.Colour) (bool, error) {
	resp, err := t.answer(ctx, request{Op: opColour, Device: t.info.ID, Colours: [][3]uint8{{c.R, c.G, c.B}}})
	return resp.Wrote, err
}
