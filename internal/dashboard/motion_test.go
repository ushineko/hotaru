package dashboard

import (
	"image"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/readings"
)

// Motion and the trail interval are checked, and an unknown value is refused
// with the ones that are known (spec 081 R1.2, R4.5).
func TestAnUnknownMotionOrIntervalIsRefused(t *testing.T) {
	ok := Dashboard{Motion: Motion{Numbers: Melt, Ring: Comet}, Trail: Trail{Every: 1}}
	require.NoError(t, ok.Check())
	require.NoError(t, Dashboard{}.Check(), "the empty dashboard is a jump at five seconds")

	for _, d := range []Dashboard{
		{Motion: Motion{Numbers: "wobble"}},
		{Motion: Motion{Ring: "spin"}},
		{Trail: Trail{Every: 3}},
	} {
		err := d.Check()
		require.Error(t, err)
	}
	require.ErrorContains(t, Dashboard{Motion: Motion{Numbers: "wobble"}}.Check(), "melt, morph, roll")

	store, err := Open(filepath.Join(t.TempDir(), "dashboards.yml"))
	require.NoError(t, err)
	require.Error(t, store.Save(Dashboard{Name: "bad", Trail: Trail{Every: 7}}))
	_, err = store.Get("bad")
	require.Error(t, err, "a refused dashboard was stored")
}

// The shipped dashboards jump, at five seconds, as before spec 081.
func TestTheShippedDashboardsDoNotMove(t *testing.T) {
	for _, d := range Shipped() {
		require.Equal(t, Motion{}, d.Motion, d.Name)
		require.Equal(t, readings.DefaultEvery, d.Trail.Interval(), d.Name)
	}
	require.Equal(t, time.Second, Trail{Every: 1}.Interval())
}

// A trail at a shorter interval has more points over the same band: 300
// one-second points reach as far left and right as 60 five-second ones.
func TestAFinerTrailSpansTheSameBand(t *testing.T) {
	d := stacked(readings.CPULoad)
	r := reading()
	r.Set(readings.CPULoad, 50)

	extent := func(tr Trails) (lo, hi int) {
		img := Render(d, r, 0, nil, tr).Image
		top, height := bandOf(d.Arrangement)
		base := Render(d, r, 0, nil, Trails{}).Image
		lo, hi = Size, 0
		for y := top - 3; y < top+height+3; y++ { // the floor and ceiling lines sit on the band's edges
			for x := range Size {
				if differs(img, base, x, y) {
					lo, hi = min(lo, x), max(hi, x)
				}
			}
		}
		return lo, hi
	}
	ramp := func(n int) Series {
		s := make(Series, n)
		for i := range s {
			s[i] = float64(i % 7)
		}
		return s
	}
	lo60, hi60 := extent(Trails{Below: ramp(60)})
	lo300, hi300 := extent(Trails{Below: ramp(300), Points: 300})
	require.InDelta(t, lo60, lo300, 2)
	require.InDelta(t, hi60, hi300, 2)
}

func differs(a, b *image.RGBA, x, y int) bool {
	i := a.PixOffset(x, y)
	return a.Pix[i] != b.Pix[i] || a.Pix[i+1] != b.Pix[i+1] || a.Pix[i+2] != b.Pix[i+2]
}
