package service

import (
	"context"
	"time"

	"github.com/ushineko/hotaru/internal/cooler"
	"github.com/ushineko/hotaru/internal/dashboard"
	"github.com/ushineko/hotaru/internal/readings"
)

/*
Readings are what the machine will say about itself.

Eleven numbers from three places: the cooler answers for the coolant, the pump
and the fans, the kernel for the processor and for memory, and the graphics
card by whichever route it has. Each is taken independently and each can be absent -- a sensor
that has gone away costs its own number rather than stopping the panel,
because the screen is decorative and the rest are still true.

Here rather than in the daemon because two things ask: the dashboard draws
them, and `hotaru readings` prints them. One sampler for both, which matters
for the processor: utilisation is a rate, and two samplers would each
difference against their own last call, so the first answer to the command
would always be a dash.
*/
func (s *Service) Readings(ctx context.Context) readings.Reading {
	var r readings.Reading

	if status, _, err := s.Cooling(ctx); err == nil {
		r.Set(readings.Coolant, status.Coolant)
		r.Set(readings.PumpRPM, float64(status.PumpRPM))
		r.Set(readings.PumpDuty, float64(status.PumpDuty))
		r.Set(readings.FanRPM, float64(status.FanRPM))
		r.Set(readings.FanDuty, float64(status.FanDuty))
	}
	if t, err := cooler.CPUPackage.Temperature(); err == nil {
		r.Set(readings.CPUTemp, float64(t))
	}
	if load, ok := s.processor.Load(); ok {
		r.Set(readings.CPULoad, load)
	}
	card := cooler.Graphics(ctx)
	if card.TemperatureOK {
		r.Set(readings.GPUTemp, card.Temperature)
	}
	if card.LoadOK {
		r.Set(readings.GPULoad, card.Load)
	}
	if percent, gigabytes, ok := s.memory.Used(); ok {
		r.Set(readings.MemUsed, percent)
		r.Set(readings.MemBytes, gigabytes)
	}

	// Every reading taken is a reading remembered. See Service.history.
	s.history.Record(r, time.Now())
	return r
}

/*
Trail is where one reading has been, for the panel to draw under the number
(spec 046).

Oldest first, and NaN for a bucket the machine had nothing to put in. An
empty trail is a service that started less than a bucket ago, which draws
nothing rather than a line through one point.
*/
func (s *Service) Trail(ctx context.Context, source readings.Source) dashboard.Trail {
	/*
		A reading first, so that a client asking only for the trail still
		fills the history. Nothing else would, on a machine whose panel is
		showing somebody's photograph: the dashboard loop is the usual
		recorder and it does not run while the screen is held.
	*/
	s.Readings(ctx)
	return s.history.Trail(source)
}
