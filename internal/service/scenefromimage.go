package service

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"strings"

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

**Effects are the caller's, not the picture's.** A picture says what colour
each light should be and nothing about what a device should be *doing* with
it, so a keyboard asked to run a rainbow is a decision somebody makes here.
Nothing is the default, which leaves every device lit with the colours it was
given. An effect the caller chose and gave no colours runs in the picture's
(see representative).
*/
func (s *Service) SceneFromImage(
	ctx context.Context, picture, name string, distance float64, effects map[string]scenes.Effect,
) (scenes.Scene, error) {
	found, err := s.storedPicture(picture)
	if err != nil {
		return scenes.Scene{}, err
	}
	decoded, err := images.First(found.Path)
	if err != nil {
		return scenes.Scene{}, err
	}

	return s.painted(ctx, scenes.Scene{
		Name: name, Screen: found.Path, Distance: distance, Effects: effects,
	}, decoded)
}

// storedPicture is a picture in the library by name.
func (s *Service) storedPicture(picture string) (images.Image, error) {
	library, err := s.library()
	if err != nil {
		return images.Image{}, err
	}
	stored, err := library.All()
	if err != nil {
		return images.Image{}, err
	}
	for _, image := range stored {
		if image.Name == picture {
			return image, nil
		}
	}
	return images.Image{}, fmt.Errorf("no picture called %q", picture)
}

// pictureNamed is a stored picture's first frame.
func (s *Service) pictureNamed(picture string) (image.Image, error) {
	found, err := s.storedPicture(picture)
	if err != nil {
		return nil, err
	}
	return images.First(found.Path) //nolint:wrapcheck // First names the file
}

/*
SceneFromDashboard builds a scene whose lights match a dashboard.

The same idea as a picture, with the frame the panel would draw as the
picture: a screen full of amber reads across the case as amber, which is what
somebody choosing a dashboard and then a set of colours was doing by hand.

The scene names the dashboard rather than a file, so applying it puts that
dashboard up -- and a dashboard somebody edits afterwards changes what the
screen shows without changing the lights, which is the honest split. The
lights were built from how it looked at the time; `hotaru scene recolour`
brings them back in line.
*/
func (s *Service) SceneFromDashboard(
	ctx context.Context, board, name string, distance float64, effects map[string]scenes.Effect,
) (scenes.Scene, error) {
	one, err := s.Dashboard(board)
	if err != nil {
		return scenes.Scene{}, err
	}
	decoded, err := s.dashboardFrame(ctx, one.Name)
	if err != nil {
		return scenes.Scene{}, err
	}

	return s.painted(ctx, scenes.Scene{
		Name: name, Screen: scenes.ScreenDashboardPrefix + one.Name, Distance: distance,
		Effects: effects,
	}, decoded)
}

// dashboardFrame is the frame the panel would draw for a dashboard, as a
// picture.
func (s *Service) dashboardFrame(ctx context.Context, board string) (image.Image, error) {
	one, err := s.Dashboard(board)
	if err != nil {
		return nil, err
	}
	frame, err := s.RenderDashboard(ctx, one)
	if err != nil {
		return nil, err
	}
	decoded, err := gif.Decode(bytes.NewReader(frame))
	if err != nil {
		return nil, fmt.Errorf("read the dashboard's own frame: %w", err)
	}
	return decoded, nil
}

/*
RecolourScene builds a scene's lights again from whatever it shows.

For the knob that has no right answer. Somebody sets a separation, looks at
the case, and wants it further apart -- and the thing they are adjusting is
not on screen anywhere except the machine itself, so it has to be adjustable
after the fact.

The source is the scene's own screen: the picture it names, or the dashboard.
A scene that shows neither has nothing to recolour from and says so, rather
than inventing a source.
*/
func (s *Service) RecolourScene(ctx context.Context, name string, distance float64) (scenes.Scene, error) {
	store, err := s.sceneStore()
	if err != nil {
		return scenes.Scene{}, err
	}
	scene, err := store.Get(name)
	if err != nil {
		return scenes.Scene{}, err
	}

	switch {
	case strings.HasPrefix(scene.Screen, scenes.ScreenDashboardPrefix):
		board := strings.TrimPrefix(scene.Screen, scenes.ScreenDashboardPrefix)
		// The scene's own effects, not none: recolouring is about the
		// colours, and a keyboard's mode is not one of them.
		return s.SceneFromDashboard(ctx, board, scene.Name, distance, scene.Effects)
	case scene.Screen == "" || scene.Screen == scenes.ScreenDashboard || scene.Screen == scenes.ScreenReadout:
		return scenes.Scene{}, fmt.Errorf(
			"%s does not show a picture or a named dashboard, so there is nothing to take its colours from", name)
	}

	picture, err := images.First(scene.Screen)
	if err != nil {
		return scenes.Scene{}, err
	}
	scene.Distance = distance
	return s.painted(ctx, scene, picture)
}

// painted fills in a scene's lights from a picture and keeps it.
func (s *Service) painted(ctx context.Context, scene scenes.Scene, picture image.Image) (scenes.Scene, error) {
	assignments, err := s.paint(ctx, picture, scene.Distance)
	if err != nil {
		return scenes.Scene{}, err
	}
	scene.Assignments = assignments
	scene = s.representative(ctx, scene, picture)

	if err := s.SaveScene(ctx, scene); err != nil {
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
func (s *Service) paint(ctx context.Context, picture image.Image, distance float64) ([]scenes.Assignment, error) {
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
		out = append(out, paintDevice(device, picture, distance)...)
	}
	return out, nil
}

// paintDevice is one device's worth of assignments.
func paintDevice(device *devices.Device, picture image.Image, distance float64) []scenes.Assignment {
	if len(device.Zones) == 0 {
		// Nothing to run a sweep along, so the whole device takes the
		// picture's middle.
		colours := lit(images.Scan(picture, 1), distance)
		return []scenes.Assignment{{
			Target: device.Name,
			Colour: written(colours[0]),
		}}
	}

	var out []scenes.Assignment
	for _, zone := range device.Zones {
		out = append(out, paintZone(device.Name, zone, picture, distance)...)
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
func paintZone(device string, zone devices.Zone, picture image.Image, distance float64) []scenes.Assignment {
	if zone.Count <= 0 {
		return nil
	}
	colours := lit(images.Scan(picture, zone.Count), distance)

	var out []scenes.Assignment
	first := 0
	anchor := colours[0]

	for i := 1; i < zone.Count; i++ {
		next := colours[i]
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

/*
lit brings a run of sampled colours up to something a light can show, and
then pushes them apart.

In that order. Lifting a shadow is a correction -- a photograph is mostly
midtones and an LED given one is an LED that is off -- and separation is a
preference about how different two lights should look. Separating first and
lifting afterwards would undo the separation on exactly the colours it was
asked for.
*/
func lit(sampled []color.NRGBA, distance float64) []color.NRGBA {
	out := make([]color.NRGBA, len(sampled))
	for i, c := range sampled {
		out[i] = images.Lit(c, images.Brightness)
	}
	return images.Separate(out, distance, images.Brightness)
}

// written is a colour as hotaru writes it.
func written(c color.NRGBA) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

/*
representative gives an effect that takes colours of its own the picture's
colours (spec 061 R5).

A picture answers "what colour is each light", and a device running a mode
that shows colours of its own cannot use that answer: it would be lit in
whatever the frame reduces to at the write, which is the same number arrived
at later and invisible in the scene until then. So the colours are picked here
and written down, where somebody can see them and change them.

**The picture's most prominent colours, as many as the mode takes up to
four**, ordered by how much of the picture each covers (images.Palette). A
Breathing between two colours on a sunset is the sunset's two colours. The
scene's distance decides what counts as two.

Only where the scene does not already name them, or where the colours it names
were picked here before (colours_from: picture), so recolouring picks again
and a colour somebody chose is kept. Only for a mode that cannot take a colour
per LED: a device in Direct is painted light by light and has nothing to pick.

An unreachable server or a device that is not there leaves the scene as it is.
The colours are still what the picture said, and the effect still has spec
050's fallback underneath it -- this is a better answer, not a required one.
*/
func (s *Service) representative(ctx context.Context, scene scenes.Scene, picture image.Image) scenes.Scene {
	if len(scene.Effects) == 0 {
		return scene
	}
	_, client, _ := s.current()
	if client == nil {
		return scene
	}
	found, err := client.Devices(ctx)
	if err != nil {
		return scene
	}

	out := cloneEffects(scene.Effects)
	for name, effect := range scene.Effects {
		if !effect.Named() {
			continue
		}
		if len(effect.Palette()) > 0 && effect.ColoursFrom != scenes.ColoursFromPicture {
			continue // somebody chose these
		}
		for i := range found {
			device := &found[i]
			if !strings.Contains(strings.ToLower(device.Name), strings.ToLower(name)) {
				continue
			}
			mode, ok := device.Mode(effect.Mode)
			if !ok || mode.PerLED {
				break
			}
			if picked := pickedColours(picture, mode, scene.Distance); len(picked) > 0 {
				effect.SetPalette(picked)
				effect.ColoursFrom = scenes.ColoursFromPicture
				out[name] = effect
			}
			break
		}
	}
	scene.Effects = out
	return scene
}

// mostPicked is as many colours as Make a scene picks for one effect, however
// many the mode takes. Past four a picture's colours are mostly shades of its
// first few, and a Spectrum through eight of them reads as one colour drifting.
const mostPicked = 4

// pickedColours is the picture's colours for one mode, as a scene writes them.
func pickedColours(picture image.Image, mode devices.Mode, distance float64) []string {
	_, most := mode.Slots()
	var out []string
	for _, c := range images.Palette(picture, min(most, mostPicked), distance) {
		out = append(out, written(c))
	}
	return out
}

/*
Picked is the colours Make a scene would give an effect that takes up to n,
from a stored picture, without making anything (spec 061 R5.3).

What the window shows before somebody presses Make it, and what it shows is
what the scene then gets: the same picture, the same distance and the same
function.
*/
func (s *Service) Picked(picture string, n int, distance float64) ([]string, error) {
	decoded, err := s.pictureNamed(picture)
	if err != nil {
		return nil, err
	}
	return pickedFrom(decoded, n, distance), nil
}

// PickedFromDashboard is Picked for a dashboard, from the frame the panel
// would draw.
func (s *Service) PickedFromDashboard(ctx context.Context, board string, n int, distance float64) ([]string, error) {
	decoded, err := s.dashboardFrame(ctx, board)
	if err != nil {
		return nil, err
	}
	return pickedFrom(decoded, n, distance), nil
}

func pickedFrom(picture image.Image, n int, distance float64) []string {
	var out []string
	for _, c := range images.Palette(picture, min(max(n, 1), mostPicked), distance) {
		out = append(out, written(c))
	}
	return out
}
