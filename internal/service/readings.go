package service

import (
	"context"
	"sync"
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
	/*
		A reading under half a second old is shared rather than taken
		again. The dashboard loop asks for one and then for the trail, which
		asks again, and every reading runs nvidia-smi on a machine with an
		NVIDIA card: about 15 ms of a core each, half the service's own work
		(spec 081's profile).
	*/
	s.recent.mu.Lock()
	defer s.recent.mu.Unlock()
	if !s.recent.at.IsZero() && time.Since(s.recent.at) < readingFresh {
		return copyReading(s.recent.r)
	}
	r := s.read(ctx)
	s.recent.r, s.recent.at = copyReading(r), time.Now()
	return r
}

// readingFresh is how long a reading is shared with a second asker.
const readingFresh = 500 * time.Millisecond

// recentReading is the last reading taken, under its own lock.
type recentReading struct {
	mu sync.Mutex
	r  readings.Reading
	at time.Time
}

// copyReading is a reading's own copy: a Reading holds a map.
func copyReading(r readings.Reading) readings.Reading {
	var out readings.Reading
	for _, s := range readings.All {
		if v, ok := r.Value(s); ok {
			out.Set(s, v)
		}
	}
	return out
}

// read takes a reading from every sensor and remembers it.
func (s *Service) read(ctx context.Context) readings.Reading {
	var r readings.Reading

	if status, _, err := s.Cooling(ctx); err == nil {
		r.Set(readings.Coolant, status.Coolant)
		r.Set(readings.PumpRPM, float64(status.PumpRPM))
		r.Set(readings.PumpDuty, float64(status.PumpDuty))
		r.Set(readings.FanRPM, float64(status.FanRPM))
		r.Set(readings.FanDuty, float64(status.FanDuty))
	}
	if t, err := cooler.Processor(); err == nil {
		r.Set(readings.CPUTemp, t)
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
Trails is where a dashboard's readings have been, for the band under its
numbers (spec 046).

Which readings is the dashboard's own decision, so it is asked rather than
told. Oldest first, and NaN for a bucket the machine had nothing to put in.
An empty series is a service that started less than a bucket ago, which draws
nothing rather than a line through one point.
*/
func (s *Service) Trails(ctx context.Context, d dashboard.Dashboard) dashboard.Trails {
	/*
		A reading first, so that a client asking only for the trail still
		fills the history. Nothing else would, on a machine whose panel is
		showing somebody's photograph: the dashboard loop is the usual
		recorder and it does not run while the screen is held.
	*/
	s.Readings(ctx)

	below, above := d.Trail.Sources(d.Headline.Source)
	every := d.Trail.Interval()
	return dashboard.Trails{
		Below:  s.history.Trail(below, every),
		Above:  s.history.Trail(above, every),
		Points: readings.PointsFor(every),
	}
}
