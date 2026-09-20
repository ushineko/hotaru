package cooler

import (
	"context"
	"fmt"
	"sync"
)

/*
Fake is a cooler that exists only in memory.

Not a stub: it streams status reports nobody asked for, the way the real device
does. Eleven of twelve reports arriving after a request on the development
machine were broadcasts rather than the reply, and a fake that answered
politely would let the bug this package was written around straight back in.

It can also misbehave on purpose. Silent makes it never answer, which is a
device that has stopped talking; Faulty makes it report the temperature a
firmware fault produces.
*/
type Fake struct {
	mu sync.Mutex

	Coolant  float64
	PumpRPM  int
	PumpDuty int
	FanRPM   int
	FanDuty  int

	// Chatter is how many unsolicited reports arrive before a reply does.
	Chatter int
	// Silent makes every exchange find nothing, as a wedged device does.
	Silent bool
	// Faulty reports 0xFFFF where a temperature belongs -- liquidctl#172.
	Faulty bool

	// Told is every command sent, in order, for a test to assert against.
	Told [][]byte

	pending []byte
	closed  bool
}

// NewFake is a cooler reporting plausible numbers.
func NewFake() *Fake {
	return &Fake{Coolant: 37.5, PumpRPM: 2608, PumpDuty: 81, FanRPM: 1190, FanDuty: 51, Chatter: 3}
}

func (f *Fake) tell(data ...byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return fmt.Errorf("closed")
	}
	f.Told = append(f.Told, append([]byte(nil), data...))
	f.pending = append([]byte(nil), data...)
	return nil
}

func (f *Fake) ask(ctx context.Context, data ...byte) ([]byte, error) {
	if err := f.tell(data...); err != nil {
		return nil, err
	}
	return f.await(ctx, data[0]+1, data[1])
}

/*
await produces the broadcasts first, then the reply.

The order is the point. A caller that matches only the first byte of a report
gets a status broadcast here, exactly as it would from the hardware, and its
test fails where the machine would merely have misled it.
*/
func (f *Fake) await(ctx context.Context, a, b byte) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for range f.Chatter {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("gave up waiting for the cooler: %w", err)
		}
		_ = f.broadcast() // read and discarded, as a mismatched report is
	}
	if f.Silent {
		return nil, fmt.Errorf("no %02x%02x reply in %d reports", a, b, attempts)
	}

	reply := make([]byte, reportLen)
	reply[0], reply[1] = a, b
	if a == 0x75 {
		f.fill(reply)
	} else {
		reply[14] = 0x01 // the result byte every other command answers with
	}
	return reply, nil
}

// broadcast is the 0x75 0x02 report the device streams unasked.
func (f *Fake) broadcast() []byte {
	report := make([]byte, reportLen)
	report[0], report[1] = 0x75, 0x02
	f.fill(report)
	return report
}

func (f *Fake) fill(report []byte) {
	if f.Faulty {
		report[15], report[16] = 0xFF, 0xFF
		return
	}
	whole := int(f.Coolant)
	report[15] = low(whole)
	report[16] = low(int(f.Coolant*10) - whole*10)
	report[17], report[18] = low(f.PumpRPM), low(f.PumpRPM>>8)
	report[19] = low(f.PumpDuty)
	report[23], report[24] = low(f.FanRPM), low(f.FanRPM>>8)
	report[25] = low(f.FanDuty)
}

// low is the bottom byte of a value, which is what a report field holds.
func low(v int) byte { return byte(v & 0xFF) }

// Close marks the fake closed; using it afterwards is a test's own bug.
func (f *Fake) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

// NewWithFake is a Cooler backed by a fake, for tests and for developing on a
// machine with no cooler in it.
func NewWithFake(f *Fake) *Cooler {
	return &Cooler{device: Device{Name: "fake cooler", Product: 0x3012}, t: f}
}
