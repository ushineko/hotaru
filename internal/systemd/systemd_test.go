package systemd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestARunningUnitIsTheOneTalkedAbout(t *testing.T) {
	facts := choose([]unit{
		{"openrgb-server.service", true, false},
		{"openrgb.service", false, true},
	}, true)

	require.True(t, facts.Active)
	require.Equal(t, "openrgb.service", facts.Unit)
	require.False(t, facts.User)
}

func TestWhenNoneIsRunningTheFirstCandidateWins(t *testing.T) {
	/*
		The development machine: the user unit installed and stopped, and the
		system unit merely present and disabled. The advice was
		`sudo systemctl start openrgb.service` -- the wrong unit, the wrong
		scope, and the only thing hotaru says when lighting is not working.

		The candidate list is ordered by what is worth trying. A later match
		used to overwrite an earlier one, which threw that order away.
	*/
	facts := choose([]unit{
		{"openrgb-server.service", true, false},
		{"openrgb.service", false, false},
	}, true)

	require.True(t, facts.Installed)
	require.False(t, facts.Active)
	require.Equal(t, "openrgb-server.service", facts.Unit)
	require.True(t, facts.User, "a user unit was reported as a system one")

	remedy := strings.Join(facts.Remedies(), " ")
	require.Contains(t, remedy, "systemctl --user start openrgb-server.service")
	require.NotContains(t, remedy, "sudo", "a user unit does not need root")
}

func TestAMachineWithNoOpenRGBSaysSo(t *testing.T) {
	facts := choose(nil, true)
	require.False(t, facts.Installed)
	require.Contains(t, strings.Join(facts.Remedies(), " "), "does not appear to be installed")
}

func TestLingeringIsReportedSeparately(t *testing.T) {
	// Lighting that works now and not at boot is its own problem, and the
	// remedy for it is not the remedy for a stopped server.
	facts := choose([]unit{{"openrgb-server.service", true, true}}, false)
	remedy := strings.Join(facts.Remedies(), " ")
	require.Contains(t, remedy, "enable-linger")
	require.NotContains(t, remedy, "start openrgb")
}
