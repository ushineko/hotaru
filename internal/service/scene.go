package service

import (
	"context"
	"errors"
	"fmt"
	"os"

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
	return s.light(ctx, scene, Request{})
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
		// The draft never went up, so nothing is holding anything.
		_, _ = s.Release(ctx, lease.Token)
		return SceneOutcome{}, err
	}
	outcome.Lease = lease
	return outcome, nil
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

	if len(assignments) > 0 || len(scene.Effects) > 0 {
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
		if err := s.Draw(ctx, Screen{Dashboard: true}); err != nil {
			return "", fmt.Sprintf("the screen: %v", err)
		}
		return scenes.ScreenDashboard, ""
	case scenes.ScreenReadout:
		if err := s.Draw(ctx, Screen{Readout: true}); err != nil {
			return "", fmt.Sprintf("the screen: %v", err)
		}
		return scenes.ScreenReadout, ""
	}

	gif, err := os.ReadFile(scene.Screen) //nolint:gosec // a path the user saved
	if err != nil {
		return "", fmt.Sprintf("the screen: %v", err)
	}
	if err := s.Draw(ctx, Screen{Image: gif}); err != nil {
		return "", fmt.Sprintf("the screen: %v", err)
	}
	return scene.Screen, ""
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
