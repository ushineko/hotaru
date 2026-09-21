# Spec 016: the keys are the monitor's keys

**Issue**: [#6](https://github.com/ushineko/hotaru/issues/6)

## Status: COMPLETE

## Context

Nine keys on this desk have meant the same nine things for a couple of years:
`Ctrl+Alt+Num1` is red, `Num9` turns the lights off, and nobody has had to
think about it since. They are the last thing peripheral-battery-monitor does
that hotaru cannot, and taking them over is what finishes the migration.

The bank was not reconstructed from memory. It is in the machine's own
configuration, where the monitor keeps it:

	"1": {"color": "red",     "lcd": "dashboard"}
	"2": {"color": "green",   "lcd": "dashboard"}
	"3": {"color": "blue",    "lcd": "dashboard"}
	"4": {"color": "purple",  "lcd": "dashboard"}
	"5": {"color": "cyan",    "lcd": "dashboard"}
	"6": {"color": "orange",  "lcd": "dashboard"}
	"7": {"color": "white",   "lcd": "liquid"}
	"8": {"color": "magenta", "lcd": ".../bluemarble.gif"}
	"9": {"color": "off",     "lcd": "liquid"}

and a second bank of nine on `Ctrl+Alt+Shift+Num1..9`, every one of which
points at a GIF under one person's `~/Pictures`.

**The first bank ships. The second does not.** The shipped nine are colours and
a screen state, which work on any machine with any hardware; the animation bank
is one desk's media and would be nine broken scenes everywhere else. The second
row of keys is *reserved and left unbound*, so somebody's own scenes have an
obvious home and hotaru is not sitting on the sequences they would use.

### What the scene model cannot say yet

`"color": "off"` is not a colour. Turning lighting off is its own path in
hotaru -- it resolves an Off mode, falls back to black in Direct, and honours
`never_blank` for the keyboard that treats black as a dead backlight -- and a
scene has no way to ask for it. Spec 015 built scenes around assignments, and
the ninth key needs a scene that turns things off.

So scenes gain `off`, and it is the whole device list rather than a colour: off
is a mode question, not a per-LED one.

### Why this is a KWin script and not a shortcut file

Global shortcuts on this desk broke four times, in four ways, and
`docs/migration.md` records each fault. They are not restated here; the spec's
requirements are written against them, and re-deriving the mechanism from KDE's
documentation reproduces all four. The short version:

- `.desktop` shortcuts install their key grabs when KDE enumerates the file at
  session start, so registering later reports success and installs no grab.
- The handle for a loaded script is a **plugin name**, not a path.
- Five different calls return success while doing nothing.
- The shortcuts live exactly as long as the loaded script.
- One process owns the bindings.

And the rule that shapes where this code lives: hotaru starts before any
session exists, so the integration **watches the bus for `org.kde.KWin` and
installs when it appears** -- at login, and again whenever KWin restarts. The
Python installed once, at start, which is why a KWin restart silently took the
shortcuts and left a program that believed it still had them.

### A door that exists because KWin gives no other

A KWin script can reach the outside world only through `callDBus`. That is the
entire reason hotaru has a D-Bus object: it is not a second API, it has one
job, and it hands "scene N was pressed" to the same service method the HTTP
route calls. One flow, two doors, and the narrow one exists under protest.

### The keys are already claimed

Eighteen `AIOScene*` entries sit in `~/.config/kglobalshortcutsrc`. KDE writes
them when a script registers shortcuts and **they outlive the script**:
unloading it does not remove them, and neither does restarting KWin. They hold
a claim on exactly the sequences hotaru wants, and a claimed key is how this
class of bug presents -- the registration reports success and the key does
nothing.

hotaru cannot silently edit that file: it is the desktop's, it is not
hotaru's to rewrite, and spec 001's rule is that anything hotaru can fix itself
is offered as a question rather than done behind somebody's back. So it
**looks, says what it found, and offers** -- which is the same shape as the
mapping wizard.

## Requirements

**R1. A scene can turn lighting off.** `off: true`, applied through the
existing off path so `never_blank` and the Off-mode fall-through still hold.

**R2. Nine shipped scenes, from the bank above.** Colour and screen state only.
They exist without a scenes file: a fresh install has them and has written
nothing, and saving a scene of the same name replaces one.

**R3. The animation bank is not shipped.** It is one machine's media. It is
documented as an example, and moving it over is a local act.

**R4. Bindings map a key to a scene name**, and ship as
`Ctrl+Alt+Num1..9` onto the nine. `Ctrl+Alt+Shift+Num1..9` are **reserved and
unbound**, for scenes somebody writes themselves.

**R5. The KWin integration attaches to the session, never depends on it.** It
watches for `org.kde.KWin` and installs on every appearance, including a KWin
restart.

**R6. Every one of the five silent failures is handled**, as `docs/migration.md`
sets out: a unique path per install, stale files swept, the predecessor
unloaded **by name** and its object stopped, the created object identified by
introspection difference rather than by the returned id, and a false from
`isScriptLoaded(name)` treated as a failed install.

**R7. The journal says what happened.** A registration count when the script
runs, and a line per activation. Without it, a key that never fired and a key
that fired and went nowhere are indistinguishable -- which is how three of the
four faults stayed alive.

**R8. The D-Bus object is thin.** One method that takes a scene name and calls
what the HTTP route calls. No second implementation of anything, and nothing
else exported.

**R9a. A claim is cleared through KDE, not through the file.** The daemon owns
that table and rewrites the file whenever anything registers a shortcut, so
editing it is undone within seconds.

**R9. Keys claimed by something else are reported, not seized.** hotaru names
the stale entries, says what they will do to the binding, and offers to remove
them. Declining leaves the file alone. **The service reports; the CLI clears.**
The service's unit gives it write access to its own two directories and nothing
else, so a route that edited the desktop's file could not work on a hardened
install and should not work on any other.

**R10. A desktop that is not Plasma loses the KWin script and nothing else.**
The CLI is the binding mechanism there -- somebody binds `hotaru scene apply
red` in their own shortcut editor -- and the absence is explained rather than
silent.

## Acceptance Criteria

- [x] AC1. A scene with `off` turns lighting off through the same path as
      `hotaru light off`, keyboard corrections included.
- [x] AC2. The nine shipped scenes exist on a machine with no scenes file, and
      `hotaru scene list` marks them as shipped.
- [x] AC3. Saving a scene named like a shipped one replaces it, and deleting
      that override restores the shipped one.
- [x] AC4. Shipped bindings cover `Ctrl+Alt+Num1..9`, and nothing is bound on
      the shifted row.
- [x] AC5. A binding to a scene that does not exist is reported when it is
      pressed, and does not stop the other keys working.
- [x] AC6. The integration installs when `org.kde.KWin` appears, and installs
      again when it appears a second time -- tested against a fake bus, since
      the trap is a restart rather than a start.
- [x] AC7. The script is installed at a unique path, the predecessor is
      unloaded by name and stopped, and the new object is identified by
      introspection difference.
- [x] AC8. A failed install is reported as failed: `isScriptLoaded(name)`
      returning false is an error, not a success.
- [x] AC9. The D-Bus method applies a scene by name and returns what happened.
- [x] AC10. `hotaru keys` reports the shortcuts, what holds them, and any stale
      claim, and offers to clear a claim rather than clearing it. The clearing
      happens in the CLI's own process, because the service is not permitted to
      write that file.
- [x] AC11. On a machine with no KWin, the service starts, says the KWin
      integration is unavailable once, and everything else works.
- [x] AC12. Verified on the development machine: the stale `AIOScene*` entries
      cleared, hotaru's keys registered, **all nine pressed by hand**, a KWin
      restart survived, and the monitor's own keys gone.

## Verified on hardware

Development machine, Plasma on Wayland, with somebody pressing the keys.

All nine fired, applying the scenes they have always applied. The observation
from the person doing it: **switching is an order of magnitude faster than the
old monitor** -- which is not a benchmark, but it is the difference between a
keypress that spawns `liquidctl` and an `openrgb` process per device and one
that writes frames down a connection the service already holds.

`hotaru keys` found what was in the way before anything was pressed, and the
cutover needed both halves of it:

**Nine stale `AIOScene1..9` entries** in `kglobalshortcutsrc`, holding exactly
hotaru's sequences. `hotaru keys release` removed those nine and left
`AIOScene11..19` -- the animation bank on the reserved row -- and every other
program's shortcuts exactly as they were.

**The `aio-scenes` KWin script was still loaded**, with its program not
running. That is the fault this spec is written against, observed rather than
recalled: `isScriptLoaded` returned true for a script whose process had exited,
and the shortcuts it registered were still grabbed. Unloading it by name and
stopping its object is what freed the keys.

KWin's own journal is the evidence the script installed:

	kwin_wayland[6240]: hotaru: registered 9 shortcuts

### The service cannot clear the claim, and should not be able to

Found at the moment of using it: `hotaru keys release` worked against a
foreground build and failed against the packaged one with

	write /home/…/.config/kglobalshortcutsrc: read-only file system

The unit sets `ProtectHome=read-only` with `ReadWritePaths` naming hotaru's own
two directories, so the service cannot write another program's configuration --
which is exactly the property that hardening is there to give, arriving as a
failure at the one moment it was being violated.

So the editing moved to the CLI, in the user's own process. The service reads
the file and reports what is in the way; removing an entry is done by whoever
asked. `POST /v1/keys/release` is gone rather than permitted.

That also settles a question the CLI's import rule would otherwise have raised:
that rule is about **devices** -- no command may write a light -- and the
desktop's shortcut file is not one.

### Deleting the lines does not clear a claim

The remedy this spec inherited -- hand-edit `kglobalshortcutsrc` -- does not
work, and the measurement is unambiguous. The nine entries were removed,
hotaru registered its eighteen shortcuts, and **all eighteen of the monitor's
entries were back in the file a second later**: `kglobalaccel` holds the table
in memory and writes it out whenever anything registers, so a deletion survives
exactly until the next save.

`org.kde.KGlobalAccel.unregister(component, action)` is the supported route. It
does both halves -- the daemon forgets and the file loses the line -- and it is
the only way that does not require logging out. Eighteen calls cleared the bank
for good.

### A leftover entry does not necessarily block anything

Also measured, and it corrects this spec's own framing. hotaru registered its
nine keys with all eighteen `AIOScene*` entries present and **every key
worked**. The grab belongs to the loaded script; the line in the file is a
record, not a lock, and KDE refuses a sequence only to a *different* component.

So the reporting says what is true: these are leftovers, the program that made
them may be long gone, hotaru's keys may work anyway -- and if one does nothing
while everything reports success, this is the first place to look. The earlier
wording promised a failure that did not happen, which is its own kind of wrong.

### The reserved row was claimed too, and hotaru could not see it

Written during this run rather than planned. `Keys` checked only the sequences
hotaru had bound, so the nine `AIOScene11..19` entries on
`Ctrl+Alt+Shift+Num1..9` were invisible to it -- and those keys are precisely
the ones somebody would bind their own scenes to next. They would have
registered successfully and done nothing, which is this whole fault a second
time with nothing pointing at it. The reserved row is checked now.

## Risks & Assumptions

- **A new dependency.** `github.com/godbus/dbus/v5` is pure Go with no cgo and
  is what everything in this ecosystem uses; the alternative is speaking the
  D-Bus wire protocol, which is a worse idea than it sounds.
- **The keys are taken from a running program.** Until the monitor's removal
  commit lands, both want the same sequences, and the loser is whichever
  registered later. The order in `docs/migration.md` exists for this reason and
  is not optional.
- **Shipped scenes are one desk's taste.** Red, green and blue on the first
  three keys is not a universal truth; it is what the machine this was written
  for has done for two years, and every one of them is a name somebody can
  rebind or replace.
- **Rollback** is to stop installing the script: the keys stop working and
  nothing else changes. Bindings are a file; shipped scenes are code and cannot
  be corrupted by one.

## Alternatives Considered

Considered the XDG `org.freedesktop.portal.GlobalShortcuts` portal, which is
the forward-looking answer and requires a portal implementation that Plasma
did not have when this was first written. Worth revisiting; not worth blocking
on.

Considered shipping the animation bank with its paths as documentation,
inside the scene file; rejected because a scene that names a file nobody has is
a broken scene in a listing, nine times over, on every machine but one.
