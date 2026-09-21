/*
Package images is the pictures somebody added, converted to what the panel
takes.

The cooler's screen accepts 640x640 GIFs and nothing else -- an image of any
other size transfers successfully, switches buckets successfully, and displays
nothing at all, which is how it says no (spec 016). So a wallpaper has to be
converted before it can be shown, and the conversion is worth keeping.

Stored under $XDG_DATA_HOME, because these are files somebody added rather than
settings they chose: the distinction matters at the moment one is lost. See
spec 019.
*/
package images

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	_ "image/jpeg" // the formats a wallpaper arrives in
	_ "image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	xdraw "golang.org/x/image/draw"
)

// Panel is the size the cooler's screen takes, square.
const Panel = 640

// Library is a directory of converted images.
type Library struct{ dir string }

// Open prepares the library, creating its directory on first use.
func Open(dir string) (*Library, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("make the image directory: %w", err)
	}
	return &Library{dir: dir}, nil
}

// Dir is where the library keeps its files, which a scene names.
func (l *Library) Dir() string { return l.dir }

// Image is one stored picture.
type Image struct {
	// Name is what somebody calls it, without an extension.
	Name string
	// Path is the file a scene names.
	Path string
	// Bytes is its size on disk, which decides how often the panel can be
	// written (spec 013's floor).
	Bytes int64
	// Frames is how many it has: one for a still picture.
	Frames int
	// Added is when it was converted.
	Added time.Time
}

// Moves reports whether the image is an animation, which is the one thing a
// listing cannot show and somebody wants to know.
func (i Image) Moves() bool { return i.Frames > 1 }

/*
Add converts a picture and stores it under a name.

The whole conversion is here rather than at the edges because every caller
wants the same thing: something the panel will display. A name that exists is
replaced -- somebody adding "wallpaper" twice means the second one.
*/
func (l *Library) Add(name string, source []byte) (Image, error) {
	name = clean(name)
	if name == "" {
		return Image{}, fmt.Errorf("an image needs a name")
	}

	converted, frames, err := Convert(source)
	if err != nil {
		return Image{}, err
	}

	path := l.path(name)
	if err := os.WriteFile(path, converted, 0o600); err != nil {
		return Image{}, fmt.Errorf("store %s: %w", name, err)
	}
	return Image{
		Name: name, Path: path, Bytes: int64(len(converted)),
		Frames: frames, Added: time.Now(),
	}, nil
}

// All is what is stored, by name.
func (l *Library) All() ([]Image, error) {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return nil, fmt.Errorf("read the image directory: %w", err)
	}

	out := make([]Image, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".gif") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".gif")
		out = append(out, Image{
			Name: name, Path: l.path(name), Bytes: info.Size(),
			Frames: frames(l.path(name)), Added: info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Remove deletes one. Removing something that is not there is not an error:
// the caller wanted it gone and it is gone.
func (l *Library) Remove(name string) error {
	if err := os.Remove(l.path(clean(name))); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", name, err)
	}
	return nil
}

// Read is a stored image's bytes, for sending to the panel.
func (l *Library) Read(name string) ([]byte, error) {
	body, err := os.ReadFile(l.path(clean(name))) //nolint:gosec // a name this package cleaned
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	return body, nil
}

func (l *Library) path(name string) string {
	return filepath.Join(l.dir, name+".gif")
}

/*
clean makes a name safe to use as a filename.

Not an escape: anything that is not a letter, a number or a dash becomes a
dash, so a name cannot climb out of the directory however it was typed. The
result is also what somebody sees in a listing, so it has to stay readable.
*/
func clean(name string) string {
	var out strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
		case r == '-' || r == '_' || r == ' ':
			out.WriteRune('-')
		}
	}
	return strings.Trim(out.String(), "-")
}

/*
Convert turns any image into one the panel will display.

Cropped to fill rather than letterboxed. A wallpaper is a wide picture and the
panel is a circle in a case: bars across somebody's photograph look like a
mistake where a centre crop looks like a photograph.

An animation keeps its frames and its timing; a still picture becomes a
one-frame GIF, because this firmware drops a static image within seconds and
holds a GIF indefinitely (spec 012).
*/
func Convert(source []byte) (converted []byte, frames int, err error) {
	if animation, err := gif.DecodeAll(bytes.NewReader(source)); err == nil && len(animation.Image) > 1 {
		return convertAnimation(animation)
	}

	decoded, _, err := image.Decode(bytes.NewReader(source))
	if err != nil {
		return nil, 0, fmt.Errorf("that is not an image this can read: %w", err)
	}

	out, err := encode([]*image.Paletted{quantise(square(decoded))}, []int{100})
	if err != nil {
		return nil, 0, err
	}
	return out, 1, nil
}

// convertAnimation scales every frame of a GIF, keeping its timing.
func convertAnimation(animation *gif.GIF) ([]byte, int, error) {
	canvas := image.NewRGBA(image.Rect(0, 0, animation.Config.Width, animation.Config.Height))
	out := make([]*image.Paletted, 0, len(animation.Image))

	for _, frame := range animation.Image {
		// Composited first: a GIF's frames are often partial, and scaling a
		// partial frame on its own scales a rectangle of changes rather than
		// a picture.
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
		out = append(out, quantise(square(canvas)))
	}

	encoded, err := encode(out, animation.Delay)
	if err != nil {
		return nil, 0, err
	}
	return encoded, len(out), nil
}

/*
square crops to the middle and scales to the panel.

CatmullRom, because this is a photograph being made eight times smaller and
the cheap samplers turn fine detail into noise at that ratio.
*/
func square(source image.Image) *image.RGBA {
	bounds := source.Bounds()
	side := min(bounds.Dx(), bounds.Dy())
	crop := image.Rect(
		bounds.Min.X+(bounds.Dx()-side)/2, bounds.Min.Y+(bounds.Dy()-side)/2,
		bounds.Min.X+(bounds.Dx()-side)/2+side, bounds.Min.Y+(bounds.Dy()-side)/2+side,
	)

	out := image.NewRGBA(image.Rect(0, 0, Panel, Panel))
	xdraw.CatmullRom.Scale(out, out.Bounds(), source, crop, xdraw.Over, nil)
	return out
}

/*
quantise reduces to the 256 colours a GIF can hold, choosing them from the
picture.

A fixed palette spreads its colours across the whole cube; a photograph uses a
small part of it. The first version used Plan 9's and a picture of a red storm
on a blue planet came back speckled, because half those 256 colours were greens
and greys it had no use for. Choosing from the picture spends every entry on a
colour that is actually in it.

Dithering is still applied, because 256 colours from anywhere still band across
a sky -- and it costs size, which on this panel costs refresh rate (spec 013's
floor). A picture is written once and looked at, so that trade goes the other
way here than it does for the dashboard.
*/
func quantise(source image.Image) *image.Paletted {
	out := image.NewPaletted(image.Rect(0, 0, Panel, Panel), chosen(source))
	draw.FloydSteinberg.Draw(out, out.Bounds(), source, image.Point{})
	return out
}

/*
chosen picks 256 colours out of a picture, by popularity in a coarse cube.

Not median cut: this is a five-bit histogram and the 255 fullest cells, which
is cruder and enough. What it has to beat is a palette chosen without looking
at the image at all, and the distance between those two is most of the quality.

Black is always present, so an image with a dark border keeps it clean rather
than dithering it into the nearest thing available.
*/
func chosen(source image.Image) color.Palette {
	const step = 8 // 32 levels per channel
	counts := map[color.RGBA]int{}

	bounds := source.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := source.At(x, y).RGBA()
			key := color.RGBA{
				R: uint8(r>>8) / step * step, //nolint:gosec // a byte by construction
				G: uint8(g>>8) / step * step, //nolint:gosec // as above
				B: uint8(b>>8) / step * step, //nolint:gosec // as above
				A: 255,
			}
			counts[key]++
		}
	}

	type cell struct {
		colour color.RGBA
		count  int
	}
	cells := make([]cell, 0, len(counts))
	for colour, count := range counts {
		cells = append(cells, cell{colour, count})
	}
	sort.Slice(cells, func(i, j int) bool { return cells[i].count > cells[j].count })

	out := color.Palette{color.RGBA{A: 255}}
	for _, c := range cells {
		if len(out) >= 256 {
			break
		}
		if c.colour.R == 0 && c.colour.G == 0 && c.colour.B == 0 {
			continue // already there
		}
		out = append(out, c.colour)
	}
	return out
}

func encode(frames []*image.Paletted, delays []int) ([]byte, error) {
	if len(delays) < len(frames) {
		delays = make([]int, len(frames))
		for i := range delays {
			delays[i] = 100
		}
	}

	var out bytes.Buffer
	err := gif.EncodeAll(&out, &gif.GIF{
		Image: frames, Delay: delays[:len(frames)], LoopCount: 0,
		Config: image.Config{ColorModel: frames[0].Palette, Width: Panel, Height: Panel},
	})
	if err != nil {
		return nil, fmt.Errorf("write the image: %w", err)
	}
	return out.Bytes(), nil
}

// frames counts a stored image's frames, for a listing that says whether it
// moves. A file that will not parse counts as one rather than failing a
// listing over a single bad entry.
func frames(path string) int {
	body, err := os.ReadFile(path) //nolint:gosec // a path this package built
	if err != nil {
		return 1
	}
	animation, err := gif.DecodeAll(bytes.NewReader(body))
	if err != nil {
		return 1
	}
	return len(animation.Image)
}
