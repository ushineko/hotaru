package readings

import (
	"math"
	"sync"
	"time"
)

/*
History is where each reading has been, kept so the panel can draw it.

**Buckets rather than samples.** A processor's temperature moved 51, 59, 51,
69, 82, 60 over eighteen seconds on an idle desk, so one instantaneous sample
every few seconds is a coin toss drawn as a line. A bucket is the mean of
whatever arrived inside it, which is the number somebody means when they look
at a trace and say the machine was busy.

**Whoever reads, records.** The service records every reading it takes, from
the dashboard loop and from the API alike, so the history costs no sensor
traffic of its own. A bucket that nothing landed in is a gap, not a zero, for
the reason `known` exists: a pump drawn at 0 RPM is the most alarming number
this machine can show, and it would rest on no evidence.

Every source rather than the one on the panel. It is sixty floats per source,
somebody can change the active dashboard at any moment, and a history that
started when they changed it would say nothing for the length of its window.
*/
type History struct {
	mu sync.Mutex

	// open is the start of the bucket still filling. Zero until the first
	// reading arrives.
	open time.Time
	// sum and n accumulate the open bucket, per source.
	sum map[Source]float64
	n   map[Source]int
	// points are the closed buckets, oldest first and at most Points long.
	points map[Source][]float64
}

/*
The shape of the window the panel draws.

Five minutes across sixty points. Long enough to show a build starting and
finishing, short enough that somebody who has just done something sees it.
The bucket is five seconds because the dashboard loop reads every one to three
seconds, so each point is the mean of two or three readings rather than one.
*/
const (
	Bucket = 5 * time.Second
	Points = 60
	Window = Bucket * Points
)

// NewHistory prepares an empty history.
func NewHistory() *History {
	return &History{
		sum:    map[Source]float64{},
		n:      map[Source]int{},
		points: map[Source][]float64{},
	}
}

/*
Record adds one reading to the bucket its time falls in.

Buckets are aligned to the clock rather than to the first reading, so two
services and two windows agree about which five seconds a point covers.
*/
func (h *History) Record(r Reading, at time.Time) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	bucket := at.Truncate(Bucket)
	switch {
	case h.open.IsZero():
		h.open = bucket
	case bucket.After(h.open):
		h.close(bucket)
	case bucket.Before(h.open):
		// A reading from before the open bucket. It belongs to a point that
		// has already been drawn, and putting it in this one would be worse
		// than dropping it.
		return
	}

	for _, source := range All {
		if v, ok := r.Value(source); ok {
			h.sum[source] += v
			h.n[source]++
		}
	}
}

/*
close settles the open bucket and every empty one up to the next.

The gaps matter. A machine that was asleep, or a panel that was showing
somebody's photograph while nothing asked for a reading, leaves buckets that
nothing landed in, and a trace drawn as though those minutes did not happen
would compress an hour into a line about five minutes.
*/
func (h *History) close(next time.Time) {
	for at := h.open; at.Before(next); at = at.Add(Bucket) {
		for _, source := range All {
			point := math.NaN()
			if n := h.n[source]; n > 0 && at.Equal(h.open) {
				point = h.sum[source] / float64(n)
			}
			h.points[source] = append(h.points[source], point)
			if len(h.points[source]) > Points {
				h.points[source] = h.points[source][len(h.points[source])-Points:]
			}
		}
		/*
			Past the window every further empty bucket says the same thing,
			and a machine resumed after a week should not walk a million of
			them. Jumping to a whole window before the next bucket leaves
			exactly Points gaps to append, which is a trail of nothing --
			which is what a week of silence is.
		*/
		if at.Add(Bucket * Points).Before(next) {
			at = next.Add(-Bucket * (Points + 1))
		}
	}
	h.open = next
	clear(h.sum)
	clear(h.n)
}

/*
Trail is what one source has been doing, oldest first.

The open bucket is left out: a point that is still filling moves under
whoever is looking at it, and the trace would wobble at its own right-hand
end for no reason anybody could see.

NaN is a bucket the machine had nothing to put in.
*/
func (h *History) Trail(s Source) []float64 {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	if len(h.points[s]) == 0 {
		return nil
	}
	out := make([]float64, len(h.points[s]))
	copy(out, h.points[s])
	return out
}
