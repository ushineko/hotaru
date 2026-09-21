package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ushineko/hotaru/internal/desktop"
	"github.com/ushineko/hotaru/internal/scenes"
)

/*
Keys are the shortcuts, what they apply, and what is in their way.

One call rather than three, because the useful question is never "what is
bound?" on its own: a key bound to a scene that does not exist, and a key
another program still claims, both look exactly like a working binding from
anywhere else.
*/
type Keys struct {
	Bindings []Binding
	// Reserved are sequences hotaru leaves free for somebody's own scenes.
	Reserved []string
	// Desktop says whether the KWin integration is running, and why not.
	Desktop string
	// Claimed are entries another program holds on hotaru's sequences.
	Claimed []desktop.Claim
}

// Binding is one key and the scene it applies.
type Binding struct {
	Key   string
	Scene string
	// Missing marks a binding whose scene is not there. Pressing it reports
	// rather than lighting something, and saying so before it is pressed is
	// the point of this field.
	Missing bool
}

// SetDesktop records what the hotkey integration is doing, for reporting. An
// empty reason means it is installed and working.
func (s *Service) SetDesktop(state string) {
	s.mu.Lock()
	s.desktop = state
	s.mu.Unlock()
}

// Keys reports the shortcuts and everything that would stop them working.
func (s *Service) Keys() (Keys, error) {
	store, err := s.sceneStore()
	if err != nil {
		return Keys{}, err
	}

	known := map[string]bool{}
	for _, scene := range store.All() {
		known[scene.Name] = true
	}

	bound := store.Bindings()
	out := Keys{Reserved: scenes.Reserved()}
	for key, scene := range bound {
		out.Bindings = append(out.Bindings, Binding{
			Key: key, Scene: scene, Missing: !known[scene],
		})
	}
	sort.Slice(out.Bindings, func(i, j int) bool { return out.Bindings[i].Key < out.Bindings[j].Key })

	s.mu.RLock()
	out.Desktop = s.desktop
	s.mu.RUnlock()

	/*
		What else holds these sequences.

		Read-only. The file belongs to the desktop, and a claim is reported so
		somebody can decide -- a key that is claimed elsewhere registers
		successfully here and then does nothing, which is the failure this
		whole area keeps producing.
	*/
	if path, err := desktop.ShortcutsFile(); err == nil {
		/*
			The reserved row is checked as well as the bound keys.

			On the machine this was written for, the program hotaru replaces
			had nine entries on Ctrl+Alt+Shift+Num1..9 for its animation bank.
			Those keys are free as far as hotaru is concerned and are not free
			at all: somebody binding their own scene there would register
			successfully and press a key that does nothing, which is this
			whole fault a second time with nothing to point at it.
		*/
		wanted := make([]string, 0, len(bound)+len(out.Reserved))
		for key := range bound {
			wanted = append(wanted, key)
		}
		wanted = append(wanted, out.Reserved...)
		if claims, err := desktop.Claimed(path, wanted); err == nil {
			out.Claimed = claims
		}
	}
	return out, nil
}

// Bind points a key at a scene. An empty scene name unbinds it, including one
// of the shipped keys.
func (s *Service) Bind(key, scene string) error {
	store, err := s.sceneStore()
	if err != nil {
		return err
	}
	if scene != "" {
		if _, err := store.Get(scene); err != nil {
			return err
		}
	}
	return store.Bind(key, scene)
}

/*
ApplyByName is the door's entry point, and is deliberately the same call the
HTTP route makes.

One flow, two doors. A keypress that took a different path through the service
would be a second implementation of applying a scene, which is how "the key
does something slightly different from the button" starts.

It returns a line for the journal rather than only an error, because a key that
fired and did something odd and a key that never fired are otherwise the same
observation -- and a scene's own problems, a missing image or an effect a
device does not have, are not failures to apply it.
*/
func (s *Service) ApplyByName(ctx context.Context, name string) (string, error) {
	outcome, err := s.ApplyScene(ctx, name)
	if err != nil {
		return "", err
	}

	lit := 0
	for _, result := range outcome.Results {
		if result.Applied {
			lit++
		}
	}
	said := fmt.Sprintf("%s: %d of %d device(s)", outcome.Scene, lit, len(outcome.Results))
	if outcome.Screen != "" {
		said += ", screen " + outcome.Screen
	}
	if len(outcome.Problems) > 0 {
		said += "; " + strings.Join(outcome.Problems, "; ")
	}
	return said, nil
}
