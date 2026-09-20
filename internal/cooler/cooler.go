package cooler

import (
	"context"
	"fmt"
)

/*
transport is the control channel, so tests need no hardware.

Only the matched exchanges are on it. A method that simply read the next report
would be the shape of the bug spec 012 records, and leaving it out of the
interface is cheaper than remembering not to call it.
*/
type transport interface {
	tell(data ...byte) error
	ask(ctx context.Context, data ...byte) ([]byte, error)
	await(ctx context.Context, a, b byte) ([]byte, error)
	Close() error
}

/*
Cooler is one liquid cooler, open.

One owner. The service holds it in a single goroutine with a single-slot
mailbox, the way lighting does, so nothing here locks: two callers writing to
one HID endpoint is a corruption risk and not a contention problem.
*/
type Cooler struct {
	device Device
	t      transport
}

// Open finds a supported cooler and opens its control channel.
func Open() (*Cooler, error) {
	device, err := Find()
	if err != nil {
		return nil, err
	}
	t, err := openHID(device.HID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", device.Name, err)
	}
	return &Cooler{device: device, t: t}, nil
}

// Device is what was found, for reporting.
func (c *Cooler) Device() Device { return c.device }

// Close releases the control channel.
func (c *Cooler) Close() error { return c.t.Close() }
