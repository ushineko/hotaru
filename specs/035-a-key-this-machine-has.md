# Spec 035: a key this machine has

**Issue**: [#83](https://github.com/ushineko/hotaru/issues/83)

## Status: COMPLETE

## Context

Every key hotaru ships is on the numpad. Nine shipped bindings on
`Ctrl+Alt+Num+1`..`9`, nine more reserved on the shifted row, and spec 028's
chooser offered exactly those eighteen -- because that is what this desk has
had for years, and the bank came across from the program hotaru replaces.

A machine without a numpad inherits eighteen shortcuts it cannot press and, in
the window, no way to choose anything else. `hotaru keys bind` at the terminal
took any sequence all along; the binder was the only place that did not, which
is the opposite of the parity rule this program holds everywhere else.

Found on the second machine, whose keyboard is a `deskflow` passthrough. The
diagnosis took half an hour and every link checked out: the KWin script was
loaded, the ten actions were in `kglobalshortcutsrc`, `kglobalaccel` listed
them, and invoking one through D-Bus applied the scene. What could not happen
was the key press.

## Requirements

**R1. The chooser offers a key somebody types**, spelled the way KDE spells
it, alongside the bank.

**R2. A key bound outside the bank is shown as itself** when the chooser
reopens, rather than as no shortcut.

**R3. Nothing about the bank changes.** The nine shipped bindings stay where
they are: they work on the desk they were built for, and moving them would
change a machine that has had them for two years to fix a machine that has
never had them.

## Acceptance Criteria

- [x] AC1. The chooser lists "something else" and a box for the sequence.
- [x] AC2. The box is inert until that option is chosen.
- [x] AC3. Binding through it reaches the service with the typed sequence.
- [x] AC4. Reopening the chooser on a scene bound to a key outside the bank
      selects "something else" with the key in the box.
- [x] AC5. An empty box says so rather than binding nothing.

## Risks & Assumptions

- **A sequence KDE will not take is KDE's answer to give**, and hotaru passes
  it on. The window does not validate the spelling: a list of what KDE accepts
  would be a second copy of a table only KDE has.
- **A typed key can collide** with another program's shortcut. That is true of
  the bank as well, which is why `hotaru keys` reports what else holds a
  sequence.
- **Detecting the absence of a numpad was considered and not done.** The
  second machine's virtual keyboard *claims* every numpad keycode, so the
  detection that would have helped a laptop would have told that machine it
  was fine.
- **Rollback** is a revert; bindings made through it are ordinary bindings the
  service already understood.
