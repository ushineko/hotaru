# Spec 086: no blink while a restore waits

**Issue**: [#240](https://github.com/ushineko/hotaru/issues/240)

## Status: COMPLETE

## Executive Summary

A restore waiting for a missing device retried a full restore up to every
30 seconds, and each one started every effect again: every breathing device
blinked. A try now only looks for the missing devices, and runs the full
restore again when one appears. Reviewers should look first at the wait loop
in `internal/daemon/reconciler.go`, and at how spec 084's in-step start at
boot is kept.

## Context

A restore that reaches fewer devices than were recorded is unfinished, and
the reconciler tries again on a backoff that grows to 30 seconds. Each try
was a full restore. Since spec 084, a full restore starts every recorded
effect again, so that the devices begin in step. A try while a device stayed
away therefore restarted every effect on every device that was already
showing it.

On the cachyos test machine the state still records the Apex under the name
OpenRGB gave it. That name has not appeared since OpenRGB was removed, so the
restore never finished. Every 30 seconds every breathing device dropped to
dark mid-breath and faded back in. The maintainer saw it as a blink at the
end of the memory's four colours. Polling `hotaru light list --json` five
times a second showed every animator starting again at the same instant,
30 seconds apart.

The desk shows the same over its first minute after a boot: five tries, each
a full restart. Any machine with a recorded device that is unplugged or
switched off blinks every 30 seconds for as long as the device is away.

## Requirements

- **R1** While a restore waits for missing devices, a try writes nothing to
  the devices already put back.
  - **R1.1** A try looks for the devices it waits for without writing:
    `Service.Missing`, the recorded devices not present now.
  - **R1.2** When a device it waits for is present, the full restore runs
    again, and starts every effect again, so that devices that appear over a
    boot still begin in step (spec 084).
  - **R1.3** A failure to look counts as a device appearing, so the restore
    runs and reports what went wrong.
- **R2** The backoff, the reports and the end of a restore are unchanged: a
  restore still says once per change what it waits for, and finishes when
  every recorded device is present.
- **R3** Cost: a device that never appears costs one device listing per
  backoff step (30 seconds at the end), and no write.

## Acceptance Criteria

- [x] While a recorded device is missing, a device already put back is sent
  no frame by further tries (`TestWaitingForADeviceWritesNothingToTheDevicesPutBack`,
  which fails on the code before this change with 20 frames in 100 ms).
- [x] A device that appears later is still restored, by the same run
  (the same test, and `TestARestoreWaitsForDevicesThatHaveNotEnumeratedYet`).
- [x] Waiting is still reported once per change, not once per try
  (`TestWaitingIsSaidOncePerChangeNotOncePerAttempt`).
- [x] Live, on cachyos with the stale record in place: after the service
  starts, no drawn device's animator starts again over two minutes.
- [x] Live, on cachyos: the maintainer sees the memory's Breathing run
  through its colours with no blink.
- [x] `docs/architecture.md` (the restore diagram, re-rendered),
  `docs/hardware.md` and the README changelog say so.

## Risks & Assumptions

- **A device that appears late starts the others again.** That is the full
  restore, as before, once per device that appears rather than once per try.
  Over a boot the lights are coming up anyway.
- **A device that leaves and returns during a restore** counts as appearing,
  and restores everything again. The restore ends once every device is
  present, and attachment then restores a returning device on its own.
- **The stale record itself** stays on cachyos until it is removed by hand.
  There is no command to forget a recorded device. After this change it
  costs a listing every 30 seconds and no write. The cachyos record is
  removed by hand once the live check is done.
- **Rollback:** revert the commit. Nothing recorded or configured changes
  shape.

## Alternatives Considered

- Retrying with only the missing devices: rejected, because a device that
  appears late would start out of step with the rest, against spec 084, and
  a cold boot often finds none on the first try.
- Not counting the name in a rule's `twin:` as missing: rejected, because
  `twin:` has been read and ignored since spec 076. It would also fix only
  this one case, not an unplugged device.

## Verification

### Checks (2026-11-22)

- The new test failed on the code before the change: `expected: 6, actual: 26`
  frames, twenty in 100 ms at a 5 ms backoff.
- `make test` and `make lint` (0 issues) pass with the change.

### Live runs (2026-11-22, cachyos)

**Before**, hotaru 2.3.2 with the stale Apex record: `hotaru light list
--json` polled five times a second for 45 seconds. Every drawn device's
animator started again at the same instant twice, at :47.0 and :17.0, 30
seconds apart. The rate window reset on all five devices sampled, and once
read no rate at all.

**After**, the branch build (`2.3.2-8b94c79-dev`) under the user service
through a drop-in, the stale record still in place. The log said once
`restore incomplete: 9 devices put back, still waiting for SteelSeries Apex
Pro TKL Gen 3 Wireless` and nothing more. Over 560 samples, 12:19:15 to
12:21:45, no drawn device's rate fell below 15 or rose to 17.8: no animator
started again. The old code would have restarted every effect at about 1, 3,
8, 23, 53, 83, 113 and 143 seconds after the start.

The maintainer watched the memory and the other breathing devices on the
branch build, with the stale record still in place: Breathing ran through
the four colours with no blink.
