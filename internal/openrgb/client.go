/*
Package openrgb talks to an OpenRGB server.

hotaru speaks the SDK's binary protocol on TCP rather than driving the openrgb
binary. The Python it replaces shelled out to `openrgb --client` and parsed the
output with regular expressions, including a hand-written parser for the
bracket-and-quote grammar of a "Modes:" line. Three things go away with the
subprocess: those parsers, the silent nine-second fallback to local detection
when the server is down, and the rule that a device name must be at least three
characters for the CLI's own matching.

The Client interface is narrow on purpose. Everything above it — the service,
the shells — depends on this and not on the SDK, so the dependency can be
replaced with a vendored protocol if it ever goes unmaintained.
*/
package openrgb

import (
	"context"

	"github.com/ushineko/hotaru/internal/colour"

	"github.com/ushineko/hotaru/internal/devices"
)

// DefaultAddress is where an OpenRGB server listens.
const DefaultAddress = "127.0.0.1:6742"

/*
Client is what hotaru needs from an OpenRGB server.

Devices are addressed by name at this boundary, never by index. A server that
has rescanned lists every device twice, and an index is meaningful only within
one listing — so an index never leaves this package, and never reaches disk.
*/
type Client interface {
	// Devices is every controller the server currently knows, with its modes,
	// zones and the colours it is showing.
	Devices(ctx context.Context) ([]devices.Device, error)

	// Device is one controller by name, for reading back what a write did.
	// Cheaper than a full listing, and a read-back happens after every write.
	Device(ctx context.Context, name string) (devices.Device, error)

	// SetMode puts a device into a mode, with whatever of that mode's own
	// settings the caller has an opinion about. A mode-specific mode set
	// without its colour shows whatever the vendor last stored in it, which is
	// a write that looks like a success from every angle the protocol offers
	// -- see spec 009.
	SetMode(ctx context.Context, device, mode string, style Style) error

	// SetFrame writes one colour per LED. The whole device, always: a frame is
	// the unit precisely so a write cannot land half-applied.
	SetFrame(ctx context.Context, device string, frame devices.Frame) error

	// ProtocolVersion is the version client and server agreed on, which health
	// reports so a mismatch is visible rather than mysterious.
	ProtocolVersion() uint32

	// Gone is whether the server has stopped answering this connection, as
	// opposed to any one call failing. A server that restarts -- an upgrade, a
	// crash, a bounce to pick up a replugged device -- leaves a socket that
	// cannot be retried, only redialled, and the daemon watches this to know
	// when to do that. See spec 058.
	Gone() bool

	Close() error
}

/*
Style is what to write into a mode besides the fact of being in it.

One argument rather than three, because these are the same kind of thing and
arrive together: a mode's own settings, each nil where the caller has no
opinion and each written only where the mode advertises it. A device asked for
a setting its mode does not have is not an error; it is a device that does not
have it.
*/
type Style struct {
	// Brightness is 0-100, scaled into whatever range the mode advertises.
	Brightness *int
	// Colour is the mode's own colour, for a mode that keeps one.
	Colour *colour.Colour
	/*
		Colours are the mode's own colours where there are several, first to
		last, and Colour is then their first (spec 061). Nil is one colour or
		none, as Colour says.

		Two fields rather than one list, because one colour is still the
		common case and everything that reads back a mode compares the first.
	*/
	Colours []colour.Colour
	/*
		Speed is in the device's own units, within the range the mode gives.

		Not scaled, and not guessed at: a range is min to max in numbers the
		mode itself supplies, and a "medium" that means 127 on a keyboard and
		2 on a fan controller would be hotaru inventing a unit. Out of range is
		clamped to the mode's own bounds.
	*/
	Speed *int
}

// Palette is every colour the style names, first to last.
func (s Style) Palette() []colour.Colour {
	if len(s.Colours) > 0 {
		return append([]colour.Colour(nil), s.Colours...)
	}
	if s.Colour != nil {
		return []colour.Colour{*s.Colour}
	}
	return nil
}

/*
Slots fills a mode's colour slots from the colours asked for.

How many slots is the mode's to say: at least least and at most most, where
most is above zero. One colour fills every slot the mode has now, have,
because a device asked for one colour wants all of them that colour rather
than one set and the rest whatever the vendor left; a stick of RAM whose
Color Pulse takes exactly two would otherwise pulse between the new colour
and an old one. Several fill one slot each, and fewer than least repeat the
last (spec 061 R3.1).

Shared with the fake, so a test of what was written fills the slots the
same way the hardware path does.
*/
func Slots(colours []colour.Colour, have, least, most int) []colour.Colour {
	if len(colours) == 0 {
		return nil
	}
	n := len(colours)
	if n == 1 {
		n = max(n, have)
	}
	n = max(n, least, 1)
	if most > 0 {
		n = min(n, most)
	}
	out := make([]colour.Colour, n)
	for i := range out {
		out[i] = colours[min(i, len(colours)-1)]
	}
	return out
}

/*
Mode flags, as OpenRGB defines them.

Not exported by the SDK, and worth having: PerLEDColour is what makes "can this
device show three fans in three colours?" a question the hardware answers. The
Python had to infer it from a mode being called "direct", which is a guess that
holds only for the vendors whose naming it was derived from.
*/
const (
	flagHasBrightness        uint32 = 1 << 4
	flagHasPerLEDColor       uint32 = 1 << 5
	flagHasModeSpecificColor uint32 = 1 << 6
	flagHasSpeed             uint32 = 1 << 0
)
