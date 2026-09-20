package state_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/colour"
	"github.com/ushineko/hotaru/internal/state"
)

func TestAFreshMachineRemembersNothingAndWritesNothing(t *testing.T) {
	// The inert rule, which is not a special case: it is what an empty file
	// does. Nothing to restore means nothing is written to any device.
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "state.yml"))
	require.NoError(t, err)

	require.True(t, store.Snapshot().Empty())
	require.Empty(t, store.Snapshot().Names())

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries, "opening state created a file before anything was recorded")
}

func TestWhatWasAskedForSurvivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	red, blue := colour.MustParse("red"), colour.MustParse("blue")

	store, err := state.Open(path)
	require.NoError(t, err)
	require.NoError(t, store.Record("NZXT Kraken", state.Device{
		Mode:    "Static",
		Colours: []colour.Colour{red, red, blue},
		Applied: time.Now(),
	}))
	require.NoError(t, store.Flush())

	again, err := state.Open(path)
	require.NoError(t, err)
	got := again.Snapshot()
	require.False(t, got.Empty())
	require.Equal(t, "Static", got.Devices["NZXT Kraken"].Mode)
	require.Equal(t, []colour.Colour{red, red, blue}, got.Devices["NZXT Kraken"].Colours)

	frame := got.Devices["NZXT Kraken"].Frame("NZXT Kraken")
	require.Equal(t, "NZXT Kraken", frame.Device)
	require.True(t, frame.PerLED(), "two colours, as recorded")
}

func TestTheStateFileIsYAMLAPersonCanRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	store, err := state.Open(path)
	require.NoError(t, err)
	require.NoError(t, store.Record("Strip", state.Device{
		Mode: "Direct", Colours: []colour.Colour{colour.MustParse("#ff8800")},
	}))
	require.NoError(t, store.Flush())

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(body), "lighting:")
	require.Contains(t, string(body), "#ff8800", "colours stay readable rather than becoming three numbers")
}

func TestUnreadableStateIsDiscardedRatherThanRefused(t *testing.T) {
	// The opposite call to the one made for the rules file, and for the
	// opposite reason. This file is hotaru's own: losing it costs one scene's
	// memory, while refusing to save again would cost every scene after it.
	path := filepath.Join(t.TempDir(), "state.yml")
	require.NoError(t, os.WriteFile(path, []byte("{ this is not yaml ["), 0o600))

	store, err := state.Open(path)
	require.NoError(t, err, "a corrupt cache is not a reason to fail")
	require.True(t, store.Snapshot().Empty())

	require.NoError(t, store.Record("Strip", state.Device{Mode: "Direct"}))
	require.NoError(t, store.Flush(), "and saving works from then on")

	again, err := state.Open(path)
	require.NoError(t, err)
	require.False(t, again.Snapshot().Empty())
}

func TestForgettingADeviceLeavesTheRestAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	store, err := state.Open(path)
	require.NoError(t, err)

	require.NoError(t, store.Record("One", state.Device{Mode: "Direct"}))
	require.NoError(t, store.Record("Two", state.Device{Mode: "Static"}))
	require.NoError(t, store.Forget("One"))
	require.NoError(t, store.Flush())

	got := store.Snapshot()
	require.NotContains(t, got.Devices, "One")
	require.Contains(t, got.Devices, "Two")
}

func TestASnapshotIsACopyNotAWindowIntoTheStore(t *testing.T) {
	// The reconciler holds a snapshot while applying it, and an apply that
	// arrives mid-reconcile must not rewrite what is being read.
	path := filepath.Join(t.TempDir(), "state.yml")
	store, err := state.Open(path)
	require.NoError(t, err)
	require.NoError(t, store.Record("One", state.Device{
		Mode: "Direct", Colours: []colour.Colour{colour.MustParse("red")},
	}))

	snapshot := store.Snapshot()
	snapshot.Devices["One"].Colours[0] = colour.MustParse("blue")
	delete(snapshot.Devices, "One")

	held := store.Snapshot()
	require.Contains(t, held.Devices, "One", "deleting from a snapshot emptied the store")
	require.Equal(t, colour.MustParse("red"), held.Devices["One"].Colours[0],
		"the store's own colours were reachable through the snapshot")
}
