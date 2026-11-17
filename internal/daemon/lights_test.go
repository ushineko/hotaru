package daemon_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/cooler"
	"github.com/ushineko/hotaru/internal/cooler/coolertest"
	"github.com/ushineko/hotaru/internal/daemon"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/sanshoku/lighting"
)

// litCooler is a cooler whose device is also a canvas, as sanshoku's Kraken
// is: one device, one handle, closed by the cooler (spec 073).
type litCooler struct {
	*coolertest.Device
	lights *canvastest.Device
}

func (l litCooler) Keys() []lighting.Key { return l.lights.Keys() }
func (l litCooler) Floor() time.Duration { return l.lights.Floor() }
func (l litCooler) Frame(ctx context.Context, px []lighting.Pixel) error {
	return l.lights.Frame(ctx, px)
}
func (l litCooler) Release(ctx context.Context) error { return l.lights.Release(ctx) }

// canvasNamed is whether the service lists a canvas device of that name.
func canvasNamed(t *testing.T, svc *service.Service, name string) bool {
	views, err := svc.List(t.Context())
	if err != nil {
		return false
	}
	for _, v := range views {
		if v.Device.Name == name && v.Device.Canvas != nil {
			return true
		}
	}
	return false
}

// The cooler's own device is drawn on as a canvas, once, and the cooler
// closes it after the canvas has gone (R1.1, R1.2).
func TestTheCoolersLightsAreDrawnOnItsOwnHandle(t *testing.T) {
	lit := litCooler{Device: coolertest.New(), lights: canvastest.New(6)}
	opens := 0
	open := func(context.Context) (*cooler.Cooler, error) {
		opens++
		return cooler.New(lit), nil
	}
	svc := service.New(nil)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		daemon.Attach(ctx, svc, open, quick, func(string, ...any) {})
	}()

	require.Eventually(t, func() bool { return canvasNamed(t, svc, lit.Identity().Name) },
		2*time.Second, 5*time.Millisecond, "the cooler's lights were not attached as a canvas")

	cancel()
	<-done
	require.Equal(t, 1, opens, "the cooler was opened more than once")
	require.True(t, lit.Closed(), "stopping the service left the cooler open")
	require.False(t, lit.lights.Closed(), "the canvas closed the device itself")
	require.False(t, canvasNamed(t, svc, lit.Identity().Name), "the canvas outlived its cooler")
}
