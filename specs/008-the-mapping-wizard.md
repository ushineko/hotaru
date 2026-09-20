# Spec 008: the mapping wizard

**Issue**: [#10](https://github.com/ushineko/hotaru/issues/10)

## Status: COMPLETE, except zone sizing (AC14)

## Context

Spec 001 established that naming a machine's lights cannot be derived, and
recorded the interaction that works. This spec builds it.

The evidence is a rehearsal. Mapping the development machine by hand took seven
rounds of "light something, say what changed" and produced a map no protocol
could have given: three top radiator fans daisy-chained into one cooler channel
at eight LEDs each, two front intake fans on one board header, the rear exhaust
alone on another — and **three zones of nine with nothing attached at all**,
every one of them accepting writes and reporting success.

It also produced the mistakes the wizard exists to avoid. An ambiguous
observation was taken as settled and built on, putting two fans on the wrong
device for several rounds. Adjacent fans were lit blue, cyan and teal, and the
answer came back "bottom cyan(?)" — fair, and then repeated later by the same
author who had already written "use unmistakable colours" into a spec.

Vendors solved this the same way: NZXT's software lights zones and asks which
fan is which. That is worth knowing twice over — it means the interaction is
right, and it means some users arrive expecting its conventions.

## What it is

A conversation, in both shells. This spec builds the CLI half; the GUI's
visual version is spec 005 and shares the same service operations.

	hotaru light map

Nothing else about a machine is a prerequisite: it runs on a fresh install with
no rules file, which is the case it exists for.

## The conversation

1. **Check first.** An unreachable OpenRGB server, or one with no devices, is
   reported with its remedy and the wizard stops. Mapping a machine hotaru
   cannot see would waste a person's attention on a question with no answer.
2. **Ask what is there.** Per device, light each zone a distinct colour and ask
   what the user can see. They answer about their own hardware in their own
   words; hotaru knows which zone it made blue.
3. **Ask how many.** Before splitting a zone, ask how many separate things are
   on it. Identical daisy-chained fans divide evenly, and one answer plus the
   LED count gives the split arithmetically.
4. **Propose, show, confirm.** Light the proposed split and ask whether each
   thing shows one colour. Only then name the parts.
5. **Bisect when the division is not clean.** Light halves, ask whether any one
   thing is showing two colours at once, and move the boundary by half the
   remaining distance.
6. **Confirm the whole map** before writing anything.
7. **Put the lights back**, whatever happened, including on an interrupt.

## The rules it follows

- **Four colours, maximally distinct**: red, green, blue, white. More zones
  than that means more rounds, not a subtler palette. Cyan beside teal is a bad
  question.
- **Every question is answerable by looking.** Never "which LEDs are the rear
  fan"; always "what colour is the rear fan", or "is anything showing two
  colours".
- **Nothing is written until the user confirms**, and what is written is shown
  first.
- **The user's file is still theirs.** On a machine with no rules file, the
  wizard offers to create one. Where one exists, it prints what to add and
  changes nothing — hotaru does not rewrite a file a person maintains, and a
  wizard is not an exception to that.
- **Interruption is safe.** Ctrl-C restores the lights and writes nothing.

## Acceptance Criteria

- [x] AC1 `hotaru light map` runs on a machine with no rules file and produces
      a usable one.
- [x] AC2 An unhealthy service is reported with its remedy and the wizard stops
      before asking anything.
- [x] AC3 Zones are lit in distinct colours from a fixed palette of four, and
      no two zones lit in one round share a colour.
- [x] AC4 A zone the user names as holding several things is divided evenly
      where the LED count allows, and the division is shown and confirmed
      before anything is named.
- [x] AC13 Several things sharing one control are named once, and said to be
      controlled together, rather than divided between LEDs that do not exist.
- [x] AC15 A question is asked only where its answer can be acted on: a zone
      with one light is named, never divided.
- [x] AC16 Every question has an answer that ends it, including "nothing is
      lit" and "cannot tell".
- [x] AC17 What the wizard lights is written as a preview and not remembered,
      so restoring puts back what the machine wanted rather than the questions.
- [x] AC19 One thing is lit at a time, with every other device dark, so no
      question can be answered about something else in the case.
- [x] AC20 Nothing the wizard says uses a word from this program's vocabulary,
      asserted by a test over everything it says.
- [x] AC21 Ways of lighting a device are probed in the order a real write would
      try them, so a confirmed one is the one that will be used.
- [x] AC22 What is written is in use before the wizard restores the lights.
- [x] AC18 Ctrl-C ends the wizard on the first press, restores the lights, and
      says one line.
- [ ] AC14 A resizable zone with no length set is asked about before it is
      mapped, and sized through the service, so an attached strip is found
      rather than recorded as an empty header.
- [x] AC5 A division that does not fit evenly, or a confirmation that fails, is
      bisected: halves are lit, the user is asked whether anything shows two
      colours, and the boundary moves by half the remaining distance.
- [x] AC6 A zone the user reports as dark is recorded as empty and named
      nothing — three of nine on the development machine.
- [x] AC7 The whole map is lit once more and confirmed before any file is
      written.
- [x] AC8 Lighting is restored on completion, on decline, and on interrupt.
- [x] AC9 With no rules file, the wizard offers to write one and does so only
      on a yes. With a file present it prints what to add and writes nothing.
- [x] AC10 The wizard's questions and answers are testable without a terminal:
      a scripted conversation drives it end to end against the in-memory
      server.
- [x] AC11 A user who answers "I do not know" or gives no answer leaves that
      zone unnamed rather than guessed.
- [x] AC12 The rules it writes load: a test round-trips the generated file
      through the config loader and asserts the segments resolve.

## What rehearsing it changed

Three things the design did not have until it was run against real hardware.

**It has to find out whether it can light a device before asking what is on
it.** On the development machine hotaru's default order picks Static for the
ASUS board, the write is accepted, the mode is reported back, and the
addressable headers stay dark — so a user would answer "nothing" to every zone
and map half their case as empty. The wizard now lights each device white and
asks whether anything happened, trying its other plain modes until something
does, and writes the resulting `solid_modes` correction into the rules with the
reason. **That correction is one no read-back can find**, because the mode
change genuinely takes; `probe` reports Static as working on that board.

**The identifying word is chosen against the machine.** Picking by position
gave `match: rog` for "ASUS ROG MAXIMUS Z790 HERO" — a brand, not a device. It
is now the longest word that matches this device and no other one present,
which yields `maximus` and `kraken`.

**An answer to a different question is not a name.** A mis-scripted rehearsal
produced a segment called `y`. Single characters, yes and no, and the palette's
own colour names are declined with an explanation and asked again, because a
slip that reaches the rules file becomes a puzzle weeks later.

## What the second machine added

Run on hardware the author does not own, the conversation needed three things
it did not have.

**A mode is probed exactly.** The first run there asked about Direct four times,
because every other mode it tried fell through to Direct and was reported as
Direct. Asking what one mode does cannot be answered by quietly trying another.

**Things sharing one control are not split.** A 12V header is a single control
for everything plugged into it: two strips daisy-chained onto one are two
strips and one LED. Dividing that LED between them reported a boundary that
could not be settled, which sounds like a fault rather than the plain fact it
is.

**A header with no length set cannot be tested at all.** An addressable header
reports `count=0, max=256` until someone says how long the strip on it is, so
hotaru lights nothing there, the user answers "nothing", and an empty header is
recorded -- which is right by accident and wrong when a strip is attached. The
wizard must ask the length before it can ask what is there, and a provisional
length is enough to find out whether anything is attached at all. **Not yet
built**; it needs a resize call hotaru does not make.

The same session settled what was on that machine in a few minutes, including
the negatives: two addressable headers with nothing attached, and a pair of
strips on the analogue one that their owner had forgotten were there. That is
the argument for the whole feature -- "I do not remember what is plugged in" is
the normal condition, and it is answerable by lighting things and asking.

## What using it taught

Everything below came from its author watching somebody else use it on their own
machine, and none of it was visible from the machine it was written on.

**Never ask what cannot be acted on.** A question implies its answer will be
used. Asked how many things were chained on a 12V header, somebody answered
"two strips" -- true about their desk, and meaningless here, because that header
is one control signal and reports one LED. hotaru knows the difference from the
LED count, so the question is not asked at all where nothing can be divided.
Handling the answer more gracefully would have been the wrong fix.

**Ask for a name, not a colour.** "What is red?" asks somebody to identify a
colour. They are naming a thing. It is "the red one -- what do you want to call
it?".

**Say when the question changes.** The mode probe lights a whole device one
colour, and the mapping round then lights its zones in several. Without a
sentence between them, "red" still means "the thing that just lit up", and
somebody named an empty header after the strips they had seen a moment earlier.

**Every question needs an answer that ends it.** Told there were two things on
an empty header, the wizard asked which of them the red part covered, and
refused every answer that was not one of three it expected -- about lights that
were not there. "Can you see it lit at all?" comes first now, and "cannot tell"
is an accepted answer to the rest.

**What the wizard lights is not what the machine wants.** Its colours were being
recorded as desired state, so putting the lights back put the questions back,
and a reboot would have restored whatever it happened to be lighting when
somebody walked away. They are previews: written, and not remembered.

**Stopping is a decision, not a fault.** Ctrl-C mid-question left the lights on
a question and printed two lines about a URL nobody typed, because the cleanup
used the context that had just been cancelled and the prompt was blocked on
stdin. It now ends on the first press, puts the lights back, and says one line.

**Light one thing at a time, with everything else dark.** Lighting a device's
parts in four colours at once and asking which was which saved rounds and cost
sense: two parts of a board had nothing attached, so somebody was asked to name
"the red one" while the only thing they could see was blue -- and "the green
one" could have been a stick of RAM, because the rest of the case was lit too.
One thing lit, nothing else, and the question is what came on.

**No jargon, at all.** "Zone", "LED", the name of a lighting mode: this
program's vocabulary, which asking somebody to learn before they can answer is
the opposite of what a wizard is for. A test fails if any of those words appear
in anything it says.

**Probe in the order a write will use.** Asking about a device's ways of
lighting in the order the device lists them let somebody confirm one that an
ordinary `light set` never chooses -- so the wizard blessed it, wrote no
correction, and setting a colour turned their strips off afterwards anyway. The
question has to be about what will actually be used.

**Put what was written to work before finishing.** The service reads its rules
when it starts, so a file the wizard writes changes nothing until it is told.
Leaving that as an instruction meant the last act of a successful run was
putting the lights back with the rules from before it ran, undoing the
correction it had just established. It reloads, and says the rules are in use.

**Only ask a question the hardware makes sensible.** "How many separate things
are chained on keyboard" is a question about a run of lights on one cable, and a
keyboard is not one -- its keys are individually addressable, so somebody
reading it carefully had no way to answer. The count alone does not distinguish
the two: a keyboard and a light strip both report many lights. OpenRGB says
which shape each part is, and the question is asked only where the shape is a
line. A grid is never chained, and a single light has nothing to divide.

## Risks & Assumptions

- **A wizard is a user interface, and this one has no pixels.** Its whole
  quality is in the wording of six or seven questions. The questions are
  therefore specified above rather than left to the implementation.
- **People answer loosely.** "rear blue, front dual is green" is a real answer
  from the rehearsal and parses as three facts, one of which is a count. The
  wizard should accept a colour and a name in either order and should not
  demand a syntax.
- **Restoring depends on knowing what was showing**, and a device in a
  non-per-LED mode misreports its colours. The wizard uses hotaru's own record
  where there is one, which is the same rule composition follows.
- **Rollback**: the wizard writes one file and only with consent. Deleting it
  returns the machine to driving every device with no corrections.

## Testing

A scripted conversation drives the whole wizard against the in-memory server,
asserting the questions and their order rather than only the file at the end.
Live runs against six real devices produced the same map that was worked out by
hand the night before, including the `solid_modes` correction when the mode
question was answered honestly.

## Alternatives Considered

- Considered asking the user to read LED indices from a listing; rejected —
  that is the thing nobody does twice, and it is why this exists.
- Considered inferring fan counts from zone sizes; rejected: 16 LEDs is two
  eight-LED fans, one sixteen-LED fan, or an empty header, and the protocol
  reports the same number for all three.
