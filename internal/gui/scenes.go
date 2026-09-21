package gui

import (
	"context"
	"fmt"
	"image/color"
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
	// picker is kept between rebuilds: one replaced under a dragging pointer
	// loses the drag, and this window rebuilds every two seconds.
	picker *Picker

	/*
		scroller is kept for the same kind of reason, and a sharper one.

		A section is a build function and the shell rebuilds it whole, so a
		scroller built inside one starts at the top every time -- and a list
		that jumps to the top while somebody is reading it is a list they
		cannot use. Keeping it means keeping the offset.
	*/
	scroller *container.Scroll
	rows     *fyne.Container
	// scrolling says which view the scroller belongs to. The list and the
	// editor are different material, so an offset from one is meaningless in
	// the other and carrying it over reads as the window losing its place.
	showing string
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

// Detach ends a preview when the section goes away, which is what leaving the
// editor means: a draft nobody is looking at is not one the hardware should
// still be showing.
/*
Detach drops what belongs to the section on screen.

**It does not end the preview**, and that was a bug worth writing down: the
shell detaches a section before *every* rebuild, not only when it is replaced,
so ending a preview here meant a draft went up and came down within one frame.
The button worked, the lights flashed, and the cause was in a method that looks
like it runs at the end of something.

A preview ends when somebody dismisses the window that is showing it, or when
the program stops. Both are explicit.
*/
func (s *ScenesSection) Detach() { s.stopPicking() }

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
		container.NewVBox(title("Scenes"), add), nil, nil, nil,
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
		facts = append(facts, "screen: "+scene.Screen)
	}

	apply := widget.NewButton("Apply", func() {
		sh.Perform("applying "+scene.Name, func(ctx context.Context) error {
			done, err := s.app.client.ApplyScene(ctx, scene.Name, api.SceneRequest{})
			if err != nil {
				return err
			}
			sh.Flash(applied(done), fd.StatusGood)
			return nil
		})
	})
	edit := widget.NewButton("Edit", func() {
		s.draft = DraftFrom(scene)
		s.picked.Clear()
		sh.Invalidate()
	})

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

	// The key first, because that is what somebody recognises the scene by:
	// the bank has been on this numpad for two years.
	left := []fyne.CanvasObject{widgets.Dim(pretty(key)), swatch, widget.NewLabel(name)}

	return container.NewBorder(nil, nil,
		container.NewHBox(left...),
		container.NewHBox(apply, edit),
		widgets.Dim(join(facts)),
	)
}

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
	left := container.NewScroll(container.NewVBox(
		s.assignments(sh), s.colours(sh), s.screen()))
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

	block := canvas.NewRectangle(sample(colours, zone.First+first, device.InScope))
	if colour, has := s.draft.Colour(spot.Target()); has {
		// What the draft will do to it, which is the point of drawing it here
		// rather than in the read-only view.
		block.FillColor = parse(colour)
	}
	block.StrokeWidth = 1
	block.StrokeColor = theme.Color(theme.ColorNameSeparator)
	if s.picked.Has(spot) {
		block.StrokeWidth = 3
		block.StrokeColor = theme.Color(theme.ColorNamePrimary)
	}

	button := widget.NewButton("", func() {
		s.stopPicking()
		s.picked.Toggle(spot)
		sh.Invalidate()
	})
	button.Importance = widget.LowImportance

	return widgets.WithTip(container.NewStack(block, button), spot.Describe())
}

// pick is one selectable spot, carrying the draft's colour for it.
func (s *ScenesSection) pick(sh *shell.Shell, spot Spot, label string) fyne.CanvasObject {
	if colour, has := s.draft.Colour(spot.Target()); has {
		label += "  " + colour
	}
	button := widget.NewButton(label, func() {
		// A fresh picker per selection: one opened on the last spot's colour
		// would misreport this one.
		s.stopPicking()
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

	// Every move goes to the hardware, throttled. The draft carries it so the
	// preview shows the scene as it would be, not the one colour in isolation.
	picker.OnPick = func(colour string) {
		s.set(colour)
		s.showDraft()
	}

	s.set(picker.Colour())
	s.showDraft()

	chooser := dialog.NewCustomConfirm("Choose a colour for "+s.targetName(), "Use it", "Cancel",
		picker.Object(),
		func(keep bool) {
			picker.Stop()
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
func (s *ScenesSection) showDraft() {
	if err := s.app.Preview(context.Background(), s.draft.Scene(s.draftName())); err != nil {
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
func (s *ScenesSection) screen() fyne.CanvasObject {
	choices := []string{leaveScreen, api.ScreenDashboard, api.ScreenReadout}
	labels := map[string]string{
		leaveScreen:         "leave it alone",
		api.ScreenDashboard: "the dashboard",
		api.ScreenReadout:   "the cooler's own display",
	}

	stored, _ := s.app.client.Images(context.Background())
	for _, image := range stored {
		choices = append(choices, image.Path)
		labels[image.Path] = "picture: " + image.Name
	}

	names := make([]string, 0, len(choices))
	for _, choice := range choices {
		names = append(names, labels[choice])
	}

	chooser := widget.NewSelect(names, func(chosen string) {
		for value, label := range labels {
			if label != chosen {
				continue
			}
			if value == leaveScreen {
				s.draft.SetScreen("")
				return
			}
			s.draft.SetScreen(value)
			return
		}
	})
	chooser.SetSelected(labels[screenOf(s.draft)])

	return widgets.Card("The screen", chooser)
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

// stopPicking drops the picker, so the next selection opens a fresh one.
func (s *ScenesSection) stopPicking() {
	if s.picker != nil {
		s.picker.Stop()
		s.picker = nil
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
	s.showDraft()
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
func (s *ScenesSection) assignments(sh *shell.Shell) fyne.CanvasObject {
	if s.draft.Empty() {
		return widgets.Note("Pick a device or a zone below, then choose a colour.", fd.StatusInfo)
	}

	rows := make([]fyne.CanvasObject, 0, len(s.draft.Targets())+1)
	if colour := s.draft.Everything(); colour != "" {
		rows = append(rows, s.entry(sh, everything, colour))
	}
	for _, target := range s.draft.Targets() {
		colour, _ := s.draft.Colour(target)
		rows = append(rows, s.entry(sh, target, colour))
	}
	return widgets.Card("This scene", rows...)
}

// entry is one line of the scene: what it says, and the two things worth
// doing to it.
func (s *ScenesSection) entry(sh *shell.Shell, target, colour string) fyne.CanvasObject {
	swatch := canvas.NewRectangle(parse(colour))
	swatch.SetMinSize(fyne.NewSize(zoneHeight, zoneHeight))

	change := widget.NewButtonWithIcon("Change", theme.ColorChromaticIcon(), func() {
		// Editing one line is a selection of one: whatever was picked in the
		// picture is set aside so the colour lands where it was asked to.
		s.picked.Clear()
		s.picked.Toggle(spotOf(target))
		s.pickColour(sh, colour)
	})
	remove := widget.NewButtonWithIcon("", theme.ContentRemoveIcon(), func() {
		if target == everything {
			s.draft.SetEverything("")
		} else {
			s.draft.Set(target, "")
		}
		sh.Invalidate()
	})

	name := target
	if target == everything {
		name = "Everything in scope"
	}

	/*
		Stacked, not spread.

		A name, a swatch, a colour and two buttons in one row needs more width
		than a pane has, and what a row does when it runs out is cut its own
		end off. Two short lines fit anything.
	*/
	label := widget.NewLabel(name)
	label.Wrapping = fyne.TextWrapWord

	return container.NewVBox(
		label,
		container.NewHBox(swatch, widgets.Dim(colour), change, remove),
	)
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
		s.stopPicking()
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
				s.app.EndPreview()
				s.stopPicking()
				s.draft = nil
				s.picked.Clear()
				sh.Flash(entry.Text+" saved.", fd.StatusGood)
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
