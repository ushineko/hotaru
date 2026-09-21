package desktop

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/godbus/dbus/v5"
)

// KWinName is the bus name a Plasma session owns.
const KWinName = "org.kde.KWin"

// KWin on the session bus.
const (
	kwinName       = KWinName
	scriptingPath  = dbus.ObjectPath("/Scripting")
	scriptingIface = "org.kde.kwin.Scripting"
	scriptIface    = "org.kde.kwin.Script"
)

// KWinBus is KWin's scripting service over D-Bus.
type KWinBus struct{ conn *dbus.Conn }

// Session connects to the session bus. A service started before any login has
// no session bus, which is an absence rather than a fault.
func Session() (*dbus.Conn, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("no session bus: %w", err)
	}
	return conn, nil
}

// NewKWin talks to KWin over an existing connection.
func NewKWin(conn *dbus.Conn) *KWinBus { return &KWinBus{conn: conn} }

func (k *KWinBus) scripting() dbus.BusObject {
	return k.conn.Object(kwinName, scriptingPath)
}

// IsLoaded asks whether a plugin name is loaded.
func (k *KWinBus) IsLoaded(name string) (bool, error) {
	var loaded bool
	err := k.scripting().Call(scriptingIface+".isScriptLoaded", 0, name).Store(&loaded)
	if err != nil {
		return false, fmt.Errorf("ask KWin whether %s is loaded: %w", name, err)
	}
	return loaded, nil
}

// Unload asks KWin to drop a plugin by name. It returns true while leaving the
// Script object alive, which is why the caller stops the object as well.
func (k *KWinBus) Unload(name string) error {
	var dropped bool
	err := k.scripting().Call(scriptingIface+".unloadScript", 0, name).Store(&dropped)
	if err != nil {
		return fmt.Errorf("unload %s: %w", name, err)
	}
	return nil
}

/*
Objects are KWin's script objects, by path, read by introspection.

The only honest way to find out what a load created. KWin's own answer -- the
id from `loadScript` -- has been seen naming an unrelated running script and
has been seen naming nothing at all.
*/
func (k *KWinBus) Objects() ([]string, error) {
	var payload string
	err := k.scripting().Call("org.freedesktop.DBus.Introspectable.Introspect", 0).Store(&payload)
	if err != nil {
		return nil, fmt.Errorf("look at KWin's scripts: %w", err)
	}

	var node struct {
		Nodes []struct {
			Name string `xml:"name,attr"`
		} `xml:"node"`
	}
	if err := xml.Unmarshal([]byte(payload), &node); err != nil {
		return nil, fmt.Errorf("read KWin's introspection: %w", err)
	}
	out := make([]string, 0, len(node.Nodes))
	for _, child := range node.Nodes {
		if child.Name == "" {
			continue
		}
		out = append(out, string(scriptingPath)+"/"+strings.TrimPrefix(child.Name, "/"))
	}
	return out, nil
}

// Load reads the script at a path under a plugin name. The two-argument form
// always: the one-argument one names the script after its path.
func (k *KWinBus) Load(path, name string) error {
	var id int32
	err := k.scripting().Call(scriptingIface+".loadScript", 0, path, name).Store(&id)
	if err != nil {
		return fmt.Errorf("load %s: %w", path, err)
	}
	return nil
}

// Run starts a script object.
func (k *KWinBus) Run(object string) error {
	call := k.conn.Object(kwinName, dbus.ObjectPath(object)).Call(scriptIface+".run", 0)
	if call.Err != nil {
		return fmt.Errorf("run %s: %w", object, call.Err)
	}
	return nil
}

// Stop ends one, which is the thing unloading does not do.
func (k *KWinBus) Stop(object string) error {
	call := k.conn.Object(kwinName, dbus.ObjectPath(object)).Call(scriptIface+".stop", 0)
	if call.Err != nil {
		return fmt.Errorf("stop %s: %w", object, call.Err)
	}
	return nil
}

/*
BusWatcher reports every appearance of a name on the session bus.

Including the first, and including one that is already there when hotaru
starts: "has it appeared since I asked?" and "is it here?" are the same
question to everything downstream, and treating them differently is how a
service that starts after its desktop ends up waiting for an event that already
happened.
*/
type BusWatcher struct {
	Conn *dbus.Conn
	Name string
}

// Appearances is a channel that receives once per appearance.
func (w *BusWatcher) Appearances(ctx context.Context) (<-chan struct{}, error) {
	signals := make(chan *dbus.Signal, 8)
	match := []dbus.MatchOption{
		dbus.WithMatchSender("org.freedesktop.DBus"),
		dbus.WithMatchInterface("org.freedesktop.DBus"),
		dbus.WithMatchMember("NameOwnerChanged"),
		dbus.WithMatchArg(0, w.Name),
	}
	if err := w.Conn.AddMatchSignalContext(ctx, match...); err != nil {
		return nil, fmt.Errorf("watch for %s: %w", w.Name, err)
	}
	w.Conn.Signal(signals)

	out := make(chan struct{}, 1)
	go func() {
		defer close(out)
		if w.here() {
			out <- struct{}{}
		}
		for {
			select {
			case <-ctx.Done():
				return
			case signal, ok := <-signals:
				if !ok {
					return
				}
				// name, old owner, new owner. A new owner is an appearance;
				// an empty one is the desktop going away, and hotaru waits.
				if len(signal.Body) == 3 {
					if owner, _ := signal.Body[2].(string); owner != "" {
						select {
						case out <- struct{}{}:
						default:
						}
					}
				}
			}
		}
	}()
	return out, nil
}

func (w *BusWatcher) here() bool {
	var has bool
	err := w.Conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, w.Name).Store(&has)
	return err == nil && has
}
