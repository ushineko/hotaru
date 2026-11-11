package canvastest

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ushineko/sanshoku/battery"
	"github.com/ushineko/sanshoku/hidraw"
	"github.com/ushineko/sanshoku/lighting"
)

/*
Keyboard is a canvas whose frames fade back to an effect its firmware stores,
as the AULA F75 through its receiver (sanshoku specs 019 and 023).

It carries only what was measured there: a frame shows for Hold and then the
firmware's effect returns; each effect keeps a colour of its own, and Off and
Self-define take none; a write that changes nothing writes nothing; a keyboard
that is off answers nothing; and a power cycle puts it on Neon stream with
every effect on its factory colours.
*/
type Keyboard struct {
	*Device
	// HoldFor is Hold; zero is 500 ms.
	HoldFor time.Duration

	kmu        sync.Mutex
	effect     string
	colours    map[string]lighting.Colour
	off        bool
	effectSets int
	colourSets int
	asked      int
	failReads  int
}

// FailReads makes the next n reads of the effect go unanswered, as the first
// read after a power-on can.
func (k *Keyboard) FailReads(n int) {
	k.kmu.Lock()
	defer k.kmu.Unlock()
	k.failReads = n
}

// FactoryEffect is the effect a power cycle leaves the keyboard on.
const FactoryEffect = "Neon stream"

var keyboardEffects = []string{
	"Off", "Fixed on", "Respire", "Rainbow", "Flash away", "Raindrops", "Rainbow wheel",
	"Ripples shining", "Stars twinkle", "Shadow disappear", "Retro snake", "Neon stream",
	"Reaction", "Sine wave", "Retinue scanning", "Rotating windmill", "Colourful waterfall",
	"Blossoming", "Rotating storm", "Self-define",
}

// NewKeyboard is a keyboard of n lights running the factory effect.
func NewKeyboard(n int) *Keyboard {
	k := &Keyboard{Device: New(n)}
	k.Name, k.Path = "fake keyboard", "fake:keyboard"
	k.PowerCycle()
	return k
}

// Hold is HoldFor, or 500 ms.
func (k *Keyboard) Hold() time.Duration {
	if k.HoldFor > 0 {
		return k.HoldFor
	}
	return 500 * time.Millisecond
}

// Effects are the firmware's effects, in its order.
func (k *Keyboard) Effects() []string { return append([]string(nil), keyboardEffects...) }

func colourless(name string) bool { return name == "Off" || name == "Self-define" }

// Effect is the running effect and its colour, or ErrSilent while off.
func (k *Keyboard) Effect(context.Context) (lighting.Effect, error) {
	k.kmu.Lock()
	defer k.kmu.Unlock()
	if k.off || k.failReads > 0 {
		if k.failReads > 0 {
			k.failReads--
		}
		return lighting.Effect{}, fmt.Errorf("reading %s: %w", k.Path, hidraw.ErrSilent)
	}
	out := lighting.Effect{Name: k.effect, Coloured: !colourless(k.effect)}
	if c, ok := k.colours[k.effect]; ok && out.Coloured {
		out.Colour = &c
	}
	return out, nil
}

// SetEffect runs a named effect, writing only where it differs.
func (k *Keyboard) SetEffect(_ context.Context, name string) (bool, error) {
	k.kmu.Lock()
	defer k.kmu.Unlock()
	found := ""
	for _, e := range keyboardEffects {
		if strings.EqualFold(e, name) {
			found = e
		}
	}
	switch {
	case found == "":
		return false, fmt.Errorf("%q: %w", name, lighting.ErrNoEffect)
	case k.off:
		return false, fmt.Errorf("setting %s: %w", k.Path, hidraw.ErrSilent)
	case found == k.effect:
		return false, nil
	}
	k.effect = found
	k.effectSets++
	return true, nil
}

// SetColour gives the running effect a colour, writing only where it differs.
func (k *Keyboard) SetColour(_ context.Context, c lighting.Colour) (bool, error) {
	k.kmu.Lock()
	defer k.kmu.Unlock()
	switch {
	case k.off:
		return false, fmt.Errorf("setting %s: %w", k.Path, hidraw.ErrSilent)
	case colourless(k.effect):
		return false, fmt.Errorf("%s: %w", k.effect, lighting.ErrNoColour)
	}
	if now, ok := k.colours[k.effect]; ok && now == c {
		return false, nil
	}
	k.colours[k.effect] = c
	k.colourSets++
	return true, nil
}

// Batteries answers the battery question, or nothing while off, as the F75
// does: no reading and no error.
func (k *Keyboard) Batteries(context.Context) ([]battery.Battery, error) {
	k.kmu.Lock()
	defer k.kmu.Unlock()
	k.asked++
	if k.off {
		return nil, nil
	}
	return []battery.Battery{{Name: k.Name}}, nil
}

// SwitchOff makes the keyboard answer nothing.
func (k *Keyboard) SwitchOff() {
	k.kmu.Lock()
	defer k.kmu.Unlock()
	k.off = true
}

// SwitchOn makes it answer again, with what it had stored.
func (k *Keyboard) SwitchOn() {
	k.kmu.Lock()
	defer k.kmu.Unlock()
	k.off = false
}

// PowerCycle switches it off and on, which loads the factory settings: Neon
// stream, and no effect with a colour of its own.
func (k *Keyboard) PowerCycle() {
	k.kmu.Lock()
	defer k.kmu.Unlock()
	k.off = false
	k.effect = FactoryEffect
	k.colours = map[string]lighting.Colour{}
}

// Press changes the effect from the keyboard's own keys, which writes no
// count: nothing hotaru sent.
func (k *Keyboard) Press(effect string, c *lighting.Colour) {
	k.kmu.Lock()
	defer k.kmu.Unlock()
	k.effect = effect
	if c != nil {
		k.colours[effect] = *c
	}
}

// Writes is how many effect and colour writes reached the storage.
func (k *Keyboard) Writes() (effects, colours int) {
	k.kmu.Lock()
	defer k.kmu.Unlock()
	return k.effectSets, k.colourSets
}

// Asked is how many times the battery question was asked.
func (k *Keyboard) Asked() int {
	k.kmu.Lock()
	defer k.kmu.Unlock()
	return k.asked
}
