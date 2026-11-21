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
		r := &remote{c: d.c, info: dev}
		r.info.Name = printable(dev.Name)
		out = append(out, sanshoku.Candidate{
			Identity: r.Identity(),
			Driver:   d.Name(),
			Key:      "pawnio:" + printable(dev.ID),
			Open:     func(context.Context) (sanshoku.Device, error) { return r, nil },
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
	resp, err := r.c.call(ctx, req)
	if err != nil {
		return fmt.Errorf("%s: %w: %w", r.info.Name, sanshoku.ErrGone, err)
	}
	if resp.Gone {
		return fmt.Errorf("%s: %w: %w", r.info.Name, sanshoku.ErrGone, ErrGone)
	}
	if resp.Error != "" {
		return fmt.Errorf("%s: %s", r.info.Name, printable(resp.Error)) //nolint:err113 // the helper's words
	}
	return nil
}
