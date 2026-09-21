package cooler

import (
	"context"
	"fmt"
	"os"
	"time"
)

// reportLen is every report this device sends or takes, in either direction.
const reportLen = 64

/*
attempts bounds a search for a reply.

The cooler streams status reports of its own accord, so a reply arrives among
them rather than instead of them. Twelve is liquidctl's number and is generous:
the reply is usually the first or second report to arrive.
*/
const attempts = 12

/*
defaultWait bounds a read whose caller set no deadline.

Generous: a reading arrives in about two milliseconds, and this is the limit
before a device that has stopped talking is reported as such rather than
waited on.
*/
const defaultWait = 2 * time.Second

/*
hid is the control channel: a character device, and the rules for talking on it.

The rules are the point. A command's reply carries the command's prefix with
the first byte incremented -- 0x32 0x01 is answered by 0x33 0x01 -- and this
device also streams status reports nobody asked for. Reading "the next report"
after a command returns a temperature reading about a third of the time, and
byte 14 of a temperature reading interpreted as a result code is noise that
looks like data.

That mistake cost an evening (spec 012), so matching is not something a caller
can forget to do: every exchange goes through ask, and there is no method that
simply reads.
*/
type hid struct {
	f *os.File
}

func openHID(path string) (*hid, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0) //nolint:gosec // a path from discovery
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return &hid{f: f}, nil
}

func (h *hid) Close() error {
	if err := h.f.Close(); err != nil {
		return fmt.Errorf("close the cooler: %w", err)
	}
	return nil
}

// tell sends a command and does not wait for anything.
func (h *hid) tell(data ...byte) error {
	report := make([]byte, reportLen)
	copy(report, data)
	if _, err := h.f.Write(report); err != nil {
		return fmt.Errorf("write report %#02x: %w", data[0], err)
	}
	return nil
}

/*
ask sends a command and returns its reply, matched by prefix.

A reply that never arrives is an error rather than a zero value: the caller is
about to read a result code out of it, and a report from somewhere else would
answer a question nobody asked.
*/
func (h *hid) ask(ctx context.Context, data ...byte) ([]byte, error) {
	if err := h.tell(data...); err != nil {
		return nil, err
	}
	return h.await(ctx, data[0]+1, data[1])
}

/*
await returns the next report with this prefix.

The deadline is the caller's, pushed down to the file. Checking the context
between reads is not enough on its own: the wrong hidraw node of a device never
answers, and a read that blocks forever is a service that hangs at startup
rather than one that reports no cooler.
*/
func (h *hid) await(ctx context.Context, a, b byte) ([]byte, error) {
	when, ok := ctx.Deadline()
	if !ok {
		when = time.Now().Add(defaultWait)
	}
	if err := h.f.SetReadDeadline(when); err != nil {
		return nil, fmt.Errorf("bound the wait on %s: %w", h.f.Name(), err)
	}

	buf := make([]byte, reportLen)
	for range attempts {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("gave up waiting for the cooler: %w", err)
		}
		n, err := h.f.Read(buf)
		if err != nil {
			return nil, fmt.Errorf("read report: %w", err)
		}
		if n >= reportLen && buf[0] == a && buf[1] == b {
			out := make([]byte, reportLen)
			copy(out, buf)
			return out, nil
		}
	}
	return nil, fmt.Errorf("no %02x%02x reply in %d reports", a, b, attempts)
}
