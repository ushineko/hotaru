# The migration contract

What specs 002 to 006 are bound by, in one place, so that nobody has to
rediscover it by reading a 1,400-line spec. The reasoning is in
[`specs/001`](../specs/001-scope-migration-and-lighting-core.md). This page
holds the part that constrains the work.

## What moves

hotaru takes **everything to do with the AIO cooler, its display, and
lighting** from `peripheral-battery-monitor`. The line is **writes**: every
write to the cooler and to every lit device belongs to hotaru and to nothing
else.

The monitor keeps peripheral batteries, bandwidth, its other sections, and a
**read-only** AIO telemetry display with its pump-failure alert. It reads from
its own sources until somebody rewrites it in Go. It then becomes a consumer
of `hotaru cooler status --json` and stops touching the cooler at all.

**Pump-failure alerting stays in the monitor.** An alert belongs where it is
visible, and the monitor is the tray widget the user sees. It moves down at
the Go rewrite with everything else. It is not a write, so it is not part of
the cutover.

## It is one cutover, not a staged retreat

The monitor keeps working untouched until hotaru reaches parity across
lighting, telemetry, LCD, dashboard, scenes and hotkeys. Then **one commit**
removes every write path. A staged removal would leave two processes writing
one hidraw node, and that is the invariant this stack has protected most
carefully for eighteen specs.

Concurrent *reads* are accepted. Concurrent *writes* are not. After the
cutover the monitor polls `liquidctl status` every five seconds while hotaru
writes. That overlap is deliberate and temporary, and it ends when the monitor
becomes a consumer. Before the cutover the monitor still writes the dashboard.
The working rule while building specs 002 and 003 is therefore: **stop the
monitor before exercising hotaru's LCD or lighting writes.**

## What has happened so far

**hotaru has the keys.** Spec 016 landed, and the nine unshifted numpad
shortcuts are hotaru's. They apply the nine scenes read out of the monitor's
own configuration. Two things had to be cleared first, and both are recorded
here because neither took a single restart:

- `hotaru keys release` removed the eighteen stale `AIOScene*` entries. It
  shows them first, then asks KDE to forget them one at a time. It leaves
  every other program's shortcuts alone.
- **`aio-scenes` was still loaded in KWin with its program not running.**
  `isScriptLoaded` returned true for a script whose process had exited, and
  KWin still held its key grabs. Unloading the script by name and stopping its
  object is what freed the keys.

What remains of the cutover is the monitor's own removal commit: its write
paths, its D-Bus object and its KWin script. Its keys no longer fire, because
hotaru holds those sequences, but the code is still there to remove.

## The order, and the verification

1. hotaru reaches parity and is installed, with no key bindings at all.
2. The monitor's removal commit lands and is pushed.
3. Somebody restarts the monitor and verifies that it released its claims.
4. The stale `kglobalshortcutsrc` entries are cleared.
5. Only then does hotaru register the bindings.

A restart is not evidence. Every layer in this stack has a documented habit of
reporting success while holding on, so confirm all five:

- **No KWin script.** `isScriptLoaded("aio-scenes")` is false, *and*
  introspecting `/Scripting` shows no orphan `ScriptN` object. An unload that
  leaves the object alive is the fault the old spec 034 exists for.
- **No `/Scenes` object** on `org.agscripts.PeripheralBatteryMonitor`. The bus
  *name* persists, because the monitor still owns it for its window
  positioning, so check the object rather than the name.
- **No liquidctl process** belonging to the monitor, and no five-second poll.
- **The keys are inert**, and `kwin_wayland`'s journal shows no `aio-scenes`
  registration line when the monitor starts.
- **The stale entries are gone.** See below. A restart does not do this one.
  `hotaru keys` reports exactly this, including for the reserved row, and
  `hotaru keys release` is the clearing step.

## The stale key entries

Eighteen `AIOScene*` entries sit in the `[kwin]` section of
`~/.config/kglobalshortcutsrc`. KDE writes them when the KWin script registers
its shortcuts, and they **outlive the script**. Unloading the script does not
remove them, and restarting KWin does not either. They are not stow-managed,
so they are machine state.

Somebody believed they blocked hotaru's own registration. **They do not, and
the cutover measured both halves of that sentence.**

hotaru registered its nine keys with all eighteen entries present, and every
key worked. The grab belongs to the loaded script, and KDE refuses a sequence
only to a *different* component. The entries are leftovers rather than locks.
Clear them, and look there first when a key does nothing while every layer
reports success.

**Editing the file by hand does not clear them.** Somebody deleted the nine
entries, hotaru registered its shortcuts, and all eighteen were back in the
file a second later. `kglobalaccel` keeps the table in memory and writes it
out whenever anything registers. The supported route is
`org.kde.KGlobalAccel.unregister(component, action)`, which is what `hotaru
keys release` calls. It clears the daemon and the file together, and it needs
no logout.

## KDE hotkey rules (spec 006 is bound by these)

Global shortcuts on this desk broke four times, in four ways. These rules are
not preferences. Each one records a fault that was diagnosed the hard way, and
deriving the mechanism from the documentation instead reproduces all of them.

**Use KWin scripting, not `.desktop` shortcuts.** KDE installs the key *grabs*
for service shortcuts when it enumerates the file at session start, so a
`.desktop` entry works only from the next login. Registering the same
component later over KGlobalAccel puts an entry in the table, and the key even
reports as claimed, but KDE installs no grab.

**The handle is a plugin name, not a file path.**

```
loadScript(in s filePath, in s pluginName, out i)
loadScript(in s filePath, out i)
unloadScript(in s pluginName, out b)
isScriptLoaded(in s pluginName, out b)
```

The one-argument `loadScript` defaults the name to the path, which is why
passing paths appears to work. It has also returned the id of an unrelated
running script.

**Five calls report success while doing nothing:**

1. `loadScript` on a path KWin has seen returns the cached script's id and
   does not re-read the file. Hence a unique path per install.
2. `run()` on an object that has already run returns success and does nothing.
3. `unloadScript` returns true and does **not** destroy the Script object.
   `stop()` on the object destroys it.
4. The id `loadScript` returns can collide with a survivor of (3). It has
   returned `2` while creating no object at all.
5. `isScriptLoaded(path)` returns true for a script that is not running.

So: use a unique path per install, sweep stale files, unload the predecessor
**by name** and `stop()` its object, identify the created object by the
difference in introspection rather than by the returned id, and treat a false
from `isScriptLoaded(name)` as a failed install.

**The shortcuts live exactly as long as the loaded script**, so reinstall them
on every start. **Print to the journal**: a registration count on run, and a
line per activation. Without those lines, a key that never fired and a key
that fired and went nowhere look the same. **One process owns the bindings.**

**Attach, do not depend.** hotaru starts before any session, so it watches the
bus for `org.kde.KWin` and installs when that name appears. That happens at
login, and again whenever KWin restarts. The old implementation installed once
at start, which is why a KWin restart took the shortcuts and left a program
that believed it still held them.

Away from Plasma, the CLI is the binding mechanism. The user binds `hotaru
scene ember` in their own shortcut editor, and hotaru explains that the KWin
integration is absent rather than leaving them to find out.
