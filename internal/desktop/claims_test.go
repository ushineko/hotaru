package desktop_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/desktop"
)

// shortcuts is a kglobalshortcutsrc as KDE writes one, with the monitor's
// entries in it and other programs' entries around them.
func shortcuts(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kglobalshortcutsrc")
	body := `[kwin]
AIOScene1=Ctrl+Alt+Num+1,none,AIO lighting scene 1
AIOScene4=Ctrl+Alt+Num+4,none,AIO lighting scene 4
AIOScene11=Ctrl+Alt+Shift+Num+1,none,AIO lighting scene 11
HotaruCtrlAltNum2=Ctrl+Alt+Num+2,none,hotaru: apply green
Switch One Desktop Down=Ctrl+Alt+Down,Ctrl+Alt+Down,Switch One Desktop Down
_k_friendly_name=KWin

[org.kde.spectacle.desktop]
RectangularRegionScreenShot=Meta+Shift+Print,none,Capture Rectangular Region
`
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestAnotherProgramsClaimOnOurKeysIsFound(t *testing.T) {
	/*
		The fault this exists for: KDE keeps the entry after the program that
		made it has gone, and while it is there hotaru's own registration
		succeeds and the key does nothing at all.
	*/
	found, err := desktop.Claimed(shortcuts(t), []string{"Ctrl+Alt+Num+1", "Ctrl+Alt+Num+4"})

	require.NoError(t, err)
	require.Len(t, found, 2)
	require.Equal(t, "AIOScene1", found[0].Entry)
	require.Equal(t, "kwin", found[0].Component)
}

func TestHotarusOwnEntriesAreNotAClaimAgainstItself(t *testing.T) {
	found, err := desktop.Claimed(shortcuts(t), []string{"Ctrl+Alt+Num+2"})
	require.NoError(t, err)
	require.Empty(t, found)
}

func TestKeysNobodyAskedAboutAreNotReported(t *testing.T) {
	// Every other program's shortcuts are none of hotaru's business, and a
	// listing that included them would be a reason not to trust the rest.
	found, err := desktop.Claimed(shortcuts(t), []string{"Ctrl+Alt+Num+1"})
	require.NoError(t, err)
	require.Len(t, found, 1)
}

func TestNoShortcutsFileIsNotAFault(t *testing.T) {
	// A machine that is not running KDE, or one that has never bound
	// anything. Neither is a problem to report.
	found, err := desktop.Claimed(filepath.Join(t.TempDir(), "absent"), []string{"Ctrl+Alt+Num+1"})
	require.NoError(t, err)
	require.Empty(t, found)
}

func TestReleasingRemovesOnlyTheNamedEntries(t *testing.T) {
	/*
		It is not hotaru's file. Every line it does not understand -- other
		programs' shortcuts, section headers, the friendly names KDE keeps --
		has to come out exactly as it went in.
	*/
	path := shortcuts(t)
	claims, err := desktop.Claimed(path, []string{"Ctrl+Alt+Num+1", "Ctrl+Alt+Num+4"})
	require.NoError(t, err)

	removed, err := desktop.Release(path, claims)
	require.NoError(t, err)
	require.Equal(t, 2, removed)

	after, err := os.ReadFile(path) //nolint:gosec // the test's own file
	require.NoError(t, err)
	body := string(after)

	require.NotContains(t, body, "AIOScene1=")
	require.NotContains(t, body, "AIOScene4=")
	require.Contains(t, body, "AIOScene11=", "an entry on a key hotaru does not want was removed")
	require.Contains(t, body, "Switch One Desktop Down=")
	require.Contains(t, body, "[org.kde.spectacle.desktop]")
	require.Contains(t, body, "_k_friendly_name=KWin")
}

func TestReleasingNothingChangesNothing(t *testing.T) {
	path := shortcuts(t)
	before, err := os.ReadFile(path) //nolint:gosec // the test's own file
	require.NoError(t, err)

	removed, err := desktop.Release(path, nil)
	require.NoError(t, err)
	require.Zero(t, removed)

	after, err := os.ReadFile(path) //nolint:gosec // the test's own file
	require.NoError(t, err)
	require.Equal(t, before, after)
}
