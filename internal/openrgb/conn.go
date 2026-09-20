package openrgb

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"

	sdk "github.com/csutorasa/go-openrgb-sdk"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/devices"
)

// clientName is what hotaru calls itself to the server. It shows up in
// OpenRGB's own client list, so it says which program is holding the socket.
const clientName = "hotaru"

/*
Conn is a connection to an OpenRGB server.

Safe from several goroutines: the SDK's client is not, and the service writes
from a goroutine per device, so every exchange is serialised here. That is
cheap — a write is sub-millisecond against a running server — and it is the
alternative to discovering the hard way that two writes interleaved on one
socket.
*/
type Conn struct {
	mu      sync.Mutex
	client  *sdk.Client
	address string
	version uint32
}

/*
Dial connects to a server.

A refused connection is an ordinary result, not a crisis: the server may not be
running yet, or at all, and hotaru's job is then to say so rather than to fail
to start. The error names the address it tried, because "connection refused"
without one is the least useful sentence in computing.
*/
func Dial(ctx context.Context, address string) (*Conn, error) {
	if address == "" {
		address = DefaultAddress
	}

	var dialer net.Dialer
	netConn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("connect to the OpenRGB server at %s: %w", address, err)
	}

	client := sdk.NewClient(netConn)
	if err := client.Initialize(clientName); err != nil {
		_ = netConn.Close()
		return nil, fmt.Errorf("introduce hotaru to the OpenRGB server at %s: %w", address, err)
	}
	// Version negotiation is the one piece of the handshake worth insisting on:
	// health reports what was agreed, so a client lagging a server release is
	// visible rather than showing up as a field that decodes oddly.
	if err := client.RequestProtocolVersion(); err != nil {
		_ = netConn.Close()
		return nil, fmt.Errorf("agree a protocol version with %s: %w", address, err)
	}

	return &Conn{client: client, address: address, version: uint32(client.CommonVersion())}, nil
}

// Address is the server this is connected to, for messages that need to name it.
func (c *Conn) Address() string { return c.address }

// ProtocolVersion is the version client and server agreed on.
func (c *Conn) ProtocolVersion() uint32 { return c.version }

// Close hangs up.
func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client.Close()
}

/*
Devices lists what the server has, in its order.

Duplicate names collapse to their first occurrence. A server that has rescanned
lists every device twice, and addressing the second copy means sending every
command to the same hardware twice — which is visible as a device that takes
two writes to change, and invisible as anything else.
*/
func (c *Conn) Devices(ctx context.Context) ([]devices.Device, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.list(ctx)
}

func (c *Conn) list(ctx context.Context) ([]devices.Device, error) {
	count, err := c.client.RequestControllerCountCtx(ctx)
	if err != nil {
		return nil, fmt.Errorf("ask %s how many devices it has: %w", c.address, err)
	}

	out := make([]devices.Device, 0, count.Count)
	seen := make(map[string]bool, count.Count)
	for i := uint32(0); i < count.Count; i++ {
		data, err := c.client.RequestControllerDataCtx(ctx, i)
		if err != nil {
			return nil, fmt.Errorf("read device %d from %s: %w", i, c.address, err)
		}
		device := convert(data.Controller)
		if device.Name == "" || seen[strings.ToLower(device.Name)] {
			continue
		}
		seen[strings.ToLower(device.Name)] = true
		out = append(out, device)
	}
	return out, nil
}

// index finds a device by name within one listing, and never outside it.
func (c *Conn) index(ctx context.Context, name string) (uint32, *sdk.ControllerData, error) {
	count, err := c.client.RequestControllerCountCtx(ctx)
	if err != nil {
		return 0, nil, fmt.Errorf("ask %s how many devices it has: %w", c.address, err)
	}
	for i := uint32(0); i < count.Count; i++ {
		data, err := c.client.RequestControllerDataCtx(ctx, i)
		if err != nil {
			return 0, nil, fmt.Errorf("read device %d from %s: %w", i, c.address, err)
		}
		if strings.EqualFold(clean(data.Controller.Name), name) {
			return i, data.Controller, nil
		}
	}
	return 0, nil, fmt.Errorf("no device called %q on %s", name, c.address)
}

/*
SetMode puts a device into a mode.

The mode is named, resolved to the device's own index here, and sent back with
the device's own definition of it — speed, direction and colour count included —
because a mode is a structure the server round-trips rather than a string it
looks up. Brightness is asserted only where the mode says it has any.
*/
func (c *Conn) SetMode(ctx context.Context, device, mode string, brightness *int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	idx, data, err := c.index(ctx, device)
	if err != nil {
		return err
	}

	for i, m := range data.Modes {
		if !strings.EqualFold(clean(m.ModeName), mode) {
			continue
		}
		wanted := *m
		if brightness != nil && wanted.ModeFlags&flagHasBrightness != 0 {
			wanted.ModeBrightness = scaleBrightness(*brightness, m.ModeBrightnessMin, m.ModeBrightnessMax)
		}
		req := &sdk.RGBControllerUpdateModeRequest{ModeIdx: int32(i), Mode: &wanted}
		if err := c.client.RGBControllerUpdateMode(idx, req); err != nil {
			return fmt.Errorf("set %s to %s: %w", device, mode, err)
		}
		return nil
	}
	return fmt.Errorf("%s has no mode called %q", device, mode)
}

/*
SetFrame writes a colour per LED.

The whole device in one exchange. A frame is the unit everywhere above this for
reasons that are about scenes and reconciliation, and it happens to be what the
protocol wants too.
*/
func (c *Conn) SetFrame(ctx context.Context, device string, frame devices.Frame) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	idx, data, err := c.index(ctx, device)
	if err != nil {
		return err
	}
	if want, got := len(data.Leds), len(frame.Colours); want != got {
		return fmt.Errorf("%s has %d LEDs and the frame has %d", device, want, got)
	}

	payload := make([]sdk.Color, len(frame.Colours))
	for i, col := range frame.Colours {
		payload[i] = sdk.Color{R: col.R, G: col.G, B: col.B}
	}
	if err := c.client.RGBControllerUpdateLeds(idx, &sdk.RGBControllerUpdateLedsRequest{LedColor: payload}); err != nil {
		return fmt.Errorf("write %d LEDs to %s: %w", len(payload), device, err)
	}
	return nil
}

/*
clean trims what the wire leaves on a string.

Every name the server sends is a C string, and the protocol carries its NUL
terminator with it: the GPU arrives as "MSI GeForce RTX 4090 Suprim Liquid X\x00"
and its mode as "Direct\x00". Left alone, the terminator is invisible in a log
line and fatal in a comparison -- a rule asking for "static" never matches
"Static\x00", so every device falls through to "no mode that shows a solid
colour" with nothing to explain why.

Found by the live test against a real server on the first run, which is exactly
what that test is for: no fake produces a NUL nobody thought to add.
*/
func clean(s string) string {
	// A C string ends at its first NUL; anything after it is padding that
	// happened to be in the buffer, not part of the name.
	if i := strings.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// convert turns the SDK's controller into hotaru's device.
func convert(data *sdk.ControllerData) devices.Device {
	device := devices.Device{
		Name:     clean(data.Name),
		Type:     fmt.Sprintf("%d", data.Type),
		LEDCount: len(data.Leds),
	}

	for i, mode := range data.Modes {
		device.Modes = append(device.Modes, devices.Mode{
			Name:       clean(mode.ModeName),
			PerLED:     mode.ModeFlags&flagHasPerLEDColor != 0,
			Brightness: mode.ModeFlags&flagHasBrightness != 0,
		})
		if int32(i) == data.ActiveMode {
			device.ActiveMode = clean(mode.ModeName)
		}
	}

	// Zones are contiguous runs in device LED order; the protocol gives their
	// sizes and leaves the offsets to be counted.
	first := 0
	for _, zone := range data.Zones {
		device.Zones = append(device.Zones, devices.Zone{
			Name:  clean(zone.ZoneName),
			First: first,
			Count: int(zone.ZoneLedsCount),
		})
		first += int(zone.ZoneLedsCount)
	}

	device.Colours = make([]colour.Colour, len(data.Colors))
	for i, col := range data.Colors {
		device.Colours[i] = colour.Colour{R: col.R, G: col.G, B: col.B}
	}
	return device
}

/*
scaleBrightness maps 0-100 onto whatever range a mode uses.

Devices disagree: some take 0-100, some 0-255, some 0-3. A rule says "100" and
means "as bright as this goes", which is the only portable reading of a number
a person typed.
*/
func scaleBrightness(percent int, min, max uint32) uint32 {
	switch {
	case percent <= 0:
		return min
	case percent >= 100:
		return max
	}
	span := float64(max) - float64(min)
	return min + uint32(span*float64(percent)/100.0+0.5)
}
