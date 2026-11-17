package gui_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/canvas/canvastest"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/gui"
	"github.com/ushineko/hotaru/internal/service/servicetest"
	"github.com/ushineko/hotaru/internal/state"
	"github.com/ushineko/hotaru/internal/testsock"
)

/*
The window's hold on the hardware, against a real service.

A preview is lights somebody has to get back, and every other test in this
package talks to fixed JSON: a fake that answers 200 to everything would pass
whether or not a lease was ever taken. So this file runs the service the window
talks to, and reads the answer off the devices.
*/
func previewing(t *testing.T) *gui.App {
	t.Helper()

	dir := t.TempDir()
	socket := testsock.Path(t)
	listener, err := api.Listen(t.Context(), socket)
	require.NoError(t, err)

	desired, err := state.Open(filepath.Join(dir, "state.yml"))
	require.NoError(t, err)

	svc := servicetest.New(t, &config.Config{Scope: []string{"maximus"}}, lit())
	svc.SetRecorder(desired)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- api.Serve(ctx, listener, svc) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("the service did not stop")
		}
	})

	return gui.New(api.NewClient(socket))
}

// lit is a device in scope, which is what a preview covers.
func lit() *canvastest.Board {
	return canvastest.NewBoard("ASUS ROG MAXIMUS Z790 HERO", canvastest.Zone("Addressable 1", 4))
}

func TestTheWindowsPreviewIsALeaseTakenOnceAndGivenBack(t *testing.T) {
	/*
		The two halves of spec 018's preview, which had no test between them:
		that the draft on the hardware is held under a lease the service can
		see, and that letting go puts the lights back.

		Taken once and kept, because releasing and re-taking it for every
		change makes the service put the lights back a moment before each new
		colour arrives -- which is the colour flashing back that made the
		picker look broken while it did exactly what it was told.
	*/
	app := previewing(t)
	ctx := t.Context()

	require.False(t, app.Previewing(), "it was holding something before it started")

	draft := api.Scene{Name: "draft", Colour: "blue"}
	require.NoError(t, app.Preview(ctx, draft))
	require.True(t, app.Previewing())

	held, err := app.Client().Devices(ctx)
	require.NoError(t, err)
	require.NotNil(t, held[0].Preview, "the service does not know the lights are held")
	require.Equal(t, "hotaru-gui", held[0].Preview.Holder)

	// A second colour is the same lease, not a release and another.
	require.NoError(t, app.Preview(ctx, api.Scene{Name: "draft", Colour: "red"}))
	still, err := app.Client().Devices(ctx)
	require.NoError(t, err)
	require.NotNil(t, still[0].Preview, "the second colour dropped the lease")
	require.Equal(t, held[0].Preview.Token, still[0].Preview.Token,
		"the lease was released and taken again between colours")

	// And letting go gives the hardware back.
	app.EndPreview()
	require.False(t, app.Previewing())

	require.Eventually(t, func() bool {
		back, err := app.Client().Devices(ctx)
		return err == nil && back[0].Preview == nil
	}, 3*time.Second, 20*time.Millisecond, "the lights are still held")
}

func TestEndingAPreviewThatIsNotThereIsFine(t *testing.T) {
	// Everything that should end a preview calls it -- leaving the editor,
	// saving, closing the window -- because a rule to be remembered in four
	// places is one that will be forgotten in one.
	app := previewing(t)
	require.NotPanics(t, app.EndPreview)
	require.False(t, app.Previewing())
}
