package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

/*
pluginName is the handle KWin knows this script by.

A name, not a path. The one-argument `loadScript` defaults the name to the path
given, which is why passing paths appears to work until it does not: unloading
by path then silently matches nothing, and the old script keeps the keys.
*/
const pluginName = "hotaru-scenes"

/*
Scripting is KWin's scripting service, as much of it as installing needs.

An interface because every one of these calls has a way of reporting success
while doing nothing, and the tests for that have to be able to make each one
lie. There is no fake KWin here -- what is faked is the five known lies, which
is knowledge this project paid for rather than a stand-in for a desktop.
*/
type Scripting interface {
	// IsLoaded answers for a plugin name. False after an install is a failed
	// install, whatever the other calls said.
	IsLoaded(name string) (bool, error)
	// Unload returns true while leaving the Script object alive, so Stop is
	// what actually ends the predecessor.
	Unload(name string) error
	// Objects are the script objects KWin currently has, by path. The
	// difference across a load identifies what was created -- the id that
	// load returns has been observed colliding with a survivor.
	Objects() ([]string, error)
	// Load returns an id that cannot be trusted to name the new object.
	Load(path, name string) error
	// Run starts a script object. Running an already-run object returns
	// success and does nothing.
	Run(object string) error
	// Stop ends one, which is what unloading does not do.
	Stop(object string) error
}

/*
Installer puts the script into a running KWin, once per appearance of it.

The sequence is the accumulation of four separate faults, and none of the steps
is optional:

	sweep the scripts written by previous installs
	write this one at a path KWin has never seen
	unload the predecessor by name, and stop its object
	note which objects exist
	load, then find the new object by difference
	run it
	ask whether the plugin is loaded, and believe the answer over everything else
*/
type Installer struct {
	// KWin is the scripting service.
	KWin Scripting
	// Dir is where generated scripts are written. Runtime state: a script
	// from a previous boot is of no use to anybody.
	Dir string
	// Bindings are the keys to register, resolved at install time so a
	// rebinding lands on the next KWin appearance without a restart.
	Bindings func() map[string]string
	// Report is where the journal lines go.
	Report func(format string, args ...any)
}

// Install writes the script and gets KWin running it, returning how many
// shortcuts it registered.
func (i *Installer) Install() (int, error) {
	bindings := i.Bindings()
	if len(bindings) == 0 {
		// Nothing to bind is not a failure. A machine whose owner unbound
		// every key gets no script rather than an empty one.
		return 0, nil
	}

	path, err := i.write(bindings)
	if err != nil {
		return 0, err
	}

	/*
		The predecessor goes first, by name, and then its object is stopped.

		`unloadScript` returns true and leaves the Script object alive. A
		surviving object keeps its shortcuts, so a second install would
		register onto keys the first one still holds -- reporting success, and
		doing nothing.
	*/
	before, err := i.KWin.Objects()
	if err != nil {
		return 0, err
	}
	if loaded, err := i.KWin.IsLoaded(pluginName); err == nil && loaded {
		if err := i.KWin.Unload(pluginName); err != nil {
			i.say("could not unload the previous script: %v", err)
		}
		for _, object := range before {
			if err := i.KWin.Stop(object); err != nil {
				i.say("could not stop %s: %v", object, err)
			}
		}
		if after, err := i.KWin.Objects(); err == nil {
			before = after
		}
	}

	if err := i.KWin.Load(path, pluginName); err != nil {
		return 0, fmt.Errorf("load the hotkey script: %w", err)
	}

	object, err := i.created(before)
	if err != nil {
		return 0, err
	}
	if err := i.KWin.Run(object); err != nil {
		return 0, fmt.Errorf("run the hotkey script: %w", err)
	}

	/*
		The last word belongs to isScriptLoaded, and only for a name.

		Every call above can succeed while nothing happens. This one asked
		about a path returns true for a script that is not running, so the
		question is asked about the plugin name and a false answer is a failed
		install rather than a warning.
	*/
	loaded, err := i.KWin.IsLoaded(pluginName)
	if err != nil {
		return 0, fmt.Errorf("ask whether the hotkey script loaded: %w", err)
	}
	if !loaded {
		return 0, errors.New("KWin accepted the hotkey script and is not running it")
	}

	i.say("keys: %d shortcuts registered with KWin", len(bindings))
	return len(bindings), nil
}

/*
created is the object a load made, found by difference.

Not the id the load returned. That id has been observed naming an unrelated
running script, and once naming no object at all -- so what exists after is
compared with what existed before, and a load that created nothing says so
instead of running something else.
*/
func (i *Installer) created(before []string) (string, error) {
	after, err := i.KWin.Objects()
	if err != nil {
		return "", err
	}
	had := make(map[string]bool, len(before))
	for _, object := range before {
		had[object] = true
	}
	var fresh []string
	for _, object := range after {
		if !had[object] {
			fresh = append(fresh, object)
		}
	}
	switch len(fresh) {
	case 1:
		return fresh[0], nil
	case 0:
		return "", errors.New("KWin loaded the hotkey script and created no script object")
	}
	// More than one: something else loaded a script at the same moment.
	// Taking the newest is the best available guess and is said out loud.
	sort.Strings(fresh)
	i.say("keys: KWin gained %d script objects at once; taking %s", len(fresh), fresh[len(fresh)-1])
	return fresh[len(fresh)-1], nil
}

/*
write puts the script at a path KWin has never seen, and sweeps the old ones.

`loadScript` on a path it has seen before returns the cached script's id
without re-reading the file, so a fixed filename means an edited script is
never picked up -- the install reports success and the old bindings stay.
*/
func (i *Installer) write(bindings map[string]string) (string, error) {
	if err := os.MkdirAll(i.Dir, 0o700); err != nil {
		return "", fmt.Errorf("make the script directory: %w", err)
	}
	stale, _ := filepath.Glob(filepath.Join(i.Dir, "hotaru-keys-*.js"))
	for _, old := range stale {
		_ = os.Remove(old)
	}

	path := filepath.Join(i.Dir, fmt.Sprintf("hotaru-keys-%d.js", time.Now().UnixNano()))
	if err := os.WriteFile(path, []byte(Script(bindings)), 0o600); err != nil {
		return "", fmt.Errorf("write the hotkey script: %w", err)
	}
	return path, nil
}

func (i *Installer) say(format string, args ...any) {
	if i.Report != nil {
		i.Report(format, args...)
	}
}

/*
Attach installs on every appearance of KWin, for as long as the context lasts.

Attachment, not dependency. hotaru starts before any session exists, so an
install at start-up would have nothing to install into; and the shortcuts live
exactly as long as the loaded script, so a KWin restart takes them with it. The
old implementation installed once and believed it still had the keys for the
rest of the session.
*/
func Attach(ctx context.Context, watch Watcher, install func() (int, error), report func(string, ...any)) {
	appeared, err := watch.Appearances(ctx)
	if err != nil {
		report("no hotkeys: %v", err)
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-appeared:
			if !ok {
				return
			}
			if _, err := install(); err != nil {
				report("could not take the keys: %v", err)
			}
		}
	}
}

// Watcher reports each time the desktop appears, including the first.
type Watcher interface {
	Appearances(ctx context.Context) (<-chan struct{}, error)
}
