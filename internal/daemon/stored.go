package daemon

import (
	"context"
	"strings"
	"time"

	"github.com/ushineko/hotaru/internal/canvas"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/state"
	"github.com/ushineko/sanshoku/lighting"
)

/*
watchEvery is how often a device with a stored effect is asked whether it is
there (spec 071 R6.1). One 20-byte report each way on the F75's receiver,
about 10 ms of radio.
*/
const watchEvery = 10 * time.Second

/*
factoryEffect is the effect the AULA F75 runs after a power cycle, with every
effect's colour back to the factory's (sanshoku spec 023 E4). Waking from
sleep leaves its settings as they were, so this is what tells a power cycle
from a nap without reading the keyboard on every tick.
*/
const factoryEffect = "Neon stream"

// watchedDevice is the part of an attached canvas the watcher uses.
type watchedDevice interface {
	Name() string
	Effects() []string
	Answers(ctx context.Context) (answered, askable bool)
	ReadFirmware(ctx context.Context) (lighting.Effect, error)
	Status() canvas.Status
	NoteAnswer(answered bool, at time.Time)
	NotePutBack(at time.Time)
}

// watchHost is the part of the service the watcher uses.
type watchHost interface {
	Desired() state.Snapshot
	PutBack(ctx context.Context, name string) (service.Restore, error)
}

/*
watchStored puts a stored effect back after the device was switched off and
on (spec 071 R6), until the context ends.

On each tick, while the device's recorded mode is one of its stored effects
and nothing is being drawn on it, the device is asked one short question.
When it answers after being silent, its effect is read. If it came back on
its factory effect and that is not what hotaru recorded, the recorded effect
and colour are written back. A device that kept answering is never written:
a change made with its own keys is somebody's choice (R6.3).

tick is the clock, a seam so that a test drives it without sleeping. now
stamps what the watcher notes.
*/
func watchStored(ctx context.Context, host watchHost, device watchedDevice, tick <-chan time.Time,
	now func() time.Time, report func(string, ...any),
) {
	if len(device.Effects()) == 0 {
		return
	}
	silent := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
		}
		recorded, ok := recordedStored(host.Desired(), device)
		if !ok || device.Status().Effect != "" {
			continue // nothing stored to keep, or frames keep the lights (R6.4)
		}
		answered, askable := device.Answers(ctx)
		if !askable {
			return
		}
		device.NoteAnswer(answered, now())
		if !answered {
			silent = true
			continue
		}
		if !silent {
			continue
		}
		running, err := device.ReadFirmware(ctx)
		if err != nil {
			/*
				Still counted as just back, so the next tick reads again.
				Measured: the first read after a power-on lost 2 of the
				palette's 35 packets, while the keyboard was starting.
			*/
			report("%s answered again and its effect could not be read yet: %v", device.Name(), err)
			continue
		}
		silent = false
		if !strings.EqualFold(running.Name, factoryEffect) {
			continue // woke from sleep, or a person chose it: kept
		}
		// Put back compares with what was just read and writes only what
		// differs, so a keyboard recorded on its factory effect costs nothing.
		restore, err := host.PutBack(ctx, device.Name())
		switch {
		case err != nil:
			report("%s came back on its factory settings, and %q could not be put back: %v",
				device.Name(), recorded, err)
		case restore.Applied > 0:
			device.NotePutBack(now())
			report("%s came back on its factory settings; put %q back", device.Name(), recorded)
		}
	}
}

// recordedStored is the device's recorded mode where it is one of the
// device's stored effects.
func recordedStored(desired state.Snapshot, device watchedDevice) (string, bool) {
	kept, ok := desired.Devices[device.Name()]
	if !ok {
		return "", false
	}
	for _, name := range device.Effects() {
		if strings.EqualFold(name, kept.Mode) {
			return name, true
		}
	}
	return "", false
}
