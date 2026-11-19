/*
proto-anim is a THROWAWAY prototype for spec 081, to see on the panel how a
tweened dashboard and an animated background look before they are built.
It drives the Kraken directly, so hotaru must be stopped while it runs.

	proto-anim -dashboard coolant -seconds 30            tween on made-up readings
	proto-anim -dashboard ice1 -picture rotatingearth    the picture plays underneath
	proto-anim -dashboard coolant -smooth=false          jumps, for comparison
*/
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"image/png"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"time"

	"github.com/ushineko/sanshoku"
	"github.com/ushineko/sanshoku/nzxt"
	"github.com/ushineko/sanshoku/screen"

	"github.com/ushineko/hotaru/internal/dashboard"
	"github.com/ushineko/hotaru/internal/images"
	"github.com/ushineko/hotaru/internal/readings"
)

// ranges are plausible values for made-up readings.
var ranges = map[readings.Source][2]float64{
	readings.Coolant: {30, 46}, readings.CPUTemp: {38, 88}, readings.CPULoad: {2, 95},
	readings.GPUTemp: {34, 78}, readings.GPULoad: {0, 99}, readings.PumpRPM: {2350, 2750},
	readings.PumpDuty: {60, 90}, readings.FanRPM: {700, 1600}, readings.FanDuty: {25, 70},
	readings.MemUsed: {40, 80}, readings.MemBytes: {50, 100},
}

func made(rng *rand.Rand) readings.Reading {
	var r readings.Reading
	for s, lim := range ranges {
		v := lim[0] + rng.Float64()*(lim[1]-lim[0])
		if s == readings.PumpRPM || s == readings.FanRPM {
			v = math.Round(v)
		} else {
			v = math.Round(v*10) / 10
		}
		r.Set(s, v)
	}
	return r
}

// between is a reading t of the way from a to b, linearly; t may pass 1 for
// an overshoot.
func between(a, b readings.Reading, t float64) readings.Reading {
	var r readings.Reading
	for _, s := range readings.All {
		bv, ok := b.Value(s)
		if !ok {
			continue
		}
		if av, ok := a.Value(s); ok {
			bv = av + (bv-av)*t
		}
		r.Set(s, bv)
	}
	return r
}

// loop is an animated picture's frames, full size, with their delays.
type loop struct {
	frames []image.Image
	delays []time.Duration
}

func loadLoop(lib *images.Library, name string) (*loop, error) {
	body, err := lib.Read(name)
	if err != nil {
		return nil, err
	}
	g, err := gif.DecodeAll(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	out := &loop{}
	canvas := image.NewRGBA(image.Rect(0, 0, g.Config.Width, g.Config.Height))
	for i, f := range g.Image {
		draw.Draw(canvas, f.Bounds(), f, f.Bounds().Min, draw.Over)
		frame := image.NewRGBA(canvas.Bounds())
		copy(frame.Pix, canvas.Pix)
		out.frames = append(out.frames, frame)
		d := time.Duration(g.Delay[i]) * 10 * time.Millisecond
		out.delays = append(out.delays, max(d, 50*time.Millisecond))
	}
	return out, nil
}

func main() {
	board := flag.String("dashboard", "coolant", "the dashboard to draw")
	picture := flag.String("picture", "", "an animated picture to play behind it, overriding its background")
	seconds := flag.Int("seconds", 30, "how long to run")
	smooth := flag.Bool("smooth", true, "tween between readings; false jumps")
	every := flag.Duration("every", 2*time.Second, "how often the made-up readings change")
	tween := flag.Duration("tween", 500*time.Millisecond, "how long a change takes")
	effect := flag.String("effect", "plain", "text: plain (numbers count), morph, gooey, roll")
	ring := flag.String("ring", "plain", "rings: plain (eased sweep), comet")
	snapDir := flag.String("snap", "", "write one change as PNGs here instead of driving the panel")
	wide := flag.Bool("wide", false, "swing the made-up coolant across its whole range, so the ring sweeps far")
	trails := flag.Bool("trails", false, "draw the dashboard's trail from a made-up history, drifting left between samples")
	step := flag.Duration("step", 5*time.Second, "with -trails: how often a sample joins the trail (hotaru's is 5 s)")
	flag.Parse()
	if *wide {
		ranges[readings.Coolant] = [2]float64{22, 68}
	}

	home, _ := os.UserHomeDir()
	store, err := dashboard.Open(filepath.Join(home, ".config/hotaru/dashboards.yml"))
	if err != nil {
		fmt.Println("dashboards:", err)
		os.Exit(1)
	}
	one, err := store.Get(*board)
	if err != nil {
		fmt.Println("dashboard:", err)
		os.Exit(1)
	}
	lib, err := images.Open(filepath.Join(home, ".local/share/hotaru/images"))
	if err != nil {
		fmt.Println("images:", err)
		os.Exit(1)
	}
	var still image.Image
	var bg *loop
	if *picture != "" {
		if bg, err = loadLoop(lib, *picture); err != nil {
			fmt.Println("picture:", err)
			os.Exit(1)
		}
		one.Background = dashboard.Background{Kind: dashboard.Picture, Picture: *picture}
		fmt.Printf("%s: %d frames\n", *picture, len(bg.frames))
	} else if one.Background.Kind == dashboard.Picture {
		still, _ = lib.Still(one.Background.Picture)
	}

	if *snapDir != "" {
		snap(*snapDir, one, still, *effect, *ring)
		return
	}

	ctx := context.Background()
	found, _ := sanshoku.Scan(ctx, nzxt.Driver{})
	var panel screen.Mover
	var dev sanshoku.Device
	for _, c := range found {
		if d, err := c.Open(ctx); err == nil {
			if m, ok := d.(screen.Mover); ok {
				panel, dev = m, d
				break
			}
			_ = d.Close()
		}
	}
	if panel == nil {
		fmt.Println("no Kraken that takes motion frames (is hotaru stopped?)")
		os.Exit(1)
	}
	defer func() { _ = dev.Close() }()

	rng := rand.New(rand.NewPCG(7, 81)) //nolint:gosec // made-up readings
	from, to := made(rng), made(rng)
	start, changed := time.Now(), time.Now()
	frame, nextFrame := 0, time.Now()
	moves, stills := 0, 0
	var moveTime, drawTime time.Duration
	settled := false

	// A made-up history for the trail: a random walk inside each source's
	// range, a sample added every step.
	hist := map[readings.Source][]float64{}
	for src, lim := range ranges {
		v := (lim[0] + lim[1]) / 2
		for range readings.Points {
			v = min(lim[1], max(lim[0], v+(rng.Float64()-0.5)*(lim[1]-lim[0])*0.15))
			hist[src] = append(hist[src], v)
		}
	}
	prevHist := map[readings.Source][]float64{}
	for k, v := range hist {
		prevHist[k] = v
	}
	stepped, lastShiftPx := time.Now(), -1
	trailState := func(now time.Time) dashboard.Trails {
		if !*trails {
			return dashboard.Trails{}
		}
		below, above := one.Trail.Sources(one.Headline.Source)
		p := min(1, float64(now.Sub(stepped))/float64(*step))
		// The sample that just left is drawn at the left edge until it has
		// drifted out, so the line keeps its length.
		withLeaving := func(src readings.Source) dashboard.Series {
			if src == "" || len(prevHist[src]) == 0 {
				return hist[src]
			}
			return append([]float64{prevHist[src][0]}, hist[src]...)
		}
		tr := dashboard.Trails{Below: withLeaving(below), Above: withLeaving(above), Shift: 1 - p,
			Ranges: map[readings.Source][2]float64{}}
		for _, src := range []readings.Source{below, above} {
			if src == "" {
				continue
			}
			lo0, hi0 := dashboard.TrailDomain(prevHist[src], src)
			lo1, hi1 := dashboard.TrailDomain(hist[src], src)
			tr.Ranges[src] = [2]float64{lo0 + (lo1-lo0)*p, hi0 + (hi1-hi0)*p}
		}
		return tr
	}

	// Per change: the old and new text layers, measured for the effect.
	var textA, textB *image.RGBA
	var shapeA, shapeB shape
	var boxes []box
	prepareChange := func() {
		t := time.Now()
		textA = dashboard.RenderLayers(one, from, nil, dashboard.Trails{}).Text
		textB = dashboard.RenderLayers(one, to, nil, dashboard.Trails{}).Text
		boxes = changedBoxes(textA, textB)
		if *effect == "morph" || *effect == "gooey" {
			shapeA, shapeB = prepare(textA), prepare(textB)
		}
		fmt.Printf("  change prepared in %.1f ms\n", float64(time.Since(t).Microseconds())/1000)
	}
	prepareChange()

	end := start.Add(time.Duration(*seconds) * time.Second)
	for time.Now().Before(end) {
		now := time.Now()
		if *trails && now.Sub(stepped) >= *step {
			for src := range hist {
				prevHist[src] = hist[src]
				v, _ := to.Value(src)
				next := append(append([]float64(nil), hist[src][1:]...), v)
				hist[src] = next
			}
			stepped = now
		}
		if now.Sub(changed) >= *every {
			from, to, changed, settled = to, made(rng), now, false
			prepareChange()
		}
		t := 1.0
		if *smooth {
			t = min(1, float64(now.Sub(changed))/float64(*tween))
		}
		behind := still
		if bg != nil {
			if now.After(nextFrame) {
				frame = (frame + 1) % len(bg.frames)
				nextFrame = now.Add(bg.delays[frame])
			}
			behind = bg.frames[frame]
		}
		tr := trailState(now)
		// A drifting trail needs a frame only when it has moved a whole pixel.
		shiftPx := int(tr.Shift * 9)
		drifted := *trails && shiftPx != lastShiftPx
		moving := bg != nil || t < 1 || drifted
		if !moving {
			// A drifting trail is never still: its frames keep coming, and a
			// still frame without it would flash.
			if !settled && !*trails {
				if err := panel.Stream(ctx, dashboard.Render(one, to, 0, behind, dashboard.Trails{}).Image); err != nil {
					fmt.Println("stream:", err)
					return
				}
				stills++
				settled = true
			}
			time.Sleep(20 * time.Millisecond)
			continue
		}

		drawStart := time.Now()
		// The background, then the rings, then the text.
		img := dashboard.RenderLayers(one, to, behind, dashboard.Trails{}).Background
		curve := ease(t)
		if *ring == "comet" {
			curve = spring(t)
		}
		rings := dashboard.RingsLayer(one, between(from, to, curve))
		if *ring == "comet" && t < 1 {
			before := dashboard.RingsLayer(one, between(from, to, spring(max(0, t-0.12))))
			glow(rings, before, 1-t)
		}
		over(img, rings)
		if *trails {
			over(img, dashboard.TrailsLayer(one, between(from, to, ease(t)), tr))
			lastShiftPx = shiftPx
		}
		var text *image.RGBA
		switch {
		case t >= 1:
			text = textB
		case *effect == "morph":
			text = morph(shapeA, shapeB, t)
		case *effect == "gooey":
			text = gooey(shapeA, shapeB, textB, boxes, t)
		case *effect == "roll":
			text = roll(textA, textB, boxes, t)
		default:
			text = dashboard.RenderLayers(one, between(from, to, ease(t)), nil, dashboard.Trails{}).Text
		}
		over(img, text)
		drawTime += time.Since(drawStart)

		sent := time.Now()
		if err := panel.Move(ctx, img); err != nil {
			fmt.Println("move:", err)
			return
		}
		moveTime += time.Since(sent)
		moves++
		if wait := 50*time.Millisecond - time.Since(now); wait > 0 {
			time.Sleep(wait)
		}
	}
	run := time.Since(start).Seconds()
	per := float64(max(moves, 1))
	fmt.Printf("%.0f s: %d motion frames (%.1f a second; %.1f ms each to draw, %.1f ms to send), %d still frames\n",
		run, moves, float64(moves)/run, float64(drawTime.Microseconds())/1000/per,
		float64(moveTime.Microseconds())/1000/per, stills)
}

// snap writes one change, from a made-up reading to another, at several
// points of its tween, for looking at without the panel.
func snap(dir string, one dashboard.Dashboard, behind image.Image, effect, ring string) {
	rng := rand.New(rand.NewPCG(7, 81)) //nolint:gosec // made-up readings
	from, to := made(rng), made(rng)
	textA := dashboard.RenderLayers(one, from, nil, dashboard.Trails{}).Text
	textB := dashboard.RenderLayers(one, to, nil, dashboard.Trails{}).Text
	shapeA, shapeB := prepare(textA), prepare(textB)
	boxes := changedBoxes(textA, textB)
	for _, t := range []float64{0, 0.2, 0.4, 0.5, 0.6, 0.8, 1} {
		img := dashboard.RenderLayers(one, to, behind, dashboard.Trails{}).Background
		curve := ease(t)
		if ring == "comet" {
			curve = spring(t)
		}
		rings := dashboard.RingsLayer(one, between(from, to, curve))
		if ring == "comet" && t < 1 {
			glow(rings, dashboard.RingsLayer(one, between(from, to, spring(max(0, t-0.12)))), 1-t)
		}
		over(img, rings)
		var text *image.RGBA
		switch effect {
		case "morph":
			text = morph(shapeA, shapeB, t)
		case "gooey":
			text = gooey(shapeA, shapeB, textB, boxes, t)
		case "roll":
			text = roll(textA, textB, boxes, t)
		default:
			text = dashboard.RenderLayers(one, between(from, to, ease(t)), nil, dashboard.Trails{}).Text
		}
		over(img, text)
		f, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s-%s-%03.0f.png", effect, ring, t*100)))
		if err != nil {
			fmt.Println(err)
			return
		}
		_ = png.Encode(f, img)
		_ = f.Close()
	}
	fmt.Printf("%d changed regions\n", len(boxes))
}
