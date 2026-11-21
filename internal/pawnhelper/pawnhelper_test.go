package pawnhelper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/lighting"
)

// fake is a device the helper holds: a canvas with the lights it is given,
// keeping every frame and release it is asked for.
type fake struct {
	name     string
	keys     []lighting.Key
	mu       sync.Mutex
	frames   [][]lighting.Pixel
	released int
	closed   bool
	err      error
}

func (f *fake) Identity() sanshoku.Identity {
	return sanshoku.Identity{Name: f.name, Path: "pawnio:SmbusPIIX4.bin:0"}
}
func (f *fake) Close() error         { f.mu.Lock(); f.closed = true; f.mu.Unlock(); return nil }
func (f *fake) Keys() []lighting.Key { return f.keys }
func (f *fake) Floor() time.Duration { return 200 * time.Millisecond }
func (f *fake) Frame(_ context.Context, px []lighting.Pixel) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.frames = append(f.frames, px)
	return nil
}
func (f *fake) Release(context.Context) error { f.mu.Lock(); f.released++; f.mu.Unlock(); return nil }

// notCanvas is a device the helper does not keep.
type notCanvas struct{ closed bool }

func (n *notCanvas) Identity() sanshoku.Identity { return sanshoku.Identity{Name: "a battery"} }
func (n *notCanvas) Close() error                { n.closed = true; return nil }

// connected is a client and a helper over an in-memory pipe, the helper
// holding devs.
func connected(t *testing.T, devs ...sanshoku.Device) (*client, *handler) {
	t.Helper()
	scans := 0
	h := &handler{scan: func(context.Context) ([]sanshoku.Device, error) {
		scans++
		if scans > 1 {
			return nil, errors.New("scanned twice")
		}
		return devs, nil
	}}
	c := &client{dial: func(ctx context.Context) (io.ReadWriteCloser, error) {
		ours, theirs := net.Pipe()
		go func() { _ = h.serveConn(ctx, theirs); _ = theirs.Close() }()
		return ours, nil
	}}
	t.Cleanup(func() { c.mu.Lock(); c.drop(); c.mu.Unlock() })
	return c, h
}

// Spec 083. The service finds the helper's canvases, draws on them and
// hands them back through the pipe, and only what is a canvas is kept.
func TestTheServiceDrawsThroughTheHelper(t *testing.T) {
	board := &fake{name: "MSI B450 TOMAHAWK MAX", keys: []lighting.Key{{ID: 0, Name: "MSI LED"}}}
	memory := &fake{name: "Corsair Vengeance RGB", keys: []lighting.Key{{ID: 0, Name: "Corsair LED"}}}
	battery := &notCanvas{}
	c, _ := connected(t, board, battery, memory)

	found, err := driver{c: c}.Find(t.Context())
	require.NoError(t, err)
	require.Len(t, found, 2)
	assert.True(t, battery.closed, "a device that is not a canvas is kept open")
	assert.Equal(t, "MSI B450 TOMAHAWK MAX", found[0].Name)

	dev, err := found[0].Open(t.Context())
	require.NoError(t, err)
	canvas, ok := dev.(lighting.Canvas)
	require.True(t, ok)
	assert.Equal(t, []lighting.Key{{ID: 0, Name: "MSI LED"}}, canvas.Keys())
	assert.Equal(t, 200*time.Millisecond, canvas.Floor())

	require.NoError(t, canvas.Frame(t.Context(), []lighting.Pixel{{ID: 0, R: 255, G: 0, B: 128}}))
	assert.Equal(t, [][]lighting.Pixel{{{ID: 0, R: 255, G: 0, B: 128}}}, board.frames)
	assert.Empty(t, memory.frames)
	require.NoError(t, canvas.Release(t.Context()))
	assert.Equal(t, 1, board.released)

	// Closing the service's end lets go of nothing: the helper keeps the
	// device and its frame.
	require.NoError(t, dev.Close())
	assert.False(t, board.closed)

	// A second list does not scan again.
	again, err := driver{c: c}.Find(t.Context())
	require.NoError(t, err)
	assert.Len(t, again, 2)
}

// A request may light only the device's own keys, and no frame larger than
// any device here; an unknown device, request or version is refused.
func TestTheHelperRefusesWhatNoDeviceHas(t *testing.T) {
	board := &fake{name: "board", keys: []lighting.Key{{ID: 0}}}
	_, h := connected(t, board)
	ctx := t.Context()
	listed := h.handle(ctx, request{V: Protocol, Op: opList})
	require.Empty(t, listed.Error)
	id := listed.Devices[0].ID

	assert.Contains(t, h.handle(ctx, request{V: Protocol, Op: opFrame, Device: id, Pixels: [][4]uint8{{7, 1, 2, 3}}}).Error, "light 7")
	big := make([][4]uint8, maxPixels+1)
	assert.Contains(t, h.handle(ctx, request{V: Protocol, Op: opFrame, Device: id, Pixels: big}).Error, "a frame of")
	assert.Empty(t, board.frames, "a refused frame reached the device")

	r := h.handle(ctx, request{V: Protocol, Op: opFrame, Device: "d9"})
	assert.True(t, r.Gone)
	assert.Contains(t, h.handle(ctx, request{V: Protocol, Op: "smbus-write"}).Error, "no such request")
	assert.Contains(t, h.handle(ctx, request{V: 2, Op: opList}).Error, "version")
}

// A device that went is gone to the service, which looks again; the helper
// drops it and scans on the next list.
func TestADeviceThatWentIsGone(t *testing.T) {
	board := &fake{name: "board", keys: []lighting.Key{{ID: 0}}, err: sanshoku.ErrGone}
	c, h := connected(t, board)
	found, err := driver{c: c}.Find(t.Context())
	require.NoError(t, err)
	dev, _ := found[0].Open(t.Context())
	err = dev.(lighting.Canvas).Frame(t.Context(), []lighting.Pixel{{ID: 0}})
	require.ErrorIs(t, err, sanshoku.ErrGone)
	assert.True(t, board.closed)
	assert.Empty(t, h.devices)
}

// A helper that is not installed is absence; one that answers with an
// error is a driver that could not look; its words reach a terminal
// without control characters.
func TestNoHelperIsAbsence(t *testing.T) {
	c := &client{dial: func(context.Context) (io.ReadWriteCloser, error) { return nil, errNoHelper }}
	_, err := driver{c: c}.Find(t.Context())
	require.ErrorIs(t, err, sanshoku.ErrAbsent)

	h := &handler{scan: func(context.Context) ([]sanshoku.Device, error) {
		return nil, errors.New("PawnIO is not installed\x1b[2J" + strings.Repeat("x", 2*maxText))
	}}
	c = &client{dial: func(ctx context.Context) (io.ReadWriteCloser, error) {
		ours, theirs := net.Pipe()
		go func() { _ = h.serveConn(ctx, theirs) }()
		return ours, nil
	}}
	_, err = driver{c: c}.Find(t.Context())
	require.ErrorIs(t, err, sanshoku.ErrUnavailable)
	for _, r := range err.Error() {
		assert.False(t, unicode.IsControl(r))
	}
	assert.Contains(t, err.Error(), "PawnIO is not installed")
}

// A line longer than the bound ends the connection.
func TestALineTooLongEndsTheConnection(t *testing.T) {
	h := &handler{scan: func(context.Context) ([]sanshoku.Device, error) { return nil, nil }}
	ours, theirs := net.Pipe()
	done := make(chan error, 1)
	go func() { done <- h.serveConn(t.Context(), theirs) }()
	go func() { _, _ = ours.Write([]byte(strings.Repeat("x", maxLine+10) + "\n")) }()
	require.Error(t, <-done)
}

// Stopping the helper closes its devices and hands none back: the last
// frame stays, as it does when the service stops.
func TestStoppingTheHelperLeavesTheLastFrame(t *testing.T) {
	board := &fake{name: "board", keys: []lighting.Key{{ID: 0}}}
	_, h := connected(t, board)
	require.Empty(t, h.handle(t.Context(), request{V: Protocol, Op: opList}).Error)
	h.close()
	assert.True(t, board.closed)
	assert.Zero(t, board.released)
}

// The modules packaging puts beside the program are the ones the helper
// checks for, byte for byte PawnIO.Modules 0.2.12's: a change to either is a
// change of what runs in the kernel, made here on purpose.
func TestTheModulesAreTheReleasesOwn(t *testing.T) {
	for name, want := range modules {
		b, err := os.ReadFile(filepath.Join("..", "..", "third_party", "pawnio", name))
		require.NoError(t, err)
		sum := sha256.Sum256(b)
		assert.Equal(t, want, hex.EncodeToString(sum[:]), name)
	}
	entries, err := os.ReadDir(filepath.Join("..", "..", "third_party", "pawnio"))
	require.NoError(t, err)
	assert.Len(t, entries, len(modules)+1, "a module the helper does not check, beside COPYING")
}

// An ID names one device of one scan of one helper: after the device went
// and the helper scanned again, the old ID is gone, not another device. A
// scan that found nothing is not repeated at the service's pace.
func TestIDsAreNotReusedAndAnEmptyScanWaits(t *testing.T) {
	now := time.Unix(1000, 0)
	board := &fake{name: "board", keys: []lighting.Key{{ID: 0}}}
	scans := 0
	var give []sanshoku.Device
	h := &handler{epoch: "e", now: func() time.Time { return now }, scan: func(context.Context) ([]sanshoku.Device, error) {
		scans++
		return give, errors.New("nothing here")
	}}
	ctx := t.Context()
	assert.NotEmpty(t, h.handle(ctx, request{V: Protocol, Op: opList}).Error)
	assert.NotEmpty(t, h.handle(ctx, request{V: Protocol, Op: opList}).Error)
	assert.Equal(t, 1, scans, "an empty scan was repeated at once")

	now = now.Add(rescanAfter + time.Second)
	give = []sanshoku.Device{board}
	first := h.handle(ctx, request{V: Protocol, Op: opList}).Devices[0].ID
	h.drop(first)
	now = now.Add(rescanAfter + time.Second)
	second := h.handle(ctx, request{V: Protocol, Op: opList}).Devices[0].ID
	assert.NotEqual(t, first, second)
	assert.True(t, h.handle(ctx, request{V: Protocol, Op: opFrame, Device: first}).Gone)
}

// A helper that stops answering costs the request, which ends with its
// context, and the next request opens a new connection.
func TestAHelperThatDoesNotAnswerIsGivenUpOn(t *testing.T) {
	dials := 0
	c := &client{dial: func(context.Context) (io.ReadWriteCloser, error) {
		dials++
		ours, theirs := net.Pipe()
		go func() { _, _ = io.Copy(io.Discard, theirs) }() // reads, never answers
		return ours, nil
	}}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	_, err := c.call(ctx, request{Op: opList})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	ctx2, cancel2 := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel2()
	_, _ = c.call(ctx2, request{Op: opList})
	assert.Equal(t, 2, dials)
}
