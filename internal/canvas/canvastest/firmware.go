package canvastest

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/ushineko/sanshoku/lighting"
)

/*
Firmware is a Board whose firmware also runs effects of its own
(lighting.Onboard and lighting.Catalogue), and whose frames hold: the K4 HE,
the RTX 4090 and the Kraken (sanshoku specs 025, 029, 031). Each effect but
the Colourless ones shows the one stored colour. A write that changes nothing
writes nothing. Running reports nothing until an effect is set, as a device
that cannot say what it runs.
*/
type Firmware struct {
	*Board
	Colourless []string

	fmu     sync.Mutex
	names   []string
	running string
	colour  lighting.Colour
	writes  int
}

// NewFirmware is b with the effects given, in the firmware's order.
func NewFirmware(b *Board, effects ...string) *Firmware {
	return &Firmware{Board: b, names: append([]string(nil), effects...)}
}

// Effects are the firmware's effects.
func (f *Firmware) Effects() []string { return append([]string(nil), f.names...) }

// Catalogue numbers the effects from 1, each taking one colour unless it is
// colourless.
func (f *Firmware) Catalogue() []lighting.EffectInfo {
	out := make([]lighting.EffectInfo, len(f.names))
	for i, name := range f.names {
		out[i] = lighting.EffectInfo{Name: name, Number: i + 1}
		if f.coloured(name) {
			out[i].MinColours, out[i].MaxColours = 1, 1
		}
	}
	return out
}

func (f *Firmware) coloured(name string) bool {
	return !slices.ContainsFunc(f.Colourless, func(c string) bool { return strings.EqualFold(c, name) })
}

// Effect is the effect set last, with the stored colour, or
// lighting.ErrUnknownEffect before one is.
func (f *Firmware) Effect(context.Context) (lighting.Effect, error) {
	f.fmu.Lock()
	defer f.fmu.Unlock()
	if f.running == "" {
		return lighting.Effect{}, fmt.Errorf("%s: %w", f.Name, lighting.ErrUnknownEffect)
	}
	out := lighting.Effect{Name: f.running, Coloured: f.coloured(f.running)}
	if out.Coloured {
		c := f.colour
		out.Colour = &c
	}
	return out, nil
}

// SetEffect runs a named effect, matched without regard to case.
func (f *Firmware) SetEffect(_ context.Context, name string) (bool, error) {
	f.fmu.Lock()
	defer f.fmu.Unlock()
	i := slices.IndexFunc(f.names, func(n string) bool { return strings.EqualFold(n, name) })
	if i < 0 {
		return false, fmt.Errorf("%q: %w", name, lighting.ErrNoEffect)
	}
	if f.running == f.names[i] {
		return false, nil
	}
	f.running = f.names[i]
	f.writes++
	return true, nil
}

// SetColour stores the effects' colour.
func (f *Firmware) SetColour(_ context.Context, c lighting.Colour) (bool, error) {
	f.fmu.Lock()
	defer f.fmu.Unlock()
	if f.colour == c {
		return false, nil
	}
	f.colour = c
	f.writes++
	return true, nil
}

// Running is the effect set last and its colour, "" before one is.
func (f *Firmware) Running() (string, lighting.Colour) {
	f.fmu.Lock()
	defer f.fmu.Unlock()
	return f.running, f.colour
}

// Writes counts the effect and colour writes that changed something.
func (f *Firmware) Writes() int {
	f.fmu.Lock()
	defer f.fmu.Unlock()
	return f.writes
}
