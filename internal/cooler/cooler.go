package cooler

import (
	"context"
	"fmt"
	"time"
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
func Open(ctx context.Context) (*Cooler, error) {
	candidates, err := Find()
	if err != nil {
		return nil, err
	}
	return pick(ctx, candidates, func(path string) (transport, error) { return openHID(path) })
}

/*
pick opens the candidate that answers.

Sysfs cannot tell one of a device's hidraw nodes from another, and the wrong
one is silent: a program that chose by glob order would work on the machine it
was written on and stop working when something else was plugged in. So each
candidate is asked for a status reading, with a short deadline, and the one
that replies is the cooler.

Asking is safe because the candidates are already filtered to a known vendor
and product -- nothing here writes to hardware it has not identified -- and it
is decisive because a mismatched device answers with its own prefix. A Corsair
power supply, asked this on the development machine, replied `74 96`, which is
not a status reply and is rejected as one.
*/
func pick(ctx context.Context, candidates []Device, dial func(string) (transport, error)) (*Cooler, error) {
	var last error
	for _, device := range candidates {
		t, err := dial(device.HID)
		if err != nil {
			last = fmt.Errorf("%s at %s: %w", device.Name, device.HID, err)
			continue
		}
		c := &Cooler{device: device, t: t}
		probe, cancel := context.WithTimeout(ctx, probeTimeout)
		_, err = c.Status(probe)
		cancel()
		if err == nil {
			return c, nil
		}
		last = fmt.Errorf("%s at %s did not answer: %w", device.Name, device.HID, err)
		_ = t.Close()
	}
	if last == nil {
		return nil, ErrNoCooler
	}
	return nil, last
}

/*
probeTimeout bounds asking one candidate whether it is the cooler.

The right node answers in about a millisecond. The wrong one never answers at
all, and without a bound that is a program that hangs at startup rather than
one that reports no cooler.
*/
const probeTimeout = 500 * time.Millisecond

// Device is what was found, for reporting.
func (c *Cooler) Device() Device { return c.device }

// Close releases the control channel.
func (c *Cooler) Close() error { return c.t.Close() }
