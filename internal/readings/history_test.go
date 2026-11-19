package readings_test

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/readings"
)

// at is a time inside the nth bucket since a fixed start.
func at(n int) time.Time {
	start := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	return start.Add(time.Duration(n) * readings.Bucket)
}

// took is a reading of one source.
func took(source readings.Source, v float64) readings.Reading {
	var r readings.Reading
	r.Set(source, v)
	return r
}

func TestABucketIsTheMeanOfWhatArrivedInIt(t *testing.T) {
	/*
		Why a mean and not a sample: a processor's temperature moved 51, 59,
		51, 69, 82, 60 over eighteen seconds on an idle desk. One
		instantaneous reading every five seconds is a coin toss drawn as a
		line.
	*/
	h := readings.NewHistory()
	h.Record(took(readings.CPUTemp, 50), at(0))
	h.Record(took(readings.CPUTemp, 80), at(0).Add(300*time.Millisecond))
	h.Record(took(readings.CPUTemp, 62), at(0).Add(600*time.Millisecond))

	// The open bucket is not handed out: a point still filling would move
	// under whoever is looking at it.
	require.Empty(t, h.Trail(readings.CPUTemp, readings.Bucket))

	h.Record(took(readings.CPUTemp, 40), at(1))
	require.Equal(t, []float64{64}, h.Trail(readings.CPUTemp, readings.Bucket))
}

func TestABucketNothingArrivedInIsAGap(t *testing.T) {
	// Not a zero. A pump drawn at 0 RPM is the most alarming number this
	// machine can show, and it would rest on no evidence.
	h := readings.NewHistory()
	h.Record(took(readings.PumpRPM, 2600), at(0))
	h.Record(took(readings.PumpRPM, 2700), at(3))

	trail := h.Trail(readings.PumpRPM, readings.Bucket)
	require.Len(t, trail, 3)
	require.Equal(t, 2600.0, trail[0])
	require.True(t, math.IsNaN(trail[1]), "a bucket nothing arrived in is not a gap")
	require.True(t, math.IsNaN(trail[2]))
}

func TestTheWindowKeepsTheLatestAndDropsTheRest(t *testing.T) {
	h := readings.NewHistory()
	for i := range readings.Buckets + 10 {
		h.Record(took(readings.CPULoad, float64(i)), at(i))
	}
	// One more, to close the last bucket that carries a number.
	h.Record(took(readings.CPULoad, -1), at(readings.Buckets+10))

	trail := h.Trail(readings.CPULoad, readings.Bucket)
	require.Len(t, trail, readings.Buckets, "the window grew")
	require.Equal(t, float64(readings.Buckets+9), trail[len(trail)-1],
		"the newest point is not last")
	require.Equal(t, float64(10), trail[0], "the oldest points were not dropped")
}

func TestALongSilenceDoesNotWalkEveryBucket(t *testing.T) {
	/*
		A machine resumed after a week is a machine with a million empty
		buckets behind it, and every one past the window says the same thing.
		The trail is gaps, and it arrives without counting to a million.
	*/
	h := readings.NewHistory()
	h.Record(took(readings.CPULoad, 20), at(0))

	done := make(chan []float64, 1)
	go func() {
		h.Record(took(readings.CPULoad, 30), at(0).Add(7*24*time.Hour))
		done <- h.Trail(readings.CPULoad, readings.Bucket)
	}()

	select {
	case trail := <-done:
		require.Len(t, trail, readings.Buckets)
		for _, v := range trail {
			require.True(t, math.IsNaN(v), "a week of silence drew numbers")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("recording after a week did not finish")
	}
}

func TestAReadingFromBeforeTheOpenBucketIsDropped(t *testing.T) {
	// It belongs to a point already drawn, and putting it in this one would
	// be worse than losing it.
	h := readings.NewHistory()
	h.Record(took(readings.CPULoad, 10), at(5))
	h.Record(took(readings.CPULoad, 90), at(2))
	h.Record(took(readings.CPULoad, 10), at(6))

	require.Equal(t, []float64{10}, h.Trail(readings.CPULoad, readings.Bucket))
}

func TestANilHistoryIsSafeToUse(t *testing.T) {
	// A service built without one, and a client asking anyway.
	var h *readings.History
	h.Record(took(readings.CPULoad, 10), at(0))
	require.Nil(t, h.Trail(readings.CPULoad, readings.Bucket))
}

/*
A trail at an interval is the mean of each interval's buckets, aligned to the
clock (spec 081 R4.5): 300, 150 and 60 points over the same five minutes, the
interval still filling left out.
*/
func TestATrailAtAnIntervalAveragesItsBuckets(t *testing.T) {
	h := readings.NewHistory()
	for i := range readings.Buckets + 1 {
		h.Record(took(readings.CPULoad, float64(i%10)), at(i))
	}
	for every, points := range map[time.Duration]int{time.Second: 300, 2 * time.Second: 150, 5 * time.Second: 60} {
		trail := h.Trail(readings.CPULoad, every)
		require.Len(t, trail, points, "%s", every)
		require.Equal(t, points, readings.PointsFor(every))
	}
	// Seconds 0..4 hold 0..4, so the first five-second point is their mean.
	require.InDelta(t, 2.0, h.Trail(readings.CPULoad, 5*time.Second)[0], 1e-9)
	require.InDelta(t, 0.5, h.Trail(readings.CPULoad, 2*time.Second)[0], 1e-9)
}

// An interval still filling is not a point, and a second nothing arrived in
// counts for nothing: the mean is of what arrived, and an interval of only
// gaps is a gap.
func TestAnIntervalIsTheMeanOfWhatArrived(t *testing.T) {
	h := readings.NewHistory()
	h.Record(took(readings.PumpRPM, 2600), at(0))
	h.Record(took(readings.PumpRPM, 2700), at(2))
	h.Record(took(readings.PumpRPM, 1), at(12))

	trail := h.Trail(readings.PumpRPM, 5*time.Second)
	require.Len(t, trail, 2, "seconds 10 and 11 are a filling interval")
	require.Equal(t, 2650.0, trail[0])
	require.True(t, math.IsNaN(trail[1]))
}

// The default interval is the one every trail had before spec 081.
func TestTheDefaultIntervalIsFiveSeconds(t *testing.T) {
	require.Equal(t, 5*time.Second, readings.DefaultEvery)
	require.Equal(t, 60, readings.Points)
	require.Equal(t, 5*time.Minute, readings.Window)
}
