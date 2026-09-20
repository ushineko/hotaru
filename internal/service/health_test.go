package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/config"
	"github.com/ushineko/hotaru/internal/openrgb"
	"github.com/ushineko/hotaru/internal/service"
)

/*
The four states have four remedies, which is the whole reason they are four.

"No lights changed" looks the same whether the server is down, the server is up
with nothing attached, or scope excludes everything present. A user cannot act
on that sentence; they can act on any of these.
*/
func TestHealthTellsTheFourApart(t *testing.T) {
	t.Run("nothing listening", func(t *testing.T) {
		got := service.New(nil, nil, "127.0.0.1:6742").Health(t.Context())
		require.Equal(t, service.StateUnreachable, got.State)
		require.False(t, got.OK())
		require.Contains(t, got.Detail, "Start it")
	})

	t.Run("answering, and knows of nothing", func(t *testing.T) {
		got := service.New(nil, openrgb.NewFake(), "").Health(t.Context())
		require.Equal(t, service.StateNoDevices, got.State)
		require.Contains(t, got.Detail, "detects hardware once",
			"the remedy is a restart, and the reason is not obvious")
	})

	t.Run("hardware present, all of it excluded", func(t *testing.T) {
		cfg := &config.Config{Scope: []string{"something-else"}}
		got := service.New(cfg, openrgb.NewFake(board(), keyboard()), "").Health(t.Context())
		require.Equal(t, service.StateNoneInScope, got.State)
		require.Equal(t, 2, got.Devices)
		require.Zero(t, got.InScope)
		require.Contains(t, got.Detail, "scope")
	})

	t.Run("healthy", func(t *testing.T) {
		got := service.New(nil, openrgb.NewFake(board(), keyboard()), "").Health(t.Context())
		require.Equal(t, service.StateHealthy, got.State)
		require.True(t, got.OK())
		require.Equal(t, 2, got.InScope)
		require.NotZero(t, got.Protocol, "which version was agreed, so a mismatch is visible")
	})
}

func TestAServerThatStopsAnsweringIsUnreachableNotEmpty(t *testing.T) {
	// The difference matters: "no devices" would send someone looking at their
	// hardware when the connection is what broke.
	server := openrgb.NewFake(board())
	server.Unreachable = errors.New("connection reset")

	got := service.New(nil, server, "").Health(t.Context())
	require.Equal(t, service.StateUnreachable, got.State)
	require.Contains(t, got.Detail, "stopped answering")
}

// A machine that offers a way out says so, and says it as a command.
type machine struct{ says []string }

func (m machine) Remedies(context.Context) []string { return m.says }

func TestHealthCarriesWhatAPersonCouldDoAboutIt(t *testing.T) {
	svc := service.New(nil, nil, "127.0.0.1:6742")
	svc.SetEnvironment(machine{says: []string{
		"The OpenRGB server is installed and not running. Start it with `systemctl --user start openrgb-server.service`.",
	}})

	got := svc.Health(t.Context())
	require.Equal(t, service.StateUnreachable, got.State)
	require.Len(t, got.Remedies, 1)
	require.Contains(t, got.Remedies[0], "systemctl --user start",
		"a remedy nobody can act on is an observation")
}

func TestAHealthyMachineIsOfferedNothing(t *testing.T) {
	// Advice when everything works is noise, and noise is what people learn
	// to skip past.
	svc := service.New(nil, openrgb.NewFake(board()), "")
	svc.SetEnvironment(machine{says: []string{"do something"}})

	require.Empty(t, svc.Health(t.Context()).Remedies)
}
