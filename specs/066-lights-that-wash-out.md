# Spec 066: lights that wash out

**Issue**: [#186](https://github.com/ushineko/hotaru/issues/186)

## Status: COMPLETE

## Executive Summary

The RTX 4090's and the MM700's LEDs wash out: a pale blue the fans show as
written looks close to white on them. Matching them by eye made the editor
show different colours for lights that looked the same.

A device rule now takes a `colour` profile. It pushes saturation and value
toward 100% and applies a gamma per channel. The service corrects every
write to that device and reports the colour that was meant, so scenes, the
editor and Make a scene need no change.

Look first at `Profile.Apply` in `internal/devices/profile.go`, then at the
read-back translation in `internal/service/correct.go`.

## Context

Some lights show a written colour less accurately than others. The MSI RTX
4090 Suprim Liquid X and the Corsair MM700 wash out. A pale colour looks close
to white on them, while the NZXT fans and the keyboards show it as written.

The scene `ice1` shows the size of it. The 4090's colour was picked by eye to
match the Kraken's fans:

| | Written | Hue | Saturation | Value |
|---|---|---|---|---|
| Kraken fan | `#80aad1` | 209° | 39% | 82% |
| 4090, matched by eye | `#003eff` | 225° | 100% | 100% |

Saturation and value are far apart, and the hue is 16° toward blue: the
card needs less green for the same colour. These LEDs show a smaller range
of colour. Two things follow:

- The editor shows different colours for lights that look the same.
- Make a scene gives these two devices washed-out colours.

A scene should hold the colour that was meant, on every device. Then the
correction is a fact about the hardware, and it belongs in the rules file
with the other corrections.

## Requirements

- **R1** A device rule takes a `colour` profile with two settings,
  `saturation` and `value`, each from 0 to 1. Each says how far toward 100%
  that property of every written colour is pushed. 0 leaves it as written;
  1 makes it 100%. The hue is kept.
  - **R1.1** A value outside its range is reported and ignored, as the other
    settings are.
  - **R1.2** Several matching rules merge, later settings winning, one
    setting at a time.
  - **R1.3** Black stays black, and saturation and value never give a grey a
    hue. A profile never lights a light that was asked to be dark.
  - **R1.4** The profile also takes a `curve`, a gamma per channel (`red`,
    `green`, `blue`, above 0 and up to 10), applied after saturation and
    value. Off and full pass it unchanged, so black, white and the pure
    colours do too. Chosen by the maintainer over a hue offset, because a
    channel that is too bright in its middle levels is what was measured.
- **R2** The service applies the profile to everything it writes to the
  device: the frame, and the colours of a mode that keeps its own. Every path
  that writes goes through it: scenes, `light set`, previews, reconcile and
  canvas devices.
- **R3** What the service reports is the colour that was meant. Reading a
  device back gives, for each light and each mode colour, the colour asked
  for where the device holds the corrected one.
  - **R3.1** So a corrected write is confirmed, not reported as unconfirmed.
  - **R3.2** A light the device reports in any other colour is reported as
    the device gives it.
- **R4** A device with no profile is written exactly as before.
- **R5** `docs/hardware.md` describes the setting, and the README's
  changelog has the entry.

## Acceptance Criteria

- [x] `ice1`'s fan colour `#80aad1`, through saturation 1, value 1 and a
      green curve of 2.15, is written `#003eff`, the 4090's hand-matched
      colour. Without the curve the hue is kept
      (`TestIce1sFanColourComesOutAsTheCardWasMatched`).
- [x] Black, white and the pure colours pass a profile unchanged, a grey
      gets no hue from saturation, and a zero profile changes nothing
      (`TestAProfileNeverLightsTheDarkOrInventsAHue`).
- [x] A rule's profile is read, merged and range-checked
      (`TestAColourProfileIsReadAndRangeChecked`,
      `TestColourProfilesMergeOneSettingAtATime`).
- [x] A scene applied to a device with a profile writes the corrected frame,
      is applied without an unconfirmed note, and lists the colour that was
      meant (`TestACorrectedDeviceIsWrittenCorrectedAndReportsWhatWasMeant`).
- [x] A mode's own colours are corrected and read back as meant
      (`TestAModesOwnColoursAreCorrected`).
- [x] A device without a profile is written as asked
      (`TestADeviceWithoutAProfileIsWrittenAsAsked`).
- [x] A light changed by something else is reported as the device gives it
      (`TestALightChangedElsewhereIsReportedAsItIs`).
- [x] `make test`, the GUI tests and `make lint` pass.
- [x] Live, with someone watching: with a profile on the 4090 and the MM700,
      a scene made from a picture gives them colours that look like the fans
      and the keyboard, and the editor shows them as the same colours.

## Risks & Assumptions

- **The model is a first guess.** Pushing saturation and value toward 100%
  matches the one measured pair. The settings are numbers so they can be
  tuned on the hardware. A gamma curve per channel is the next thing to try
  if the guess is wrong.
- **A near-grey becomes a strong colour at high saturation settings.** A
  colour with 2% saturation, pushed all the way, shows its hue at full
  strength. Exact greys are kept (R1.3). This is the price of the setting.
- **A colour already corrected by hand is corrected twice.** `ice1`'s
  hand-matched `#003eff` passes saturation and value unchanged, and the
  green curve then writes it `#000cff`. A scene matched by hand is picked
  again, or made again from its picture.
- **The read-back is remembered, not computed.** Many colours give the same
  corrected one, so it cannot be undone by arithmetic. The service keeps,
  per device, what it last asked for and what it wrote. After a restart it
  has no record, and lists the corrected colours until its first write to
  that device. A restart puts the lights back, which is that write.
- **Cost**: a few float operations per light per write. Nothing runs between
  writes.
- **Rollback**: remove the `colour` setting from the rules file and run
  `hotaru reload`, or revert the merge. An older build reading a rules file
  with the setting ignores the key.

## Alternatives Considered

- Correct only in Make a scene. Rejected: colours picked by hand stay
  uncorrected, and the editor shows different numbers for matching lights.
- A hue offset. Rejected by the maintainer: it turns every hue, red toward
  magenta as well, where the measurement shows one channel too bright.

## Verification

Tests: the criteria above, each named. Each fix was checked against a
mutation that undoes it:

- Not translating the read-back fails the listing, mode and changed-elsewhere
  tests.
- Writing a mode's colours uncorrected fails
  `TestAModesOwnColoursAreCorrected`.

`make test`, `go test -tags migrated_fynedo ./internal/gui/...` and
`make lint` pass. `govulncheck ./...`: no vulnerabilities.

Live, with the development build as the user's service, and the rules file
given the profile in `docs/hardware.md` for the 4090 and saturation 1,
value 1 for the MM700:

- The service restarted and put back 6 devices. `/v1/devices` lists the
  4090 as `#003eff` and the MM700 as `#7faad2`, `#79a6d0`, `#6996c3`: the
  colours `ice1` asked for, not the ones written.
- The maintainer made a new scene from a picture with these profiles and
  looked at the machine: the 4090 and the MM700 matched the fans and the
  keyboard well enough to release ("seems to work"). The numbers were not
  changed from the ones above.
