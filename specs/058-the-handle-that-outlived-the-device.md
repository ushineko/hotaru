# Spec 058: the handle that outlived the device

**Issue**: [#167](https://github.com/ushineko/hotaru/issues/167)

## Status: COMPLETE

## Executive Summary

A device unplugged and plugged back in left the OpenRGB server writing to a
connection that no longer existed: the device stayed in the listing, health
read `6 of 6 devices are in scope`, and nothing lit. The service now notices
the server going away and reconnects on its own, the clients notice a dead
connection and name the device holding it, and `hotaru light rescan` -- in the
window as "Look for replugged devices" -- restarts the server and puts the
lights back.

Look first at `internal/stale`, and at why the check lives in the clients
rather than in the service: the service's sandbox makes reading another
process's descriptors impossible, and relaxing it would have cost six hardening
options including one this repository records catching a real bug. Second at
`Conn.Gone` and the supervision loop in `internal/daemon/serve.go`, which is
what makes any OpenRGB restart survivable. Three things here were found by
running the code against the hardware and could not have been found otherwise;
each is written up where it was found.

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

### The service cannot do the looking

Found with a keyboard actually replugged, on 29 Sep, with everything above
written and passing. `light health` said `healthy: 6 of 6` while the server
held `fd 31 -> /dev/hidraw17 (deleted)`.

Reading another process's descriptors has to be done from that process's mount
namespace. The service runs in its own, and every systemd option that remounts
anything creates one -- measured here one at a time, each sufficient on its
own:

```
baseline, NoNewPrivileges, SystemCallFilter, RestrictNamespaces   deleted=true
PrivateTmp, ProtectSystem, ProtectHome, ProtectControlGroups,
ProtectKernelTunables, ProtectKernelModules                       deleted=false
```

The first version reported `known=true` from in there, because the directory
listed all forty-two entries and only the readlinks were refused. So it
announced that it had looked and found nothing wrong, which is the worst answer
available. Listing is not reading, and a refused link is now not known.

**The clients do the looking instead.** Both are ordinary processes of the
user's, in the user's namespace. Relaxing the unit was the alternative and was
rejected: the six options that would have to go include the one whose comment
in that unit records it catching a real bug (spec 016), and this state is a
convenience rather than a repair -- the bounce fixes the machine, and the
bounce works from inside the sandbox untouched.

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

**R5. Where it cannot look, it says nothing.** No PID, no unit, a PID that is
not the server any more, or a descriptor it is refused: each degrades to the
advice that is there now and never to a guess. Being refused every link is
**not** "looked, found nothing" -- that distinction is the whole of this
requirement, and getting it wrong is what made the first version claim an
all-clear it could not back.

**R6. One command does the bounce**: restart the server's unit, wait for it to
listen, re-dial, reconcile, and report what it did.

**R7. It runs only when asked, and it asks the machine before giving up.**
Detecting a stale handle names the command; it does not run it. A user unit is
this user's own. A system unit is root's, and is bounced too where this user
already holds that command without a password -- `sudo -n`, which does the real
thing minus the prompt and fails instantly rather than waiting. Only where root
would actually ask does it hand back the command, with what to change if they
would rather not do it by hand. Nothing here ever prompts.

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
- [x] AC7. A user unit, a system unit with passwordless root, and a system
      unit without it are told apart, and only the last is advice
      (`TestHowAUnitGetsRestartedDependsOnWhoseItIs`,
      `TestAUnitItCannotRestartIsAdvice`). The bounce can never prompt
      (`TestABounceNeverAsksForAPassword`).
- [x] AC8. `light health`, the API, and the window all offer it, and the
      window carries the note (`TestRescanIsOfferedEverywhereHealthIs`).
- [x] AC9. Verified on the development machine, against real hardware, on
      29 Sep. The keyboard replugged and the server left holding
      `fd 31 -> /dev/hidraw17 (deleted)`:

      ```
      $ hotaru light health
      stale: the OpenRGB server is still addressing Keychron K4 HE at a
      connection that has gone. ...
        Have the server look again with `hotaru light rescan`. ...
        127.0.0.1:6742, protocol 3, 6 devices, 6 in scope

      $ hotaru light rescan
      the server found 6 devices; restored 6          (17 seconds)

      $ hotaru light health
      healthy: 6 of 6 devices are in scope
      ```

      The device was picked out of six the server listed and eleven the kernel
      had. `6 devices, 6 in scope` on the stale line is the listing that read
      as healthy all evening.

## Alternatives Considered

- **Comparing node times to the server's start time.** Ruled out by the
  machine, as above: two healthy devices would have been called stale.
- **Reconnecting without restarting the server.** It does not fix this. The
  server's enumeration is the stale thing; re-dialling the same process gains
  nothing for a device that moved. It is necessary, and it is not sufficient,
  which is why R1 and R6 are both here.
- **Relaxing the service's sandbox** so it could read the descriptors itself,
  keeping all of health in one place. Six options would have to go, including
  `ProtectHome=read-only`, whose comment in that unit records it catching the
  attempt to edit the desktop's own shortcut file (spec 016). A guardrail that
  has already caught a real bug is worth more than being told about a fault
  whose repair works without it.
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
- **Health is now decided in two places**, and that is the price of keeping the
  unit hardened. Four states are the service's, reached from the connection it
  holds; this one is the clients', reached by looking at the machine. A reader
  expecting one answer from one place will not find it, which is why
  `internal/stale` opens by saying so.
- **Detection is still unavailable where the server runs as root.** A system
  unit's descriptors belong to root, and no client of the user's can read them.
  The bounce is not bound by the same limit -- a machine granting this user
  passwordless root over the command is bounced normally -- so on a packaged
  install the button works and the automatic noticing goes quiet. This improves
  the machines it can see and regresses none.
- **Both clients now spawn subprocesses to answer health.** Finding the PID is
  several `systemctl` calls, so it is found once and reused while it stays an
  OpenRGB server; the repeated part is a `/proc` read. The window asks every
  two seconds and must not be running `systemctl` at that rate.
- **Running `sudo` at all is new for this program.** Only ever `sudo -n`, only
  ever to restart a unit this package chose from its own candidate list, never
  through a shell, and never in a way that can block. Whether it is permitted
  at all was decided by whoever configured the machine, before hotaru ran.
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
