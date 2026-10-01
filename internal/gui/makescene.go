package gui

import (
	"context"
	"fmt"
	"maps"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/images"
)

/*
Making a scene from something, and how far apart to push its colours.

One dialog for both sources -- a picture and a dashboard -- because the
question is the same one: what to call it, and how different the lights should
look from each other.

**The separation is a knob for the eye, not a correction with a right
answer.** What somebody picks and what an LED shows are not the same thing: a
strip's colour is filtered through a diffuser, a case window and whatever else
is lit in the room, and a set of colours that differ clearly in the picture
can arrive as one wash on the hardware. How far to push them apart depends on
the machine, the room and the person, and none of those is knowable from here.

Which is also why it can be changed afterwards: the thing being adjusted is
not on screen anywhere except the case itself, so nobody gets it right first
time.

**An effect that takes colours shows the picture's before the scene is made**
(spec 061 R5.3). Choosing one asks pick for as many as the mode takes, and
they appear under it, marked as picked; moving the separation picks them
again. Changing one makes it somebody's choice, which the scene keeps.
*/
func makeScene(
	sh *shell.Shell, from, suggest string, devices []api.Device,
	build func(ctx context.Context, name string, distance float64, effects map[string]api.Effect) (api.Scene, error),
	pick func(ctx context.Context, count int, distance float64) ([]string, error),
) {
	name := widget.NewEntry()
	name.SetText(suggest)

	/*
		And what each device should do with the colours.

		The picture answers "what colour is each light" and nothing else, so
		a keyboard that should ripple rather than sit still is a decision
		nobody could make here until now. Nothing is the default, which is
		every light showing the colours the picture gave it.
	*/
	effects := map[string]api.Effect{}

	distance := widget.NewSlider(1, images.MostDistance)
	distance.Step = 0.1
	distance.Value = 1
	shown := widgets.Dim(separationSaid(1))
	distance.OnChanged = func(v float64) { shown.(*widget.Label).SetText(separationSaid(v)) }

	fields := container.NewVBox()
	var draw func()
	picking := picker{sh: sh, from: from, devices: devices, effects: effects, pick: pick}
	set := func(device string, effect api.Effect) {
		if !effect.Named() {
			delete(effects, device)
			return
		}
		was := effects[device]
		effects[device] = effect
		fromPicture := len(effect.Palette()) == 0 || effect.ColoursFrom == api.ColoursFromPicture
		if effect.Mode != was.Mode && fromPicture {
			picking.again(distance.Value, []string{device}, draw)
		}
	}
	draw = func() {
		fields.Objects = []fyne.CanvasObject{effectFields(sh.Window, devices,
			func(device string) api.Effect { return effects[device] }, set)}
		fields.Refresh()
	}
	draw()
	distance.OnChangeEnded = func(v float64) { picking.again(v, picking.picked(), draw) }

	body := container.NewVBox(
		labelled("Name", name),
		labelled("Separation", container.NewBorder(nil, nil, nil, shown, distance)),
		widgets.DimWrapped("Colours that differ clearly in the picture can arrive as one "+
			"wash on the lights. Push them further apart until the case looks right; "+
			"you can change it afterwards."),
		widget.NewSeparator(),
		fields,
	)

	ask := dialog.NewCustomConfirm("Make a scene from "+from, "Make it", "Cancel", body,
		func(ok bool) {
			if !ok || name.Text == "" {
				return
			}
			made := name.Text
			apart := distance.Value
			chosen := maps.Clone(effects)
			sh.Perform("reading "+from, func(ctx context.Context) error {
				scene, err := build(ctx, made, apart, chosen)
				if err != nil {
					return err
				}
				onScreen(func() {
					sh.Flash(fmt.Sprintf("%s: %d assignment(s). It is in Scenes.",
						scene.Name, len(scene.Assignments)), fd.StatusGood)
					sh.Invalidate()
				})
				return nil
			})
		}, sh.Window)
	ask.Resize(fyne.NewSize(sceneDialogWidth, sceneDialogHeight))
	ask.Show()
}

/*
picker asks for the picture's colours for the effects that take them, and
writes them into the dialog's effects.

A request is answered by whichever effects still want it when it returns: a
device whose mode changed, or whose colours somebody chose, in the meantime is
left as it now is.
*/
type picker struct {
	sh      *shell.Shell
	from    string
	devices []api.Device
	effects map[string]api.Effect
	pick    func(ctx context.Context, count int, distance float64) ([]string, error)
}

// picked is every device whose effect holds colours picked from the picture.
func (p picker) picked() []string {
	var out []string
	for device, effect := range p.effects {
		if effect.ColoursFrom == api.ColoursFromPicture {
			out = append(out, device)
		}
	}
	return out
}

// again picks colours for these devices at this distance, and redraws.
func (p picker) again(distance float64, devices []string, redraw func()) {
	if p.pick == nil {
		return
	}
	for _, name := range devices {
		effect := p.effects[name]
		device, ok := deviceNamed(p.devices, name)
		if !ok || !effect.Named() {
			continue
		}
		slots, takes := colourSlots(device, effect.Mode)
		if !takes {
			continue
		}
		mode := effect.Mode
		p.sh.Perform("reading colours from "+p.from, func(ctx context.Context) error {
			colours, err := p.pick(ctx, slots.Most, distance)
			if err != nil {
				return err
			}
			onScreen(func() {
				now := p.effects[name]
				wants := now.Mode == mode && (len(now.Palette()) == 0 || now.ColoursFrom == api.ColoursFromPicture)
				if !wants || len(colours) == 0 {
					return
				}
				now.SetPalette(colours)
				now.ColoursFrom = api.ColoursFromPicture
				p.effects[name] = now
				redraw()
			})
			return nil
		})
	}
}

// deviceNamed is a device from the dialog's list by its name.
func deviceNamed(devices []api.Device, name string) (api.Device, bool) {
	for _, d := range devices {
		if d.Name == name {
			return d, true
		}
	}
	return api.Device{}, false
}

// The dialog's size. Wide enough that the slider is worth dragging and the
// note under it is two lines rather than six.
const (
	sceneDialogWidth  = 560
	sceneDialogHeight = 460
)

/*
separationSaid is the slider's number in words as well as figures.

"1.0" says nothing to somebody who has not read the code. What they are
choosing is how different the lights look, so that is what it says.
*/
func separationSaid(distance float64) string {
	switch {
	case distance < 1.15:
		return fmt.Sprintf("%.1f — as the picture is", distance)
	case distance < 2:
		return fmt.Sprintf("%.1f — a little further apart", distance)
	case distance < 2.6:
		return fmt.Sprintf("%.1f — clearly different", distance)
	}
	return fmt.Sprintf("%.1f — as far as it goes", distance)
}

// MakeScene opens the dialog, for tests: it is reached by a button on a
// picture or a dashboard, and a test that pressed it would be about the card.
func MakeScene(
	sh *shell.Shell, from string, devices []api.Device,
	build func(ctx context.Context, name string, distance float64, effects map[string]api.Effect) (api.Scene, error),
	pick func(ctx context.Context, count int, distance float64) ([]string, error),
) {
	makeScene(sh, from, from, devices, build, pick)
}
