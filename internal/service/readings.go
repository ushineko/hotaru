package service

import (
	"context"

	"github.com/ushineko/hotaru/internal/cooler"
	"github.com/ushineko/hotaru/internal/readings"
)

/*
Readings are what the machine will say about itself.

Nine numbers from three places: the cooler answers for the coolant, the pump
and the fans, the kernel for the processor, and the graphics card by whichever
route it has. Each is taken independently and each can be absent -- a sensor
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
	return r
}
