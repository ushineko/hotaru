package services

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// What hotaru says about OpenRGB on Windows names PowerShell, not systemctl,
// and says nothing about lingering, which Windows has no use for.
func TestWindowsRemediesNamePowerShell(t *testing.T) {
	stopped := Facts{Manager: SCM, Installed: true, Unit: "OpenRGB", Lingering: true}
	require.Equal(t, []string{"The OpenRGB server is installed and not running. Start it with `Start-Service OpenRGB (in PowerShell, as an administrator)`."},
		stopped.Remedies())
	require.Equal(t, "Restart-Service OpenRGB (in PowerShell, as an administrator)", stopped.RestartCommand())

	absent := Facts{Manager: SCM, Lingering: true}
	require.Len(t, absent.Remedies(), 1)
	require.True(t, strings.HasPrefix(absent.Remedies()[0], "OpenRGB does not appear to be installed"))

	running := Facts{Manager: SCM, Installed: true, Unit: "OpenRGB", Active: true}
	require.Empty(t, running.Remedies(), "no lingering advice on Windows, whatever the field says")
}

func TestLookAsksTheServiceManager(t *testing.T) {
	facts := look(t.Context())
	require.Equal(t, SCM, facts.Manager)
	if facts.Installed {
		require.Equal(t, "OpenRGB", facts.Unit)
	}
}

func TestBouncingNothingIsNotOurs(t *testing.T) {
	require.ErrorIs(t, bounce(context.Background(), Facts{Manager: SCM}), ErrNotRestartable)
}
