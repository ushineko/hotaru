package daemon

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/ushineko/hotaru/internal/canvas"
	"github.com/ushineko/hotaru/internal/service"
)

/*
canvasOpener is how canvas devices are found, a seam so a test can hand back
fakes. skip names the paths already attached. The production one is
canvas.Open.
*/
type canvasOpener func(ctx context.Context, skip func(path string) bool) ([]*canvas.Device, error)

/*
presentEvery is how often an attached canvas that is holding its frame is
checked for still being there.

A device sent nothing does not answer ErrGone, and the first canvas device
reboots whenever OpenRGB exits (sanshoku docs/contention.md). Reading one
sysfs link every two seconds is how hotaru notices and puts the scene back
within the backoff, rather than at the next scene somebody applies.
*/
const presentEvery = 2 * time.Second

/*
dyingFor is how long a lost device's node is not attached to again.

The kernel removed a rebooting keyboard's node about two seconds after it
stopped answering (measured 2026-10-01, on its cable). Through its receiver
the node is never removed at all, so this is a window, not a ban.
*/
const dyingFor = 4 * time.Second

/*
attachCanvases keeps every canvas device on the machine attached, as attach
keeps the cooler (spec 059): it waits for one with the same backoff, draws on
it until it goes, and looks for it again.

Several canvases can be attached at once, each with its own animator, and one
going does not detach the others. A canvas that goes restarts the backoff
from its first step, because a device that went away is usually a device
coming back moments later on a new node: a reboot after OpenRGB exits, or
after somebody asked for the firmware's lighting back.
*/
func attachCanvases(ctx context.Context, svc *service.Service, open canvasOpener,
	backoff []time.Duration, report func(string, ...any),
) {
	var (
		mu   sync.Mutex
		held = map[string]bool{}
		/*
			dying is the kernel names of devices that went. The kernel keeps a
			rebooting device's node for a couple of seconds after it stops
			answering, and the look that follows a loss straight away found
			that node and attached to it, only to lose it again (measured
			2026-10-01: twice per reboot). A node still carrying a lost name
			is skipped; the device comes back under a new one.
		*/
		dying     = map[string]time.Time{}
		wg        sync.WaitGroup
		lost      = make(chan struct{}, 1)
		announced bool
	)
	defer wg.Wait()

	attached := func(path string) bool {
		mu.Lock()
		defer mu.Unlock()
		if held[path] {
			return true
		}
		// Only while the kernel could still be removing it. Through a USB
		// receiver the node and its name never change across a reboot, and
		// skipping it for longer would never attach the device again.
		lost, ok := dying[canvas.KernelNode(path)]
		return ok && time.Since(lost) < dyingFor
	}
	for attempt := 0; ; attempt++ {
		found, err := open(ctx, attached)
		for _, device := range found {
			path := device.Path()
			mu.Lock()
			held[path] = true
			clear(dying) // what was being removed has gone by the time a device is back
			mu.Unlock()
			attempt = 0
			wg.Add(1)
			go func() {
				defer wg.Done()
				gone := serveCanvas(ctx, svc, device, report)
				mu.Lock()
				delete(held, path)
				if gone && device.Node() != "" {
					dying[device.Node()] = time.Now()
				}
				mu.Unlock()
				if gone {
					select {
					case lost <- struct{}{}:
					default:
					}
				}
			}()
		}
		if ctx.Err() != nil {
			return
		}
		if err != nil && len(found) == 0 && !announced {
			mu.Lock()
			none := len(held) == 0
			mu.Unlock()
			if none {
				// Once, not every tick: most machines have no such device.
				report("no device to draw on yet: %v", err)
				announced = true
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-lost:
			attempt = -1 // the next look is now, then the backoff from its start
		case <-time.After(backoff[min(attempt, len(backoff)-1)]):
		}
	}
}

/*
serveCanvas draws on one canvas device until the service stops or the device
goes, and reports whether it went.

The animator starts with the attachment and stops with it, and stopping sends
nothing: the device keeps its last frame (spec 060 R4.1). A new attachment
puts back what was last asked of the device, because a device that has just
appeared shows its firmware's own effect, not the scene.
*/
func serveCanvas(ctx context.Context, svc *service.Service, device *canvas.Device,
	report func(string, ...any),
) bool {
	drawing, stop := context.WithCancel(ctx)
	defer stop()

	animator := canvas.NewAnimator(device, nil, func(format string, args ...any) {
		report(device.Name()+": "+format, args...)
	})
	device.SetReport(report)
	attachment := canvas.Attached{Device: device, Animator: animator}
	drawn := make(chan struct{})
	go func() {
		defer close(drawn)
		_ = animator.Run(drawing) // ErrGone closes the adapter, which Gone below sees
	}()

	restore := svc.Reconcile
	if len(device.Effects()) > 0 {
		// What the firmware runs, so that listings and reconciles compare
		// against the device rather than ask it (spec 071 R5.1).
		running, err := device.ReadFirmware(ctx)
		switch {
		case err != nil:
			report("%s's own effect could not be read yet: %v", device.Name(), err)
		case strings.EqualFold(running.Name, factoryEffect):
			// Attached on its factory settings: switched off and on while
			// nobody watched, as the watcher would have seen (R6.2).
			restore = func(ctx context.Context, only []string) (service.Restore, error) {
				return svc.PutBack(ctx, only[0])
			}
		}
	}
	svc.AttachCanvas(attachment)
	report("drawing on %s at %s", device.Name(), device.Path())
	if _, err := restore(ctx, []string{device.Name()}); err != nil {
		// No server yet: the reconciler restores this device with the rest
		// when one answers, because it is recorded and was missing.
		report("%s waits for the OpenRGB server to put its scene back: %v", device.Name(), err)
	}

	watch := time.NewTicker(watchEvery)
	defer watch.Stop()
	watched := make(chan struct{})
	go func() {
		defer close(watched)
		watchStored(drawing, svc, attachment, watch.C, time.Now, report)
	}()

	check := time.NewTicker(presentEvery)
	defer check.Stop()
	gone := false
	for !gone {
		select {
		case <-ctx.Done():
			stop()
			<-drawn
			<-watched
			svc.DetachCanvas(attachment)
			_ = device.Close()
			return false
		case <-device.Gone():
			gone = true
		case <-check.C:
			if !device.Present() {
				device.Lose()
			}
		}
	}
	stop()
	<-drawn
	<-watched
	svc.DetachCanvas(attachment)
	report("%s went away; looking for it again", device.Name())
	return true
}
