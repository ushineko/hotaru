/*
Package servicetest is a service drawing on canvas devices in memory, as the
daemon draws on sanshoku's: each device adapted, an animator running on a
test clock, and the canvas attached. Tests in every package that needs lights
build their machine here (spec 076).
*/
package servicetest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ushineko/hotaru/internal/canvas"
	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/sanshoku"
)

// Drawn is a device attached to a service.
type Drawn struct {
	Device *canvas.Device
	Clock  *canvastest.Clock
	Attach canvas.Attached
}

/*
Attach draws on dev as the daemon does, and stops drawing when the test ends.
Lengths a test wants are given with dev's Resize before it is attached, as
the daemon gives a rule's.
*/
func Attach(t testing.TB, svc *service.Service, dev sanshoku.Device) *Drawn {
	t.Helper()
	device, ok := canvas.New(dev)
	require.True(t, ok, "%s is not a canvas", dev.Identity().Name)
	clock := canvastest.NewClock()
	anim := canvas.NewAnimator(device, clock, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = anim.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	d := &Drawn{Device: device, Clock: clock, Attach: canvas.Attached{Device: device, Animator: anim}}
	svc.AttachCanvas(d.Attach)
	return d
}

// New is a service with cfg, drawing on devs.
func New(t testing.TB, cfg *config.Config, devs ...sanshoku.Device) *service.Service {
	t.Helper()
	svc := service.New(cfg)
	for _, dev := range devs {
		Attach(t, svc, dev)
	}
	return svc
}
