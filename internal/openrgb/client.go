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

	// SetMode puts a device into a mode, optionally asserting a brightness on
	// a mode that supports one.
	SetMode(ctx context.Context, device, mode string, brightness *int) error

	// SetFrame writes one colour per LED. The whole device, always: a frame is
	// the unit precisely so a write cannot land half-applied.
	SetFrame(ctx context.Context, device string, frame devices.Frame) error

	// ProtocolVersion is the version client and server agreed on, which health
	// reports so a mismatch is visible rather than mysterious.
	ProtocolVersion() uint32

	Close() error
}

/*
Mode flags, as OpenRGB defines them.

Not exported by the SDK, and worth having: PerLEDColour is what makes "can this
device show three fans in three colours?" a question the hardware answers. The
Python had to infer it from a mode being called "direct", which is a guess that
holds only for the vendors whose naming it was derived from.
*/
const (
	flagHasBrightness  uint32 = 1 << 4
	flagHasPerLEDColor uint32 = 1 << 5
)
