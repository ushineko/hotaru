package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/cooler"
	"github.com/ushineko/hotaru/internal/scenes"
)

/*
SceneStore is where named scenes live. An interface so the service does not
depend on how they are stored, and so a machine whose scenes file will not
parse is the nil case rather than a broken service.
*/
type SceneStore interface {
	All() []scenes.Scene
	Get(name string) (scenes.Scene, error)
	Save(scene scenes.Scene) error
	Delete(name string) error
	// Bindings are the keys, shipped ones merged with anything somebody has
	// changed. They live with the scenes because they are edited by the same
	// hands and belong to the same file.
	Bindings() map[string]string
	Bind(key, scene string) error
}

// SetScenes gives the service somewhere to keep named lighting.
func (s *Service) SetScenes(store SceneStore) {
	s.mu.Lock()
	s.scenes = store
	s.mu.Unlock()
}

// ErrNoScenes is a machine whose scenes could not be read. Distinguished from
// "no scenes saved", which is an empty list and an ordinary answer.
var ErrNoScenes = errors.New("scenes are unavailable on this machine")

func (s *Service) sceneStore() (SceneStore, error) {
	s.mu.RLock()
	store := s.scenes
	s.mu.RUnlock()
	if store == nil {
		return nil, ErrNoScenes
	}
	return store, nil
}

// Scenes is every saved scene.
func (s *Service) Scenes() ([]scenes.Scene, error) {
	store, err := s.sceneStore()
	if err != nil {
		return nil, err
	}
	return store.All(), nil
}

// Scene finds one by name, or by any unambiguous prefix of one.
func (s *Service) Scene(name string) (scenes.Scene, error) {
	store, err := s.sceneStore()
	if err != nil {
		return scenes.Scene{}, err
	}
	return store.Get(name)
}

// SaveScene writes a scene, replacing one of the same name.
func (s *Service) SaveScene(scene scenes.Scene) error {
	store, err := s.sceneStore()
	if err != nil {
		return err
	}
	return store.Save(scene)
}

// DeleteScene removes one.
func (s *Service) DeleteScene(name string) error {
	store, err := s.sceneStore()
	if err != nil {
		return err
	}
	return store.Delete(name)
}

/*
SceneOutcome is what happened when a scene was applied.

Per device, because "the scene worked" says nothing useful about six devices
when one of them is dark -- and separately, the parts of a scene that are not
about a device at all: a screen that could not be set, a line that would not
parse.
*/
type SceneOutcome struct {
	// Scene is the name applied, in full, whatever prefix was asked for.
	Scene   string
	Results []Result
	// Problems are the scene's own: an unreadable assignment, a GIF that is
	// not there. A scene that cannot be fully applied applies the rest.
	Problems []string
	// Screen is what the panel was set to, empty when the scene said nothing
	// about it and the screen was therefore left alone.
	Screen string
	// Lease is set when this was a preview.
	Lease *Lease
}

/*
ApplyScene lights a scene and remembers it.

What somebody wants their machine to look like, so it is recorded and the
reconciler holds it against hardware that forgets.
*/
func (s *Service) ApplyScene(ctx context.Context, name string) (SceneOutcome, error) {
	scene, err := s.Scene(name)
	if err != nil {
		return SceneOutcome{}, err
	}
	out, err := s.light(ctx, scene, Request{})
	if err == nil {
		// Applied, not previewed. A preview is not what anybody asked the
		// machine to be, which is why it is remembered here and not in
		// light.
		s.applying(scene.Name)
	}
	return out, err
}

/*
Preview lights a scene that has no name.

A draft in an editor is not a saved scene and must not have to become one to be
looked at: saving a scratch scene to preview it would put a half-finished thing
in somebody's list and make "saved" stop meaning anything.

Everything else is the named form's: the same lease, ending with its holder,
suspending re-assertion for the devices it covers.
*/
func (s *Service) Preview(ctx context.Context, scene scenes.Scene, holder string, connection bool) (SceneOutcome, error) {
	covered, err := s.covers(ctx, scene)
	if err != nil {
		return SceneOutcome{}, err
	}
	lease, err := s.Take(scene.Name, holder, covered, connection)
	if err != nil {
		return SceneOutcome{}, err
	}

	outcome, err := s.light(ctx, scene, Request{Preview: true})
	if err != nil {
		_, _ = s.Release(ctx, lease.Token)
		return SceneOutcome{}, err
	}
	outcome.Lease = lease
	return outcome, nil
}

/*
PreviewScene lights a scene without meaning it.

Not recorded, not reconciled over, and held by whoever asked for it. connection
says the caller is sitting on an open request and the lease should end when
that socket does; a caller that cannot do so gets an expiry to renew instead.
*/
func (s *Service) PreviewScene(ctx context.Context, name, holder string, connection bool) (SceneOutcome, error) {
	scene, err := s.Scene(name)
	if err != nil {
		return SceneOutcome{}, err
	}

	// The draft never going up means nothing is holding anything, which
	// Preview takes care of.
	return s.Preview(ctx, scene, holder, connection)
}

/*
light applies a scene's colours, effects and screen.

One pass, because a scene is applied as a unit: a machine lit halfway through
one is not a state anybody asked for.
*/
func (s *Service) light(ctx context.Context, scene scenes.Scene, req Request) (SceneOutcome, error) {
	outcome := SceneOutcome{Scene: scene.Name}

	assignments, problems := scene.Resolve()
	for _, problem := range problems {
		outcome.Problems = append(outcome.Problems, problem.Error())
	}

	/*
		Off is not a colour, so it is not an assignment.

		It resolves the device's own Off mode, falls back to black in Direct,
		and honours the correction for a keyboard where black is a dead
		backlight rather than darkness -- none of which a colour can ask for.
		A scene that is off ignores whatever colours it also carries.
	*/
	switch {
	case scene.Off:
		req.Off = true
	case scene.Colour != "":
		c, err := colour.Parse(scene.Colour)
		if err != nil {
			outcome.Problems = append(outcome.Problems, err.Error())
			break
		}
		req.Colour = &c
	}

	if req.Off || req.Colour != nil || len(assignments) > 0 || len(scene.Effects) > 0 {
		req.Assignments = assignments
		req.Effects = scene.Effects
		results, err := s.Apply(ctx, req)
		if err != nil {
			return SceneOutcome{}, err
		}
		outcome.Results = results
	}

	screen, problem := s.screen(ctx, scene)
	outcome.Screen = screen
	if problem != "" {
		outcome.Screen = ""
		outcome.Problems = append(outcome.Problems, problem)
	}
	return outcome, nil
}

/*
screen applies a scene's screen state, and says what it did.

A scene that says nothing about the screen changes nothing about it. That is
the only rule that makes the absent case predictable -- a lighting scene
written by somebody who never thought about the panel must not take the
dashboard away at midday -- and it is why this returns early rather than
defaulting to anything.

A machine with no cooler is not a failed scene. Neither is a missing image: the
colours are still what somebody asked for, and the file is reported so they
know which one to go and find.
*/
func (s *Service) screen(ctx context.Context, scene scenes.Scene) (string, string) {
	switch scene.Screen {
	case "":
		return "", ""
	case scenes.ScreenDashboard:
		return scene.Screen, s.draw(ctx, Screen{Dashboard: true})
	case scenes.ScreenReadout:
		s.drawing("the cooler's own readout")
		return scene.Screen, s.draw(ctx, Screen{Readout: true})
	}

	/*
		A scene can name a dashboard, which is how one keypress changes the
		lights and the screen together.

		Choosing it is a saved change rather than a momentary one: `hotaru
		dashboard use` is the same call, and a scene that put a dashboard up
		for as long as it was applied would leave the panel showing whatever
		the last scene happened to be when somebody stopped using scenes.
	*/
	if name, ok := strings.CutPrefix(scene.Screen, scenes.ScreenDashboardPrefix); ok {
		if err := s.UseDashboard(name); err != nil {
			return "", fmt.Sprintf("the screen: %v", err)
		}
		return scene.Screen, s.draw(ctx, Screen{Dashboard: true})
	}

	gif, err := os.ReadFile(scene.Screen) //nolint:gosec // a path the user saved
	if err != nil {
		return "", fmt.Sprintf("the screen: %v", err)
	}
	s.drawing("picture: " + pictureName(scene.Screen))
	return scene.Screen, s.draw(ctx, Screen{Image: gif})
}

// pictureName is a stored picture's name, from the path a scene keeps: what
// somebody called it, rather than where it ended up.
func pictureName(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

/*
draw sets the screen and says what went wrong, if anything did.

**A machine with no cooler is not a failed scene.** The nine shipped scenes
name a screen state because this desk has one, and most machines do not: absence
is an ordinary answer here as it is everywhere else in hotaru, and reporting it
would put a permanent complaint on every scene on every machine without a
panel. Found by a keypress reporting failure on a machine whose lights had just
done exactly what was asked.
*/
func (s *Service) draw(ctx context.Context, what Screen) string {
	err := s.Draw(ctx, what)
	switch {
	case err == nil, errors.Is(err, cooler.ErrNoCooler):
		return ""
	}
	return fmt.Sprintf("the screen: %v", err)
}

/*
covers is the devices a scene touches, as the server names them.

A scene says "keychron" and the hardware calls itself "Keychron K4 HE"; a lease
has to be held over the second, because that is what re-assertion and the
listing check against. A name matching nothing is not an error here: the scene
still applies what it can, and a preview over no devices is simply a preview
that suspends nothing.
*/
func (s *Service) covers(ctx context.Context, scene scenes.Scene) ([]string, error) {
	_, client, addr := s.current()
	if client == nil {
		return nil, unreachable(addr)
	}
	found, err := client.Devices(ctx)
	if err != nil {
		return nil, err
	}

	wanted := scene.Devices()
	var out []string
	for i := range found {
		if named(wanted, found[i].Name) {
			out = append(out, found[i].Name)
		}
	}
	return out, nil
}

// SceneFrom captures what the lights are showing now as a named scene, which
// is how the GUI will save one and how somebody keeps a colour they arrived at
// by fiddling.
func (s *Service) SceneFrom(name string, screen string) (scenes.Scene, error) {
	desired := s.Desired()
	scene := scenes.Scene{Name: name, Screen: screen, Effects: map[string]string{}}

	for _, device := range sorted(desired.Names()) {
		want := desired.Devices[device]
		frame := want.Frame(device)
		c, uniform := frame.Uniform()
		if !uniform {
			// Per-LED scenes are a listing of assignments the caller builds,
			// not something this shortcut can honestly reduce to one colour.
			return scenes.Scene{}, fmt.Errorf("%s is showing more than one colour; save it from the editor", device)
		}
		scene.Assignments = append(scene.Assignments, scenes.Assignment{
			Target: device, Colour: c.String(),
		})
		if want.Mode != "" {
			scene.Effects[device] = want.Mode
		}
	}
	if len(scene.Assignments) == 0 {
		return scenes.Scene{}, errors.New("nothing is recorded to save")
	}
	return scene, nil
}
