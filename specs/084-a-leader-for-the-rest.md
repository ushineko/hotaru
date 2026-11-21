# Spec 084: a leader for the rest

**Issue**: [#236](https://github.com/ushineko/hotaru/issues/236)

## Status: COMPLETE

## Executive Summary

Effects given to several devices at once now start together, leader or not,
and any device can lead every device that has no leader of its own. On the
Linux test machine, which has no cooler, one memory module ran out of step
with the rest after every device was given Breathing. Review `followOf` and
`givenEffects` in `internal/service/follow.go`, the restore change in
`reconcile.go`, and the lead handling in `SetRule`.

## Context

Spec 077 starts a leader and its followers in step (R3.5) and leaves other
devices alone: an apply that gives a device the effect it already runs
carries on from where it was. A machine with no cooler has no device the
others follow, so an apply giving all of them Breathing started only the
devices whose effect changed. The maintainer, on the test machine
(2026-11-20): any device should be able to lead when none is the obvious one.

## Requirements

- R1 An apply (and so a scene) that gives more than one device in scope a
  named effect restarts all of them, so they start together.
- R2 A full restore restarts every device with a recorded effect, as it did
  for leaders and followers.
- R3 A rule's `lead: true` makes that device the leader of every device
  whose rule names no leader of its own; a device's own `follow` wins; the
  leader follows nothing; a device a canvas draws follows nothing.
- R4 One device leads: setting the lead on a device clears it from every
  other rule and clears that device's own `follow`. The followers take the
  leader's recorded effect at once, in step.
- R5 Parity: `POST /v1/rules/device` takes `lead`; `GET /v1/devices` says
  `leads`; `hotaru light lead <device|none>`; a Leads the rest check on each
  device's card in the window.
- R6 The cooler is not made the leader without being asked: the desk's
  devices name it in their own rules, and an implicit leader would pull in
  devices set on their own.

## Acceptance Criteria

- [x] `make test`, `make lint`, `make lint-windows`, GUI tests.
- [x] Tests: who follows whom with a leader, a device's own follow, the
      leader itself and a canvas's twin
      (`TestEveryDeviceWithoutALeaderFollowsTheOneThatLeads`); several
      effects counted and a device not named left out
      (`TestSeveralEffectsAreCounted`); a leader's effect taken by a device
      with no rule (`TestADeviceThatLeadsIsFollowedWithNoCooler`); the lead
      written, moved and stopped (`TestLeadIsSetMovedAndStartsNow`); the
      rule key round-trips (`TestEveryRuleKeySurvivesBeingWrittenBack`).
- [x] Live on the Linux test machine: the four memory modules given
      Breathing together breathe in step, with the maintainer watching.
- [x] `docs/api.md`, `docs/hardware.md` and the changelog say what changed.

## Risks & Assumptions

- **A restart on every multi-device apply**: re-applying the same scene
  starts its effects again from the beginning, which is what "in step"
  means; reconciles and re-asserts do not apply, so nothing restarts on a
  timer.
- **A full restore** at start retries until every recorded device is found;
  each retry restarts the effects already showing, as leaders and followers
  already did.
- **Rollback:** revert; a rules file with `lead: true` is read by an older
  build as an unknown key and ignored.

## Verification

### Checks (2026-11-20)

`make test`, `make lint`, `make lint-windows` and the GUI tests pass.

### Live run (2026-11-20, the Linux test machine)

A build of this branch, against sanshoku's EVGA zone fix (sanshoku #119),
ran under the packaged user unit through a drop-in. Re-applying the
maintainer's `landscape1` scene, which gives every device Breathing, lit 9
of 10 devices; the four Corsair DDR5 modules, the EVGA card, the board, the
dock and the keyboard reported Breathing at about 16.5 frames a second. The
maintainer, watching: the card breathes, and the four modules breathe in
step with each other and the rest. Before, one module ran out of step and the
card stayed static.

Seen on the way: right after effects restart, `light list` shows a rate
measured over the first two frames (tens of thousands a second, or 0.0)
until a few seconds have passed; a reporting quirk of the rate window, not
of the frames. The Basilisk V3 Pro shows nothing for `landscape1`: the
machine's `razer` rule names a zone `Base` the mouse does not have, a rules
matter for that machine.

The lead itself was not set on the machine, to leave its rules file as the
maintainer wrote it; the tests cover it.
