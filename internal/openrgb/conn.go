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
	if err := c.client.Close(); err != nil {
		return fmt.Errorf("hang up on %s: %w", c.address, err)
	}
	return nil
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
	count, err := c.client.RequestControllerCountCtx(ctx)
	if err != nil {
		return nil, fmt.Errorf("ask %s how many devices it has: %w", c.address, err)
	}

	out := make([]located, 0, count.Count)
	for i := uint32(0); i < count.Count; i++ {
		data, err := c.client.RequestControllerDataCtx(ctx, i)
		if err != nil {
			return nil, fmt.Errorf("read device %d from %s: %w", i, c.address, err)
		}
		out = append(out, located{index: i, device: convert(data.Controller)})
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
func (c *Conn) Device(ctx context.Context, name string) (devices.Device, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

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
	data, err := c.client.RequestControllerDataCtx(ctx, index)
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
func (c *Conn) SetMode(ctx context.Context, device, mode string, brightness *int, want *colour.Colour) error {
	c.mu.Lock()
	defer c.mu.Unlock()

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
		if brightness != nil && wanted.ModeFlags&flagHasBrightness != 0 {
			wanted.ModeBrightness = scaleBrightness(*brightness, m.ModeBrightnessMin, m.ModeBrightnessMax)
		}
		/*
			A mode that takes its own colour needs it set here. The frame
			written afterwards goes to a buffer such a mode does not read, so
			without this the device shows the colour its vendor left behind --
			a Kraken put into Static displayed NZXT's red while the buffer, and
			every read-back of it, held purple.

			Only the slots the mode actually has are filled: ModeColorsMin is
			how many it insists on, and a mode advertising none is left alone.
		*/
		if want != nil && wanted.ModeFlags&flagHasModeSpecificColor != 0 {
			wanted.ModeColors = modeColours(m, *want)
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
func (c *Conn) SetFrame(ctx context.Context, device string, frame devices.Frame) error {
	c.mu.Lock()
	defer c.mu.Unlock()

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

	for i, mode := range data.Modes {
		m := devices.Mode{
			Name:       clean(mode.ModeName),
			PerLED:     mode.ModeFlags&flagHasPerLEDColor != 0,
			Brightness: mode.ModeFlags&flagHasBrightness != 0,
			ModeColour: mode.ModeFlags&flagHasModeSpecificColor != 0,
		}
		if m.ModeColour && len(mode.ModeColors) > 0 {
			m.Colour = colour.Colour{R: mode.ModeColors[0].R, G: mode.ModeColors[0].G, B: mode.ModeColors[0].B}
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
modeColours fills a mode's colour slots with one colour.

A mode declares how many it takes. Most take one; a few take several, and a
device asked for a solid colour wants all of them the same rather than one set
and the rest whatever they were.
*/
func modeColours(m *sdk.Mode, c colour.Colour) []sdk.Color {
	n := len(m.ModeColors)
	if n < int(m.ModeColorsMin) {
		n = int(m.ModeColorsMin)
	}
	if n == 0 {
		n = 1
	}
	if most := int(m.ModeColorsMax); most > 0 && n > most {
		n = most
	}
	out := make([]sdk.Color, n)
	for i := range out {
		out[i] = sdk.Color{R: c.R, G: c.G, B: c.B}
	}
	return out
}
