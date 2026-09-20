package cooler

import (
	"context"
	"fmt"
	"time"
)

/*
Status is what the cooler reports about itself.

Raw measurements and when they were taken, rather than what any particular view
of hotaru needs: the pump-failure alert moves here later, and a future Go
rewrite of peripheral-battery-monitor reads the same snapshot. See spec 012.
*/
type Status struct {
	Coolant  float64 // degrees C
	PumpRPM  int
	PumpDuty int // percent
	FanRPM   int
	FanDuty  int // percent
	Taken    time.Time
}

/*
The status exchange.

Asking is `0x74 0x01` and the answer is `0x75 0x01`. The device also streams
`0x75 0x02` reports unasked -- eleven of them arrived in the twelve reads after
one request, on the development machine -- so a reader that matches only the
first byte parses a broadcast and calls it a reply. It carries status too, so
it even looks right.
*/
const (
	askStatus   = 0x74
	askStatusB  = 0x01
	statusFault = 0xFF // both temperature bytes, on a firmware fault
)

/*
Status reads the cooler.

Offsets are into the report as hidraw delivers it, report number included,
matching liquidctl's own indices so the two can be compared directly -- which
is how this was checked.
*/
func (c *Cooler) Status(ctx context.Context) (Status, error) {
	reply, err := c.t.ask(ctx, askStatus, askStatusB)
	if err != nil {
		return Status{}, err
	}
	if reply[15] == statusFault && reply[16] == statusFault {
		// liquidctl#172: a firmware fault reports 0xFFFF rather than a
		// temperature. Reported as such: a cooler claiming 255.5 degrees
		// would raise an alarm about the wrong thing.
		return Status{}, fmt.Errorf("the cooler reported no temperature, which is a firmware fault")
	}
	return Status{
		Coolant:  float64(reply[15]) + float64(reply[16])/10,
		PumpRPM:  int(reply[18])<<8 | int(reply[17]),
		PumpDuty: int(reply[19]),
		FanRPM:   int(reply[24])<<8 | int(reply[23]),
		FanDuty:  int(reply[25]),
		Taken:    time.Now(),
	}, nil
}
