package api_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/api"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/scenes"
	"github.com/ushineko/hotaru/internal/service"
	"github.com/ushineko/hotaru/internal/state"
)

/*
withScenes is the service over a real socket, with somewhere to keep scenes.

A real socket and the real client, because what is being tested here is a lease
bound to a connection: a handler holding a request open and a kernel reporting
the far end going away. An httptest recorder has no far end.
*/
func withScenes(t *testing.T, saved ...scenes.Scene) (*api.Client, *openrgb.Fake) {
	t.Helper()

	dir := t.TempDir()
	socket := filepath.Join(dir, "s")
	listener, err := api.Listen(t.Context(), socket)
	require.NoError(t, err)

	server := openrgb.NewFake(board(), keyboard())
	svc := service.New(nil, server, "127.0.0.1:6742")

	desired, err := state.Open(filepath.Join(dir, "state.yml"))
	require.NoError(t, err)
	svc.SetRecorder(desired)

	store, err := scenes.Open(filepath.Join(dir, "scenes.yml"))
	require.NoError(t, err)
	for _, scene := range saved {
		require.NoError(t, store.Save(scene))
	}
	svc.SetScenes(store)

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
	return api.NewClient(socket), server
}

func blue() scenes.Scene {
	return scenes.Scene{
		Name:        "blue",
		Assignments: []scenes.Assignment{{Target: "Keychron", Colour: "blue"}},
	}
}

func red() scenes.Scene {
	return scenes.Scene{
		Name:        "red",
		Assignments: []scenes.Assignment{{Target: "Keychron", Colour: "red"}},
	}
}

func byName(t *testing.T, saved []api.Scene, name string) api.Scene {
	t.Helper()
	for _, scene := range saved {
		if scene.Name == name {
			return scene
		}
	}
	t.Fatalf("no scene called %q", name)
	return api.Scene{}
}

func colours(t *testing.T, server *openrgb.Fake) string {
	t.Helper()
	frame, ok := server.Showing("Keychron K4 HE")
	require.True(t, ok)
	return frame.Colours[0].String()
}

func TestAScenesRoundTripThroughTheSocket(t *testing.T) {
	client, _ := withScenes(t)

	require.NoError(t, client.SaveScene(t.Context(), api.Scene{
		Name:        "evening",
		Assignments: []api.SceneAssignment{{Target: "Keychron", Colour: "#201040"}},
		Effects:     map[string]string{"Keychron": "Direct"},
		Screen:      api.ScreenDashboard,
	}))

	saved, err := client.Scenes(t.Context())
	require.NoError(t, err)
	mine := byName(t, saved, "evening")
	require.Equal(t, "Direct", mine.Effects["Keychron"])
	require.Equal(t, api.ScreenDashboard, mine.Screen)
	require.False(t, mine.Shipped)

	// The nine shipped scenes are there alongside it, and stay there.
	require.True(t, byName(t, saved, "red").Shipped)

	require.NoError(t, client.DeleteScene(t.Context(), "evening"))
	saved, err = client.Scenes(t.Context())
	require.NoError(t, err)
	for _, scene := range saved {
		require.NotEqual(t, "evening", scene.Name)
	}
}

func TestAPreviewHeldOnAConnectionEndsWhenTheConnectionDoes(t *testing.T) {
	/*
		The whole point of binding a lease to a connection. The client is
		holding an open request; letting go of it -- deliberately here, by
		dying in the real case -- is what puts somebody's lights back, with no
		clock and no heartbeat involved.
	*/
	client, server := withScenes(t, blue(), red())
	_, err := client.ApplyScene(t.Context(), "blue", api.SceneRequest{})
	require.NoError(t, err)
	require.Equal(t, "#0000ff", colours(t, server))

	done, release, err := client.HoldScene(t.Context(), "red", api.SceneRequest{Holder: "a test"})
	require.NoError(t, err)
	require.NotNil(t, done.Preview)
	require.Nil(t, done.Preview.Expires, "a held preview was given a clock as well")
	require.Equal(t, "#ff0000", colours(t, server))

	// Not a release call: the socket simply goes away, which is what happens
	// when the process on the other end is killed.
	release()

	require.Eventually(t, func() bool { return colours(t, server) == "#0000ff" },
		3*time.Second, 10*time.Millisecond,
		"the draft stayed on the hardware after its holder let go")
}

func TestADeviceShowingADraftSaysSoInTheListing(t *testing.T) {
	// A device whose re-assertion is suspended otherwise looks exactly like
	// one that is simply behaving.
	client, _ := withScenes(t, red())

	done, release, err := client.HoldScene(t.Context(), "red", api.SceneRequest{Holder: "a test"})
	require.NoError(t, err)
	defer release()

	found, err := client.Devices(t.Context())
	require.NoError(t, err)

	var marked int
	for _, device := range found {
		if device.Preview != nil {
			marked++
			require.Equal(t, "a test", device.Preview.Holder)
			require.Equal(t, "red", device.Preview.Scene)
			require.Equal(t, done.Preview.Token, device.Preview.Token)
		}
	}
	require.Equal(t, 1, marked)
}

func TestARenewedPreviewIsReleasedByToken(t *testing.T) {
	// The path for a client that cannot sit on a connection: a token it
	// renews, and lets go of explicitly.
	client, server := withScenes(t, blue(), red())
	_, err := client.ApplyScene(t.Context(), "blue", api.SceneRequest{})
	require.NoError(t, err)

	done, err := client.PreviewScene(t.Context(), "red", api.SceneRequest{Holder: "a script"})
	require.NoError(t, err)
	require.NotNil(t, done.Preview.Expires, "an unheld preview was given no expiry to renew")
	require.Equal(t, "#ff0000", colours(t, server))

	require.NoError(t, client.RenewPreview(t.Context(), done.Preview.Token))

	restore, err := client.ReleasePreview(t.Context(), done.Preview.Token)
	require.NoError(t, err)
	require.Equal(t, 1, restore.Applied)
	require.Equal(t, "#0000ff", colours(t, server))
}

func TestCapturingSavesWhatIsShowing(t *testing.T) {
	client, _ := withScenes(t, blue())
	_, err := client.ApplyScene(t.Context(), "blue", api.SceneRequest{})
	require.NoError(t, err)

	captured, err := client.CaptureScene(t.Context(), "kept", api.ScreenReadout)
	require.NoError(t, err)
	require.Equal(t, "kept", captured.Name)
	require.Equal(t, api.ScreenReadout, captured.Screen)

	saved, err := client.Scenes(t.Context())
	require.NoError(t, err)
	require.Equal(t, api.ScreenReadout, byName(t, saved, "kept").Screen)
}
