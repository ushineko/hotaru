package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/gif"

	"github.com/ushineko/hotaru/internal/dashboard"
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

// UseDashboard makes one the dashboard the panel draws.
func (s *Service) UseDashboard(name string) error {
	store, err := s.boards()
	if err != nil {
		return err
	}
	if err := store.Use(name); err != nil {
		return err
	}
	s.redrawPanel()
	return nil
}

/*
redrawPanel tells the dashboard loop that what it is drawing has changed.

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
	body, err := library.Read(one.Background.Picture)
	if err != nil {
		return nil
	}
	decoded, err := gif.Decode(bytes.NewReader(body))
	if err != nil {
		return nil
	}
	return decoded
}

/*
RenderDashboard draws one frame of a dashboard, for somebody to look at before
it goes on the panel.

The readings are the machine's own, because a preview of a dashboard full of
invented numbers does not answer the question somebody is asking, which is
whether their own numbers fit.
*/
func (s *Service) RenderDashboard(ctx context.Context, one dashboard.Dashboard) ([]byte, error) {
	frame := dashboard.Render(one, s.Readings(ctx), 0, s.behind(ctx, one))
	return frame.GIF, nil
}
