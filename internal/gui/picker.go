package gui

import (
	"image/color"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

/*
Picker is the wheel, a brightness slider, the three numbers, and the hex.

Four ways of saying one thing, kept in step. Somebody choosing a colour for a
light works on the wheel; somebody matching a colour they already have types
the hex; somebody who knows the value types it. None of them should have to
leave and come back.
*/
type Picker struct {
	wheel *Wheel
	// value and sat tune what the wheel is coarse at.
	//
	// A disc puts hue around and saturation outward, so a fully saturated red
	// exists only on the rim and everything inside it is pink -- which is
	// correct, and useless for tuning. The sliders are how somebody lands on
	// a colour rather than near it.
	value  *widget.Slider
	sat    *widget.Slider
	fields [3]*widget.Entry
	hex    *widget.Entry
	swatch *canvas.Rectangle

	// OnPick is called whenever the colour changes by any route, already
	// throttled: see live.
	OnPick func(string)

	/*
		syncing marks the moment the picker is updating its own controls.

		Without it the control feeds itself: showing a colour sets the
		brightness slider, the slider reports a change, the change goes back
		into the wheel -- and because the slider snaps to its step, the colour
		that came back was not the colour that went in. Dragging the wheel
		produced a colour a shade off the one under the pointer, on the
		hardware and in the numbers.
	*/
	syncing bool

	last time.Time
	// pending is the colour a throttled call held back, sent when the pause
	// ends so the last move is never the one that is dropped.
	pending string
	timer   *time.Timer
}

/*
live is how often a colour in motion reaches the hardware.

The wheel reports every drag step, which is far more than a device wants; the
service coalesces per device, so the cost of asking too often lands on the USB
bus rather than being absorbed. This is slow enough to be kind and fast enough
that the lights follow the pointer rather than catching up afterwards.
*/
const live = 120 * time.Millisecond

// NewPicker builds the control, opened on a colour.
func NewPicker(start color.Color) *Picker {
	p := &Picker{wheel: NewWheel()}
	p.wheel.Set(start)

	p.value = widget.NewSlider(0, 1)
	p.value.Step = 0.01
	p.value.SetValue(p.wheel.Value())
	p.value.OnChanged = func(v float64) {
		if p.syncing {
			return
		}
		p.wheel.SetValue(v)
	}

	p.sat = widget.NewSlider(0, 1)
	p.sat.Step = 0.01
	p.sat.SetValue(p.wheel.Saturation())
	p.sat.OnChanged = func(v float64) {
		if p.syncing {
			return
		}
		p.wheel.SetSaturation(v)
	}

	for i := range p.fields {
		p.fields[i] = widget.NewEntry()
		// As it is typed, not on Enter. Somebody adjusting a value one digit
		// at a time is tuning, and tuning against a control that only
		// responds when it is finished with is not tuning.
		p.fields[i].OnChanged = func(string) {
			if !p.syncing {
				p.fromFields()
			}
		}
	}
	p.hex = widget.NewEntry()
	p.hex.OnSubmitted = func(text string) {
		if p.syncing {
			return
		}
		p.wheel.Set(parse(text))
		p.show(true)
	}

	p.swatch = canvas.NewRectangle(start)
	p.swatch.SetMinSize(fyne.NewSize(wheelSize/3, 24))

	p.wheel.OnPick = func(color.Color) { p.show(true) }
	p.show(false)
	return p
}

// Object is the control, laid out.
func (p *Picker) Object() fyne.CanvasObject {
	numbers := container.NewGridWithColumns(3,
		labelled("R", p.fields[0]), labelled("G", p.fields[1]), labelled("B", p.fields[2]))

	return container.NewBorder(nil, nil, p.wheel, nil,
		container.NewVBox(
			p.exact(),
			labelled("Sat", p.sat),
			labelled("Lum", p.value),
			numbers,
			container.NewBorder(nil, nil, widget.NewLabel("Hex"), p.swatch, p.hex),
		),
	)
}

/*
exact is a row of the colours hotaru ships scenes for.

A wheel is for finding a colour and terrible at hitting an exact one: pure red
lives on a single pixel of the rim. These are the values the shipped scenes use
and the ones somebody means when they say "red".
*/
func (p *Picker) exact() fyne.CanvasObject {
	swatches := make([]fyne.CanvasObject, 0, len(namedColours))
	for _, name := range namedColours {
		c := parse(name)
		button := widget.NewButton("", func() { p.Choose(c) })
		button.Importance = widget.LowImportance

		block := canvas.NewRectangle(c)
		block.SetMinSize(fyne.NewSize(22, 22))
		swatches = append(swatches, container.NewStack(block, button))
	}
	return container.NewHBox(swatches...)
}

// namedColours are the nine hotaru ships scenes for, less "off".
var namedColours = []string{
	"red", "green", "blue", "purple", "cyan", "orange", "white", "magenta",
}

// Colour is what the picker is showing, as hotaru writes colours.
func (p *Picker) Colour() string { return hex(p.wheel.Colour()) }

/*
Choose moves the picker to a colour as though somebody had pointed at it,
reporting the change.

For tests, and for the one place the program does the same: opening the wheel
sends the colour it opened on, so the hardware shows what the picker shows from
the first moment rather than after the first drag.
*/
func (p *Picker) Choose(c color.Color) {
	p.wheel.Set(c)
	p.show(true)
}

// Stop cancels a throttled call that has not fired. The editor calls it when
// the picker goes away, so a colour cannot land on the hardware after the
// control that chose it has gone.
func (p *Picker) Stop() {
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
}

// show updates every other part of the control from the wheel, and reports the
// change onward if asked.
func (p *Picker) show(report bool) {
	p.syncing = true
	defer func() { p.syncing = false }()

	c := p.wheel.Colour()
	r, g, b, _ := c.RGBA()
	for i, v := range [3]uint8{eighth(r), eighth(g), eighth(b)} {
		p.fields[i].SetText(strconv.Itoa(int(v)))
	}
	p.hex.SetText(hex(c))
	p.swatch.FillColor = c
	p.swatch.Refresh()
	p.value.SetValue(p.wheel.Value())
	p.sat.SetValue(p.wheel.Saturation())

	if report {
		p.throttled(hex(c))
	}
}

/*
throttled sends a colour onward at most every `live`, and always sends the last
one.

Dropping intermediate steps is the point; dropping the final one is the bug
that shape of code usually has. A drag that ends between two ticks would
otherwise leave the hardware on the second-to-last colour, which looks exactly
like the picker being wrong.
*/
func (p *Picker) throttled(colour string) {
	if p.OnPick == nil {
		return
	}
	p.pending = colour
	if time.Since(p.last) >= live {
		p.last = time.Now()
		p.OnPick(colour)
		return
	}
	if p.timer != nil {
		return
	}
	p.timer = time.AfterFunc(live, func() {
		p.last, p.timer = time.Now(), nil
		fyne.Do(func() { p.OnPick(p.pending) })
	})
}

// fromFields reads the three numbers back into the wheel.
func (p *Picker) fromFields() {
	var rgb [3]uint8
	for i, field := range p.fields {
		v, err := strconv.Atoi(field.Text)
		if err != nil || v < 0 || v > 255 {
			// A number that is not one is left for the user to fix rather
			// than corrected under them mid-typing.
			return
		}
		rgb[i] = uint8(v)
	}
	p.wheel.Set(color.NRGBA{R: rgb[0], G: rgb[1], B: rgb[2], A: 255})
	p.show(true)
}

// labelled puts a name to the left of a control, which is the shape every row
// of this picker takes.
func labelled(name string, control fyne.CanvasObject) fyne.CanvasObject {
	return container.NewBorder(nil, nil, widget.NewLabel(name), nil, control)
}
