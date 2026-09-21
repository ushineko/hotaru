package gui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"
	"github.com/ushineko/hotaru/internal/api"
)

/*
Binding a key, from the row that shows it.

The key was already the first thing on a scene's line -- it is what somebody
recognises a scene by, because the bank has been on this numpad for years --
and it was the one thing on that line they could not change without the
terminal.

Every key is offered, not only the free ones. A key already pointing at
another scene is a legitimate thing to choose: somebody rearranging their bank
is moving keys between scenes, and a chooser that hid the taken ones would
hide the rearrangement. What it holds is shown beside it, so the cost of
taking it is on screen.

The nine unshifted keys are hotaru's own bank and the nine shifted ones are
left free for somebody's own scenes (spec 016). Both are offered: the shipped
bank is a default, not a reservation the other way round.
*/
func (s *ScenesSection) bind(sh *shell.Shell, scene api.Scene, current string) {
	got := s.app.machine.Read()
	held := map[string]string{}
	for _, binding := range got.Keys.Bindings {
		held[binding.Key] = binding.Scene
	}

	keys := keyBank(got.Keys)
	labels := make([]string, 0, len(keys)+1)
	labels = append(labels, noKey)
	for _, key := range keys {
		labels = append(labels, keyLabel(key, held[key], scene.Name))
	}

	/*
		And a key that is not in the bank at all.

		Every key hotaru ships is on the numpad, because that is what this
		desk has had for years -- so a machine without one inherits eighteen
		shortcuts it cannot press and, until this, a chooser offering
		eighteen more of them. A laptop, a keyboard with no numeric pad, or a
		desktop whose keys arrive over the network are all that machine.

		The service has always taken any sequence KDE spells; it was the
		window that only offered its own list.
	*/
	typed := widget.NewEntry()
	typed.SetPlaceHolder("Meta+Shift+L")
	typed.Disable()

	labels = append(labels, someKey)

	chooser := widget.NewRadioGroup(labels, func(chosen string) {
		if chosen == someKey {
			typed.Enable()
			return
		}
		typed.Disable()
	})
	chooser.Selected = noKey
	if current != "" {
		chooser.Selected = keyLabel(current, held[current], scene.Name)
	}
	if current != "" && !inBank(current, keys) {
		// A key bound from the terminal, or from this dialog last time.
		chooser.Selected, typed.Text = someKey, current
		typed.Enable()
	}
	chooser.Required = true

	body := container.NewVBox(
		widgets.DimWrapped("The nine plain ones are hotaru's own bank; the nine with Shift "+
			"are left free for scenes you write. Taking a key from another scene leaves "+
			"that one without a key."),
		chooser,
		typed,
		widgets.DimWrapped("Spelled the way KDE spells it: Ctrl, Alt, Shift, Meta and the "+
			"key, joined with +. A numpad key is Num+1."),
	)

	/*
		Shown roomy, for the reason the file chooser was.

		A scroller's minimum size is almost nothing, so a dialog built around
		one opens at the size of its buttons: eighteen keys were offered
		through a slot showing one and a half of them, clipped at both ends.
		Fyne sizes a dialog to its content and a scroller declines to say how
		big it would like to be.
	*/
	ask := dialog.NewCustomConfirm("Shortcut for "+scene.Name, "Bind", "Cancel",
		container.NewVScroll(body), func(ok bool) {
			if !ok {
				return
			}
			key := keyOf(chooser.Selected, keys, held)
			if chooser.Selected == someKey {
				key = strings.TrimSpace(typed.Text)
				if key == "" {
					sh.Flash("That needs a key to bind.", fd.StatusWarn)
					return
				}
			}
			s.rebind(sh, scene.Name, key, current)
		}, sh.Window)
	roomy(ask, sh)
}

// BindKey opens the chooser, for tests: it is reached by clicking the key on
// a scene's row, and a test that clicked it would be a test about buttons.
func BindKey(s *ScenesSection, sh *shell.Shell, scene api.Scene, current string) {
	s.bind(sh, scene, current)
}

// noKey is the first option, and what a scene with no shortcut shows as.
const noKey = "no shortcut"

// someKey is the last option: a sequence somebody types, for a machine whose
// keyboard hotaru's bank does not fit.
const someKey = "something else"

// inBank says whether a key is one the chooser already lists, which is how a
// binding made at the terminal finds its way into the entry rather than
// looking like no shortcut at all.
func inBank(key string, keys []string) bool {
	for _, one := range keys {
		if one == key {
			return true
		}
	}
	return false
}

/*
keyBank is every key somebody may bind, in the order the numpad is laid out.

From what the service reported rather than from a list here: the bank is the
service's, and a window with its own copy would be a second answer the day one
of them changed.
*/
func keyBank(keys api.KeysResponse) []string {
	seen := map[string]bool{}
	var out []string
	for _, binding := range keys.Bindings {
		if !seen[binding.Key] {
			seen[binding.Key] = true
			out = append(out, binding.Key)
		}
	}
	for _, key := range keys.Reserved {
		if !seen[key] {
			seen[key] = true
			out = append(out, key)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		// Unshifted first, then by the number on the key.
		if shifted(out[i]) != shifted(out[j]) {
			return !shifted(out[i])
		}
		return out[i] < out[j]
	})
	return out
}

// keyLabel is one line of the chooser: the key, and what it holds.
func keyLabel(key, holder, mine string) string {
	switch holder {
	case "":
		return fmt.Sprintf("%s — free", spell(key))
	case mine:
		return fmt.Sprintf("%s — this scene", spell(key))
	}
	return fmt.Sprintf("%s — %s", spell(key), holder)
}

/*
spell writes a key out in full.

`pretty` is for the column on the left of a scene's row, where eighteen keys
have to be told apart at a glance and the only things that differ are the
number and whether Shift is held. A chooser is the other case: somebody is
about to press this, once, and needs to know what to press.

"Num+1" is how KDE stores it and not how anybody says it.
*/
func spell(key string) string {
	return strings.ReplaceAll(key, "Num+", "Numpad ")
}

// keyOf reads a chosen line back to the key it names.
func keyOf(chosen string, keys []string, held map[string]string) string {
	for _, key := range keys {
		if keyLabel(key, held[key], "") == chosen ||
			keyLabel(key, held[key], held[key]) == chosen {
			return key
		}
	}
	return ""
}

/*
rebind points a key at a scene, and takes it off whatever had it.

Two calls where it looks like one. The service binds a key to a scene; it does
not know that a scene should have at most one key, because a key is the thing
being bound and the scene is what it points at. So moving a scene's shortcut
is unbinding the old one and binding the new, and this is the only place that
knows both halves.
*/
func (s *ScenesSection) rebind(sh *shell.Shell, scene, key, was string) {
	if key == was {
		return
	}
	sh.Perform("binding "+scene, func(ctx context.Context) error {
		if was != "" {
			if err := s.app.client.Bind(ctx, was, ""); err != nil {
				return err
			}
		}
		if key != "" {
			if err := s.app.client.Bind(ctx, key, scene); err != nil {
				return err
			}
		}
		onScreen(func() {
			sh.Flash(said(scene, key), fd.StatusGood)
			sh.Invalidate()
		})
		return nil
	})
}

// said is what happened, in the words somebody would use.
func said(scene, key string) string {
	if key == "" {
		return scene + " has no shortcut now."
	}
	return pretty(key) + " applies " + scene + "."
}

// KeyBank is the chooser's list of keys, for tests.
func KeyBank(keys api.KeysResponse) []string { return keyBank(keys) }
