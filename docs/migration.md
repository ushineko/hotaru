# The migration contract

What specs 002 to 006 are bound by, in one place, so nothing here has to be
rediscovered by reading a 1,400-line spec. The reasoning is in
[`specs/001`](../specs/001-scope-migration-and-lighting-core.md); this is the
part that constrains work.

## What moves

hotaru takes **everything to do with the AIO cooler, its display, and
lighting** from `peripheral-battery-monitor`. The line is **writes**: every
write to the cooler and to every lit device belongs to hotaru and to nothing
else.

The monitor keeps peripheral batteries, bandwidth, its other sections, and a
**read-only** AIO telemetry display with its pump-failure alert. It reads from
its own sources until it is rewritten in Go, at which point it becomes a
consumer of `hotaru cooler status --json` and stops touching the cooler at all.

**Pump-failure alerting stays in the monitor.** An alert is owned where it is
visible: the monitor is the tray widget the user sees, and it moves down at the
Go rewrite with everything else. It is not a write, so it is not part of the
cutover.

## It is one cutover, not a staged retreat

The monitor keeps working untouched until hotaru has parity across lighting,
telemetry, LCD, dashboard, scenes and hotkeys. Then **one commit** removes every
write path. A staged removal would leave two processes writing one hidraw node,
which is the invariant this stack has been most careful about for eighteen
specs.

Concurrent *reads* are accepted; concurrent *writes* are not. After cutover the
monitor polls `liquidctl status` every five seconds while hotaru writes — a
deliberate, temporary overlap that ends when the monitor becomes a consumer.
Before cutover the monitor still writes the dashboard, so the working rule while
building specs 002 and 003 is: **stop the monitor before exercising hotaru's
LCD or lighting writes.**

## What has happened so far

**hotaru has the keys.** Spec 016 landed and the nine unshifted numpad
shortcuts are hotaru's, applying the nine scenes read out of the monitor's own
configuration. Two things had to be cleared first, and both are worth recording
because neither was a bounce away:

- The nine stale `AIOScene1..9` entries were removed with `hotaru keys
  release`, which showed them first and left `AIOScene11..19` and every other
  program's shortcuts untouched.
- **`aio-scenes` was still loaded in KWin with its program not running** --
  `isScriptLoaded` returned true for a script whose process had exited, and its
  shortcuts were still grabbed. Unloading it by name and stopping its object is
  what actually freed the keys.

What remains of the cutover is the monitor's own removal commit: its write
paths, its D-Bus object and its KWin script. Its keys no longer fire because
hotaru holds those sequences, but the code is still there to be taken out.

## The order, and the verification

1. hotaru reaches parity and is installed, with no key bindings at all.
2. The monitor's removal commit lands and is pushed.
3. The monitor is bounced, and the release of its claims is verified.
4. The stale `kglobalshortcutsrc` entries are cleared.
5. Only then does hotaru register the bindings.

A bounce is not evidence. Every layer in this stack has a documented habit of
reporting success while holding on, so confirm all five:

- **No KWin script.** `isScriptLoaded("aio-scenes")` is false *and* introspecting
  `/Scripting` shows no orphan `ScriptN` object. An unload that left the object
  alive is the fault the old spec 034 exists for.
- **No `/Scenes` object** on `org.agscripts.PeripheralBatteryMonitor`. The bus
  *name* persists — the monitor still owns it for its window positioning — so
  the object is what is checked.
- **No liquidctl process** belonging to the monitor, and no five-second poll.
- **The keys are inert**, and `kwin_wayland`'s journal shows no `aio-scenes`
  registration line on monitor start.
- **The stale entries are gone.** See below; the bounce does not do this one.
  `hotaru keys` reports exactly this, including for the reserved row, and
  `hotaru keys release` is the clearing step.

## The stale key entries

Eighteen `AIOScene*` entries sit in the `[kwin]` section of
`~/.config/kglobalshortcutsrc`. KDE writes them when the KWin script registers
its shortcuts, and they **outlive the script** — unloading it does not remove
them, and neither does restarting KWin. They are not stow-managed, so this is
machine state cleared by hand-editing that file.

They must go before hotaru registers anything, because they hold a claim on
exactly the sequences hotaru wants, and a claimed key is how this class of bug
presents: the registration reports success and the key does nothing.

## KDE hotkey rules (spec 006 is bound by these)

Global shortcuts on this desk broke four times, in four ways. These are not
preferences; each is a fault diagnosed the hard way, and re-deriving the
mechanism from the documentation reproduces all of them.

**Use KWin scripting, not `.desktop` shortcuts.** Service shortcuts have their
key *grabs* installed when KDE enumerates the file at session start, so a
`.desktop` entry works only from the next login. Registering the same component
later over KGlobalAccel puts an entry in the table — the key even reports as
claimed — and installs no grab.

**The handle is a plugin name, not a file path.**

```
loadScript(in s filePath, in s pluginName, out i)
loadScript(in s filePath, out i)
unloadScript(in s pluginName, out b)
isScriptLoaded(in s pluginName, out b)
```

The one-argument `loadScript` defaults the name to the path, which is why
passing paths appears to work. It has been observed returning the id of an
unrelated running script.

**Five calls report success while doing nothing:**

1. `loadScript` on a path KWin has seen returns the cached script's id without
   re-reading the file. Hence a unique path per install.
2. `run()` on an already-run object returns success and does nothing.
3. `unloadScript` returns true and does **not** destroy the Script object.
   `stop()` on the object is what does.
4. The id `loadScript` returns can collide with a survivor of (3) — observed
   returning `2` while creating no object at all.
5. `isScriptLoaded(path)` returns true for a script that is not running.

So: unique path per install, sweep stale files, unload the predecessor **by
name** and `stop()` its object, identify the created object by introspection
difference rather than the returned id, and treat a false from
`isScriptLoaded(name)` as a failed install.

**The shortcuts live exactly as long as the loaded script**, so reinstall on
every start. **Print to the journal** — a registration count on run and a line
per activation — because otherwise a key that never fired and a key that fired
and went nowhere are indistinguishable. **One process owns the bindings.**

**Attach, do not depend.** hotaru starts before any session, so it watches the
bus for `org.kde.KWin` and installs when it appears — at login, and again
whenever KWin restarts. The old implementation installed once at start, which
is why a KWin restart silently took the shortcuts and left a program that
believed it still had them.

Elsewhere than Plasma, the CLI is the binding mechanism: the user binds
`hotaru scene ember` in their own shortcut editor, and the absence of the KWin
integration is explained rather than silent.
