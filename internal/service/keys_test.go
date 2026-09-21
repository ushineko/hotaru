package service_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/scenes"
)

// desk is a fake desktop that records how often the bindings were installed.
type desk struct {
	installs int
	err      error
}

func (d *desk) Install() (int, error) {
	d.installs++
	return 9, d.err
}

func TestBindingPutsTheChangeOnTheDesktop(t *testing.T) {
	/*
		Reported as "changing the hotkey didn't take effect". KWin's script
		carries the scene name in the call it makes rather than the key, so
		the binding a keypress acts on is the one written into the script when
		it was installed -- and nothing rewrote it. The file was right and the
		key kept firing the old scene until the service restarted.
	*/
	svc, _ := lit(t, blue(), scenes.Scene{Name: "evening"})
	desktop := &desk{}
	svc.SetShortcuts(desktop, nil)

	require.NoError(t, svc.Bind("Ctrl+Alt+Num+1", "evening"))
	require.Equal(t, 1, desktop.installs, "a binding did not reach the desktop")
}

func TestABindingThatCannotReachTheDesktopIsStillABinding(t *testing.T) {
	/*
		The store is the record and the script is a copy of it. A machine
		whose KWin is not running has a correct file and no shortcuts, which
		is the state it was already in and the one the installer resolves when
		KWin next appears.
	*/
	svc, _ := lit(t, blue())
	svc.SetShortcuts(&desk{err: errors.New("no kwin")}, nil)

	require.NoError(t, svc.Bind("Ctrl+Alt+Num+1", "blue"))

	keys, err := svc.Keys()
	require.NoError(t, err)
	for _, binding := range keys.Bindings {
		if binding.Key == "Ctrl+Alt+Num+1" {
			require.Equal(t, "blue", binding.Scene)
			return
		}
	}
	require.Fail(t, "the binding was not written")
}

func TestAMachineWithNoDesktopBindsAnyway(t *testing.T) {
	svc, _ := lit(t, blue())
	require.NoError(t, svc.Bind("Ctrl+Alt+Num+1", "blue"))
}
