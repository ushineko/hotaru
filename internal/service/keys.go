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

/*
Shortcuts is the desktop's copy of the bindings, where the desktop keeps one.

KWin's script carries the scene name in the call it makes, not the key -- so
the binding a keypress acts on is the one that was written into the script
when it was installed, and changing the store changes nothing until the script
is written again. The service knows when the store changed; the daemon knows
how to install. This is the seam.
*/
type Shortcuts interface {
	// Install writes the script and registers what it holds, returning how
	// many shortcuts it registered.
	Install() (int, error)
}

/*
SetShortcuts gives the service a way to put the bindings on the desktop, and
somewhere to say when it could not.

A machine with no session bus, or a build with no desktop integration, has
neither and binds into the file alone.
*/
func (s *Service) SetShortcuts(k Shortcuts, report func(string, ...any)) {
	s.mu.Lock()
	s.shortcuts, s.report = k, report
	s.mu.Unlock()
}

/*
reinstall puts the changed bindings on the desktop, and does not fail a bind
when it cannot.

The store is the record and the script is a copy of it. A machine whose KWin
is not running has a correct file and no shortcuts, which is the state it is
already in and the one the installer resolves when KWin next appears -- so a
bind that could not reach the desktop is still a bind.
*/
func (s *Service) reinstall() {
	s.mu.RLock()
	keys, report := s.shortcuts, s.report
	s.mu.RUnlock()

	if keys == nil {
		return
	}
	count, err := keys.Install()
	switch {
	case err != nil && report != nil:
		report("hotaru: keys: %v", err)
	case report != nil:
		report("hotaru: keys: %d shortcuts registered", count)
	}
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
	if err := store.Bind(key, scene); err != nil {
		return err
	}
	/*
		And on the desktop, because the script carries the scene name rather
		than the key. Without this a rebinding was written to the file and
		the old scene kept firing until the service or KWin restarted --
		which is how it was reported: "changing the hotkey didn't take
		effect".
	*/
	s.reinstall()
	return nil
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
