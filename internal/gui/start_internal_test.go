package gui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The card names the command of the system the window runs on (spec 072
// R5.1).
func TestTheStartCommandIsThisSystems(t *testing.T) {
	require.Equal(t, "systemctl --user start hotaru", startCommand("linux"))
	require.Equal(t, "Start-Service hotaru", startCommand("windows"))
}
