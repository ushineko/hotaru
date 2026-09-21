package dashboard

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func reading() Reading {
	return Reading{
		Coolant: 37.5, CoolantOK: true,
		CPU: 52, CPUOK: true,
		GPU: 38, GPUOK: true,
		PumpRPM: 2608, PumpOK: true,
	}
}

func TestTheSameReadingLooksTheSame(t *testing.T) {
	/*
		The gate that stops hotaru writing to a panel that would not change.
		It matters more than it sounds: every push costs the device a settling
		period, and an idle machine's coolant and pump hold steady for hours.
	*/
	first := Render(reading(), 1)
	second := Render(reading(), 2)

	require.Equal(t, first.Content, second.Content,
		"two identical readings were treated as different pictures")
}

func TestTheTickAdvancesWithoutDefeatingTheGate(t *testing.T) {
	/*
		A screen that has stopped being written is indistinguishable from an
		idle machine, so something has to change on every accepted push. But
		an indicator inside the comparison would make every frame differ and
		hotaru would write forever.

		So the tick is drawn after the content is hashed: the frames differ,
		what they *say* does not.
	*/
	first := Render(reading(), 1)
	second := Render(reading(), 2)

	require.Equal(t, first.Content, second.Content)
	require.NotEqual(t, first.GIF, second.GIF, "the tick did not advance")
}

func TestAChangedReadingIsANewPicture(t *testing.T) {
	warmer := reading()
	warmer.Coolant = 41.0

	require.NotEqual(t, Render(reading(), 1).Content, Render(warmer, 1).Content)
}

func TestAMissingMetricDrawsAPlaceholder(t *testing.T) {
	// The panel is decorative: a reading hotaru could not take must not stop
	// it drawing, and must not be drawn as a zero.
	absent := Reading{Coolant: 37.5, CoolantOK: true}

	frame := Render(absent, 0)
	require.NotEmpty(t, frame.GIF)
	require.NotEqual(t, Render(reading(), 0).Content, frame.Content)
}

func TestNothingKnownStillRenders(t *testing.T) {
	// The worst case: the cooler answered nothing at all. It still draws,
	// because a blank screen says less than a screen full of dashes.
	frame := Render(Reading{}, 0)
	require.NotEmpty(t, frame.GIF)
}

func TestColourBandsFollowTheAlertThresholds(t *testing.T) {
	/*
		The one colour on this screen that carries meaning rather than style,
		and it has to agree with what the alerts say: a screen showing calm
		green while a notification says critical is worse than either alone.
	*/
	require.Equal(t, colOK, coolantColour(49.9, true))
	require.Equal(t, colWarn, coolantColour(50.0, true))
	require.Equal(t, colWarn, coolantColour(59.9, true))
	require.Equal(t, colCrit, coolantColour(60.0, true))
	require.Equal(t, colMuted, coolantColour(0, false), "an unknown temperature was graded as though known")
}

func TestAStoppedPumpIsNotDrawnCalmly(t *testing.T) {
	// The most alarming number this screen can show, and it used to be drawn
	// in the same colour as a healthy CPU.
	stopped := reading()
	stopped.PumpRPM = 0

	require.NotEqual(t, Render(reading(), 0).Content, Render(stopped, 0).Content)
}

func TestTheBackgroundIsBuiltOnce(t *testing.T) {
	// Re-quantising four hundred thousand pixels per frame cost 128 ms
	// against 7.8 ms for copying a quantised one.
	require.Same(t, background(), background())
}

func TestAFrameIsSmallEnoughToSettle(t *testing.T) {
	/*
		Frame size decides how often the panel can be written: 5 KB lands at
		one second, 21 KB needs two, and a frame that is too large for the
		interval simply never appears. A change to the palette or the
		background can push it over without touching the pushing logic, so
		the size is asserted here where that change would be made.
	*/
	frame := Render(reading(), 0)
	require.Less(t, len(frame.GIF), 32*1024,
		"the dashboard grew; check the floor in push.go before raising this")
	t.Logf("frame is %d bytes", len(frame.GIF))
}

func BenchmarkRender(b *testing.B) {
	// It runs for the life of the machine. Allocation that grows per frame is
	// the thing that would make that untrue.
	r := reading()
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		_ = Render(r, i)
	}
}

// decode reads a frame back as pixels, which is the only way to ask what is
// actually on the screen rather than what the drawing code intended.
func decode(t *testing.T, frame Frame) *image.Paletted {
	t.Helper()
	g, err := gif.DecodeAll(bytes.NewReader(frame.GIF))
	require.NoError(t, err)
	require.Len(t, g.Image, 1)
	return g.Image[0]
}

func pixels(img *image.Paletted, c color.Color) int {
	want := img.Palette.Index(c)
	n := 0
	for _, p := range img.Pix {
		if int(p) == want {
			n++
		}
	}
	return n
}

func TestTheLayoutIsThePythonsLayout(t *testing.T) {
	/*
		The coordinates are corrections, not preferences. The metric row's
		inset exists because the ring curves inward far enough at that height
		to have been drawn through the word "RPM", and the columns are sized
		so a four-digit pump reading does not collide with its neighbour.

		So: the three columns sit inside the inset, they do not overlap, and
		the ring does not reach them. Checked as geometry rather than as
		constants, because the constants are what somebody would change.
	*/
	width := (Size - 2*metricInset) / metricSlots
	require.Positive(t, width)
	require.LessOrEqual(t, metricInset+metricSlots*width, Size-metricInset)

	// The ring's outer edge at the metric row, against the row's left edge.
	centre, radius := float64(Size)/2, float64(Size-2*margin)/2
	dy := 446.0 - centre // the value band, the tallest thing in the row
	outer := radius + ringWidth/2
	dx := math.Sqrt(math.Max(0, outer*outer-dy*dy))
	require.Less(t, centre-dx, float64(metricInset),
		"the ring reaches into the metric columns, which is the mistake the inset was added to fix")
}

func TestAHotProcessorIsNotAnAlarm(t *testing.T) {
	/*
		A chip boosting to 100 C is a chip doing its job, and reddening it
		would cry wolf on every compile. A pump reading zero is the opposite:
		the most alarming number this screen can show, whatever else is calm.
	*/
	hot := reading()
	hot.CPU = 101
	require.Zero(t, pixels(decode(t, Render(hot, 0)), colCrit),
		"a busy processor was drawn as a fault")

	stopped := reading()
	stopped.PumpRPM = 0
	require.Positive(t, pixels(decode(t, Render(stopped, 0)), colCrit),
		"a stopped pump was drawn like a healthy one")
}
