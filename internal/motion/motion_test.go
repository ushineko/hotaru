package motion_test

import (
	"image"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/dashboard"
	"github.com/ushineko/hotaru/internal/motion"
	"github.com/ushineko/hotaru/internal/readings"
)

// coolant is the shipped dashboard that has a ring, a headline and three
// slots.
func coolant(t testing.TB) dashboard.Dashboard {
	t.Helper()
	for _, d := range dashboard.Shipped() {
		if d.Name == "coolant" {
			return d
		}
	}
	t.Fatal("no shipped coolant dashboard")
	return dashboard.Dashboard{}
}

func reading(coolant float64) readings.Reading {
	var r readings.Reading
	r.Set(readings.Coolant, coolant)
	r.Set(readings.CPUTemp, 52)
	r.Set(readings.GPUTemp, 41)
	r.Set(readings.PumpRPM, 2600)
	return r
}

// text is the dashboard's text layer for a coolant reading.
func text(coolant float64) *image.RGBA {
	return dashboard.TextLayer(coolantDashboard, reading(coolant), false)
}

var coolantDashboard = func() dashboard.Dashboard {
	for _, d := range dashboard.Shipped() {
		if d.Name == "coolant" {
			return d
		}
	}
	return dashboard.Dashboard{}
}()

/*
A change of the headline is one region around the number, and the label above
it and the slots below are outside it: only what changed moves (R2.1).
*/
func TestAChangedNumberIsOneRegionAndTheLabelsAreOutside(t *testing.T) {
	_ = coolant(t)
	before, after := text(41.2), text(48.7)
	regions := motion.Regions(before, after)
	require.NotEmpty(t, regions)
	for _, r := range regions {
		assert.Greater(t, r.Min.Y, 140, "a region reaches into the label above the number: %v", r)
		assert.Less(t, r.Max.Y, 400, "a region reaches into the slots below: %v", r)
	}
	require.Empty(t, motion.Regions(before, before), "nothing changed, and something moves")
}

// Every transition starts as the old text and ends as the new, and outside
// the changed regions it is the new text all the way through.
func TestEachTransitionGoesFromTheOldTextToTheNew(t *testing.T) {
	before, after := text(41.2), text(48.7)
	for _, kind := range []string{motion.Melt, motion.Morph, motion.Roll} {
		c := motion.Prepare(kind, before, after)
		require.Equal(t, before.Pix, c.At(0).Pix, kind)
		require.Equal(t, after.Pix, c.At(1).Pix, kind)

		mid := c.At(0.5)
		inside := func(x, y int) bool {
			for _, r := range c.Regions() {
				if image.Pt(x, y).In(r) {
					return true
				}
			}
			return false
		}
		moved := false
		for y := range dashboard.Size {
			for x := range dashboard.Size {
				o := mid.PixOffset(x, y)
				same := mid.Pix[o] == after.Pix[o] && mid.Pix[o+1] == after.Pix[o+1] &&
					mid.Pix[o+2] == after.Pix[o+2] && mid.Pix[o+3] == after.Pix[o+3]
				if !inside(x, y) {
					require.True(t, same, "%s: unchanged text moved at %d,%d", kind, x, y)
				} else if !same {
					moved = true
				}
			}
		}
		require.True(t, moved, "%s: nothing moved halfway", kind)
	}
}

// A kind this package does not know is a jump.
func TestAnUnknownTransitionJumps(t *testing.T) {
	before, after := text(41.2), text(48.7)
	require.Equal(t, after.Pix, motion.Prepare("", before, after).At(0.5).Pix)
}

// The curves start at 0 and end at 1; the spring passes 1 on the way.
func TestTheCurves(t *testing.T) {
	assert.Zero(t, motion.Ease(0))
	assert.InDelta(t, 1, motion.Ease(1), 1e-9)
	assert.Zero(t, motion.Spring(0))
	assert.InDelta(t, 1, motion.Spring(1), 1e-9)
	peak := 0.0
	for i := range 100 {
		peak = max(peak, motion.Spring(float64(i)/100))
	}
	assert.Greater(t, peak, 1.05, "the spring does not overshoot")
}

// The comet glows where the arcs just swept, and only there.
func TestTheCometGlowsWhereTheRingSwept(t *testing.T) {
	d := coolantDashboard
	before := dashboard.RingsLayer(d, reading(30))
	now := dashboard.RingsLayer(d, reading(50))
	plain := image.NewRGBA(now.Bounds())
	copy(plain.Pix, now.Pix)
	motion.Glow(now, before, 1)
	changed := 0
	for i := 0; i < len(now.Pix); i += 4 {
		if now.Pix[i] != plain.Pix[i] {
			changed++
		}
	}
	require.Positive(t, changed)
	require.Less(t, changed, dashboard.Size*dashboard.Size/4, "the glow is not confined to the sweep")

	unmoved := dashboard.RingsLayer(d, reading(30))
	again := dashboard.RingsLayer(d, reading(30))
	motion.Glow(again, unmoved, 1)
	require.Equal(t, unmoved.Pix, again.Pix, "a ring that did not move glowed")
}

/*
Preparing a change is work done on the regions alone (R2.6): at most 10 ms for
a shipped dashboard's headline changing.

	go test ./internal/motion -run - -bench Prepare
*/
func BenchmarkPrepare(b *testing.B) {
	before, after := text(41.2), text(48.7)
	for _, kind := range []string{motion.Melt, motion.Morph, motion.Roll} {
		b.Run(kind, func(b *testing.B) {
			for range b.N {
				_ = motion.Prepare(kind, before, after)
			}
		})
	}
}

// A frame of each transition, halfway: drawn up to 20 times a second.
func BenchmarkAt(b *testing.B) {
	before, after := text(41.2), text(48.7)
	for _, kind := range []string{motion.Melt, motion.Morph, motion.Roll} {
		c := motion.Prepare(kind, before, after)
		b.Run(kind, func(b *testing.B) {
			for range b.N {
				_ = c.At(0.5)
			}
		})
	}
}
