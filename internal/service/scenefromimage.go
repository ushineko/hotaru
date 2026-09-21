package service

import (
	"context"
	"fmt"
	"image"
	"image/color"

	"github.com/ushineko/hotaru/internal/devices"
	"github.com/ushineko/hotaru/internal/images"
	"github.com/ushineko/hotaru/internal/scenes"
)

/*
SceneFromImage builds a scene whose lights match a picture.

The quick way to a theme. Picking six colours by hand and hoping they go
together is the tedious half of making one, and the picture has already
answered that question -- somebody chose a wallpaper because they liked how its
colours sit beside each other.

**Sampled across the picture, not averaged.** Every light on the machine gets
the part of the image that corresponds to its position in its zone, so a
twenty-four light ring carries the picture's own left-to-right sweep. Averaging
a red storm on a blue sky gives mud, and a machine lit with mud looks broken
rather than themed.

The scene shows the picture on the panel too, because a machine lit by an image
with a different image on its screen is two themes at once.
*/
func (s *Service) SceneFromImage(ctx context.Context, picture, name string) (scenes.Scene, error) {
	library, err := s.library()
	if err != nil {
		return scenes.Scene{}, err
	}
	stored, err := library.All()
	if err != nil {
		return scenes.Scene{}, err
	}

	var found images.Image
	for _, image := range stored {
		if image.Name == picture {
			found = image
			break
		}
	}
	if found.Name == "" {
		return scenes.Scene{}, fmt.Errorf("no picture called %q", picture)
	}

	decoded, err := images.First(found.Path)
	if err != nil {
		return scenes.Scene{}, err
	}

	scene := scenes.Scene{Name: name, Screen: found.Path}
	assignments, err := s.paint(ctx, decoded)
	if err != nil {
		return scenes.Scene{}, err
	}
	scene.Assignments = assignments

	if err := s.SaveScene(scene); err != nil {
		return scenes.Scene{}, err
	}
	return scene, nil
}

/*
paint reads the picture onto every device in scope.

Per zone rather than per device: a zone is what the hardware treats as a run of
lights, so running the picture across one makes the sweep line up with
something physical -- a ring goes round, a strip goes along.

A zone of one light gets the colour of the slice it would have had, which is
the middle of the picture for a single-LED logo.
*/
func (s *Service) paint(ctx context.Context, picture image.Image) ([]scenes.Assignment, error) {
	_, client, addr := s.current()
	if client == nil {
		return nil, unreachable(addr)
	}
	found, err := client.Devices(ctx)
	if err != nil {
		return nil, err
	}
	cfg := s.config()

	var out []scenes.Assignment
	for i := range found {
		device := &found[i]
		if !cfg.InScope(device.Name) {
			continue
		}
		out = append(out, paintDevice(device, picture)...)
	}
	return out, nil
}

// paintDevice is one device's worth of assignments.
func paintDevice(device *devices.Device, picture image.Image) []scenes.Assignment {
	if len(device.Zones) == 0 {
		// Nothing to run a sweep along, so the whole device takes the
		// picture's middle.
		colours := images.Scan(picture, 1)
		return []scenes.Assignment{{
			Target: device.Name,
			Colour: written(images.Lit(colours[0], images.Brightness)),
		}}
	}

	var out []scenes.Assignment
	for _, zone := range device.Zones {
		out = append(out, paintZone(device.Name, zone, picture)...)
	}
	return out
}

/*
paintZone runs the picture along one zone, and says it in as few targets as it
takes.

Adjacent lights that come out the same colour become one run: a keyboard of a
hundred keys across a picture with four colours in it is four assignments, not
a hundred, and a scene somebody opens afterwards is a scene they can read.
*/
func paintZone(device string, zone devices.Zone, picture image.Image) []scenes.Assignment {
	if zone.Count <= 0 {
		return nil
	}
	colours := images.Scan(picture, zone.Count)

	var out []scenes.Assignment
	first := 0
	anchor := images.Lit(colours[0], images.Brightness)

	for i := 1; i < zone.Count; i++ {
		next := images.Lit(colours[i], images.Brightness)
		if alike(anchor, next) {
			continue
		}
		out = append(out, run(device, zone.Name, first, i-1, written(anchor)))
		first, anchor = i, next
	}
	return append(out, run(device, zone.Name, first, zone.Count-1, written(anchor)))
}

/*
alike is two colours nobody could tell apart.

Compared against the run's first colour rather than its neighbour, so a
gradient still comes out as a gradient: neighbour-to-neighbour each step is
tiny, and a rule built that way would flatten a whole ring to its first
colour.

The tolerance exists because the picture arrives as a GIF. A flat red area
stored in 256 colours comes back as several reds a hair apart, and writing
four rules for what the photograph calls one colour is noise in something
somebody has to read.
*/
func alike(a, b color.NRGBA) bool {
	const tolerance = 3 // out of 255; below what an LED can show apart
	return abs(int(a.R)-int(b.R)) <= tolerance &&
		abs(int(a.G)-int(b.G)) <= tolerance &&
		abs(int(a.B)-int(b.B)) <= tolerance
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// run is one assignment covering a stretch of a zone, written the way somebody
// would type it: a bare number for a single light.
func run(device, zone string, first, last int, colour string) scenes.Assignment {
	target := fmt.Sprintf("%s/%s[%d:%d]", device, zone, first, last)
	if first == last {
		target = fmt.Sprintf("%s/%s[%d]", device, zone, first)
	}
	return scenes.Assignment{Target: target, Colour: colour}
}

// written is a colour as hotaru writes it.
func written(c color.NRGBA) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}
