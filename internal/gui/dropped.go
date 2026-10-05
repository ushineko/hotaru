package gui

import (
	"context"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/hotaru/internal/api"
)

/*
A picture dropped on Screen or Scenes, and what it becomes there (spec 062).

The library keeps it first, with the same questions a drop on Pictures asks.
Then the window offers the saved screens as layouts, each drawn with the
picture behind it, and goes on to what the tab makes: a screen in the editor,
or a scene through Make a scene.

**The saved screens are the layouts.** A screen is more than its arrangement:
it is the readings in each slot, the theme and the lettering somebody already
chose. Copying one and putting the picture behind it is what making
`custom1` by hand was.
*/

// droppedOn is the part of Create a drop belongs to, when it is one that
// makes something from a picture and is not in the middle of an edit.
func (a *App) droppedOn(sh *shell.Shell) Section {
	if a.create == nil || sh.Current() != shell.Section(a.create) {
		return nil
	}
	part := a.create.current()
	if busy, ok := part.(Busy); ok && busy.Busy() {
		return nil
	}
	switch part.(type) {
	case *DashboardsSection, *ScenesSection:
		return part
	}
	return nil
}

// fromPicture is what Screen does with a picture that was dropped on it.
func (d *DashboardsSection) fromPicture(sh *shell.Shell, picture api.Image) {
	boards, err := d.app.client.Dashboards(context.Background())
	if err != nil {
		sh.Flash("Cannot list the screens: "+err.Error(), fd.StatusBad)
		return
	}
	chooseLayout(sh, d.app, boards, picture, false, func(template *api.Dashboard) {
		draft := withPicture(*template, picture)
		d.editing, d.name = &draft, unusedName(boards.Dashboards, picture.Name)
		d.frame, d.cost = nil, ""
		sh.Invalidate()
	})
}

/*
fromPicture is what Scenes does with a picture that was dropped on it.

The screen is saved only when "Make it" is pressed, so a cancelled dialog
leaves nothing behind. Its name is chosen then too, against the screens as
they are at that moment.
*/
func (s *ScenesSection) fromPicture(sh *shell.Shell, picture api.Image) {
	boards, err := s.app.client.Dashboards(context.Background())
	if err != nil {
		sh.Flash("Cannot list the screens: "+err.Error(), fd.StatusBad)
		return
	}
	chooseLayout(sh, s.app, boards, picture, true, func(template *api.Dashboard) {
		client := s.app.client
		makeScene(sh, picture.Name, picture.Name, s.app.machine.Read().Devices,
			func(ctx context.Context, name string, distance float64, effects map[string]api.Effect) (api.Scene, error) {
				if template == nil {
					return client.SceneFromImage(ctx, picture.Name, name, distance, effects)
				}
				now, err := client.Dashboards(ctx)
				if err != nil {
					return api.Scene{}, err
				}
				screen := withPicture(*template, picture)
				screen.Name = unusedName(now.Dashboards, picture.Name)
				if err := client.SaveDashboard(ctx, screen); err != nil {
					return api.Scene{}, err
				}
				s.forgetScreens()
				return client.SceneFromImageOn(ctx, picture.Name, name,
					api.ScreenDashboardPrefix+screen.Name, distance, effects)
			},
			func(ctx context.Context, count int, distance float64) ([]string, error) {
				return client.PickColours(ctx, picture.Name, count, distance)
			})
	})
}

/*
withPicture is a saved screen as the start of a new one: its layout, readings
and lettering, with the picture behind them.

The dim is the template's. A template with no picture behind it has none, and
none means the default, which is what a screen with a picture added in the
editor gets.
*/
func withPicture(template api.Dashboard, picture api.Image) api.Dashboard {
	out := template
	out.Name, out.Shipped = "", false
	out.Background = api.DashboardBackground{
		Kind: "picture", Picture: picture.Name, Dim: template.Background.Dim,
	}
	return out
}

// unusedName is the picture's name, or the first of name-2, name-3 and on
// that no screen has. Saving over a screen of the same name would replace it.
func unusedName(boards []api.Dashboard, name string) string {
	taken := map[string]bool{}
	for _, one := range boards {
		taken[one.Name] = true
	}
	if !taken[name] {
		return name
	}
	for i := 2; ; i++ {
		if try := fmt.Sprintf("%s-%d", name, i); !taken[try] {
			return try
		}
	}
}

// pictureAlone is the chooser's option for a scene that shows the picture
// itself, which is what Make a scene from Pictures does.
const pictureAlone = "the picture alone"

/*
chooseLayout asks which saved screen to start from, with the picture drawn
behind each one.

The shape of the scene editor's screen chooser: a radio group with the
pictures beside it at its own pitch. The thumbnails are the service's render
of each screen with this picture in it, so the choice is made looking at the
result rather than at the screen it was copied from.

chosen gets nil for "the picture alone", which is offered only when alone is
set.
*/
func chooseLayout(
	sh *shell.Shell, app *App, boards api.DashboardsResponse, picture api.Image, alone bool,
	chosen func(*api.Dashboard),
) {
	labels := make([]string, 0, len(boards.Dashboards)+1)
	shots := make([]fyne.CanvasObject, 0, len(boards.Dashboards)+1)
	templates := make([]*api.Dashboard, 0, len(boards.Dashboards)+1)
	if alone {
		labels = append(labels, pictureAlone)
		shots = append(shots, pictureShot(picture))
		templates = append(templates, nil)
	}
	selected := ""
	for i := range boards.Dashboards {
		one := boards.Dashboards[i]
		label := LayoutLabel(one)
		if one.Name == boards.Active && selected == "" && !alone {
			selected = label
		}
		labels = append(labels, label)
		shots = append(shots, dashboardShot(app, withPicture(one, picture)))
		templates = append(templates, &one)
	}
	if len(labels) == 0 {
		sh.Flash("There are no screens to start from.", fd.StatusWarn)
		return
	}
	if selected == "" {
		selected = labels[0]
	}

	picked := selected
	list := widget.NewRadioGroup(labels, func(chosen string) { picked = chosen })
	list.Required = true
	list.Selected = selected

	// The pictures beside the list at its pitch, as chooseScreen does and
	// for the reason given there.
	pitch := list.MinSize().Height / float32(len(labels))
	rows := make([]fyne.CanvasObject, 0, len(shots))
	for _, shot := range shots {
		rows = append(rows, container.NewCenter(sized(pitch-theme.Padding()*2, shot)))
	}
	body := container.NewBorder(nil, nil,
		container.New(Beside{Pitch: pitch}, rows...), nil,
		container.NewVBox(list))

	ask := dialog.NewCustomConfirm("A layout for "+picture.Name, "Use it", "Cancel",
		container.NewVScroll(body), func(ok bool) {
			if !ok {
				return
			}
			for i, label := range labels {
				if label == picked {
					chosen(templates[i])
					return
				}
			}
		}, sh.Window)
	roomy(ask, sh)
}

// LayoutLabel is how the layout chooser names a saved screen, for tests that
// pick one.
func LayoutLabel(one api.Dashboard) string {
	return one.Name + " (" + arrangementName(one) + ")"
}

// PictureAlone is the layout chooser's option for the picture itself, for
// tests.
const PictureAlone = pictureAlone

// shownScreen is the saved screen a scene puts on the panel, if it names one.
func shownScreen(scene api.Scene) (string, bool) {
	return strings.CutPrefix(scene.Screen, api.ScreenDashboardPrefix)
}
