package openrgb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	sdk "github.com/csutorasa/go-openrgb-sdk"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/devices"
)

// clientName is what hotaru calls itself to the server. It shows up in
// OpenRGB's own client list, so it says which program is holding the socket.
const clientName = "hotaru"

/*
exchangeTimeout is how long one exchange waits for its answer.

A listing takes under a millisecond against a local server, so this is a
server that has stopped answering, not a slow one. Without it, a server that
died between a request and its answer held Conn.mu for good: the SDK does not
wake a waiting exchange when the socket closes. See spec 063.
*/
const exchangeTimeout = 5 * time.Second

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
	socket  net.Conn
	address string
	version uint32

	// wait is how long one exchange waits for its answer: exchangeTimeout,
	// shortened by a test that should not spend seconds proving a timeout.
	wait time.Duration

	// gone is whether this connection has failed at the connection level.
	// Sticky, because a socket whose peer has gone does not come back: the
	// server that answers next is a different process with its own
	// enumeration, reached by dialling again. Atomic rather than guarded by
	// mu, so asking does not wait behind an exchange that is stuck on the
	// very server that went (spec 063 R3).
	gone atomic.Bool
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
	return handshake(ctx, netConn, address, exchangeTimeout)
}

// handshake introduces hotaru on a connected socket. Apart from Dial so a
// test can hand it one end of a pipe, and say how long an exchange may wait.
func handshake(ctx context.Context, netConn net.Conn, address string, wait time.Duration) (*Conn, error) {
	client := sdk.NewClient(netConn)
	if err := client.Initialize(clientName); err != nil {
		_ = netConn.Close()
		return nil, fmt.Errorf("introduce hotaru to the OpenRGB server at %s: %w", address, err)
	}
	// Version negotiation is the one piece of the handshake worth insisting on:
	// health reports what was agreed, so a client lagging a server release is
	// visible rather than showing up as a field that decodes oddly. Bounded,
	// like every exchange, so a server that accepts and never answers cannot
	// hold a redial. The caller's context still counts here: nothing else
	// will use this connection if the handshake is abandoned.
	agree, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	if err := client.RequestProtocolVersionCtx(agree); err != nil {
		_ = netConn.Close()
		return nil, fmt.Errorf("agree a protocol version with %s: %w", address, err)
	}

	return &Conn{
		client: client, socket: netConn, address: address,
		version: uint32(client.CommonVersion()), wait: wait,
	}, nil
}

/*
exchange is the context one request waits on: the caller's values, and a
deadline of the connection's own in place of the caller's cancellation.

The caller cannot cut an exchange short, because the SDK cannot abandon one
safely. A request that gives up leaves its reply channel registered, and the
answer that still arrives blocks the SDK's read loop for good -- every later
exchange on the connection then waits for nothing. A client hanging up on the
API mid-request was enough to do that to a healthy server. So the only thing
that ends an exchange early is this deadline, and reaching it means the server
stopped answering (spec 063 R1.1, R2).
*/
func (c *Conn) exchange(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), c.wait)
}

// Address is the server this is connected to, for messages that need to name it.
func (c *Conn) Address() string { return c.address }

// ProtocolVersion is the version client and server agreed on.
func (c *Conn) ProtocolVersion() uint32 { return c.version }

/*
Close hangs up.

The socket, not the SDK's client. The client's Close hands nil to every reply
channel it still holds, and after an exchange has timed out nobody is
receiving on one, so it blocks for good -- in the redial that is replacing
this connection. Closing the socket is all the hanging up a server sees. No
lock either: closing a socket is safe from any goroutine, and it is how an
exchange stuck on this connection learns to stop.
*/
func (c *Conn) Close() error {
	if err := c.socket.Close(); err != nil {
		return fmt.Errorf("hang up on %s: %w", c.address, err)
	}
	return nil
}

/*
Gone is whether the server this was connected to has stopped answering.

Distinct from any one call failing. A device that is not there, a mode a device
does not have, a frame of the wrong length: those are answers, and the
connection is fine. This is the other kind -- the socket itself -- and it is
the one that cannot be retried, only redialled.

Sticky by design. Asking "is it back" of a dead connection has no useful answer,
because the server that comes back is a new process; the caller's move is to
dial again and replace this.
*/
func (c *Conn) Gone() bool {
	return c.gone.Load()
}

/*
note records a connection-level failure, and leaves every other kind alone.

The test is the error chain rather than the text of it: every exchange in this
file wraps the SDK's error with %w, and the three that do not are about a
device, a mode and an LED count -- none of which are the socket.
*/
func (c *Conn) note(err error) {
	if isGone(err) {
		c.gone.Store(true)
	}
}

// isGone is whether an error is the socket rather than the answer. Shared with
// the fake, so a test that says "the server went away" means by this what the
// hardware path means by it. An exchange that ran out of time counts: only
// the connection's own deadline ends one (see exchange), and a server that
// does not answer within it has gone as surely as one that hung up.
func isGone(err error) bool {
	if err == nil {
		return false
	}
	var timedOut *sdk.ResponseTimeoutError
	return errors.As(err, &timedOut) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF)
}

/*
Devices lists what the server has, in its order.

Duplicate names collapse to their first occurrence. A server that has rescanned
lists every device twice, and addressing the second copy means sending every
command to the same hardware twice — which is visible as a device that takes
two writes to change, and invisible as anything else.
*/
func (c *Conn) Devices(ctx context.Context) (found []devices.Device, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	defer func() { c.note(err) }()
	return c.list(ctx)
}

func (c *Conn) list(ctx context.Context) ([]devices.Device, error) {
	found, err := c.catalogue(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]devices.Device, 0, len(found))
	for _, entry := range found {
		out = append(out, entry.device)
	}
	return out, nil
}

/*
located is a device and the index it had in the listing it came from.

Indices are meaningful only within one listing -- a rescanned server renumbers
everything -- so they are produced and used in the same breath and never stored.
*/
type located struct {
	index  uint32
	device devices.Device
}

// catalogue is one listing: every controller, duplicates removed, names made
// distinguishable, each with the index it was found at.
func (c *Conn) catalogue(ctx context.Context) ([]located, error) {
	asked, cancel := c.exchange(ctx)
	count, err := c.client.RequestControllerCountCtx(asked)
	cancel()
	if err != nil {
		return nil, fmt.Errorf("ask %s how many devices it has: %w", c.address, err)
	}

	out := make([]located, 0, count.Count)
	for i := uint32(0); i < count.Count; i++ {
		data, err := c.raw(ctx, i)
		if err != nil {
			return nil, err
		}
		out = append(out, located{index: i, device: convert(data)})
	}
	return collapse(out), nil
}

/*
collapse removes entries that are the same controller listed twice, and makes
the rest distinguishable.

A rescanned server lists everything twice, and addressing the second copy sends
every command to the same hardware twice. The obvious rule -- keep the first of
each name -- is wrong, and a second machine proved it: four sticks of Corsair
DDR5 are four controllers with one name between them, at I2C addresses 0x18 to
0x1B, and collapsing by name hid three of them. A motherboard turned up twice
the same way, as two controllers with different hidraw nodes.

So identity is the name *and* where it is attached. Two entries at one location
are one device; two devices at different locations are two, whatever they are
called.

What is left can still share a name, and a name is how everything above this
addresses a device, so duplicates are given something to tell them apart:
a serial where there is one, otherwise the tail of the location -- "0x19" for a
stick of RAM, "hidraw8" for a board. Both are what the hardware itself says,
which is better than a number counted here that would move if a device were
unplugged.
*/
func collapse(in []located) []located {
	out := make([]located, 0, len(in))
	seen := make(map[string]bool, len(in))
	names := make(map[string]int, len(in))

	for _, entry := range in {
		if entry.device.Name == "" {
			continue
		}
		identity := strings.ToLower(entry.device.Name + "\x00" + entry.device.Location + "\x00" + entry.device.Serial)
		if seen[identity] {
			continue
		}
		seen[identity] = true
		names[strings.ToLower(entry.device.Name)]++
		out = append(out, entry)
	}

	for i := range out {
		if names[strings.ToLower(out[i].device.Name)] > 1 {
			if tag := distinguish(out[i].device); tag != "" {
				out[i].device.Name += " (" + tag + ")"
			}
		}
	}
	return out
}

/*
distinguish is the shortest thing that tells two of the same model apart.

The serial when the device has one, and the tail of its location when it does
not: an I2C address or a hidraw node. Corsair's DDR5 reports no serial and four
addresses; Gigabyte's board reports two serials and two hidraw nodes.
*/
func distinguish(device devices.Device) string {
	if device.Serial != "" {
		return device.Serial
	}
	location := device.Location
	if at := strings.LastIndex(location, "address "); at >= 0 {
		return strings.TrimSpace(location[at+len("address "):])
	}
	if slash := strings.LastIndex(location, "/"); slash >= 0 {
		return strings.TrimSpace(location[slash+1:])
	}
	return strings.TrimSpace(location)
}

/*
Device reads one controller by name.

Used to confirm a write landed: a device can accept a mode and not honour it, so
the active mode is read back and compared. A zero exit code establishes nothing.
*/
func (c *Conn) Device(ctx context.Context, name string) (found devices.Device, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	defer func() { c.note(err) }()

	entry, err := c.find(ctx, name)
	if err != nil {
		return devices.Device{}, err
	}
	return entry.device, nil
}

/*
find locates a device by the name hotaru knows it by, within one listing.

Through the same catalogue a listing comes from, which matters where a name has
been made distinguishable: "Corsair Dominator Platinum RGB DDR5 (0x19)" is not
what the controller calls itself, and looking it up against the raw name finds
nothing. Doing that was a real bug on a machine with four identical sticks --
every write to one of them failed, reported as "none of the modes took", which
sounded like the hardware refusing rather than hotaru looking for a name that
only it uses.
*/
func (c *Conn) find(ctx context.Context, name string) (located, error) {
	catalogue, err := c.catalogue(ctx)
	if err != nil {
		return located{}, err
	}
	for _, entry := range catalogue {
		if strings.EqualFold(entry.device.Name, name) {
			return entry, nil
		}
	}
	return located{}, fmt.Errorf("no device called %q on %s", name, c.address)
}

// raw is the controller as the server describes it, for a write that needs the
// mode structures the protocol round-trips.
func (c *Conn) raw(ctx context.Context, index uint32) (*sdk.ControllerData, error) {
	asked, cancel := c.exchange(ctx)
	defer cancel()
	data, err := c.client.RequestControllerDataCtx(asked, index)
	if err != nil {
		return nil, fmt.Errorf("read device %d from %s: %w", index, c.address, err)
	}
	return data.Controller, nil
}

/*
SetMode puts a device into a mode.

The mode is named, resolved to the device's own index here, and sent back with
the device's own definition of it — speed, direction and colour count included —
because a mode is a structure the server round-trips rather than a string it
looks up. Brightness is asserted only where the mode says it has any.
*/
func (c *Conn) SetMode(ctx context.Context, device, mode string, style Style) (err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	defer func() { c.note(err) }()

	entry, err := c.find(ctx, device)
	if err != nil {
		return err
	}
	data, err := c.raw(ctx, entry.index)
	if err != nil {
		return err
	}

	for i, m := range data.Modes {
		if !strings.EqualFold(clean(m.ModeName), mode) {
			continue
		}
		wanted := *m
		if style.Brightness != nil && wanted.ModeFlags&flagHasBrightness != 0 {
			wanted.ModeBrightness = scaleBrightness(*style.Brightness, m.ModeBrightnessMin, m.ModeBrightnessMax)
		}
		// A speed is in the mode's own units, so it is clamped rather than
		// scaled: the range came from this mode and the number is meant for it.
		if style.Speed != nil && wanted.ModeFlags&flagHasSpeed != 0 {
			wanted.ModeSpeed = clampSpeed(*style.Speed, m.ModeSpeedMin, m.ModeSpeedMax)
		}
		/*
			A mode that takes its own colour needs it set here. The frame
			written afterwards goes to a buffer such a mode does not read, so
			without this the device shows the colour its vendor left behind --
			a Kraken put into Static displayed NZXT's red while the buffer, and
			every read-back of it, held purple.

			Only the slots the mode allows are filled: ModeColorsMin is how
			many it insists on and ModeColorsMax how many it takes, and a mode
			advertising none is left alone. Several colours go one to a slot
			(spec 061 R3.1).

			The colour mode is set to mode-specific as well. A mode that can
			also run in random colours and was left in them ignores every
			slot, and somebody who names colours has answered that question.
		*/
		if palette := style.Palette(); len(palette) > 0 && wanted.ModeFlags&flagHasModeSpecificColor != 0 {
			wanted.ModeColors = modeColours(m, palette)
			wanted.ModeColorMode = colourModeSpecific
		}
		req := &sdk.RGBControllerUpdateModeRequest{ModeIdx: int32(i), Mode: &wanted}
		if err := c.client.RGBControllerUpdateMode(entry.index, req); err != nil {
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
func (c *Conn) SetFrame(ctx context.Context, device string, frame devices.Frame) (err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	defer func() { c.note(err) }()

	entry, err := c.find(ctx, device)
	if err != nil {
		return err
	}
	data, err := c.raw(ctx, entry.index)
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
	/*
		One request per zone, not one for the device.

		A device's zones are what the hardware treats as separate: an NZXT
		cooler's two Hue 2 channels are independent controllers behind one
		USB endpoint, and handing OpenRGB a single array spanning both meant
		they were never delivered together. That cooler showed a stale colour
		on one channel while the other moved, rendered a frame torn partway
		along a chain of fans, and stopped responding for minutes at a time
		under a run of writes.

		Written per zone, it tracks. See spec 011.
	*/
	for i, leds := range byZone(data.Zones, payload) {
		if len(leds) == 0 {
			continue
		}
		if err := c.client.RGBControllerUpdateZoneLeds(entry.index,
			&sdk.RGBControllerUpdateZoneLedsRequest{ZoneIdx: uint32(i), LedColor: leds}); err != nil {
			return fmt.Errorf("write %s zone %d: %w", device, i, err)
		}
	}
	return nil
}

/*
byZone cuts a device's frame into one run of colours per zone.

Zones are contiguous in device LED order and the protocol gives their sizes,
which is the same assumption the catalogue makes when it counts their offsets.
A device that reports no zones, or fewer LEDs in its zones than it has in
total, keeps the remainder in the last run rather than losing it: a frame is
the whole device or it is a bug.
*/
func byZone(zones []*sdk.Zone, payload []sdk.Color) [][]sdk.Color {
	if len(zones) == 0 {
		return [][]sdk.Color{payload}
	}
	out := make([][]sdk.Color, 0, len(zones))
	at := 0
	for i, zone := range zones {
		n := int(zone.ZoneLedsCount)
		if last := i == len(zones)-1; last || at+n > len(payload) {
			n = len(payload) - at
		}
		if n <= 0 {
			out = append(out, nil)
			continue
		}
		out = append(out, payload[at:at+n])
		at += n
	}
	return out
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
		Location: clean(data.Location),
		Serial:   clean(data.Serial),
		Type:     fmt.Sprintf("%d", data.Type),
		LEDCount: len(data.Leds),
	}
	// The server names each light ("Key: Escape"). Kept because a scene
	// written against a device's OpenRGB listing is carried to the canvas
	// device that draws the same hardware by these names (spec 060).
	for _, led := range data.Leds {
		device.LEDNames = append(device.LEDNames, clean(led.LedName))
	}

	for i, mode := range data.Modes {
		m := devices.Mode{
			Name:       clean(mode.ModeName),
			PerLED:     mode.ModeFlags&flagHasPerLEDColor != 0,
			Brightness: mode.ModeFlags&flagHasBrightness != 0,
			ModeColour: mode.ModeFlags&flagHasModeSpecificColor != 0,
		}
		if mode.ModeFlags&flagHasSpeed != 0 {
			m.Speed = &devices.Speed{
				Slowest: int(mode.ModeSpeedMin), Fastest: int(mode.ModeSpeedMax), Now: int(mode.ModeSpeed),
			}
		}
		if m.ModeColour {
			for _, c := range mode.ModeColors {
				m.Colours = append(m.Colours, colour.Colour{R: c.R, G: c.G, B: c.B})
			}
			if len(m.Colours) > 0 {
				m.Colour = m.Colours[0]
			}
			m.ColoursMin, m.ColoursMax = colourSlots(mode)
		}
		device.Modes = append(device.Modes, m)
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
			Shape: shapeOf(zone.ZoneType),
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
shapeOf reads OpenRGB's zone type.

0 is a single light, 1 a line of them, 2 a grid. The numbers are the protocol's
and the words are for people: what a caller needs to know is whether several
separate things could be attached to it, and "line" answers that where "1"
does not.
*/
func shapeOf(zoneType int32) devices.Shape {
	switch zoneType {
	case 0:
		return devices.ShapeSingle
	case 2:
		return devices.ShapeGrid
	default:
		return devices.ShapeLine
	}
}

/*
scaleBrightness maps 0-100 onto whatever range a mode uses.

Devices disagree: some take 0-100, some 0-255, some 0-3. A rule says "100" and
means "as bright as this goes", which is the only portable reading of a number
a person typed.
*/
func scaleBrightness(percent int, lowest, highest uint32) uint32 {
	switch {
	case percent <= 0:
		return lowest
	case percent >= 100:
		return highest
	}
	span := float64(highest) - float64(lowest)
	return lowest + uint32(span*float64(percent)/100.0+0.5)
}

/*
clampSpeed holds a speed inside the range its mode gives.

The bounds can arrive either way round -- OpenRGB has drivers where a smaller
number is faster, and they say so by giving a min above the max -- so the pair
is sorted before the number is held between them rather than trusted to be in
order.
*/
func clampSpeed(speed int, low, high uint32) uint32 {
	first, last := low, high
	if first > last {
		first, last = last, first
	}
	// Compared in the mode's own width rather than in int, which is 32 bits on
	// some machines and would turn a large bound negative on the way past.
	if speed < 0 {
		return first
	}
	switch want := uint64(speed); {
	case want < uint64(first):
		return first
	case want > uint64(last):
		return last
	default:
		return uint32(want) //nolint:gosec // between two uint32s by the cases above
	}
}

// modeColours is a mode's colour slots, filled as Slots says.
func modeColours(m *sdk.Mode, colours []colour.Colour) []sdk.Color {
	filled := Slots(colours, len(m.ModeColors), int(m.ModeColorsMin), int(m.ModeColorsMax))
	out := make([]sdk.Color, len(filled))
	for i, c := range filled {
		out[i] = sdk.Color{R: c.R, G: c.G, B: c.B}
	}
	return out
}

/*
colourSlots is how many colours of its own a mode takes, as the mode reports
it.

Read only for a mode with the mode-specific flag, which convert checks first:
a GPU's Direct reports one and one beside a per-LED colour mode, and offering
a colour for it would be offering a slot the device does not read.

A mode with the flag that reports a maximum of zero still takes one. None was
measured (spec 061, Verification), and one is what hotaru has always written
to such a mode, so the editor keeps offering it.
*/
func colourSlots(mode *sdk.Mode) (least, most int) {
	least, most = int(mode.ModeColorsMin), int(mode.ModeColorsMax)
	if most == 0 {
		most = max(1, least, len(mode.ModeColors))
	}
	return min(least, most), most
}

// colourModeSpecific is OpenRGB's MODE_COLORS_MODE_SPECIFIC: the mode shows
// the colours in its own slots, rather than none, per-LED or random ones.
const colourModeSpecific = 2
