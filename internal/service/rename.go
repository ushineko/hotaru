package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ushineko/hotaru/internal/dashboard"
	"github.com/ushineko/hotaru/internal/scenes"
)

/*
Renaming, and the references a rename has to carry with it.

A name in hotaru is a key: the map key in the scenes file, the map key in the
dashboards file, the filename in the image directory. Nothing else in the
program has an identity, so renaming one thing means rewriting everything that
spelled its old name -- the keys bound to a scene, the scenes naming a screen,
the screens drawing a picture behind their numbers.

Doing it by hand loses those, and loses them quietly: the panel falls back to
the theme's plain colour over a picture that is no longer there, and nothing
says why. See spec 053.
*/

// Renamed is what a rename changed besides the thing itself.
type Renamed struct {
	// From and To are the names, as stored. A picture's is cleaned before it
	// becomes a filename, so what was asked for and what was given are not
	// always the same string.
	From, To string

	// Scenes, Screens and Keys are how many of each named the old one and now
	// name the new one. Nothing pointing at it is the ordinary case and counts
	// nothing.
	Scenes, Screens, Keys int

	// Active says the renamed screen was the one the panel draws, which is a
	// reference like the others and is not one anybody can see in a file.
	Active bool
}

/*
Changed is what else moved, in a line, or empty when nothing did.

Here rather than in each shell because both of them say it and a second
spelling of the same sentence is a second thing to keep in step.
*/
func (r Renamed) Changed() string {
	var parts []string
	for _, count := range []struct {
		n    int
		one  string
		many string
	}{
		{r.Scenes, "scene", "scenes"},
		{r.Screens, "screen", "screens"},
		{r.Keys, "key", "keys"},
	} {
		switch {
		case count.n == 1:
			parts = append(parts, "1 "+count.one)
		case count.n > 1:
			parts = append(parts, fmt.Sprintf("%d %s", count.n, count.many))
		}
	}
	if r.Active {
		parts = append(parts, "the screen on the panel")
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ") + " updated"
}

// ErrShipped is a scene or a screen hotaru carries in code, which has no file
// entry to rename.
var ErrShipped = errors.New("that one is shipped with hotaru")

// naming checks the pair of names a rename or a clone was given. Neutral
// wording, because a clone is the same pair of names and is not a rename.
func naming(from, to string) (string, string, error) {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	switch {
	case from == "":
		return "", "", errors.New("which one?")
	case to == "":
		return "", "", errors.New("and call it what?")
	case from == to:
		return "", "", fmt.Errorf("%s is already called that", from)
	}
	return from, to, nil
}

/*
RenameScene moves a scene and repoints every key bound to it.

The keys are the whole of what names a scene. A binding is written into the
same file and is installed into the desktop's own shortcuts, so a rename that
left them behind would leave a keypress firing a scene that is not there --
which is a key that does nothing, reported as "the hotkey stopped working".
*/
func (s *Service) RenameScene(from, to string) (Renamed, error) {
	store, err := s.sceneStore()
	if err != nil {
		return Renamed{}, err
	}
	from, to, err = naming(from, to)
	if err != nil {
		return Renamed{}, err
	}

	scene, err := store.Get(from)
	if err != nil {
		return Renamed{}, err
	}
	if scene.Shipped {
		return Renamed{}, fmt.Errorf("%w: save %s under the new name instead", ErrShipped, scene.Name)
	}
	if taken, err := store.Get(to); err == nil && taken.Name == to {
		return Renamed{}, fmt.Errorf("there is already a scene called %q", to)
	}

	out := Renamed{From: scene.Name, To: to}
	scene.Name = to
	if err := store.Save(scene); err != nil {
		return Renamed{}, err
	}
	if err := store.Delete(out.From); err != nil {
		return Renamed{}, err
	}

	for key, bound := range store.Bindings() {
		if bound != out.From {
			continue
		}
		if err := s.Bind(key, to); err != nil {
			return out, fmt.Errorf("point %s at %s: %w", key, to, err)
		}
		out.Keys++
	}

	// The label follows, so status after a rename is not the name of a scene
	// that no longer exists.
	if s.Applied() == out.From {
		s.applying(to)
	}
	return out, nil
}

/*
RenameDashboard moves a screen and rewrites every scene naming it.

A scene spells a particular screen as `dashboard:<name>`, which is a reference
by name and the only one in the scenes file. The active screen is the other:
renaming the one on the panel and not following it would put the panel back on
the shipped default, mid-rename, for no reason a person could see.
*/
func (s *Service) RenameDashboard(from, to string) (Renamed, error) {
	store, err := s.boards()
	if err != nil {
		return Renamed{}, err
	}
	from, to, err = naming(from, to)
	if err != nil {
		return Renamed{}, err
	}

	one, err := store.Get(from)
	if err != nil {
		return Renamed{}, err
	}
	if one.Shipped {
		return Renamed{}, fmt.Errorf("%w: clone %s instead", ErrShipped, one.Name)
	}
	if taken, err := store.Get(to); err == nil && taken.Name == to {
		return Renamed{}, fmt.Errorf("there is already a screen called %q", to)
	}

	out := Renamed{From: one.Name, To: to}
	out.Active = store.Active().Name == out.From

	one.Name = to
	if err := store.Save(one); err != nil {
		return Renamed{}, err
	}
	if err := store.Delete(out.From); err != nil {
		return Renamed{}, err
	}
	if out.Active {
		if err := store.Use(to); err != nil {
			return out, fmt.Errorf("keep %s on the panel: %w", to, err)
		}
	}

	out.Scenes, err = s.rewriteScenes(
		scenes.ScreenDashboardPrefix+out.From, scenes.ScreenDashboardPrefix+to)
	if err != nil {
		return out, err
	}
	s.redrawPanel()
	return out, nil
}

/*
RenameImage moves a picture, and every scene and screen that names it.

Two different references to the same file: a scene keeps the path, because it
is what the panel is handed, and a screen keeps the name, because a background
is chosen from the library. Both have to move or the rename shows up later as
a scene with no picture and a screen with a plain colour behind it.
*/
func (s *Service) RenameImage(from, to string) (Renamed, error) {
	library, err := s.library()
	if err != nil {
		return Renamed{}, err
	}
	from, to, err = naming(from, to)
	if err != nil {
		return Renamed{}, err
	}

	was := library.Path(from)
	stored, err := library.Rename(from, to)
	if err != nil {
		return Renamed{}, err
	}

	out := Renamed{From: from, To: stored.Name}
	if out.Scenes, err = s.rewriteScenes(was, stored.Path); err != nil {
		return out, err
	}
	if out.Screens, err = s.rewriteBackgrounds(from, stored.Name); err != nil {
		return out, err
	}
	if out.Screens > 0 {
		s.redrawPanel()
	}
	return out, nil
}

/*
CloneDashboard copies a screen under another name.

A dashboard is a dozen decisions -- the arrangement, the theme, its rings, the
lettering, the caption -- and wanting the same dozen with a different picture
behind them means rebuilding all twelve by eye without this.

A shipped one can be cloned, and that is the point: it is the one way to start
from one. The copy is not shipped, because it is in somebody's file now, and
is not put on the panel, because copying a screen is not asking to see it.
*/
func (s *Service) CloneDashboard(from, to string) (dashboard.Dashboard, error) {
	store, err := s.boards()
	if err != nil {
		return dashboard.Dashboard{}, err
	}
	from, to, err = naming(from, to)
	if err != nil {
		return dashboard.Dashboard{}, err
	}

	one, err := store.Get(from)
	if err != nil {
		return dashboard.Dashboard{}, err
	}
	if taken, err := store.Get(to); err == nil && taken.Name == to {
		return dashboard.Dashboard{}, fmt.Errorf("there is already a screen called %q", to)
	}

	one.Name, one.Shipped = to, false
	if err := store.Save(one); err != nil {
		return dashboard.Dashboard{}, err
	}
	return one, nil
}

/*
rewriteScenes repoints every scene whose screen was one thing at another.

Saved scenes only. A shipped scene naming a shipped screen is code on both
sides and cannot be stale; one naming a picture somebody renamed is a shipped
scene that has been overridden, and the override is what is in the file.
*/
func (s *Service) rewriteScenes(was, now string) (int, error) {
	store, err := s.sceneStore()
	if err != nil {
		// A machine with no scenes has no scenes to rewrite, which is not a
		// failed rename.
		return 0, nil //nolint:nilerr // absence is an ordinary answer here
	}

	changed := 0
	for _, scene := range store.All() {
		if scene.Shipped || scene.Screen != was {
			continue
		}
		scene.Screen = now
		if err := store.Save(scene); err != nil {
			return changed, fmt.Errorf("point %s at %s: %w", scene.Name, now, err)
		}
		changed++
	}
	return changed, nil
}

// rewriteBackgrounds repoints every saved screen drawing one picture at
// another.
func (s *Service) rewriteBackgrounds(was, now string) (int, error) {
	store, err := s.boards()
	if err != nil {
		return 0, nil //nolint:nilerr // a machine with no screens has none to rewrite
	}

	changed := 0
	for _, one := range store.All() {
		if one.Shipped || one.Background.Picture != was {
			continue
		}
		one.Background.Picture = now
		if err := store.Save(one); err != nil {
			return changed, fmt.Errorf("point %s at %s: %w", one.Name, now, err)
		}
		changed++
	}
	return changed, nil
}
