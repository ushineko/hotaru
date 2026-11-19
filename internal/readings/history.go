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

Every source rather than the one on the panel. It is three hundred floats per
source, somebody can change the active dashboard at any moment, and a history
that started when they changed it would say nothing for the length of its
window.
*/
type History struct {
	mu sync.Mutex

	// open is the start of the bucket still filling. Zero until the first
	// reading arrives.
	open time.Time
	// sum and n accumulate the open bucket, per source.
	sum map[Source]float64
	n   map[Source]int
	// points are the closed buckets, oldest first and at most Buckets long.
	points map[Source][]float64
}

/*
The shape of the window the panel draws.

Five minutes. Long enough to show a build starting and finishing, short enough
that somebody who has just done something sees it.

**Kept by the second, drawn by the dashboard's interval** (spec 081). The
history holds one bucket a second, the dashboard loop's own pace since spec
080, and a dashboard's trail averages them into its interval: 1, 2 or 5
seconds, 300, 150 or 60 points across the same five minutes. Five seconds is
the default and what every trail was before: each point the mean of five
readings, which is what makes a processor's spiky temperature plottable.
*/
const (
	Bucket       = time.Second
	Window       = 5 * time.Minute
	Buckets      = int(Window / Bucket)
	DefaultEvery = 5 * time.Second
	// Points is how many a trail at the default interval has.
	Points = int(Window / DefaultEvery)
)

// Intervals are the trail intervals a dashboard may choose.
var Intervals = []time.Duration{time.Second, 2 * time.Second, 5 * time.Second}

// PointsFor is how many points a trail at an interval has across the window.
func PointsFor(every time.Duration) int {
	if every < Bucket {
		every = Bucket
	}
	return int(Window / every)
}

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
			if len(h.points[source]) > Buckets {
				h.points[source] = h.points[source][len(h.points[source])-Buckets:]
			}
		}
		/*
			Past the window every further empty bucket says the same thing,
			and a machine resumed after a week should not walk a million of
			them. Jumping to a whole window before the next bucket leaves
			exactly Buckets gaps to append, which is a trail of nothing --
			which is what a week of silence is.
		*/
		if at.Add(Bucket * time.Duration(Buckets)).Before(next) {
			at = next.Add(-Bucket * time.Duration(Buckets+1))
		}
	}
	h.open = next
	clear(h.sum)
	clear(h.n)
}

/*
Trail is what one source has been doing, oldest first, one point per `every`.

Each point is the mean of the buckets in its interval, which is aligned to the
clock as the buckets are, so a point keeps its value as time passes. A bucket
nothing landed in counts for nothing, and an interval with nothing in it is
NaN: a gap.

The interval still filling is left out, and so is one the window has cut into:
a point that is still filling moves under whoever is looking at it, and the
trace would wobble at its own right-hand end for no reason anybody could see.
*/
func (h *History) Trail(s Source, every time.Duration) []float64 {
	if h == nil {
		return nil
	}
	if every < Bucket {
		every = Bucket
	}
	per := int(every / Bucket)

	h.mu.Lock()
	defer h.mu.Unlock()

	buckets := h.points[s]
	if len(buckets) == 0 {
		return nil
	}
	first := h.open.Add(-Bucket * time.Duration(len(buckets)))
	var out []float64
	var sum float64
	var n, members int
	group := first.Truncate(every)
	flush := func() {
		if members == per {
			if n == 0 {
				out = append(out, math.NaN())
			} else {
				out = append(out, sum/float64(n))
			}
		}
		sum, n, members = 0, 0, 0
	}
	for i, v := range buckets {
		start := first.Add(Bucket * time.Duration(i))
		if g := start.Truncate(every); !g.Equal(group) {
			flush()
			group = g
		}
		members++
		if !math.IsNaN(v) {
			sum += v
			n++
		}
	}
	flush()
	return out
}
