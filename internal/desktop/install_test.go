package desktop_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ushineko/hotaru/internal/desktop"
)

/*
kwin is a KWin that lies the way the real one does.

Not a stand-in for a desktop: there is no pretending here that a shortcut was
pressed or that a compositor exists. What it reproduces is the five calls that
report success while doing nothing, each of which cost somebody a day on this
desk and none of which can be provoked on demand from a real KWin. The
knowledge is the asset; this is where it is kept executable.
*/
type kwin struct {
	loaded  bool
	objects []string
	runs    []string
	stopped []string
	loads   []string

	// unloadLeavesObject reproduces the real unloadScript: it returns true
	// and the Script object keeps running, shortcuts and all.
	unloadLeavesObject bool
	// loadCreatesNothing reproduces a load that returns an id naming no
	// object at all.
	loadCreatesNothing bool
	// staysUnloaded reproduces the install that reports success throughout
	// and leaves the plugin not running.
	staysUnloaded bool
}

func (k *kwin) IsLoaded(string) (bool, error) { return k.loaded && !k.staysUnloaded, nil }

func (k *kwin) Unload(string) error {
	k.loaded = false
	if !k.unloadLeavesObject {
		k.objects = nil
	}
	return nil
}

func (k *kwin) Objects() ([]string, error) { return append([]string(nil), k.objects...), nil }

func (k *kwin) Load(path, name string) error {
	k.loads = append(k.loads, path)
	k.loaded = true
	if k.loadCreatesNothing {
		return nil
	}
	k.objects = append(k.objects, "/Scripting/Script"+name+string(rune('0'+len(k.objects))))
	return nil
}

func (k *kwin) Run(object string) error { k.runs = append(k.runs, object); return nil }

func (k *kwin) Stop(object string) error {
	k.stopped = append(k.stopped, object)
	var kept []string
	for _, existing := range k.objects {
		if existing != object {
			kept = append(kept, existing)
		}
	}
	k.objects = kept
	return nil
}

func installer(t *testing.T, k *kwin) *desktop.Installer {
	t.Helper()
	return &desktop.Installer{
		KWin:     k,
		Dir:      t.TempDir(),
		Bindings: func() map[string]string { return map[string]string{"Ctrl+Alt+Num+1": "red"} },
		Report:   func(string, ...any) {},
	}
}

func TestInstallingRegistersTheKeys(t *testing.T) {
	k := &kwin{}
	count, err := installer(t, k).Install()

	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Len(t, k.runs, 1, "the script was loaded and never run")
}

func TestEachInstallUsesAPathKWinHasNotSeen(t *testing.T) {
	/*
		loadScript on a path KWin has seen before returns the cached script's
		id without re-reading the file. With a fixed filename, a rebinding is
		written to disk, reported as installed, and never takes effect.
	*/
	k := &kwin{}
	install := installer(t, k)

	_, err := install.Install()
	require.NoError(t, err)
	_, err = install.Install()
	require.NoError(t, err)

	require.Len(t, k.loads, 2)
	require.NotEqual(t, k.loads[0], k.loads[1], "two installs shared a path")
}

func TestOldScriptsAreSweptUp(t *testing.T) {
	// The directory would otherwise collect one file per KWin restart for as
	// long as the session lasts.
	k := &kwin{}
	install := installer(t, k)

	_, err := install.Install()
	require.NoError(t, err)
	_, err = install.Install()
	require.NoError(t, err)

	left, err := filepath.Glob(filepath.Join(install.Dir, "*.js"))
	require.NoError(t, err)
	require.Len(t, left, 1)
}

func TestThePredecessorIsStoppedAndNotMerelyUnloaded(t *testing.T) {
	/*
		unloadScript returns true and leaves the Script object alive. A
		surviving object keeps its shortcuts, so the new script registers onto
		keys the old one still holds -- and the second install reports success
		while the keys go on doing what they did before.
	*/
	k := &kwin{loaded: true, objects: []string{"/Scripting/Script1"}, unloadLeavesObject: true}

	_, err := installer(t, k).Install()
	require.NoError(t, err)
	require.Contains(t, k.stopped, "/Scripting/Script1",
		"the previous script was unloaded but left running")
}

func TestTheNewObjectIsFoundByDifference(t *testing.T) {
	// The id a load returns has been seen naming an unrelated running script.
	// What exists after is compared with what existed before instead.
	k := &kwin{objects: []string{"/Scripting/Script7"}}

	_, err := installer(t, k).Install()
	require.NoError(t, err)
	require.Len(t, k.runs, 1)
	require.NotEqual(t, "/Scripting/Script7", k.runs[0],
		"hotaru ran somebody else's script")
}

func TestALoadThatCreatesNothingIsAFailure(t *testing.T) {
	k := &kwin{loadCreatesNothing: true}

	_, err := installer(t, k).Install()
	require.ErrorContains(t, err, "created no script object")
}

func TestAnInstallThatDidNotTakeIsReportedAsFailed(t *testing.T) {
	/*
		Every call in the sequence can succeed while nothing happens, so the
		last word belongs to isScriptLoaded -- asked about the plugin name,
		because asked about a path it returns true for a script that is not
		running.
	*/
	k := &kwin{staysUnloaded: true}

	_, err := installer(t, k).Install()
	require.ErrorContains(t, err, "is not running it")
}

func TestNothingBoundInstallsNothing(t *testing.T) {
	// A machine whose owner unbound every key gets no script, rather than an
	// empty one that claims to have registered nothing.
	k := &kwin{}
	install := installer(t, k)
	install.Bindings = func() map[string]string { return nil }

	count, err := install.Install()
	require.NoError(t, err)
	require.Zero(t, count)
	require.Empty(t, k.loads)
}

func TestTheScriptSaysWhatItDid(t *testing.T) {
	/*
		A key that never fired and a key that fired and went nowhere are
		indistinguishable without this, which is why three of the four faults
		on this desk stayed alive as long as they did.
	*/
	script := desktop.Script(map[string]string{"Ctrl+Alt+Num+1": "red"})

	require.Contains(t, script, `registerShortcut("HotaruCtrlAltNum1"`)
	require.Contains(t, script, `"Ctrl+Alt+Num+1"`)
	require.Contains(t, script, `"Apply", "red"`)
	require.Contains(t, script, "registered ")
	require.Contains(t, script, `print("hotaru: Ctrl+Alt+Num+1 -> red")`)
}

func TestAShortcutIsNamedForItsKeyNotItsScene(t *testing.T) {
	/*
		KDE's entries outlive the script that made them. An identifier
		carrying the scene name would orphan one entry per rebinding -- which
		is how eighteen AIOScene* entries came to be holding sequences their
		program no longer uses.
	*/
	first := desktop.Script(map[string]string{"Ctrl+Alt+Num+1": "red"})
	rebound := desktop.Script(map[string]string{"Ctrl+Alt+Num+1": "evening"})

	require.Contains(t, first, `"HotaruCtrlAltNum1"`)
	require.Contains(t, rebound, `"HotaruCtrlAltNum1"`)
}

// desk reports an appearance whenever the test says so.
type desk struct{ appears chan struct{} }

func (d *desk) Appearances(context.Context) (<-chan struct{}, error) { return d.appears, nil }

func TestTheKeysAreTakenAgainWhenTheDesktopComesBack(t *testing.T) {
	/*
		The shortcuts live exactly as long as the loaded script, so a KWin
		restart takes them with it. The implementation this replaces installed
		once at start-up and spent the rest of the session believing it still
		had the keys.
	*/
	appears := make(chan struct{}, 2)
	installs := make(chan struct{}, 4)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go desktop.Attach(ctx, &desk{appears: appears}, func() (int, error) {
		installs <- struct{}{}
		return 1, nil
	}, func(string, ...any) {})

	appears <- struct{}{}
	appears <- struct{}{}

	for range 2 {
		select {
		case <-installs:
		case <-time.After(2 * time.Second):
			t.Fatal("the keys were not taken again when the desktop reappeared")
		}
	}
}

func TestAFailedInstallIsReportedAndDoesNotStopWatching(t *testing.T) {
	appears := make(chan struct{}, 2)
	said := make(chan string, 4)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go desktop.Attach(ctx, &desk{appears: appears}, func() (int, error) {
		return 0, errors.New("KWin said no")
	}, func(format string, _ ...any) { said <- format })

	appears <- struct{}{}
	select {
	case line := <-said:
		require.Contains(t, line, "could not take the keys")
	case <-time.After(2 * time.Second):
		t.Fatal("a failed install said nothing")
	}

	appears <- struct{}{}
	select {
	case <-said:
	case <-time.After(2 * time.Second):
		t.Fatal("watching stopped after one failure")
	}
}

func TestTheScriptIsWrittenWhereOnlyTheUserCanReadIt(t *testing.T) {
	k := &kwin{}
	install := installer(t, k)
	_, err := install.Install()
	require.NoError(t, err)

	written, err := filepath.Glob(filepath.Join(install.Dir, "*.js"))
	require.NoError(t, err)
	info, err := os.Stat(written[0])
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	body, err := os.ReadFile(written[0]) //nolint:gosec // the test's own file
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(body), "// Generated by hotaru"))
}
