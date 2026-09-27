package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/scenes"
)

/*
Every way a scene reaches the hardware writes the same packets in the same
order.

The ordering is spec 056's whole subject, and it lives in one place -- but
"one place" is a claim about the code, and this is the claim about the
behaviour. A scene applied from a list, previewed from an editor, changed
under a lease, or re-asserted after one is released must all arrive the same
way, because the device cannot tell which button was pressed and the failure
is invisible when it happens.
*/

// reactiveScene is the shape that exposes the ordering: a one-colour effect
// with its own colour, over per-key colours nothing like it.
func reactiveScene(name string) scenes.Scene {
	return scenes.Scene{
		Name:        name,
		Assignments: []scenes.Assignment{{Target: "Keychron", Colour: "#8c6f6a"}},
		Effects: map[string]scenes.Effect{
			"Keychron": {Mode: "Solid Reactive", Colour: "#67798c"},
		},
	}
}

/*
arrivingOrder is what a device arriving at a colour-carrying mode receives:
the mode, the frame, and the mode again once it has entered it.

heldOrder is what a device already in that mode receives. One packet, because
it took the colour on the first one -- which is spec 052's finding and is why
re-applying the scene the machine is showing costs nothing.
*/
var (
	arrivingOrder = []string{"mode:Solid Reactive", "frame", "mode:Solid Reactive"}
	heldOrder     = []string{"mode:Solid Reactive", "frame"}
)

func TestEveryPathToTheHardwareWritesTheSameOrder(t *testing.T) {
	ctx := context.Background()

	t.Run("applied from a list or a keypress", func(t *testing.T) {
		svc, server := lit(t, reactiveScene("evening"))
		_, err := svc.ApplyScene(ctx, "evening")
		require.NoError(t, err)
		require.Equal(t, arrivingOrder, server.Sequence)
	})

	t.Run("previewed from an editor", func(t *testing.T) {
		svc, server := lit(t)
		_, err := svc.Preview(ctx, reactiveScene(""), "editor", false)
		require.NoError(t, err)
		require.Equal(t, arrivingOrder, server.Sequence)
	})

	t.Run("changed under a lease the editor holds", func(t *testing.T) {
		// The draft put the device in the mode, so changing it finds it
		// already there: one packet, and no pause, on every keystroke an
		// editor sends.
		svc, server := lit(t)
		up, err := svc.Preview(ctx, reactiveScene(""), "editor", false)
		require.NoError(t, err)
		server.Sequence = nil

		_, err = svc.Redraft(ctx, up.Lease.Token, reactiveScene(""))
		require.NoError(t, err)
		require.Equal(t, heldOrder, server.Sequence)
	})

	t.Run("applied while the window holds a preview", func(t *testing.T) {
		// What the GUI does that the terminal never does: the editor has
		// been open, so a lease is still standing when somebody presses
		// Apply on another scene.
		svc, server := lit(t, reactiveScene("evening"))
		_, err := svc.Preview(ctx, scenes.Scene{
			Assignments: []scenes.Assignment{{Target: "ASUS", Colour: "blue"}},
		}, "the window", false)
		require.NoError(t, err)
		server.Sequence = nil

		_, err = svc.ApplyScene(ctx, "evening")
		require.NoError(t, err)
		require.Equal(t, arrivingOrder, server.Sequence)
	})

	// The re-assert is the one case where the device is already in the mode,
	// so it is the one that writes fewer packets. Asserted rather than
	// glossed over: it is the same code reaching the same conclusion, and a
	// re-assert that wrote twice would be a pause nobody asked for on a loop
	// that runs for as long as the machine is on.
	t.Run("re-asserted after a preview is released", func(t *testing.T) {
		svc, server := lit(t, reactiveScene("evening"))
		_, err := svc.ApplyScene(ctx, "evening")
		require.NoError(t, err)
		server.Sequence = nil

		_, err = svc.Reconcile(ctx, []string{"Keychron K4 HE"})
		require.NoError(t, err)
		require.Equal(t, heldOrder, server.Sequence,
			"a re-assert of a device already in the mode should not write it twice")
	})
}

/*
The pause between the two mode packets, and who pays for it.

**Measured on the keyboard with hotaru out of the loop.** Hotaru's exact three
packets sent back to back show the previous colour; the identical packets with
a gap between the two mode writes show the right one. A gap after the frame
instead does nothing. So the device needs time to finish entering the mode
before the packet that colours it, and nothing else in the sequence cares.

The cost is scoped in the two ways that matter: a device already in the mode
took the colour on the first packet, and a mode with no colour of its own has
nothing to commit.
*/
func TestOnlyAModeArrivingWithAColourOfItsOwnWaits(t *testing.T) {
	ctx := context.Background()

	t.Run("arriving at a colour-carrying mode waits", func(t *testing.T) {
		svc, _ := lit(t, reactiveScene("evening"))
		start := time.Now()
		_, err := svc.ApplyScene(ctx, "evening")
		require.NoError(t, err)
		require.GreaterOrEqual(t, time.Since(start), 100*time.Millisecond,
			"the colour was committed without giving the mode time to take")
	})

	t.Run("already in the mode does not wait", func(t *testing.T) {
		svc, _ := lit(t, reactiveScene("evening"))
		_, err := svc.ApplyScene(ctx, "evening")
		require.NoError(t, err)

		// The device is in the mode now, so the second packet has nothing to
		// wait for: applying the same scene again is the common case and must
		// stay fast.
		start := time.Now()
		_, err = svc.ApplyScene(ctx, "evening")
		require.NoError(t, err)
		require.Less(t, time.Since(start), 50*time.Millisecond,
			"re-applying a scene the machine is already showing is sleeping")
	})

	t.Run("a mode with no colour of its own does not wait", func(t *testing.T) {
		// Every device solid, which is the commonest scene there is.
		svc, _ := lit(t, scenes.Scene{Name: "red", Colour: "red"})
		start := time.Now()
		_, err := svc.ApplyScene(ctx, "red")
		require.NoError(t, err)
		require.Less(t, time.Since(start), 50*time.Millisecond,
			"the ordinary write path is sleeping")
	})
}
