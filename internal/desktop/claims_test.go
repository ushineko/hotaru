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

func TestAClaimCarriesWhatKDENeedsToForgetIt(t *testing.T) {
	/*
		Releasing goes through kglobalaccel rather than the file, so a claim
		has to carry the pair that call takes: the component and the action.

		Editing the file was tried and does not work. The lines were deleted,
		hotaru registered its eighteen shortcuts, and all eighteen of the old
		ones were back a second later -- the daemon holds the table in memory
		and writes it out whenever anything registers.
	*/
	found, err := desktop.Claimed(shortcuts(t), []string{"Ctrl+Alt+Num+4"})
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Equal(t, "AIOScene4", found[0].Entry)
	require.Equal(t, "kwin", found[0].Component,
		"a claim without its component cannot be unregistered")
}
