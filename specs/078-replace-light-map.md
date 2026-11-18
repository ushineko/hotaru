# Spec 078: replace light map with commands that show their answer

**Issue**: [#220](https://github.com/ushineko/hotaru/issues/220)

## Status: COMPLETE

## Executive Summary

`hotaru light map`, a wizard that could not be got through on the desk, is
replaced by commands that each show their answer: `light length` lights a
header with its last light red and moves the end onto the strip's last light;
`light fans` and `light strip` say what a header holds; `light name` names a
part once it is seen lit, or goes through a device's parts. Each device's card
in the window has Headers and Names. All write through the service's rules
route, which now takes lengths, fans, shapes and names. Reviewers should look
first at `internal/cli/headers.go`, `internal/service/rules.go` (`divide`,
`changeZones`) and `internal/gui/headers.go`.

## Context

`hotaru light map` is a wizard: it lights each device in turn and asks what
is lit, how many things are chained, whether anything should be dimmer, and
whether the result is right. It dates from OpenRGB, when hotaru knew almost
nothing about the hardware it lit.

On 2026-11-17 the maintainer ran it on the desk for spec 077's live checks
and could not get through it. What went wrong:

- "Should any of the lights be dimmer than full?" could not be understood.
- "No" at "Is that right?" stopped the run and lost every answer.
- The Kraken's fans were written with no name, the Kraken's former name was
  gone through as a device, hand-written rules were not found, and the
  keyboard's caps and num segments were dropped. The saved file had to be
  put back.
- Header 3 came out at 18 lights where it has 20. Halving the range cannot
  recover from one misjudged answer, and nothing shows it went wrong.

The last two groups were fixed on spec 077's branch. The wizard is still
long, and each answer is a judgement made on a question, not something seen.

sanshoku now reports what the wizard used to ask for: each zone and its name,
the Kraken's fans and their lights, shapes, and the effects that work. Three
things are still unknown to hotaru:

1. **A header's length.** The board cannot count what is plugged in.
2. **What is on a header:** a strip, or fans, and how many fans.
3. **Names** a person wants for a zone or a part of one. These are optional:
   a scene can say `maximus/"Addressable RGB Header 3"=red` today.

`hotaru light match` (spec 077 R3.9) showed a shape that works: one thing on
the desk, changed by a key at a time, with the result in plain sight, kept
on Return.

## Requirements

### R1. A header's length, seen

- R1.1 `hotaru light length <device>/<zone>`, with no number, finds a length
  by looking. The zone is lit one colour, with its last light another, and a
  marker every tenth light. `+` and `-` move the end by one, `++` and `--` by
  ten, a typed number sets it, Return keeps it, and `q` leaves it as it was.
- R1.2 Kept is written to the rule's `lengths`, through the same path as
  `light match` writes `pace`, and is in use at once.
- R1.3 `hotaru light length <device>/<zone> <n>` stays: it tries a length
  until the service stops.
- R1.4 A zone whose length the device counts is refused, with why.

### R2. What is on a header

- R2.1 `hotaru light fans <device>/<zone> <n>` says a header holds n fans:
  it divides the zone into n equal parts, writes `shapes` as `ring`, and
  lights each part a colour so the division can be checked. A length that
  does not divide by n is refused, with the nearest lengths that do.
- R2.2 `hotaru light strip <device>/<zone>` says a header holds a strip:
  `shapes` as `line`, one part.
- R2.3 A zone whose parts the device counts (the Kraken's fans) is refused,
  with what the device counts.

### R3. Names

- R3.1 `hotaru light name <target> <name>` names a zone, a fan from R2.1, or
  a range, writing a segment. The target is lit while the command runs, so
  the name is given to something seen.
- R3.2 `hotaru light name <device>` with no name goes through the device's
  zones and fans one at a time, each lit, offering its current name.
  Return keeps a name, a new word replaces it, and `-` removes it.
- R3.3 Names follow the rules already in place: lowercase words and dashes,
  unique per device. Toggles (caps, num) are segments like any other.

### R4. light map goes

Decided by the maintainer on 2026-11-17: removed entirely, not kept as a
guided list, and the settings only it wrote are set by hand.

- R4.1 `hotaru light map` is removed. Run, it says which commands replace it.
- R4.2 What only the wizard set:
  - `never_blank` and a stay-dark `solid_modes` (a keyboard that cannot go
    dark): set by hand in the rules, documented in `docs/hardware.md`;
  - `brightness`: set by hand, or by `hotaru light set --brightness`, as
    now.
- R4.3 The README's first-run steps say: install, start the service, then
  `hotaru light length` for each header with lights on it, where the board
  has headers. Naming is optional and said to be.

### R5. The window

- R5.1 A device's page has the same three: a header's length with a stepper
  and the lights shown live, what is on a header, and a name per zone or
  fan, each lighting what it is about while it is open.
- R5.2 CLI, window and API parity: each write is an API route, reached from
  both.

### R6. Documentation

`docs/api.md` for any new route, `docs/hardware.md` for headers and the
rules set by hand, `docs/architecture.md` if the shape of the CLI changes,
and the README changelog and first-run steps.

## Acceptance Criteria

- [x] `make test`, `make lint` and the GUI tests pass (three picture-drop GUI
  tests skipped until #219, as on spec 077).
- [x] Tests:
  - `light length` with keys: `+`, `-`, `++`, `--`, a number, Return writes
    `lengths`, `q` writes nothing; a zone the device counts is refused;
  - `light fans` divides and writes `shapes`; a length that does not divide
    is refused with the nearest that do; a counted zone is refused;
  - `light name` writes a segment for a zone, a fan and a range; the walk
    offers current names and removes one on `-`; other rule keys are kept;
  - `light map` says what replaces it, and nothing else;
  - the parity tests cover every new route: there is none; the commands and
    the window write through `POST /v1/rules/device`, which takes
    `lengths`, `fans`, `shapes` and `segments` now, and is already covered.
- [x] Live, with the maintainer watching:
  - the Z790's three headers set to 18, 16 and 20 with `light length`, each
    end seen on the desk, in under a minute each;
  - the front header said to be two fans, and the two named;
  - the Kraken's three fans named with `light name kraken`;
  - the same from the window on one header.
- [x] Docs per R6.

## Risks & Assumptions

- **People with a mapped machine keep it.** Rules written by the wizard stay
  valid; nothing reads them differently.
- **The rules file is written by the client**, as `light match` and the
  wizard did, and the service reloads it. The window does the same. A route
  that writes rules in the service is an alternative (below).
- **Comments in the rules file are lost on a write**, as they are today. The
  commands say so before the first write, as the wizard did.
- **Removing a command is a break** for anyone who scripted it. It prints
  its replacements, so the break says what to do.
- **Rollback:** revert the branch; `light map` returns.

## Alternatives Considered

- Keep `light map` as a short guided list that runs these commands in turn.
  Rejected by the maintainer: removed entirely.
- Keep the wizard and fix it further. Rejected: the bugs were fixed, and it
  is still a long run of judgements with one place to go wrong per answer.
- Find a header's length by halving, as spec 077 R4.3 does. Rejected: one
  wrong answer gives a wrong length and nothing shows it.
- A service route that edits the rules. Deferred: the client writes the file
  today for `light match`, and moving every writer is its own change.

## Verification

### Tests

`make test` and `make lint` pass. The commands are tested as a person uses
them (`internal/cli/headers_test.go`): moving a length's end by one, by ten
and to a typed number, `q` writing nothing, a counted zone refused; fans
divided and named, a kept name, a dividing segment dropped, a toggle kept, a
length that does not divide refused with the lengths that do, counted fans
refused; a range named once seen, and a device's parts gone through with
Return, a new word and `-`, under a former zone's names. The service refuses
what a device would not take (`internal/service/rules_test.go`), and the
window's parts, renames and holds are tested (`headers_internal_test.go`).

### The desk (2026-11-17, the maintainer watching)

- `light length` on the Z790's three headers: 18, 16 and 20, each end seen
  and each easy; written to the rules.
- `light fans maximus/"Addressable RGB Header 2" 2`: two colours, the names
  `top-front` and `bottom-front` kept; Header 2 written as a ring.
- `light name kraken`: the ring and the three top fans offered in turn with
  their names, each on the part lit; nothing changed.
- The window: the Z790's Headers moved a header's red end live, and Names lit
  a part white on Show; both clear. Writing this up found that Headers read
  the rear fan, a ring named whole, as a strip, and would have saved it so;
  fixed before any save (a fan named whole reads as one fan, and Save sends
  only what changed).
- A first run of the length and fans checks left the rules unchanged, for a
  reason not found; the same commands run again wrote them, at 21:35.
