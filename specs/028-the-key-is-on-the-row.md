# Spec 028: the key is on the row

**Issue**: [#66](https://github.com/ushineko/hotaru/issues/66)

## Status: COMPLETE

## Context

The key is the first thing on a scene's row, and it is there for a reason
spec 016 settled: the bank has been on this numpad for years, so the key is
what somebody recognises a scene by, ahead of its name or its colour.

It was also the one thing on that line the window could not change.
`hotaru keys bind` was the only way, which makes rebinding a thing somebody
does in a terminal about a list they are looking at in a window.

Spec 017 listed a hotkey binder as pending and left it there.

### What looks like the thing you click

The same rule that moved scene editing onto the lines it describes (spec 018).
The key is drawn where somebody would reach to change it, so that is what
takes the click.

### Every key, not only the free ones

A key already pointing at another scene is a legitimate thing to choose:
somebody rearranging their bank is moving keys between scenes, and a chooser
that hid the taken ones would hide the rearrangement. What each key holds is
shown beside it, so the cost of taking one is on screen before it is taken.

Both banks are offered -- the nine plain keys hotaru ships with and the nine
with Shift it leaves free. The shipped bank is a default, not a reservation
in the other direction.

### Moving a shortcut is two calls

The service binds a *key* to a scene. It does not know that a scene should
have at most one key, because the key is the thing being bound and the scene
is what it points at -- which is right: a key can only mean one thing, and
that is the constraint the service enforces.

So moving a scene's shortcut is unbinding the old key and binding the new one,
and the window is the only place that knows both halves. A test asserts it
makes exactly two calls.

### And the binding did not reach the desktop

Reported the moment it was usable: the chooser worked, the file changed, and
pressing the key applied the old scene.

KWin's script carries the **scene name** in the call it makes rather than the
key:

	registerShortcut("hotaru-ctrl-alt-shift-num-1", ..., function() {
	    callDBus(..., "Apply", "corgis");
	});

so the binding a keypress acts on is the one written into the script when it
was installed -- and the installer only ran when KWin *appeared*. A rebinding
was correct in the file and invisible to the desktop until the service or KWin
restarted.

`hotaru keys bind` had it too, which is why the fix is in the service: the
store knows when the bindings changed and the daemon knows how to install, and
`Service.Shortcuts` is the seam between them.

The old message said so, and that is the part worth remembering. "It takes
effect the next time the desktop starts hotaru's script" was accurate, written
by somebody who knew, and describes a thing that should never have been true.

### The dialog was unusable, and had been before

It opened showing one and a half of the eighteen keys, clipped at both ends.
Fyne sizes a dialog to its content's minimum and a scroller's minimum is
almost nothing, so a dialog built around one opens at the size of its buttons
-- which is exactly what the file chooser did earlier, and this window already
had a `roomy` helper written for that and nobody reached for it.

Fixed by using it, and then by deleting it: it is `dialogs.Roomy` in
fynedesygn now (its spec 026), because the same fault was found in two
programs a week apart.

## Requirements

**R1. The key on a scene's row is a button** that opens a chooser.

**R2. Every key is offered**, with what it holds, both banks, plus none.

**R3. Moving a shortcut leaves the old key free.**

**R3a. A rebinding reaches the desktop at once**, because the script carries
the scene name rather than the key. A machine whose desktop is not there still
records the binding.

**R4. The chooser is readable** -- shown at most of the window rather than at
the size of its buttons.

## Acceptance Criteria

- [x] AC1. The chooser lists every bound and reserved key, once each,
      unshifted first and then by number.
- [x] AC2. Moving a scene's shortcut makes two calls: the old key unbound,
      the new one bound.
- [x] AC3. Choosing "no shortcut" unbinds and binds nothing.
- [x] AC4. The chooser opens roomy.
- [x] AC4a. Binding a key reinstalls the desktop's script, and a bind that
      cannot reach the desktop is still written to the file.
- [x] AC5. Verified on the development machine, by somebody rebinding a scene
      and pressing the key.

## Risks & Assumptions

- **A key taken from another scene leaves that scene without one**, silently
  as far as that scene's row is concerned until the list redraws. The chooser
  says what each key holds, which is where somebody sees it coming.
- **The bank comes from the service**, so a window built against an older
  service offers whatever that service reports rather than a list of its own.
- **Rollback** is a revert; `hotaru keys bind` is unchanged and remains the
  other way to do it.

## Alternatives Considered

Considered a Keys section showing the whole bank at once, with what is bound
and what another program has claimed; rejected for now because the binding
belongs where the key is drawn, and the conflict reporting that a section
would add is already in `hotaru keys` where somebody fixing a conflict is
already working.
