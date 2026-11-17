package gui

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/forms"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"

	"github.com/ushineko/hotaru/internal/api"
)

/*
Following, on a device's card (spec 077 R3.2, R3.9): what the device follows,
and a match of its pace to the leader's by eye, as `hotaru light follow` and
`hotaru light match` do. Both write the rules through the service, which owns
them.
*/

// noLeader is the Follows choice for a device set on its own.
const noLeader = "No one"

/*
follows is the card's Follows choice and, for a device that follows one,
Match pace. Choosing writes the rule at once, and a device given a leader
starts on the leader's effect.
*/
func (s *SystemSection) follows(sh *shell.Shell, device api.Device, all []api.Device) fyne.CanvasObject {
	options := []string{noLeader}
	for _, other := range all {
		if other.Name != device.Name && other.HandedTo == "" && other.LEDs > 0 {
			options = append(options, other.Name)
		}
	}
	choice := widget.NewSelect(options, nil)
	leader, found := leaderNamed(all, device)
	if found {
		choice.Selected = leader.Name
	} else {
		choice.Selected = noLeader
	}
	// Set after Selected, so building the card writes nothing.
	choice.OnChanged = func(picked string) {
		name := picked
		if name == noLeader {
			name = ""
		}
		sh.Perform("setting what "+device.Name+" follows", func(ctx context.Context) error {
			if _, err := s.app.client.Rule(ctx, api.RuleRequest{Device: device.Name, Follow: &name}); err != nil {
				return err
			}
			said := device.Name + " is set on its own now."
			if name != "" {
				said = device.Name + " follows " + name + " now."
			}
			onScreen(func() { sh.Flash(said, fd.StatusGood) })
			return nil
		})
	}

	row := []fyne.CanvasObject{widget.NewLabel("Follows"), choice}
	if found && len(pacedEffects(device, leader)) > 0 {
		match := widget.NewButtonWithIcon("Match pace…", theme.MediaPlayIcon(), func() {
			s.matchPace(sh, device, leader)
		})
		row = append(row, match)
	}
	return container.NewHBox(row...)
}

// leaderNamed is the device a device's rule follows, by any part of its
// name, as the service reads it.
func leaderNamed(all []api.Device, device api.Device) (api.Device, bool) {
	want := strings.ToLower(strings.TrimSpace(device.Follows))
	if want == "" {
		return api.Device{}, false
	}
	for _, other := range all {
		if other.Name != device.Name && other.HandedTo == "" && strings.Contains(strings.ToLower(other.Name), want) {
			return other, true
		}
	}
	return api.Device{}, false
}

/*
pacedEffects is what a device's pace can be matched on: the leader's own
effects the device draws a copy of. An effect the device runs from its own
firmware has no pace of hotaru's to trim.
*/
func pacedEffects(device, leader api.Device) []string {
	var out []string
	for _, name := range leader.Stored {
		if slices.Contains(device.Modes, name) && !slices.Contains(device.Stored, name) {
			out = append(out, name)
		}
	}
	return out
}

// paceOf is a device's pace for an effect, or 1.
func paceOf(device api.Device, effect string) float64 {
	if pace, ok := device.Pace[strings.ToLower(effect)]; ok && pace > 0 {
		return pace
	}
	return 1
}

/*
matchRequest runs an effect on a device and its leader together, from the
start, at a trial pace: a preview, so nothing is recorded, and the lights go
back when the dialog closes.
*/
func matchRequest(device, leader, effect string, pace float64) api.ApplyRequest {
	return api.ApplyRequest{
		Colour: "red", Devices: []string{device, leader}, Mode: effect, Exactly: true,
		Preview: true, Restart: true, Pace: map[string]float64{device: pace},
	}
}

// Pace limits as the rules take them; the slider covers the useful part.
const (
	slowestPace = 4.0
	fastestPace = 0.25
)

/*
matchPace opens the match: the effect on the device and its leader side by
side, a pace found by dragging or typed (2.16 when a step is too coarse),
Start again, and Save. Each change starts both again, so they are compared
from the same moment.
*/
func (s *SystemSection) matchPace(sh *shell.Shell, device, leader api.Device) {
	effects := pacedEffects(device, leader)
	effect := effects[0]
	if slices.Contains(effects, leader.ActiveMode) {
		effect = leader.ActiveMode
	}

	show := func(effect string, pace float64) {
		go func() {
			_, err := s.app.client.Apply(context.Background(), matchRequest(device.Name, leader.Name, effect, pace))
			if err != nil {
				onScreen(func() { sh.Flash(err.Error(), fd.StatusBad) })
			}
		}()
	}

	var pace *forms.SliderEntry
	pace = forms.NewSliderEntry(forms.SliderOptions{
		Min: fastestPace, Max: slowestPace, Step: 0.01, Value: paceOf(device, effect),
		Format: func(v float64) string { return fmt.Sprintf("%.2f", v) },
		Commit: func(v float64) {
			v = math.Max(0.01, math.Min(10, math.Round(v*100)/100))
			pace.Set(v)
			show(effect, v)
		},
	})
	pick := widget.NewSelect(effects, func(picked string) {
		effect = picked
		pace.Set(paceOf(device, picked))
		show(effect, pace.Value())
	})
	pick.Selected = effect
	again := widget.NewButtonWithIcon("Start both again", theme.MediaReplayIcon(), func() {
		show(effect, pace.Value())
	})

	body := container.NewVBox(
		widgets.Note(fmt.Sprintf("Watch %s beside %s. Higher is slower: 2 takes twice as long.",
			device.Name, leader.Name), fd.StatusInfo),
		container.NewBorder(nil, nil, widget.NewLabel("Effect"), nil, pick),
		container.NewBorder(nil, nil, widget.NewLabel("Pace"), nil, pace.Widget()),
		again,
	)
	ask := dialog.NewCustomConfirm("Match the pace", "Save", "Cancel", body, func(save bool) {
		chosen, name := pace.Value(), effect
		sh.Perform("matching "+device.Name+"'s pace", func(ctx context.Context) error {
			// Whatever happens, the lights go back to what was last asked.
			defer func() { _, _ = s.app.client.Reconcile(context.WithoutCancel(ctx)) }()
			if !save {
				return nil
			}
			if _, err := s.app.client.Rule(ctx, api.RuleRequest{
				Device: device.Name, Pace: map[string]float64{name: chosen},
			}); err != nil {
				return err
			}
			said := fmt.Sprintf("%s's pace for %s is %.2f.", device.Name, name, chosen)
			onScreen(func() { sh.Flash(said, fd.StatusGood) })
			return nil
		})
	}, sh.Window)
	ask.Resize(fyne.NewSize(520, ask.MinSize().Height))
	ask.Show()
	show(effect, pace.Value())
}
