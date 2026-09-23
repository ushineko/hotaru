package gui

import (
	"context"
	"fmt"
	"image/color"
	"path/filepath"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fd "github.com/ushineko/fynedesygn"
	"github.com/ushineko/fynedesygn/shell"
	"github.com/ushineko/fynedesygn/widgets"
	"github.com/ushineko/hotaru/internal/api"
)

/*
ScenesSection is the saved scenes, and the editor over a draft.

The reason the window exists. `kraken/ring[12:23]` is a thing somebody can type
and a thing they have to work out first, by lighting LEDs and looking at the
case; here it is a fan they click.
*/
type ScenesSection struct {
	app *App

	// draft is what is being edited, nil when nothing is.
	draft *Draft
	// picked is what the next colour lands on: one light, or forty.
	picked Selection
	/*
		scroller is kept for the same kind of reason, and a sharper one.

		A section is a build function and the shell rebuilds it whole, so a
		scroller built inside one starts at the top every time -- and a list
		that jumps to the top while somebody is reading it is a list they
		cannot use. Keeping it means keeping the offset.
	*/
	scroller *container.Scroll
	rows     *fyne.Container
	// kept holds the chooser's list of screens between openings.
	kept

	// scrolling says which view the scroller belongs to. The list and the
	// editor are different material, so an offset from one is meaningless in
	// the other and carrying it over reads as the window losing its place.
	showing string
}

// OpenScenes gives a section its app, which the shell normally does. For
// tests, like OpenEditor.
func OpenScenes(s *ScenesSection, app *App) { s.app = app }

// ChooseEffects opens the effects chooser, for tests: it is reached by a
// button in a card, and a test that clicked it would be a test about cards.
func ChooseEffects(s *ScenesSection, sh *shell.Shell, got Snapshot) {
	s.chooseEffects(sh, got)
}

// ShareStyle opens the chooser that gives other scenes this one's effects,
// for tests: it is reached by a button inside a dialog.
func ShareStyle(s *ScenesSection, sh *shell.Shell, got Snapshot) { s.share(sh, got) }

// ChooseScreen opens the chooser, for tests: it is reached by a button in a
// card, and a test that clicked it would be a test about the card.
func ChooseScreen(s *ScenesSection, sh *shell.Shell) {
	s.chooseScreen(sh, screenOf(s.draft))
}

// Rebind moves a scene's shortcut, for tests: the dialog that calls it is a
// dialog, and a test that drove it would be a test about radio buttons.
func (s *ScenesSection) Rebind(sh *shell.Shell, scene, key, was string) {
	s.rebind(sh, scene, key, was)
}

/*
OpenEditor puts a section into its editing state.

For tests. The editor is reached by clicking a button, and a test that has to
find and press it is a test about buttons rather than about what the editor
does with what it is given.
*/
func OpenEditor(s *ScenesSection, app *App, draft *Draft) {
	s.app, s.draft = app, draft
	s.picked.Clear()
}

/*
Busy reports that the editor is open.

While it is, a poll that finds the machine changed leaves this section alone:
the draft, the selection and the picker are the user's work in progress, and a
rebuild loses all three.
*/
func (s *ScenesSection) Busy() bool { return s.draft != nil }

/*
Changed says what this section draws: the scenes, and the keys they sit on.

Not the cooling, which moves on every poll and is the reason a list of eighteen
scenes jumped under the pointer every two seconds.
*/
func (s *ScenesSection) Changed(before, after Snapshot) bool {
	if len(before.Scenes) != len(after.Scenes) ||
		len(before.Keys.Bindings) != len(after.Keys.Bindings) {
		return true
	}
	for i := range after.Scenes {
		if before.Scenes[i].Name != after.Scenes[i].Name {
			return true
		}
	}
	for i := range after.Keys.Bindings {
		if before.Keys.Bindings[i] != after.Keys.Bindings[i] {
			return true
		}
	}
	return (before.Err == nil) != (after.Err == nil)
}

// OpenList puts a section back to its listing, which is where it starts. For
// tests, like OpenEditor.
func OpenList(s *ScenesSection, app *App) {
	s.app, s.draft = app, nil
	s.picked.Clear()
}

// Select picks spots, as clicking them does. For tests.
func Select(s *ScenesSection, spots ...Spot) {
	for _, spot := range spots {
		s.picked.Toggle(spot)
	}
}

// SetColour puts a colour on the selection, as the picker does. For tests.
func SetColour(s *ScenesSection, colour string) { s.set(colour) }

// EditLine selects what one of the scene's own lines names, as its Change
// button does. For tests.
func EditLine(s *ScenesSection, target string) {
	s.picked.Clear()
	s.picked.Toggle(spotOf(target))
}

// Title is the name in the navigation.
func (s *ScenesSection) Title() string { return "Scenes" }

// Icon is the navigation's icon for this section.
func (s *ScenesSection) Icon() fyne.Resource { return theme.ColorPaletteIcon() }

// Build draws the list, or the editor when one is open.
func (s *ScenesSection) Build(sh *shell.Shell) fyne.CanvasObject {
	got := s.app.machine.Read()
	if got.Err != nil {
		return notRunning(got.Err)
	}
	if s.draft != nil {
		return s.editor(sh, got)
	}
	return s.list(sh)
}

/*
Detach does nothing, on purpose, and that is the whole of what it is for.

**The shell detaches a section before *every* rebuild**, not only when it is
replaced, and this window rebuilds every two seconds. So anything ended here is
ended a couple of times a minute while somebody is working.

It has been the wrong home twice. Ending the preview here put a draft up and
took it down within one frame -- the button worked, the lights flashed, and the
cause was in a method that looks like it runs at the end of something. Stopping
the colour sender here would drop it under a pointer that was still dragging.

Both of those end when somebody dismisses the window showing them, or when the
program stops. Both are explicit, and neither is a rebuild.
*/
func (s *ScenesSection) Detach() {}

/*
Arrive is the navigation coming to this section, which is the only moment the
scroll position should be forgotten.

**Not Detach.** The shell detaches before every rebuild, so resetting there
threw the picture back to the top on every click -- the same trap that made the
preview flash, in a different method. Detach means "you may be about to be
replaced"; Arrive means "somebody just came here".
*/
func (s *ScenesSection) Arrive() {
	s.scroller, s.rows, s.showing = nil, nil, ""

	// And the chooser's list is fetched again, off the thread drawing this,
	// because a picture added in another tab belongs in it.
	s.forgetScreens()
	go s.warmScreens()
}

func (s *ScenesSection) list(sh *shell.Shell) fyne.CanvasObject {
	add := widget.NewButtonWithIcon("New scene", theme.ContentAddIcon(), func() {
		s.draft = NewDraft()
		s.picked.Clear()
		sh.Invalidate()
	})
	add.Importance = widget.HighImportance

	got := s.app.machine.Read()
	scenes, keys := byKey(got.Scenes, got.Keys)

	rows := make([]fyne.CanvasObject, 0, len(scenes))
	for _, scene := range scenes {
		rows = append(rows, s.row(sh, scene, keys[scene.Name]))
	}

	/*
		The control is affixed and the scenes scroll under it.

		fynedesygn's rule, and its reason is sharper than "keep it visible":
		Fyne hands a wheel event to the innermost scroller under the pointer
		and does not pass it on, so a control below a long list is not
		inconvenient, it is unreachable. Eighteen scenes deep, somebody opened
		this window and asked where the buttons were.
	*/
	return container.NewBorder(
		container.NewVBox(add), nil, nil, nil,
		s.scrolling("list", rows),
	)
}

/*
scrolling refills the section's scroller, keeping where it was.

The scroller and its content live on the section rather than being built fresh,
because the shell rebuilds a section whole and a new scroller starts at the
top. Refilling and restoring the offset is what makes a rebuild invisible to
somebody halfway down a list.
*/
func (s *ScenesSection) scrolling(what string, rows []fyne.CanvasObject) fyne.CanvasObject {
	if s.scroller == nil || s.showing != what {
		s.rows = container.NewVBox(rows...)
		s.scroller = container.NewVScroll(s.rows)
		s.showing = what
		return s.scroller
	}

	/*
		Refill, then scroll back -- in that order, and through
		ScrollToOffset.

		A scroller clamps its offset against the size of what it holds, so
		restoring the offset before the new content is in place clamps it to
		nothing. Setting the field directly has the same problem from the
		other side: it skips the clamp entirely and can leave the scroller
		showing past the end of its own content.
	*/
	was := s.scroller.Offset
	s.rows.Objects = rows
	s.rows.Refresh()
	s.scroller.Refresh()
	s.scroller.ScrollToOffset(was)
	return s.scroller
}

/*
byKey orders the scenes the way somebody's hands know them.

The nine on the numpad first, in key order, then the nine on the shifted row,
then everything nobody has bound -- alphabetically, because there is nothing
better to sort them by. An alphabetical list of eighteen puts "blue" above
"cats" and the keyboard's own order nowhere, which is a list you read rather
than recognise.
*/
func byKey(scenes []api.Scene, keys api.KeysResponse) ([]api.Scene, map[string]string) {
	bound := map[string]string{}
	order := map[string]int{}
	for _, binding := range keys.Bindings {
		// The first key wins where two are bound to one scene: a scene is
		// shown under the key somebody would reach for first.
		if _, seen := bound[binding.Scene]; !seen {
			bound[binding.Scene] = binding.Key
		}
	}
	for i, binding := range sortedBindings(keys.Bindings) {
		if _, seen := order[binding.Scene]; !seen {
			order[binding.Scene] = i
		}
	}

	out := append([]api.Scene(nil), scenes...)
	sort.SliceStable(out, func(i, j int) bool {
		a, hasA := order[out[i].Name]
		b, hasB := order[out[j].Name]
		switch {
		case hasA && hasB:
			return a < b
		case hasA != hasB:
			return hasA
		}
		return out[i].Name < out[j].Name
	})
	return out, bound
}

// sortedBindings puts the keys in the order they sit on the keyboard, which
// is what the sequence string sorts as: the unshifted row, then the shifted.
func sortedBindings(bindings []api.Binding) []api.Binding {
	out := append([]api.Binding(nil), bindings...)
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].Key, out[j].Key
		if shifted(a) != shifted(b) {
			return !shifted(a)
		}
		return a < b
	})
	return out
}

func shifted(key string) bool { return strings.Contains(key, "Shift") }

// row is one scene: what it does, the key that fires it, and the two things
// worth doing to it here.
func (s *ScenesSection) row(sh *shell.Shell, scene api.Scene, key string) fyne.CanvasObject {
	facts := []string{}
	if scene.Colour != "" {
		facts = append(facts, scene.Colour)
	}
	if scene.Off {
		facts = append(facts, "off")
	}
	if len(scene.Assignments) > 0 {
		facts = append(facts, fmt.Sprintf("%d target(s)", len(scene.Assignments)))
	}
	if scene.Screen != "" {
		/*
			The picture's name, not the path it is kept at.

			`/home/nverenin/.local/share/hotaru/images/screenshot-20260920-233906.gif`
			is the same forty characters on every line, and the eight that
			differ are at the end -- so the line reads as a path rather than
			as a scene, and it is long enough to push everything after it off
			the useful part of the window.
		*/
		facts = append(facts, "screen: "+s.screenName(scene.Screen))
	}

	apply := widget.NewButton("Apply", func() {
		sh.Perform("applying "+scene.Name, func(ctx context.Context) error {
			done, err := s.app.client.ApplyScene(ctx, scene.Name, api.SceneRequest{})
			if err != nil {
				return err
			}
			onScreen(func() { sh.Flash(applied(done), fd.StatusGood) })
			return nil
		})
	})
	edit := widget.NewButton("Edit", func() {
		s.draft = DraftFrom(scene)
		s.picked.Clear()
		sh.Invalidate()
	})

	/*
		Deleting is a button on the row, and only on a row that has something
		to delete.

		A shipped scene is hotaru's own and cannot be removed -- the store
		takes the request and the scene is still there afterwards, which is
		the silent no-op this project keeps paying for. Saving over it is how
		it changes, and deleting that replacement is how it comes back.
	*/
	buttons := []fyne.CanvasObject{apply, edit}
	if !scene.Shipped {
		forget := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
			dialog.ShowConfirm("Delete "+scene.Name+"?", "", func(yes bool) {
				if !yes {
					return
				}
				sh.Perform("deleting "+scene.Name, func(ctx context.Context) error {
					if err := s.app.client.DeleteScene(ctx, scene.Name); err != nil {
						return err
					}
					onScreen(func() {
						sh.Flash(scene.Name+" is gone.", fd.StatusGood)
						sh.Invalidate()
					})
					return nil
				})
			}, sh.Window)
		})
		buttons = append(buttons, forget)
	}

	name := scene.Name
	if scene.Shipped {
		// Worth marking: it is hotaru's, it can be replaced by saving over
		// it, and deleting that replacement brings it back.
		name += "  (shipped)"
	}

	/*
		One line per scene, not a card each.

		Eighteen cards is a page somebody scrolls rather than reads, and what
		tells two scenes apart is a name and a colour -- both of which fit on a
		line beside the buttons that act on them. These two buttons belong to
		the row they sit in, which is the exception the affixing rule names:
		the control is the material.
	*/
	swatch := canvas.NewRectangle(parse(sceneColour(scene)))
	swatch.SetMinSize(fyne.NewSize(zoneHeight, zoneHeight))

	/*
		The key first, because that is what somebody recognises the scene by:
		the bank has been on this numpad for two years.

		And it is a button, because it was the one thing on this line that
		could not be changed without the terminal. What looks like the thing
		you click has to be the thing you click.
	*/
	shortcut := widget.NewButton(pretty(key), func() { s.bind(sh, scene, key) })
	shortcut.Importance = widget.LowImportance

	title := widget.NewLabel(name)
	title.Truncation = fyne.TextTruncateEllipsis
	said := widgets.Dim(join(facts))
	if label, ok := said.(*widget.Label); ok {
		label.Truncation = fyne.TextTruncateEllipsis
	}

	return listRow([]fyne.CanvasObject{
		column(sceneKeyWidth, shortcut), swatch,
		column(sceneNameWidth, title),
		column(sceneFactsWidth, said),
	}, buttons...)
}

// The scene list's columns. Wide enough for the names and the facts on this
// desk, and fixed so the buttons are in the same place on every line.
const (
	// Wide enough for "\u21e71" and "12" alike: the key is the first thing on
	// the line, so everything after it moves when it does not hold a width.
	sceneKeyWidth   = 52
	sceneNameWidth  = 170
	sceneFactsWidth = 300
)

/*
pretty shortens a key for the listing.

"Ctrl+Alt+Num+4" is the spelling KDE stores and reads as noise in a column of
eighteen; what tells them apart is the number and whether Shift is held.
*/
func pretty(key string) string {
	if key == "" {
		return "      "
	}
	number := key[strings.LastIndex(key, "+")+1:]
	if shifted(key) {
		return "⇧" + number
	}
	return " " + number
}

/*
spotOf reads a target back into the parts it was made of.

The scene stores what hotaru writes, and editing one of its lines means
selecting the thing it names -- which is the string going back the way it came.
A target hotaru understands and this does not (a segment somebody named in the
rules file) selects as a whole, which is what it is.
*/
func spotOf(target string) Spot {
	device, part, hasPart := strings.Cut(target, "/")
	if !hasPart {
		return WholeDevice(device)
	}
	zone, rang, hasRange := strings.Cut(part, "[")
	if !hasRange {
		return WholeZone(device, zone)
	}
	var first, last int
	if _, err := fmt.Sscanf(strings.TrimSuffix(rang, "]"), "%d:%d", &first, &last); err != nil {
		return WholeZone(device, zone)
	}
	return Lights(device, zone, first, last)
}

// sceneColour is the one colour that stands for a scene in a listing: what it
// paints everything, or its first exception.
func sceneColour(scene api.Scene) string {
	if scene.Colour != "" {
		return scene.Colour
	}
	if len(scene.Assignments) > 0 {
		return scene.Assignments[0].Colour
	}
	return ""
}

/*
editor is the draft, the machine, and the three things that can happen to a
draft: a colour, a look at it, a name.
*/
func (s *ScenesSection) editor(sh *shell.Shell, got Snapshot) fyne.CanvasObject {
	head := []fyne.CanvasObject{title(s.heading()), s.actions(sh)}
	if s.app.Previewing() {
		/*
			Said out loud, with the way back beside it.

			Somebody who closes the editor and later wonders why the mouse is
			green has been handed a puzzle by the program.
		*/
		head = append(head, widgets.Note("The hardware is showing this draft.", fd.StatusWarn))
	}

	/*
		Two panes, because the picture must not move.

		The draft and the picker were above the machine, so clicking a light
		-- which is how the picker appears at all -- pushed the picture down
		under the pointer that had just clicked it. Nothing transient may
		reflow the interface, and the interface a picker appears in front of
		is the one somebody is working in.

		Side by side, what happens on the left cannot move what is on the
		right. The left pane scrolls on its own for the scene that grows a
		line every time a colour is chosen.
	*/
	/*
		The scene's own lines scroll; the controls under them do not.

		A list rather than a column of rows. A scene with a light named
		individually has 225 lines, each of them a label, a swatch, a colour
		and two buttons -- about 1,575 objects, every one of which Fyne
		measures on every layout, and a wrapping label re-measures at every
		width. A 25-second drag of this pane cost 9.16s of CPU with
		Container.MinSize at 31% of it. A list builds the rows that are on
		screen and recycles them. See spec 026.
	*/
	left := container.NewBorder(nil,
		container.NewVBox(s.colours(sh), s.screen(sh), s.effects(sh, got)), nil, nil,
		s.assignments(sh))
	right := s.scrolling("editor", []fyne.CanvasObject{
		widgets.Dim("Pick a device, a zone, or a light:"),
		s.picture(sh, got),
	})

	/*
		A divider rather than a fixed pane.

		Fixed at 340 the two panes cut their own content off: a device on this
		desk is called "NZXT Kraken 2024 ELITE Series RGB", and neither half
		of the window had room for it. A divider is the honest answer -- the
		program cannot know how long a name is or how wide a window will be,
		and the person looking at it can.

		Through the shell's own HSplit, so the position is remembered with
		every other divider in the program rather than in a setting of its
		own.
	*/
	return container.NewBorder(container.NewVBox(head...), nil, nil, nil,
		sh.HSplit("scenes.editor", 0.42, left, right))
}

func (s *ScenesSection) heading() string {
	if s.draft.From != "" {
		return "Editing " + s.draft.From
	}
	return "New scene"
}

/*
everything is the target that means every device in scope.

A scene's commonest shape -- "make the machine blue" -- and it was not
expressible here: the picture offered devices and zones, so a colour for all of
them meant colouring each in turn and a scene six lines long.
*/
const everything = "*"

// picture is the machine, with every zone a button that selects it.
func (s *ScenesSection) picture(sh *shell.Shell, got Snapshot) fyne.CanvasObject {
	cards := make([]fyne.CanvasObject, 0, len(got.Devices)+1)
	cards = append(cards, widgets.Card("Everything",
		s.pick(sh, WholeDevice(everything), "every device in scope")))
	for _, device := range got.Devices {
		if !device.InScope {
			// Not selectable: hotaru does not drive it, so a colour here
			// would be a control that does nothing.
			continue
		}
		cards = append(cards, widgets.Card(device.Name, s.targets(sh, device)...))
	}
	return container.NewVBox(cards...)
}

/*
targets is a device: the whole thing, each zone, and each of its lights.

The lights are individually selectable because they are individually drawn.
Showing somebody twenty-four blocks and letting them address the zone is an
interface making a promise it does not keep -- and addressing one light is what
`kraken/ring[12:12]` means, which is exactly the typing this window exists to
replace.
*/
func (s *ScenesSection) targets(sh *shell.Shell, device api.Device) []fyne.CanvasObject {
	rows := []fyne.CanvasObject{s.pick(sh, WholeDevice(device.Name), "the whole device")}
	for _, zone := range device.Zones {
		rows = append(rows,
			container.NewHBox(s.pick(sh, WholeZone(device.Name, zone.Name),
				fmt.Sprintf("%s · %d", zone.Name, zone.Count))),
			s.leds(sh, device, zone),
		)
	}
	if len(device.Zones) == 0 {
		rows = append(rows, lights(device))
	}
	return rows
}

/*
leds draws a zone's lights as blocks somebody can click.

One block per light where a zone is small enough to draw that way, and a block
per run where it is not: a hundred-key keyboard at one block each is a smear
nobody can aim at, and a run is still an address -- `keyboard[24:27]` is a
thing hotaru understands and a thing a pointer can hit.
*/
func (s *ScenesSection) leds(sh *shell.Shell, device api.Device, zone api.Zone) fyne.CanvasObject {
	if zone.Count <= 0 {
		return widgets.Dim("no lights")
	}
	colours := shownColours(device)
	blocks := make([]fyne.CanvasObject, 0, mostBlocks)

	per := (zone.Count + mostBlocks - 1) / mostBlocks
	for first := 0; first < zone.Count; first += per {
		last := min(first+per-1, zone.Count-1)
		blocks = append(blocks, s.led(sh, device, zone, first, last, colours))
	}
	return container.NewGridWrap(fyne.NewSize(blockSize, blockSize), blocks...)
}

// The most blocks a zone is drawn as, and how big each one is. Twenty-four is
// the Kraken's ring exactly, which is the zone this was built to address.
const (
	mostBlocks = 24
	blockSize  = 20
)

// led is one block: the light or run it stands for, in the colour the device
// is showing, over a button that selects it.
func (s *ScenesSection) led(sh *shell.Shell, device api.Device, zone api.Zone,
	first, last int, colours []color.Color,
) fyne.CanvasObject {
	spot := Lights(device.Name, zone.Name, first, last)

	/*
		A swatch rather than a rectangle under an invisible button.

		This is the widget the editor has most of -- one per run of lights,
		across every device -- and a scroller lays out everything it holds
		rather than what is visible. A resize profile put
		`buttonRenderer.MinSize` at 14% of all samples: a theme lookup, a
		padding calculation and a RichText.MinSize per block, for blocks
		whose label is the empty string. See spec 025.
	*/
	block := widgets.NewSwatch(fyne.NewSize(blockSize, blockSize))
	block.Fill = sample(colours, zone.First+first, device.InScope)
	if colour, has := s.draft.Colour(spot.Target()); has {
		// What the draft will do to it, which is the point of drawing it here
		// rather than in the read-only view.
		block.Fill = parse(colour)
	}
	block.StrokeWidth = 1
	block.Stroke = theme.Color(theme.ColorNameSeparator)
	if s.picked.Has(spot) {
		block.StrokeWidth = 3
		block.Stroke = theme.Color(theme.ColorNamePrimary)
	}
	block.OnTapped = func() {
		s.picked.Toggle(spot)
		sh.Invalidate()
	}

	return widgets.WithTip(block, spot.Describe())
}

// pick is one selectable spot, carrying the draft's colour for it.
func (s *ScenesSection) pick(sh *shell.Shell, spot Spot, label string) fyne.CanvasObject {
	if colour, has := s.draft.Colour(spot.Target()); has {
		label += "  " + colour
	}
	button := widget.NewButton(label, func() {
		s.picked.Toggle(spot)
		sh.Invalidate()
	})
	if s.picked.Has(spot) {
		button.Importance = widget.HighImportance
	}
	return button
}

/*
colours offers the picker for whatever is selected.

A button rather than the wheel itself: the wheel lives in a modal, because what
it needs is the hardware showing the colour while somebody chooses it, and a
control that changes the lights has to have an unmistakable end. A modal has
one -- it is dismissed, and the lights go back.
*/
func (s *ScenesSection) colours(sh *shell.Shell) fyne.CanvasObject {
	if s.picked.Empty() {
		// Drawn even with nothing selected, so that selecting something adds
		// no height to anything: what appears is a control becoming usable,
		// not a card arriving.
		return widgets.Card("Nothing selected",
			widgets.Dim("Click a device, a zone or a light on the right."))
	}
	current := s.chosen()

	choose := widget.NewButtonWithIcon("Choose a colour", theme.ColorChromaticIcon(), func() {
		s.pickColour(sh, current)
	})
	choose.Importance = widget.HighImportance

	remove := widget.NewButtonWithIcon("Clear", theme.ContentClearIcon(), func() {
		// Taking these back out of the scene.
		s.set("")
		sh.Invalidate()
	})
	none := widget.NewButton("Select nothing", func() {
		s.picked.Clear()
		sh.Invalidate()
	})

	swatch := canvas.NewRectangle(parse(current))
	swatch.SetMinSize(fyne.NewSize(zoneHeight*2, zoneHeight))

	name := widget.NewLabel(s.targetName())
	name.Wrapping = fyne.TextWrapWord

	return widgets.Card("Colour",
		name,
		container.NewHBox(swatch, widgets.Dim(describe(current))),
		container.NewHBox(choose, remove, none),
	)
}

/*
pickColour opens the wheel, previewing on the hardware while it is open.

The whole argument for a window rather than a configuration file: the machine
is right there, so a colour can be *shown* rather than described. The preview
is held for exactly as long as the modal, which is what makes that safe --
dismissing it puts the lights back, and there is no state left behind to
wonder about later.
*/
func (s *ScenesSection) pickColour(sh *shell.Shell, current string) {
	before, had := s.chosen(), s.chosen() != ""
	picker := NewPicker(parse(opening(current)))

	/*
		The sender's life is the modal's, which is why it is a local.

		**It must not be reachable from a rebuild.** The shell calls Detach
		before every one, and this window rebuilds every two seconds: a sender
		kept in a field would be stopped and dropped under a pointer that was
		still dragging. The modal is the thing with an unmistakable end, and
		its callback below is that end.
	*/
	sender := newLimiter(s.showDraft)

	/*
		Every move reaches the draft; the limiter decides how many of them
		reach the hardware.

		The draft is updated here, on the UI thread, because the window reads
		it -- and the scene handed to the limiter is built here for the same
		reason. What crosses to the sender is a value, so there is nothing
		shared to race on. The whole draft rather than the one colour, so the
		preview shows the scene as it would be rather than a light on its own.
	*/
	picker.OnPick = func(colour string) {
		s.set(colour)
		sender.offer(s.draft.Scene(s.draftName()))
	}

	s.set(picker.Colour())
	sender.offer(s.draft.Scene(s.draftName()))

	chooser := dialog.NewCustomConfirm("Choose a colour for "+s.targetName(), "Use it", "Cancel",
		picker.Object(),
		func(keep bool) {
			// Before the lease goes: an apply still in flight would land after
			// the release and leave the lights where nothing is holding them.
			sender.stop()
			if keep {
				s.set(picker.Colour())
			} else {
				// Back to what the draft said before the wheel opened.
				s.set("")
				if had {
					s.set(before)
				}
			}
			s.app.EndPreview()
			sh.Invalidate()
		}, sh.Window)
	roomy(chooser, sh)
}

/*
showDraft puts the draft on the hardware, taking the lease if it is not already
held.

Not released between colours: releasing is what tells the service to put the
lights back, so a picker that released and re-took for every move sent a revert
to the previous colour a moment ahead of every new one. What that looked like
was the wheel not working.
*/
func (s *ScenesSection) showDraft(scene api.Scene) {
	if err := s.app.Preview(context.Background(), scene); err != nil {
		s.app.EndPreview()
	}
}

// describe is a colour as the card reports it, or the absence of one.
func describe(colour string) string {
	if colour == "" {
		return "nothing yet"
	}
	return colour
}

/*
screen is what the scene puts on the cooler's panel.

A scene carries this already -- the dashboard, the firmware's own readout, or a
picture -- and the editor had no way to set it, so editing a scene that showed
a GIF and saving it kept the GIF by luck rather than by choice.

**A scene that says nothing about the screen leaves it alone**, which is the
option somebody picks most and therefore the one that is offered first.
*/
/*
screen is what the scene puts on the cooler's panel.

A chooser with pictures in it rather than a dropdown of words. What somebody
is choosing between is six wallpapers and five dashboards, and the difference
between two wallpapers is not in their names -- `wallhaven-2kkreg` and
`wallhaven-ymqmj7` are the same word twice to anybody who did not download
them.

Fyne's Select draws text, so this is a button showing the current choice and a
dialog showing the rest. The same shape as the shortcut chooser, for the same
reason: a list somebody has to look at does not fit in a dropdown.
*/
func (s *ScenesSection) screen(sh *shell.Shell) fyne.CanvasObject {
	current := screenOf(s.draft)
	shown := widget.NewButton(s.screenName(current), func() { s.chooseScreen(sh, current) })

	return widgets.Card("The screen",
		container.NewBorder(nil, nil,
			sized(screenShotSize, s.screenShot(s.screens(), current)), nil, shown))
}

/*
effects is what each device should be doing with this scene's colours.

Beside the screen chooser because it is the same kind of question -- what the
scene does beyond the colours themselves -- and because both are about the
scene as a whole rather than about the light somebody has selected.
*/
func (s *ScenesSection) effects(sh *shell.Shell, got Snapshot) fyne.CanvasObject {
	shown := widget.NewButton(effectsSaid(s.draft.Effects()), func() { s.chooseEffects(sh, got) })
	return widgets.Card("Effects", shown)
}

/*
effectsSaid is what the scene says about what its devices are doing, in one
line.

The card is a button like the screen's, for the reason the screen's is one:
five devices, each with a chooser and a line saying what it is doing now, is
three hundred pixels of a pane whose job is to stay out of the way. The editor
already does not fit a default window with the scene's own lines in it.
*/
func effectsSaid(effects map[string]string) string {
	switch len(effects) {
	case 0:
		return "every device left alone"
	case 1:
		for device, mode := range effects {
			return device + ": " + mode
		}
	}
	return fmt.Sprintf("%d devices", len(effects))
}

/*
chooseEffects is what each device should be doing with this scene's colours.

A dialog rather than a pane, so the editor's controls stay the size they were,
and the same shape as the screen chooser next to it: a card that says what is
set, and a dialog that sets it.
*/
func (s *ScenesSection) chooseEffects(sh *shell.Shell, got Snapshot) {
	rows := []fyne.CanvasObject{effectFields(got.Devices,
		func(device string) string { return s.draft.Effect(device) },
		func(device, mode string) { s.draft.SetEffect(device, mode) })}

	/*
		And the way to stop setting it nine times.

		A style is a decision about a device, not about a scene: somebody who
		wants their keyboard reactive wants it reactive, and a bank of nine
		scenes meant opening nine. Offered where there is something to copy,
		somewhere to copy it to, and a saved scene to copy from -- the
		service copies what a scene holds.
	*/
	switch {
	case len(got.Scenes) < 2:
	case s.draft.From == "":
		rows = append(rows, widgets.Dim("Save this scene to use its style in others."))
	default:
		share := widget.NewButtonWithIcon("Use this style in other scenes",
			theme.ContentCopyIcon(), func() { s.share(sh, got) })
		rows = append(rows, share)
	}

	ask := dialog.NewCustomConfirm("What the devices do", "Done", "Cancel",
		container.NewVScroll(container.NewVBox(rows...)),
		func(bool) { sh.Invalidate() }, sh.Window)
	roomy(ask, sh)
}

/*
share gives other scenes this draft's effects.

Saved scenes, from the service, because what is being copied to is a scene on
disk rather than a draft in this window -- and the draft's own effects are
what spread, so it is what is on screen rather than what was last saved.
*/
func (s *ScenesSection) share(sh *shell.Shell, got Snapshot) {
	// The scene's own name, not the editor's heading for it: this is what
	// the service is asked about.
	from := s.draft.From

	/*
		In the order the Scenes list shows them, which is the order of the
		keys they sit on.

		The list somebody is choosing from here is the list they were looking
		at a moment ago, and two orderings of the same nine scenes is two
		lists to learn.
	*/
	ordered, _ := byKey(got.Scenes, got.Keys)
	others := make([]string, 0, len(ordered))
	for _, scene := range ordered {
		if scene.Name != from {
			others = append(others, scene.Name)
		}
	}

	picked := map[string]bool{}
	rows := make([]fyne.CanvasObject, 0, len(others)+1)
	for _, name := range others {
		check := widget.NewCheck(name, func(on bool) { picked[name] = on })
		rows = append(rows, check)
	}

	body := container.NewBorder(
		widgets.DimWrapped("The effects only, not the colours. Each scene ends up with "+
			"exactly what this one does, including nothing."), nil, nil, nil,
		container.NewVScroll(container.NewVBox(rows...)))

	ask := dialog.NewCustomConfirm("Use "+from+"'s style in", "Use it", "Cancel", body,
		func(ok bool) {
			if !ok {
				return
			}
			var to []string
			for _, name := range others {
				if picked[name] {
					to = append(to, name)
				}
			}
			if len(to) == 0 {
				return
			}
			s.restyle(sh, from, to)
		}, sh.Window)
	roomy(ask, sh)
}

// restyle saves the draft and hands its effects to the scenes chosen.
func (s *ScenesSection) restyle(sh *shell.Shell, from string, to []string) {
	scene := s.draft.Scene(from)
	sh.Perform("restyling", func(ctx context.Context) error {
		// Saved first: the service copies what a scene holds, and the
		// effects being copied are the ones on screen.
		if err := s.app.client.SaveScene(ctx, scene); err != nil {
			return err
		}
		changed, err := s.app.client.CopyEffects(ctx, from, to)
		if err != nil {
			return err
		}
		onScreen(func() {
			sh.Flash(fmt.Sprintf("%d scene(s) now do what %s does.", len(changed), from),
				fd.StatusGood)
			sh.Invalidate()
		})
		return nil
	})
}

// screenName is a choice in the words the chooser offers it in.
func (s *ScenesSection) screenName(choice string) string {
	switch {
	case choice == leaveScreen || choice == "":
		return "leave it alone"
	case choice == api.ScreenDashboard:
		return "the dashboard, whichever is set"
	case choice == api.ScreenReadout:
		return "the cooler's own display"
	case strings.HasPrefix(choice, api.ScreenDashboardPrefix):
		return "dashboard: " + strings.TrimPrefix(choice, api.ScreenDashboardPrefix)
	}
	return "picture: " + pictureName(choice)
}

// pictureName is a stored picture's name, from the path a scene keeps.
func pictureName(path string) string {
	return suggested(filepath.Base(path))
}

/*
screenShot is a small picture of what a choice puts on the panel.

The same pictures the Pictures and Screen sections draw, through the same
cache, so choosing between them here costs nothing they have not already
paid.
*/
func (s *ScenesSection) screenShot(got screens, choice string) fyne.CanvasObject {
	switch {
	case strings.HasPrefix(choice, api.ScreenDashboardPrefix):
		for _, one := range got.boards {
			if one.Name == strings.TrimPrefix(choice, api.ScreenDashboardPrefix) {
				return dashboardShot(s.app, one)
			}
		}
	case strings.HasSuffix(choice, ".gif"):
		for _, stored := range got.pictures {
			if stored.Path == choice {
				return pictureShot(stored)
			}
		}
	}
	return canvas.NewRectangle(color.Transparent)
}

// screenShotSize is how big a choice is drawn beside the button. Small: it
// says which picture, not what is in it.
const screenShotSize = 48

// sized holds a picture to the chooser's own size.
func sized(side float32, o fyne.CanvasObject) fyne.CanvasObject {
	if picture, ok := o.(*canvas.Image); ok {
		picture.SetMinSize(fyne.NewSize(side, side))
	}
	return container.NewGridWrap(fyne.NewSize(side, side), o)
}

/*
chooseScreen is the list of everything the panel can be asked to show.

Pictures and dashboards together, because from a scene's point of view they
are the same choice: something to put on the screen when these colours go on
the lights.
*/
func (s *ScenesSection) chooseScreen(sh *shell.Shell, current string) {
	got := s.screens()

	choices := []string{leaveScreen, api.ScreenDashboard, api.ScreenReadout}
	for _, one := range got.boards {
		choices = append(choices, api.ScreenDashboardPrefix+one.Name)
	}
	for _, stored := range got.pictures {
		choices = append(choices, stored.Path)
	}

	rows := make([]fyne.CanvasObject, 0, len(choices))
	picked := current
	var list *widget.RadioGroup

	labels := make([]string, 0, len(choices))
	for _, choice := range choices {
		labels = append(labels, s.screenName(choice))
	}
	list = widget.NewRadioGroup(labels, func(chosen string) {
		for i, label := range labels {
			if label == chosen {
				picked = choices[i]
				return
			}
		}
	})
	list.Required = true
	list.Selected = s.screenName(current)

	/*
		The pictures beside the list rather than in it, at the height one
		option takes.

		Fyne's radio group draws text and nothing else, so a thumbnail can
		only sit next to its option -- and "next to" means the column is laid
		out at the group's own pitch, which is its height over the number of
		options. Asking for that number is why the group is built first.
	*/
	pitch := list.MinSize().Height / float32(len(labels))
	for _, choice := range choices {
		rows = append(rows, container.NewCenter(
			sized(pitch-theme.Padding()*2, s.screenShot(got, choice))))
	}

	// The group at its own height, not the dialog's: stretched, its options
	// spread out and the pictures no longer line up with them.
	body := container.NewBorder(nil, nil,
		container.New(Beside{Pitch: pitch}, rows...), nil,
		container.NewVBox(list))
	ask := dialog.NewCustomConfirm("What the screen shows", "Choose", "Cancel",
		container.NewVScroll(body), func(ok bool) {
			if !ok {
				return
			}
			if picked == leaveScreen {
				s.draft.SetScreen("")
			} else {
				s.draft.SetScreen(picked)
			}
			sh.Invalidate()
		}, sh.Window)
	roomy(ask, sh)
}

// leaveScreen is the choice that changes nothing, which is what a scene
// saying nothing about the screen does.
const leaveScreen = "-"

// screenOf is the draft's screen as the chooser's value.
func screenOf(draft *Draft) string {
	if draft.Screen() == "" {
		return leaveScreen
	}
	return draft.Screen()
}

/*
chosen is what the draft says about the selected target, if anything.

One question with two answers behind it -- a target's own colour, or the
machine-wide one -- asked in enough places that having it in two shapes went
wrong once already.
*/
func (s *ScenesSection) chosen() string {
	targets := s.picked.Targets()
	if len(targets) != 1 {
		// Several spots may disagree, and reporting one of their colours as
		// though it were theirs would be a guess presented as a fact.
		return ""
	}
	if targets[0] == everything {
		return s.draft.Everything()
	}
	colour, _ := s.draft.Colour(targets[0])
	return colour
}

/*
opening is the colour the wheel starts on when the draft has none.

White, not whatever an unparseable empty string happens to become. It became
the theme's disabled grey, and since opening the wheel sends its colour
straight to the hardware, selecting a fan that had no colour yet turned the
machine grey before anybody had chosen anything.
*/
func opening(current string) string { return Opening(current) }

// Opening is opening, for a test outside this package.
func Opening(current string) string {
	if current == "" {
		return "#ffffff"
	}
	return current
}

// set puts a colour on everything selected, merged into as few targets as
// cover it.
func (s *ScenesSection) set(colour string) {
	for _, target := range s.picked.Targets() {
		if target == everything {
			s.draft.SetEverything(colour)
			continue
		}
		s.draft.Set(target, colour)
	}
}

/*
preview shows the whole draft, for as long as a modal is open.

The modal is the mechanism rather than the decoration. A preview is hardware
somebody has to remember to put back, and a control with no obvious end is how
it gets forgotten -- so the thing holding the lights is a window in front of
everything else, and dismissing it is the revert.
*/
func (s *ScenesSection) preview(sh *shell.Shell) {
	s.showDraft(s.draft.Scene(s.draftName()))
	if !s.app.Previewing() {
		sh.Flash("The service would not show it.", fd.StatusWarn)
		return
	}

	body := container.NewVBox(
		widget.NewLabel(s.draft.Summary()),
		widgets.Dim("Nothing is recorded. Close this and the lights go back."),
	)
	modal := dialog.NewCustom("Showing "+s.draftName(), "Done", body, sh.Window)
	modal.SetOnClosed(func() {
		s.app.EndPreview()
		sh.Invalidate()
	})
	modal.Show()
}

/*
targetName is what the picker is titled.

One spot by its own name, several by their count and their first: "4 selected,
from ring, light 3" tells somebody what they are about to colour without a list
of four addresses in a card heading.
*/
func (s *ScenesSection) targetName() string {
	spots := s.picked.Spots()
	switch len(spots) {
	case 0:
		return "Nothing selected"
	case 1:
		return spots[0].Describe()
	}
	return fmt.Sprintf("%d selected, from %s", len(spots), spots[0].Describe())
}

/*
assignments is what the draft says, and where it is edited.

Each line is a control rather than a summary. It was a summary: the scene's
entries were listed exactly where somebody would reach to change one, and
changing one meant scrolling down to find the device it named and starting
again. What looks like the thing you click has to be the thing you click.
*/
/*
assignments is what the draft says, one line each.

A widget.List rather than a column of rows, because a scene that names lights
individually has hundreds of lines and Fyne measures every widget in a
container on every layout. The list builds what is on screen and recycles it,
which is what makes the pane resize without pausing.
*/
func (s *ScenesSection) assignments(sh *shell.Shell) fyne.CanvasObject {
	if s.draft.Empty() {
		return widgets.Note("Pick a device or a zone below, then choose a colour.", fd.StatusInfo)
	}

	lines := s.lines()
	list := widget.NewList(
		func() int { return len(lines) },
		func() fyne.CanvasObject { return s.entry() },
		func(i widget.ListItemID, row fyne.CanvasObject) {
			if i < len(lines) {
				fill(sh, s, row, lines[i])
			}
		},
	)
	return container.NewBorder(widgets.Heading("This scene", ""), nil, nil, nil, list)
}

// line is one assignment as the list draws it.
type line struct{ target, colour string }

// lines is the draft in the order it is shown: everything first, because it
// is what the rest are exceptions to.
func (s *ScenesSection) lines() []line {
	out := make([]line, 0, len(s.draft.Targets())+1)
	if colour := s.draft.Everything(); colour != "" {
		out = append(out, line{everything, colour})
	}
	for _, target := range s.draft.Targets() {
		colour, _ := s.draft.Colour(target)
		out = append(out, line{target, colour})
	}
	return out
}

// entry is one line of the scene: what it says, and the two things worth
// doing to it.
/*
entry is the shape of one line, built once and filled many times.

A list makes one of these per visible row and hands it back with a different
index, so nothing here knows which assignment it is drawing. `fill` is what
says.

The name truncates rather than wraps. A wrapping label measures itself against
whatever width it is given, which in a list of a fixed row height is a
measurement taken on every layout to produce a height that cannot change --
and a device on this desk is called "NZXT Kraken 2024 ELITE Series RGB/Hue 2
Channel 1[12:23]", which no pane fits anyway. The tip carries the whole of it.
*/
func (s *ScenesSection) entry() fyne.CanvasObject {
	swatch := canvas.NewRectangle(color.Transparent)
	swatch.SetMinSize(fyne.NewSize(zoneHeight, zoneHeight))

	name := widget.NewLabel("")
	name.Truncation = fyne.TextTruncateEllipsis

	// Labelled, not a bare icon. Only the rows on screen exist now, so the
	// dozen RichText measurements a label costs are affordable where 225 of
	// them were not -- and "Change" says what a colour wheel icon does not.
	change := widget.NewButtonWithIcon("Change", theme.ColorChromaticIcon(), func() {})
	remove := widget.NewButtonWithIcon("", theme.ContentRemoveIcon(), func() {})

	return container.NewBorder(nil, nil,
		swatch, container.NewHBox(change, remove), name)
}

// fill points a recycled row at one assignment.
func fill(sh *shell.Shell, s *ScenesSection, row fyne.CanvasObject, at line) {
	box, ok := row.(*fyne.Container)
	if !ok || len(box.Objects) < 3 {
		return
	}
	name, _ := box.Objects[0].(*widget.Label)
	swatch, _ := box.Objects[1].(*canvas.Rectangle)
	buttons, _ := box.Objects[2].(*fyne.Container)
	if name == nil || swatch == nil || buttons == nil || len(buttons.Objects) < 2 {
		return
	}

	shown := at.target
	if at.target == everything {
		shown = "Everything in scope"
	}
	name.SetText(shown + "  " + at.colour)
	swatch.FillColor = parse(at.colour)
	swatch.Refresh()

	if change, ok := buttons.Objects[0].(*widget.Button); ok {
		change.OnTapped = func() {
			// Editing one line is a selection of one: whatever was picked in
			// the picture is set aside so the colour lands where it was asked
			// to.
			s.picked.Clear()
			s.picked.Toggle(spotOf(at.target))
			s.pickColour(sh, at.colour)
		}
	}
	if drop, ok := buttons.Objects[1].(*widget.Button); ok {
		drop.OnTapped = func() {
			if at.target == everything {
				s.draft.SetEverything("")
			} else {
				s.draft.Set(at.target, "")
			}
			sh.Invalidate()
		}
	}
}

// actions are the three things that can happen to a draft.
func (s *ScenesSection) actions(sh *shell.Shell) fyne.CanvasObject {
	show := widget.NewButtonWithIcon("Show it on the hardware", theme.VisibilityIcon(), func() {
		s.preview(sh)
	})
	if s.draft.Empty() {
		show.Disable()
	}

	save := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), func() {
		s.save(sh)
	})
	save.Importance = widget.HighImportance
	if s.draft.Empty() {
		save.Disable()
	}

	cancel := widget.NewButton("Close", func() {
		s.app.EndPreview()
		s.draft = nil
		s.picked.Clear()
		sh.Invalidate()
	})

	return container.NewHBox(show, save, cancel)
}

// save asks for a name, offering the one it was opened from.
func (s *ScenesSection) save(sh *shell.Shell) {
	entry := widget.NewEntry()
	entry.SetText(s.draft.From)
	entry.SetPlaceHolder("evening")

	dialog.ShowForm("Save the scene", "Save", "Cancel",
		[]*widget.FormItem{widget.NewFormItem("Name", entry)},
		func(ok bool) {
			if !ok || entry.Text == "" {
				return
			}
			sh.Perform("saving "+entry.Text, func(ctx context.Context) error {
				if err := s.app.client.SaveScene(ctx, s.draft.Scene(entry.Text)); err != nil {
					return err
				}
				onScreen(func() {
					s.app.EndPreview()
					s.draft = nil
					s.picked.Clear()
					sh.Flash(entry.Text+" saved.", fd.StatusGood)
				})
				return nil
			})
		}, sh.Window)
}

// draftName is what a preview of this draft calls itself in a listing.
func (s *ScenesSection) draftName() string {
	if s.draft.From != "" {
		return s.draft.From + " (edited)"
	}
	return "a draft"
}

func hex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", eighth(r), eighth(g), eighth(b))
}

func applied(done api.SceneResponse) string {
	lit := 0
	for _, result := range done.Results {
		if result.Applied {
			lit++
		}
	}
	return fmt.Sprintf("%s: %d of %d device(s).", done.Scene, lit, len(done.Results))
}

func join(facts []string) string {
	if len(facts) == 0 {
		return "nothing"
	}
	out := facts[0]
	for _, fact := range facts[1:] {
		out += " · " + fact
	}
	return out
}
