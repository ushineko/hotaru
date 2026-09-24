package dashboard

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/readings"
)

func reading() Reading {
	var r Reading
	r.Set(readings.Coolant, 37.5)
	r.Set(readings.CPUTemp, 52)
	r.Set(readings.GPUTemp, 38)
	r.Set(readings.PumpRPM, 2608)
	return r
}

func TestTheSameReadingLooksTheSame(t *testing.T) {
	/*
		The gate that stops hotaru writing to a panel that would not change.
		It matters more than it sounds: every push costs the device a settling
		period, and an idle machine's coolant and pump hold steady for hours.
	*/
	first := shown(reading(), 1)
	second := shown(reading(), 2)

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
	first := shown(reading(), 1)
	second := shown(reading(), 2)

	require.Equal(t, first.Content, second.Content)
	require.NotEqual(t, first.GIF, second.GIF, "the tick did not advance")
}

func TestAChangedReadingIsANewPicture(t *testing.T) {
	warmer := reading()
	warmer.Set(readings.Coolant, 41.0)

	require.NotEqual(t, shown(reading(), 1).Content, shown(warmer, 1).Content)
}

func TestAMissingMetricDrawsAPlaceholder(t *testing.T) {
	// The panel is decorative: a reading hotaru could not take must not stop
	// it drawing, and must not be drawn as a zero.
	var absent Reading
	absent.Set(readings.Coolant, 37.5)

	frame := shown(absent, 0)
	require.NotEmpty(t, frame.GIF)
	require.NotEqual(t, shown(reading(), 0).Content, frame.Content)
}

func TestNothingKnownStillRenders(t *testing.T) {
	// The worst case: the cooler answered nothing at all. It still draws,
	// because a blank screen says less than a screen full of dashes.
	frame := shown(Reading{}, 0)
	require.NotEmpty(t, frame.GIF)
}

func TestColourBandsFollowTheAlertThresholds(t *testing.T) {
	/*
		The one colour on this screen that carries meaning rather than style,
		and it has to agree with what the alerts say: a screen showing calm
		green while a notification says critical is worse than either alone.
	*/
	require.Equal(t, colOK, coolantColour(49.9, true, ThemeOf("")))
	require.Equal(t, colWarn, coolantColour(50.0, true, ThemeOf("")))
	require.Equal(t, colWarn, coolantColour(59.9, true, ThemeOf("")))
	require.Equal(t, colCrit, coolantColour(60.0, true, ThemeOf("")))
	require.Equal(t, ThemeOf("").Muted, coolantColour(0, false, ThemeOf("")),
		"an unknown temperature was graded as though known")
}

func TestAStoppedPumpIsNotDrawnCalmly(t *testing.T) {
	// The most alarming number this screen can show, and it used to be drawn
	// in the same colour as a healthy CPU.
	stopped := reading()
	stopped.Set(readings.PumpRPM, 0)

	require.NotEqual(t, shown(reading(), 0).Content, shown(stopped, 0).Content)
}

func TestTheBackgroundIsBuiltOnce(t *testing.T) {
	// Re-quantising four hundred thousand pixels per frame cost 128 ms
	// against 7.8 ms for copying a quantised one.
	require.Same(t, background(ThemeOf("")), background(ThemeOf("")))
}

func TestAFrameIsSmallEnoughToSettle(t *testing.T) {
	/*
		Frame size decides how often the panel can be written: 5 KB lands at
		one second, 21 KB needs two, and a frame that is too large for the
		interval simply never appears. A change to the palette or the
		background can push it over without touching the pushing logic, so
		the size is asserted here where that change would be made.
	*/
	frame := shown(reading(), 0)
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
		_ = shown(r, i)
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
	hot.Set(readings.CPUTemp, 101)
	require.Zero(t, pixels(decode(t, shown(hot, 0)), colCrit),
		"a busy processor was drawn as a fault")

	stopped := reading()
	stopped.Set(readings.PumpRPM, 0)
	require.Positive(t, pixels(decode(t, shown(stopped, 0)), colCrit),
		"a stopped pump was drawn like a healthy one")
}

// draw is the shipped dashboard, which is spec 013's screen. Most of these
// tests are about that screen rather than about choosing another one.
func shown(r Reading, tick int) Frame { return Render(Shipped()[0], r, tick, nil, Trails{}) }

/*
The shipped dashboard does not move by accident.

It held spec 013's screen to the pixel for twenty-odd releases, on spec 023's
promise that a machine which upgrades and touches nothing sees exactly what it
saw. **Spec 041 broke that promise on purpose.** Taking the unit line out
changes the panel, there is no version of that change which leaves the default
screen alone, and keeping the line alive for the shipped dashboards would have
kept the promise by keeping the redundancy in the four screens most people are
looking at.

So the golden was regenerated, once, by this renderer, and the test means what
it meant before: the default screen is what somebody decided it should be, and
it does not move again without somebody deciding that too. A failure here is
either a change worth a line in the changelog or a change nobody intended, and
both are worth stopping for.

Colours, not palette indices. Making the rings a list added a colour to the
palette for the outline, which moved every index after it without moving a
single pixel: comparing `Content` would fail on a screen that is identical,
which is a test failing for the tidiness of a byte array rather than for
anything anybody can see.
*/
func TestTheShippedDashboardDoesNotMoveByAccident(t *testing.T) {
	want := decodeFile(t, filepath.Join("testdata", "shipped-coolant.gif"))
	got := decode(t, shown(reading(), 0))

	require.Equal(t, want.Bounds(), got.Bounds())
	for y := range want.Bounds().Dy() {
		for x := range want.Bounds().Dx() {
			if want.At(x, y) != got.At(x, y) {
				require.Failf(t, "the default screen moved",
					"pixel %d,%d was %v and is %v; if that was meant, regenerate "+
						"testdata/shipped-coolant.gif and say so in the changelog",
					x, y, want.At(x, y), got.At(x, y))
			}
		}
	}
}

func decodeFile(t *testing.T, path string) image.Image {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	decoded, err := gif.Decode(bytes.NewReader(body))
	require.NoError(t, err)
	return decoded
}

/*
Two renders at once do not corrupt the font.

The preview route renders per request while the push loop renders for the
life of the service, and an opentype.Face is not safe for concurrent use:
two goroutines shaping text through one panicked inside sfnt with an index
out of range. The HTTP server recovered from that and answered EOF; the push
loop had nothing to recover it, and the service exited.

Run with -race this also catches the maps.
*/
func TestTwoRendersAtOnceDoNotCorruptTheFont(t *testing.T) {
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 4 {
				frame := Render(Shipped()[i%len(Shipped())], reading(), i, nil, Trails{})
				require.NotEmpty(t, frame.GIF)
			}
		}()
	}
	wg.Wait()
}
