/*
Package desktop is the part of hotaru that attaches to a session.

The service starts with the machine and a desktop arrives later, or not at all,
so nothing here is a precondition for anything: this package watches for KWin,
installs a script when it appears, and takes the keys. A machine with no Plasma
loses these features and nothing else.

Two pieces, and they exist for one reason each. **The D-Bus object** is here
because a KWin script can reach the outside world only through `callDBus` -- it
is not a second API, it has one method, and it hands what it receives to the
same service call the HTTP route makes. **The script** is here because KDE's
other ways of binding a key report success and install no grab.

Every rule this package follows was a fault on somebody's desk first; they are
catalogued in docs/migration.md and cited at the code that answers them. See
spec 016.
*/
package desktop

import (
	"context"
	"fmt"

	"github.com/godbus/dbus/v5"
)

// The names hotaru answers to. One object, one interface, one method.
const (
	BusName    = "org.ushineko.hotaru"
	ObjectPath = dbus.ObjectPath("/Scenes")
	Interface  = "org.ushineko.hotaru.Scenes"
)

/*
Applier is the service, as much of it as a keypress needs.

Deliberately one method. The door is narrow because KWin gives no other, not
because there is a second interface worth having here: everything else a client
could want is on the HTTP API, which anything that is not a KWin script can
reach.
*/
type Applier interface {
	// ApplyScene lights a scene and says what happened, in a line meant for
	// the journal: a key that fired and did something odd and a key that
	// never fired are otherwise the same observation.
	ApplyScene(ctx context.Context, name string) (string, error)
}

/*
Door is the D-Bus object a shortcut talks to.

Its whole job is to receive "this scene was pressed" and pass it on. It reports
what happened because the alternative is a key that fires into silence, which
is how three of the four hotkey faults on this desk stayed alive for months.
*/
type Door struct {
	apply  Applier
	report func(format string, args ...any)
}

// Apply is the D-Bus method. The signature is a scene name in, an error out,
// because that is all a keypress has to say and all it needs to hear.
func (d *Door) Apply(name string) *dbus.Error {
	said, err := d.apply.ApplyScene(context.Background(), name)
	if err != nil {
		d.report("key: %s: %v", name, err)
		return dbus.MakeFailedError(err)
	}
	d.report("key: %s", said)
	return nil
}

/*
Export puts the door on the session bus until the context ends.

The name already being taken is not a fight to win: another hotaru is running,
and one process owns the bindings. This one says so and does without.
*/
func Export(ctx context.Context, conn *dbus.Conn, apply Applier, report func(string, ...any)) error {
	door := &Door{apply: apply, report: report}
	if err := conn.Export(door, ObjectPath, Interface); err != nil {
		return fmt.Errorf("export %s: %w", Interface, err)
	}

	reply, err := conn.RequestName(BusName, dbus.NameFlagDoNotQueue)
	if err != nil {
		return fmt.Errorf("claim %s: %w", BusName, err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		return fmt.Errorf("%s is already owned; another hotaru holds the keys", BusName)
	}

	go func() {
		<-ctx.Done()
		_, _ = conn.ReleaseName(BusName)
	}()
	return nil
}
