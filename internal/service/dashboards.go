package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"image/png"
	"os"
	"sync"
	"time"

	"github.com/ushineko/hotaru/internal/dashboard"
	"github.com/ushineko/hotaru/internal/state"
)

/*
The dashboards, as the service offers them.

Here rather than in the window for the reason the image library is here: the
CLI would otherwise be unable to do something the GUI can, and a file two
shells both write into is the one-writer question with a worse answer.

And the rendering is here because the panel takes a 640x640 GIF and this is
what makes them. A second renderer in the window would be a second answer
about what the screen looks like, and the one nobody checks is the one on
screen.
*/

// ErrNoDashboards is a machine whose dashboards file could not be opened.
var ErrNoDashboards = errors.New("the dashboards are unavailable on this machine")

// SetDashboards gives the service somewhere to keep them.
func (s *Service) SetDashboards(store *dashboard.Store) {
	s.mu.Lock()
	s.dashboards = store
	s.mu.Unlock()
}

func (s *Service) boards() (*dashboard.Store, error) {
	s.mu.RLock()
	store := s.dashboards
	s.mu.RUnlock()
	if store == nil {
		return nil, ErrNoDashboards
	}
	return store, nil
}

// Dashboards is every one, shipped included.
func (s *Service) Dashboards() ([]dashboard.Dashboard, error) {
	store, err := s.boards()
	if err != nil {
		return nil, err
	}
	return store.All(), nil
}

// Dashboard finds one by name or unambiguous prefix.
func (s *Service) Dashboard(name string) (dashboard.Dashboard, error) {
	store, err := s.boards()
	if err != nil {
		return dashboard.Dashboard{}, err
	}
	return store.Get(name)
}

// ActiveDashboard is the one the panel draws.
func (s *Service) ActiveDashboard() (dashboard.Dashboard, error) {
	store, err := s.boards()
	if err != nil {
		return dashboard.Dashboard{}, err
	}
	return store.Active(), nil
}

// SaveDashboard writes one, replacing any of the same name.
func (s *Service) SaveDashboard(one dashboard.Dashboard) error {
	store, err := s.boards()
	if err != nil {
		return err
	}
	if err := store.Save(one); err != nil {
		return err
	}
	s.redrawPanel()
	return nil
}

// DeleteDashboard forgets one.
func (s *Service) DeleteDashboard(name string) error {
	store, err := s.boards()
	if err != nil {
		return err
	}
	if err := store.Delete(name); err != nil {
		return err
	}
	s.redrawPanel()
	return nil
}

/*
UseDashboard makes one the dashboard the panel draws, and takes the panel
back to do it.

The taking back is the part that was missing. Anything that puts a picture on
the screen holds the dashboard -- `hotaru screen show`, the readout, and every
scene that names an image -- and only asking for the dashboard gives it up.
Choosing a dashboard *is* asking for the dashboard, so it releases as well as
redraws.

Without this, somebody who had applied a scene with a picture in it could
choose dashboards all day and watch the picture: the store changed, the panel
did not, and nothing said why.
*/
func (s *Service) UseDashboard(name string) error {
	store, err := s.boards()
	if err != nil {
		return err
	}
	one, err := store.Get(name)
	if err != nil {
		return err
	}
	if err := store.Use(one.Name); err != nil {
		return err
	}

	s.mu.RLock()
	panel := s.panel
	s.mu.RUnlock()
	if panel != nil {
		panel.Release() // Release redraws; see Dashboard.
	}
	s.shown(state.Screen{Showing: "dashboard: " + one.Name}, nil)
	return nil
}

/*
redrawPanel tells the dashboard loop that what it is drawing has changed.

It does not take the panel back. Saving a dashboard is not the same as asking
to see it: somebody editing one while a picture is on the screen has not asked
for the picture to go away.

Without it, editing the dashboard on screen shows nothing until a reading
moves: the push gate compares what the frame *says*, and a new arrangement of
the same numbers says the same thing to a hash of the old frame's content.
*/
func (s *Service) redrawPanel() {
	s.mu.RLock()
	panel := s.panel
	s.mu.RUnlock()
	if panel != nil {
		panel.Redraw()
	}
}

/*
Look is the dashboard to draw and the picture behind it.

Asked once per push rather than held, because somebody can change the active
dashboard or edit the one on screen while the loop is running.
*/
func (s *Service) Look(ctx context.Context) (dashboard.Dashboard, image.Image) {
	store, err := s.boards()
	if err != nil {
		return dashboard.Shipped()[0], nil
	}
	one := store.Active()
	return one, s.behind(ctx, one)
}

/*
behind is the picture a dashboard names, decoded.

A picture that is no longer in the library costs the background and not the
frame: the renderer falls back to the theme's plain colour, because a panel
that went blank over a deleted file would be a fault where there is only a
missing decoration.
*/
func (s *Service) behind(_ context.Context, one dashboard.Dashboard) image.Image {
	if one.Background.Kind != dashboard.Picture || one.Background.Picture == "" {
		return nil
	}
	library, err := s.library()
	if err != nil {
		return nil
	}
	/*
		In full colour where the library kept a copy (spec 080), decoded once
		per picture file: the dashboard loop asks every second, and decoding
		a picture again each time was a tenth of the service's own work
		(spec 081's profile).
	*/
	info, err := os.Stat(library.Path(one.Background.Picture))
	if err != nil {
		return nil
	}
	key := fmt.Sprintf("%s|%d|%d", info.Name(), info.Size(), info.ModTime().UnixNano())
	s.still.mu.Lock()
	defer s.still.mu.Unlock()
	if s.still.key == key {
		return s.still.img
	}
	still, err := library.Still(one.Background.Picture)
	if err != nil {
		return nil
	}
	s.still.key, s.still.img = key, still
	return still
}

// stillCache is the one still background decoded, for the file the key names.
// A copy kept beside the GIF (images.Library.Still) changes with it.
type stillCache struct {
	mu  sync.Mutex
	key string
	img image.Image
}

// backdropCache is the one animated background decoded: the frames of the
// picture file the key names.
type backdropCache struct {
	mu     sync.Mutex
	key    string
	frames dashboard.Backdrop
}

/*
Backdrop is a dashboard's background picture as frames, for a panel that
takes motion frames to play (spec 081). A still picture, a missing one and a
background that is not a picture are no frames.

Decoded once per picture file and kept: a file added again under the same
name has another size or time, and is decoded again. The library stores an
animation as whole 640x640 frames, kept here as they are, 256 colours each.
*/
func (s *Service) Backdrop(_ context.Context, one dashboard.Dashboard) dashboard.Backdrop {
	if one.Background.Kind != dashboard.Picture || one.Background.Picture == "" {
		return dashboard.Backdrop{}
	}
	library, err := s.library()
	if err != nil {
		return dashboard.Backdrop{}
	}
	info, err := os.Stat(library.Path(one.Background.Picture))
	if err != nil {
		return dashboard.Backdrop{}
	}
	key := fmt.Sprintf("%s|%d|%d", info.Name(), info.Size(), info.ModTime().UnixNano())

	s.backdrop.mu.Lock()
	defer s.backdrop.mu.Unlock()
	if s.backdrop.key == key {
		return s.backdrop.frames
	}
	s.backdrop.key, s.backdrop.frames = key, dashboard.Backdrop{}
	body, err := library.Read(one.Background.Picture)
	if err != nil {
		return dashboard.Backdrop{}
	}
	g, err := gif.DecodeAll(bytes.NewReader(body))
	if err != nil || len(g.Image) < 2 {
		return dashboard.Backdrop{}
	}
	out := dashboard.Backdrop{}
	whole := image.Rect(0, 0, g.Config.Width, g.Config.Height)
	canvas := image.NewRGBA(whole)
	for i, frame := range g.Image {
		var img image.Image = frame
		if !frame.Bounds().Eq(whole) {
			// A partial frame, from a file the library did not convert:
			// laid over the frames before it.
			draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
			copied := image.NewRGBA(whole)
			copy(copied.Pix, canvas.Pix)
			img = copied
		} else {
			draw.Draw(canvas, whole, frame, image.Point{}, draw.Src)
		}
		out.Frames = append(out.Frames, img)
		out.Delays = append(out.Delays, time.Duration(g.Delay[i])*10*time.Millisecond)
	}
	s.backdrop.frames = out
	return out
}

// Preview is a dashboard frame encoded as the panel would show it.
type Preview struct {
	Image []byte
	// Format is "png" for a panel that takes frames, in full colour, and
	// "gif" for one that takes GIFs (spec 080).
	Format string
}

/*
RenderDashboard draws one frame of a dashboard, for somebody to look at before
it goes on the panel.

The readings are the machine's own, because a preview of a dashboard full of
invented numbers does not answer the question somebody is asking, which is
whether their own numbers fit. The encoding is the panel's: a PNG of the full
colour frame where the panel takes frames, the GIF it would be sent where it
does not, so the preview is not a better or worse picture than the panel's.
*/
func (s *Service) RenderDashboard(ctx context.Context, one dashboard.Dashboard) (Preview, error) {
	frame := dashboard.Render(one, s.Readings(ctx), 0, s.behind(ctx, one),
		s.Trails(ctx, one))
	if !s.Frames() {
		return Preview{Image: frame.GIF(), Format: "gif"}, nil
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, frame.Image); err != nil {
		return Preview{}, fmt.Errorf("encode the preview: %w", err)
	}
	return Preview{Image: buf.Bytes(), Format: "png"}, nil
}
