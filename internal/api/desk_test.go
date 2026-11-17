package api_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/service/servicetest"
	"github.com/ushineko/hotaru/internal/testsock"
	"github.com/ushineko/sanshoku"
)

/*
The machine the API tests draw on since spec 076: canvases, as sanshoku's
drivers give them, named as the desk's devices are. Each is a fresh fake.
*/

// auraBoard is one header of four lights.
func auraBoard() *canvastest.Board {
	return canvastest.NewBoard("ASUS ROG MAXIMUS Z790 HERO", canvastest.Zone("Addressable 1", 4))
}

// k4 is a keyboard of two keys with a reactive effect of its own.
func k4() *canvastest.Firmware {
	return canvastest.NewFirmware(canvastest.NewBoard("Keychron K4 HE", canvastest.Zone("Keyboard", 2)),
		"Solid Reactive")
}

// drawing is the service over a real socket, as a shell meets it, drawing on
// devs with no OpenRGB server.
func drawing(t *testing.T, cfg *config.Config, devs ...sanshoku.Device) *api.Client {
	t.Helper()
	return serve(t, servicetest.New(t, cfg, devs...))
}

// serve is svc over a real socket.
func serve(t *testing.T, svc *service.Service) *api.Client {
	t.Helper()

	// A short path: a Unix socket's address has a low length limit, and
	// t.TempDir() under a long test name can exceed it.
	socket := testsock.Path(t)
	listener, err := api.Listen(t.Context(), socket)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- api.Serve(ctx, listener, svc) }()

	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(3 * time.Second):
			t.Fatal("the service did not stop")
		}
	})
	return api.NewClient(socket)
}

// hexAt is what a fake shows on its key id (numbered from 1), or "" for
// nothing yet.
func hexAt(d *canvastest.Device, id byte) string {
	p, ok := d.Showing(id)
	if !ok {
		return ""
	}
	return "#" + hex2(p.R) + hex2(p.G) + hex2(p.B)
}

func hex2(b uint8) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[b>>4], digits[b&15]})
}

// shows waits for a fake to show want on key id.
func shows(t *testing.T, d *canvastest.Device, id byte, want string) {
	t.Helper()
	require.Eventually(t, func() bool { return hexAt(d, id) == want }, 3*time.Second, 5*time.Millisecond,
		"%s key %d shows %q, not %s", d.Name, id, hexAt(d, id), want)
}
