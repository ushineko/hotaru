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
*/
func makeScene(
	sh *shell.Shell, from, suggest string, devices []api.Device,
	build func(ctx context.Context, name string, distance float64, effects map[string]api.Effect) (api.Scene, error),
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

	body := container.NewVBox(
		labelled("Name", name),
		labelled("Separation", container.NewBorder(nil, nil, nil, shown, distance)),
		widgets.DimWrapped("Colours that differ clearly in the picture can arrive as one "+
			"wash on the lights. Push them further apart until the case looks right; "+
			"you can change it afterwards."),
		widget.NewSeparator(),
		effectFields(sh.Window, devices,
			func(device string) api.Effect { return effects[device] },
			func(device string, effect api.Effect) {
				if !effect.Named() {
					delete(effects, device)
					return
				}
				effects[device] = effect
			}),
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
