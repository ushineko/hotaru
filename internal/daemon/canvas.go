package daemon

import (
	"context"
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
		mu        sync.Mutex
		held      = map[string]bool{}
		wg        sync.WaitGroup
		lost      = make(chan struct{}, 1)
		announced bool
	)
	defer wg.Wait()

	attached := func(path string) bool {
		mu.Lock()
		defer mu.Unlock()
		return held[path]
	}
	for attempt := 0; ; attempt++ {
		found, err := open(ctx, attached)
		for _, device := range found {
			path := device.Path()
			mu.Lock()
			held[path] = true
			mu.Unlock()
			attempt = 0
			wg.Add(1)
			go func() {
				defer wg.Done()
				gone := serveCanvas(ctx, svc, device, report)
				mu.Lock()
				delete(held, path)
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
	attachment := canvas.Attached{Device: device, Animator: animator}
	drawn := make(chan struct{})
	go func() {
		defer close(drawn)
		_ = animator.Run(drawing) // ErrGone closes the adapter, which Gone below sees
	}()

	svc.AttachCanvas(attachment)
	report("drawing on %s at %s", device.Name(), device.Path())
	if _, err := svc.Reconcile(ctx, []string{device.Name()}); err != nil {
		// No server yet: the reconciler restores this device with the rest
		// when one answers, because it is recorded and was missing.
		report("%s waits for the OpenRGB server to put its scene back: %v", device.Name(), err)
	}

	check := time.NewTicker(presentEvery)
	defer check.Stop()
	gone := false
	for !gone {
		select {
		case <-ctx.Done():
			stop()
			<-drawn
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
	svc.DetachCanvas(attachment)
	report("%s went away; looking for it again", device.Name())
	return true
}
