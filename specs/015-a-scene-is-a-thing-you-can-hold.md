# Spec 015: a scene is a thing you can hold

**Issue**: [#4](https://github.com/ushineko/hotaru/issues/4)

## Status: COMPLETE

## Context

Everything a scene needs already exists, one layer down, and nothing assembles
it.

A target addresses a device, a zone, an LED range within a zone, or a segment
named once in the rules. `Compose` turns a list of assignments into one frame
per device, later assignments winning where they overlap, and returns the ones
it could not satisfy rather than dropping them. `Request.Mode` prefers an
effect over the usual order. `Request.Preview` writes without remembering.
Desired state is recorded per device and the reconciler re-asserts it.

So "top fan red, bottom fan blue, the keyboard rippling under your fingers" is
expressible today, as a command somebody types. What is missing is the ability
to **name it, keep it, and hold it** -- and the ability to look at one before
deciding.

### Three operations that are not the same thing

`Apply`, `Preview` and revert-to-desired are distinct, and the difference is
about intent rather than mechanism:

- **Apply** is what somebody wants their machine to look like. It is recorded,
  and the reconciler holds it against hardware that forgets -- the wireless
  G502 restores onboard colour when it wakes, and nothing else puts the scene
  back.
- **Preview** is what somebody is *looking at*. Never a statement of intent: it
  is a question, the same way the mapping wizard's writes are questions. It is
  not recorded, it suspends re-assertion for the devices it covers, and it
  ends.
- **Revert** puts the machine back to what was recorded, which is what ending a
  preview means.

The middle one is where the design work is. hotaru already has a `Preview`
flag, and all it does is skip the recording step: the re-assert timer restores
desired state underneath the person looking at the draft, on the G502 within
sixty seconds. A preview that a timer can overwrite is not a preview.

### A preview must not outlive its holder

A draft is on somebody's hardware. If the program showing it dies, the
hardware is stuck on a state nobody chose and nothing will correct it, because
the correction was suspended on that program's behalf.

So a preview is a **lease**, and a lease has to end when its holder does. Two
kinds of client need this and they have different shapes:

- A client that can hold a connection open -- the GUI, and `hotaru light
  preview` sitting there until somebody presses a key -- holds the preview
  request itself open. That socket closing *is* the client going away, reported
  by the kernel, instantly and with no clock involved.
- A client that cannot -- a shell script, twenty lines of curl, anything
  one-shot -- takes a lease with a short expiry and renews it while it wants
  the draft.

Rather than choose one mechanism for both, hotaru uses whichever the client
already has. A held connection costs no heartbeat for something the socket
already says; an expiry costs no connection for a client that has none.

(`/v1/events` is in the architecture as the GUI's update channel and is not
built. When it is, it is a third thing a lease can be bound to, and nothing
here changes shape.)

### A scene carries the screen

The issue that asked for this was written before hotaru could reach the cooler
at all, and it already said a scene is "a set of colour assignments plus an LCD
mode". That is still right, and now it is cheap: spec 013 made the dashboard
the screen's default author and gave it `Hold` and `Release` precisely so
something else could take the panel and give it back.

A scene therefore names what the screen shows -- the dashboard, a GIF, or the
cooler's own readout. **A scene that says nothing about the screen changes
nothing about it**, which is the only rule that makes the absent case
predictable: a lighting scene applied at midday should not take somebody's
dashboard away because its author never thought about the screen.

### Effects are part of a scene, not of the machine

A scene is not only colours. The one the development machine actually wants is
"the keyboard rippling under typing, the cooler steady, the case fans steady",
and the rippling is a mode -- `Solid Splash` -- rather than a colour. hotaru
learned that in spec 014 and records it per device in the rules file, which is
the wrong place for it: the rules file says what a device *is*, and an effect
is what somebody wants it to *do* this evening.

`Request.Mode` prefers one mode for every device in a request, which is the
wizard's need and not a scene's. A scene names an effect per device, with the
same "preferred, not forced" semantics: a mode that cannot carry the frame is
no use, and silently showing one colour where three were asked for is a worse
answer than choosing a mode that works.

### An effect is a name; some of them are hotaru's own

`Solid Splash` lives in a keyboard's firmware. **Storm** -- the OpenLinkHub
effect for Corsair hardware that this project keeps being asked about -- does
not live anywhere: it is a program writing a new frame several times a second,
across every device it controls, so the whole machine moves together. No
firmware mode can do that, because no device knows what the others are showing.

hotaru can already write those frames. A per-LED frame to every device in scope
is what `hotaru light set` does, at about 0.6 ms per device service-side, and
spec 001's whole argument for owning the write path was that it makes exactly
this possible.

So an effect in a scene is a **name**, and what it resolves to is decided at
apply time:

- a mode the device advertises, preferred and falling through as today
- an effect hotaru renders itself, across every device the scene covers

This spec defines that seam and implements the first kind. The renderer, its
frame rate, what a machine-wide animation costs, and what it means for
reconciliation -- a device being written thirty times a second has nothing to
re-assert -- are their own spec, because they are their own set of measurements
and this one is already about three things.

A scene naming an effect hotaru cannot resolve applies its colours, says which
effect it could not find, and does not fail.

### Names, not numbers

The monitor had banks of nine, because numpad keys are numbers. A name survives
a rebind; a slot number belongs to the binding layer and is spec 006's problem.
Shipped defaults are colour-only: the monitor's animation bank points at GIFs
under one person's `~/Pictures`, and as defaults they would be eighteen broken
scenes on every other machine.

## Requirements

**R1. A scene is a name, assignments, effects and a screen state.** Assignments
are the existing targets -- device, zone, LED range, named segment -- composed
into one frame per device before anything is written, which is what makes a
write atomic and keeps a scene from interleaving with a reconcile.

**R2. An effect is named per device**, preferred rather than forced, with the
fall-through hotaru already applies when a mode cannot carry a frame. The name
is resolved at apply time against the device's own modes first; a name hotaru
does not recognise costs the effect and not the scene, and leaves room for the
rendered kind without the file changing shape.

**R3. A scene's screen state is applied through the dashboard's hold.** A scene
naming a GIF or the readout takes the panel; a scene naming the dashboard gives
it back; a scene that says nothing leaves it alone.

**R4. Applying records desired state, and the reconciler holds it.** A scene is
what somebody wants their machine to look like, so it survives a device that
forgets, a server that restarts, and a reboot.

**R5. A preview is not recorded, and is not reconciled over.** Re-assertion is
suspended for the devices a preview covers, for as long as the preview is held.

**R6. A preview is a lease that ends with its holder.** Bound to the request
connection where the caller holds one open, and to a renewed expiry where it
does not.
Either way the hardware returns to desired state when the lease ends, including
when it ends because the client died.

**R7. A preview is visibly a preview.** `hotaru light list` says which devices
are showing a draft and who holds it, because a machine showing something
nobody chose is exactly the confusion this project keeps finding.

**R8. A scene that cannot be fully applied applies the rest and says so.** A
missing GIF costs the screen, not the colours; an unknown segment costs that
assignment, not the scene. Both are reported per device, the way `Problems`
already reports a misspelled segment.

**R9. Scenes are machine-written, in `scenes.yml`.** The GUI is their editor, so
this is the one file hotaru serialises. Rules stay the user's and are never
rewritten.

**R10. Shipped defaults are colour-only**, and there are few of them.

## Acceptance Criteria

- [x] AC1. A scene applies assignments down to an LED range, composing one
      frame per device, with later assignments winning.
- [x] AC2. A scene names an effect per device, and a device whose named effect
      cannot carry the frame falls through as it does today.
- [x] AC2a. A scene naming an effect that does not exist applies its colours,
      reports the name, and does not fail -- the case a rendered effect will
      arrive into.
- [x] AC3. Applying a scene records desired state; the reconciler re-asserts it
      on a device that forgets.
- [x] AC4. A scene naming a GIF or the readout takes the screen from the
      dashboard; one naming the dashboard gives it back; one saying nothing
      leaves the screen as it was.
- [x] AC5. A preview writes nothing to desired state, and re-assertion does not
      run against a device while its preview is held.
- [x] AC6. A preview bound to a held connection ends when that connection
      closes, and the hardware returns to desired state.
- [x] AC7. A preview held by a client with no stream expires without renewal,
      and the hardware returns to desired state. A test kills the holder rather
      than asking it politely.
- [x] AC8. Two callers cannot preview the same device at once, and the second
      is told who holds it.
- [x] AC9. `hotaru light list` marks a device showing a preview.
- [x] AC10. A scene whose GIF is missing applies its colours, reports the file,
      and leaves the screen alone.
- [x] AC11. Scenes round-trip through `scenes.yml`: written by the service,
      read back identically, and a hand-edited file that is malformed is
      reported rather than overwritten.
- [x] AC12. Verified on the development machine: a scene held across a G502
      wake, a preview that survives the re-assert timer, and a preview that
      reverts when its holder is killed.

## Verified on hardware

Development machine, six devices, with somebody watching the room.

A scene written out, saved, shown and applied:

	$ hotaru scene set evening kraken=#201040 keychron=#100820 \
	      --effect keychron="Solid Splash" --screen dashboard
	$ hotaru scene apply evening
	evening: 2 of 2 device(s) lit.
	The screen is showing dashboard.

The Keychron took `Solid Splash` from the scene's effect while the cooler took
Direct, which is the per-device effect doing what `Request.Mode` could not.

**The lease was tested by killing its holder**, not by asking it to stop.
`hotaru scene preview loud` turned the cooler and keyboard red; `hotaru
preview` named the holder and the devices; `hotaru light list` marked both as
`preview`. A `kill -9` on that process put both back to the evening scene
within a second, with no release call made and no clock involved -- the socket
closing was the whole signal.

The unheld form was watched lapsing on its own: a preview taken at 18:39:10
with nothing renewing it was gone by 18:39:20, and the devices came back to
`#201040` and `#100820`.

**The Keychron came back in `Solid Splash`, not Direct**, which is the
reconcile fix this spec needed. Desired state had always recorded the mode and
reconciling discarded it -- invisible while every scene was a solid colour, and
wrong the moment one carries an effect.

## Risks & Assumptions

- **A lapsed lease stops being reported before it stops being shown.** The
  listing hides a lease the moment it expires; the write that puts the hardware
  back happens on the reconciler's next tick, up to five seconds later. So
  there is a short window where a device is showing a draft and nothing says
  so. Closing it properly means the expiry itself doing the write, which is a
  timer per lease.

- **A suspended reconciler is a suspended safety net.** For the devices a
  preview covers, hotaru stops correcting drift. That is the point, and it is
  bounded by the lease -- but a lease that renews forever is a reconciler
  turned off indefinitely, and nothing will report that except R7's marking.
- **The screen has one author and now two things want it.** Spec 013's
  `Hold`/`Release` is the whole mechanism, and a scene taking the panel is the
  first caller other than the CLI. A scene that takes it and a person who runs
  `hotaru screen dashboard` afterwards must not fight; the last instruction
  wins, which is the same rule as everywhere else here.
- **Half an effect model.** This spec resolves an effect name to a device mode
  and leaves the rendered kind unimplemented, which means a scene file can name
  something that works on one release and not the previous one. The mitigation
  is AC2a: an unknown effect is reported, never silently ignored.
- **Effects in two places.** A device's rule names `solid_modes` and a scene
  names an effect. Precedence has to be stated once: the scene is what somebody
  asked for now, so it wins, and the rule remains what the device is like.
- **Rollback** is that scenes are additive -- a machine with no `scenes.yml`
  behaves exactly as it does today, and the preview lease is inert until
  something asks for one.

## Alternatives Considered

Considered one lease mechanism for every client, expiry only; rejected because
the GUI's connection already reports its own death instantly and exactly, and a
heartbeat would be hotaru asking a question the kernel has already answered.

Considered a scene carrying no screen state, leaving the panel to the dashboard
alone; rejected because the monitor's animation bank is the feature being
replaced and it is half of what people used it for.
