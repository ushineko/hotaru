# Spec 058: the handle that outlived the device

**Issue**: [#167](https://github.com/ushineko/hotaru/issues/167)

## Status: INCOMPLETE

## Context

A keyboard was unplugged and plugged back in. For the next hour it was listed,
in scope, and reported healthy, and nothing sent to it arrived.

OpenRGB detects hardware once, when it starts. This is already written down --
`StateNoDevices` says "a device connected since then is invisible until it
restarts" -- but that state is only reached when the server knows of no devices
at all. A device that was there when the server started and has been replugged
since is worse than invisible: it is still in the list, with its LED count and
its modes, and the server holds a descriptor for a node the kernel has removed.

```
Sep 27 13:08:26 kernel: usb 1-5.2: USB disconnect, device number 4
Sep 27 13:08:26 kernel: usb 1-5.2: new full-speed USB device number 46
Sep 27 13:08:26 kernel: hid-generic 0003:3434:0E40.0023: input,hidraw15: [Keychron Keychron K4 HE]

$ ls -l /proc/16211/fd | grep hidraw
31 -> /dev/hidraw1 (deleted)
```

The keyboard was on hidraw15/16 and the server held nothing for either. Writes
returned success. `light health` said `healthy: 6 of 6 devices are in scope`,
because the count and the scope were exactly what they had always been.

### Recovering it took two restarts, in order

OpenRGB first, so that it enumerates again. Then hotaru, which was left holding
a socket to a process that no longer existed:

```
$ hotaru reconcile
hotaru: ask 127.0.0.1:6742 how many devices it has: write tcp ...: broken pipe
```

The second restart should not have been necessary. `daemon.connect` retries
until its first success and then returns, so nothing is watching an established
connection for a drop. Every OpenRGB restart -- an upgrade, a crash, this --
leaves the service needing one of its own.

### Timestamps cannot detect this

The first idea was to compare device-node times against the server's start
time. It does not survive contact with the machine. On Sep 25 at 19:03 the AURA
and Kraken node mtimes both moved with no USB re-attach anywhere in the journal,
and both devices were working. hidraw numbers are recycled too: a virtual
gamepad held hidraw15 and hidraw16 that day, the numbers the keyboard has now.
The comparison would have called two healthy devices stale, which is the one
thing a health check must not do.

What is exact is the descriptor. A hidraw fd held by the server whose target is
marked `(deleted)` is a device that vanished underneath it -- no heuristic, no
threshold. It needs the server's PID and a readable `/proc/<pid>/fd`, which
holds for a user unit and does not for a root-owned system unit or a server
somebody started by hand. Where it cannot look, it says nothing.

### A server answers before it is ready

Found by running the finished command on the machine, on 28 Sep. The first
rescan reported "the server found 0 devices; restored 0" while the lights were
visibly coming back: a restarted OpenRGB answers a device listing before it has
finished enumerating, and answers it with an empty list and no error.

This is spec 033's observation from a new direction -- the reconciler already
retries a restore for it, because a cold boot was once seen finding two devices
of six. A reply is not readiness, so the bounce settles for what the server has
rather than sampling it once.

### What this changes about the systemd package

`internal/systemd` opens by saying hotaru does not fix anything there, because
starting a daemon is the user's decision. That stays true of boot
configuration: nothing here enables a unit or turns on lingering. It stops
being true of a restart the user explicitly asks for by running a command whose
entire purpose is to perform one. The package doc is amended to say which of
the two it means.

## Requirements

**R1. A dropped connection is re-dialled.** When the server goes away, the
service returns to the backoff that brought it up at boot and reconnects
without being restarted.

**R2. A reconnection puts the lights back.** A re-enumerated server has every
device in its power-on state, so reconnecting reconciles, exactly as starting
up does.

**R3. A stale handle is a state of its own**, between healthy and no-devices: a
server that answers, lists the device, and holds a dead descriptor for it.

**R4. The state names the device where it can.** A live hidraw node whose HID
name matches an in-scope device, for which the server holds no descriptor, is
the device that moved. Without that, the state still reports that something
moved.

**R5. Where it cannot look, it says nothing.** No PID, no readable `/proc`, or
no unit at all degrades to the advice that is there now, and never to a guess.

**R6. One command does the bounce**: restart the server's unit, wait for it to
listen, re-dial, reconcile, and report what it did.

**R7. It runs only when asked.** Detecting a stale handle names the command; it
does not run it. A unit the user cannot restart without root reports that,
with the command, rather than prompting.

**R8. It is reachable from the CLI, the API, and the window**, and the window
says what it is for: a device that has been unplugged and plugged back in.

## Acceptance Criteria

- [x] AC1. A service whose connection drops reconnects without a restart, and
      the reconnection reconciles
      (`TestAServiceThatLosesTheServerReconnectsAndPutsTheLightsBack`, with
      `TestATransientErrorIsNotADepartedServer` for the other half). Confirmed
      on the development machine on 28 Sep: the keyboard was replugged, the
      server showed `fd 30 -> /dev/hidraw16 (deleted)`, and bouncing the server
      alone brought the service back by itself --
      "the OpenRGB server at 127.0.0.1:6742 came back ... putting the lights
      back", then "restored 6 devices", with nothing restarting hotaru.
- [x] AC2. A server holding a hidraw descriptor marked `(deleted)` reports the
      stale state, not the healthy one (`TestADeletedHandleIsNotHealthy`).
- [x] AC3. A live device node with no descriptor in the server is named in the
      state (`TestTheStaleStateNamesTheDeviceThatMoved`).
- [x] AC4. An unreadable PID, an absent unit, and a non-systemd machine each
      report what they report now, with no stale claim
      (`TestWhatItCannotSeeItDoesNotClaim`).
- [x] AC5. Node mtimes moving without a re-attach does not produce a stale
      claim -- the Sep 25 case, as a test
      (`TestATouchedNodeIsNotAReplug`).
- [x] AC6. The bounce restarts the unit, re-dials, reconciles, and reports the
      device count (`TestRescanBouncesTheServerAndRestores`, with
      `TestARescanWaitsForDevicesThatHaveNotEnumeratedYet` for the enumeration
      lag below). Confirmed on the development machine on 28 Sep:
      `hotaru light rescan` reported "the server found 6 devices; restored 6"
      in 24 seconds.
- [x] AC7. A unit that needs root to restart reports the command instead of
      attempting it (`TestAUnitItCannotRestartIsAdvice`).
- [x] AC8. `light health`, the API, and the window all offer it, and the
      window carries the note (`TestRescanIsOfferedEverywhereHealthIs`).
- [ ] AC9. **Outstanding -- needs a replug to confirm the stale half.**
      Verified on the development machine, against real hardware: replug
      the keyboard, confirm `light health` reports it stale and names it, run
      the bounce, and confirm the keyboard takes a colour again -- the sequence
      that was done by hand on Sep 27.

## Alternatives Considered

- **Comparing node times to the server's start time.** Ruled out by the
  machine, as above: two healthy devices would have been called stale.
- **Reconnecting without restarting the server.** It does not fix this. The
  server's enumeration is the stale thing; re-dialling the same process gains
  nothing for a device that moved. It is necessary, and it is not sufficient,
  which is why R1 and R6 are both here.
- **Bouncing automatically on detection.** The state is detectable, so a
  program could act on it unasked. Rejected for the reason the systemd package
  was written the way it was: a restart drops every other device's lighting for
  a moment, and choosing that moment is the user's.
- **Restarting hotaru as part of the bounce**, which is what was done by hand.
  Unnecessary once R1 exists, and a service that restarts itself to recover
  from a dropped socket is a service that has not learned to reconnect.

## Risks & Assumptions

- **The descriptor check assumes the server keeps hidraw descriptors open.** On
  the development machine it holds one per HID device, and the GPU, which is
  I2C, has none -- so a device with no node is not evidence of anything. A
  controller that opens, writes, and closes would read as stale while working.
  AC3 has to be confirmed per device class on real hardware, and R4's naming
  is a refinement of R3, which rests only on the `(deleted)` marker.
- **`/proc/<pid>/fd` is readable for a user unit and not for a root system
  unit.** The Arch package ships a system unit, so the check is unavailable to
  the most common installation, which is what R5 is for. This is a diagnosis
  that improves on the machines it can see and regresses none.
- **A restart drops all lighting briefly**, including devices that were fine.
  R2 puts it back; R7 keeps the timing the user's choice.
- **The server does not stop on SIGTERM.** Observed on 28 Sep: systemd waited
  the full ten seconds and then killed it
  (`State 'stop-sigterm' timed out. Killing.`). So R6 is a ten-second
  operation, not an instant one, and it has to say so while it runs rather
  than look hung. The window in R8 needs the same, and neither should offer it
  as something that happens on a click and returns.
- **The unit may have a readiness gate of its own.** This machine's runs
  `openrgb-wait-for-devices.sh` before the server counts as started. R6 waits
  for the unit to be active *and* the port to answer, because the two are not
  the same moment, and re-dialling into the gap is what produced the
  "no OpenRGB server yet" line in the 28 Sep log.
- **Reading another process's `/proc` entry** is the one new privilege-adjacent
  thing here. It reads the symlink target of descriptors and nothing else, for
  a PID that came from `systemctl show`, never from a request.
- **Rollback** is a revert. Nothing is stored differently and no rules-file
  key is added.
