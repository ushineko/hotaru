# Spec 012: the cooler, without liquidctl

**Issue**: [#2](https://github.com/ushineko/hotaru/issues/2)

## Status: INCOMPLETE

## Context

peripheral-battery-monitor reaches the cooler by running `liquidctl`, once per
reading and once per screen write. That was the only option available to
Python: liquidctl is a program, not a library anyone can link, and its LCD
support needs PyUSB and Pillow besides.

hotaru is not Python, and the question was whether it has to inherit any of
that. It does not. Measured on the development machine's NZXT Kraken Elite V2
(`1e71:3012`, firmware 1.2.0):

| | liquidctl | native |
|---|---|---|
| one status reading | 105 ms | **2 ms** |
| a GIF onto the screen | 105 ms of startup, then the protocol | **40 ms, all protocol** |
| runtime dependencies | Python, liquidctl, PyUSB, Pillow | **none** |
| cgo | -- | **none** |

The 2 ms is the device's own USB interrupt interval. The 105 ms is a Python
interpreter starting.

### What the device actually is

Two USB interfaces, and they do not fight:

| interface | class | endpoint | carries |
|---|---|---|---|
| 0 | vendor specific | `0x02` OUT, bulk | LCD image data |
| 1 | HID | `0x81` IN / `0x01` OUT | status, modes, bucket control |

Interface 1 is bound by `usbhid` and reachable as `/dev/hidraw*`. Interface 0
has **no driver bound**, so it can be claimed through usbfs
(`/dev/bus/usb/BBB/DDD`) while hidraw stays open in the same process. Both
nodes carry an ACL granting the logged-in user read-write, so none of this
needs root or a new udev rule.

Status is a 64-byte report exchange: write `0x74 0x01`, read the reply, and the
numbers are at fixed offsets. Go's standard library covers it -- `os.OpenFile`,
`Write`, `Read` -- and `image/gif` covers what Pillow was there for.

### The LCD stores assets; it does not take a video feed

Putting an image on the screen means allocating a *bucket* in 24 KB of device
memory: query all sixteen, delete one, tell the device where the image will go,
stream it over the bulk endpoint, then point the screen at the bucket.

**A transfer the device fully accepts can display nothing.** Every HID step
returns success and the bulk write completes, and the screen keeps showing
whatever it showed before, because a freshly written bucket is not displayed
until `0x38 0x01 0x04 <index>` says so. This is the same shape as the lighting
defect in spec 009 and deserves the same suspicion: success from the device is
not evidence that anybody can see anything.

### Three bugs that looked like hardware findings

An earlier draft of this spec reported, as measured facts about the panel: that
bucket refusals were not rate-related, that they were memory exhaustion with
addresses climbing to a 24320-byte ceiling, that `reply[14]` values `0x04` and
`0x05` meant two kinds of bad placement, and that a dashboard at 1 Hz landed
one update in thirty.

**All of it was wrong, and every cause was in this prototype.**

**1. Replies were not matched to commands.** A command's reply carries the
command's prefix with the first byte incremented -- `0x32 0x01` is answered by
`0x33 0x01`. This cooler also streams status reports continuously, unasked. The
prototype sent a command and read whatever report arrived next, so it regularly
read a temperature reading and interpreted byte 14 of it as a result code.
liquidctl matches the prefix; the prototype only did so for deletes. Every
refusal measured before the fix was noise, including the entire rate ramp.

With replies matched: **30 updates at 1 Hz, 30 landed, 0 refused.**

**2. Reclaiming the previous bucket stops the panel updating.** Releasing the
bucket that was on screen once a new one is shown looked like obvious hygiene,
and was invented here rather than taken from liquidctl. With it on, the screen
holds its last image while every push reports success. With it off, every push
appears. liquidctl allocates and does not reclaim, and this is presumably why.

**3. The last packet was short.** The transfer is declared to the device in
whole 1024-byte packets and the payload rarely divides evenly. Sending only the
payload leaves the tail of the final packet holding whatever was in that memory,
which the panel draws as a band of noise along the bottom of the image. The
payload is now padded to the declared length.

### What is actually true

- A dashboard works. 1 Hz and 2 s intervals, every push landing, roughly 48 ms
  each, with the rendered numbers changing on every tick.
- A still image, an animated GIF, brightness, orientation and the return to the
  firmware readout all work.
- The panel's memory persists across host restarts, so a program that pushes
  images needs to be able to clear it rather than inherit what was left behind.
- A transfer the device fully accepts can still display nothing, because a
  bucket is not shown until it is selected. That one survived the rewrite: it
  is the reason bug 2 was invisible.

### How this went wrong, since it will happen again

peripheral-battery-monitor has driven a dashboard onto this exact panel for
months. Every time the prototype contradicted that, the contradiction was
evidence about the prototype, and it was read as a discovery about the
hardware -- three times, each producing a confident paragraph in a spec.

The rule earned in spec 010 was "no change to the write path is accepted on
hotaru's own telemetry: somebody looks at the machine". This adds the other
half: **when a working implementation disagrees with a new one, the new one is
wrong until proven otherwise.** Reproducing the old behaviour comes before
characterising the hardware.

### The path that would be right, and is not ours yet

The vendor's software animates this panel, and offers a webcam feed on it, so a
streaming path exists. liquidctl contains something shaped like it --
`_send_2023_data_fw2`, raw RGB565 with no bucket at all, start / stream / end --
gated to product `0x300E` on firmware 2.x.

Tried here anyway: it reaches the panel and produces visible motion at 19 fps
with no refusals across 120 frames, but a single frame never latches and the
result flickers and tears. It is a glimpse of the right mechanism through the
wrong door. Identifying the real one means capturing the vendor software's USB
traffic, which is a project of its own and is not this spec.

### Where this reasoning stops

This spec is not an argument that hotaru should speak to hardware directly
wherever it can. It is an argument about one device where there was nothing to
speak to.

**OpenRGB stays the lighting backend.** It is a server with a documented
protocol, maintained, covering hundreds of controllers that hotaru will never
own. Reimplementing it would mean adopting every one of those device protocols
and tracking them forever, in exchange for latency that is already sub-
millisecond. The integration point exists and is the right one.

liquidctl offers no such thing. It is a command-line program: there is no
library to link and no socket to open, so "using liquidctl" could only ever
mean starting a Python interpreter per call. The 105 ms was not liquidctl being
slow at its job; it was the absence of any way to ask it a question.

So the rule is about interfaces rather than about speed: **use the integration
point where one exists, and only write the protocol where none does.** A native
lighting backend is an interesting experiment for another day, and is
deliberately not this.

## Requirements

**R1. No process is spawned to reach the cooler.** Status and screen writes go
over `/dev/hidraw*` and usbfs directly. liquidctl stops being a dependency.

**R2. Temperatures come from the kernel.** CPU package from `coretemp`, the
board from `nct6798`, the PSU from `corsairpsu`, all under `/sys/class/hwmon`.
The GPU comes from `nvidia-smi` for want of an hwmon node. OpenLinkHub is not a
dependency of hotaru: everything the Python used it for is a file read.

**R3. Devices are found, not configured.** USB vendor and product ids identify
the cooler, and the hidraw node and usbfs path are derived from sysfs. No
hardcoded `/dev/hidraw7`, which is what the prototype does and is the first
thing that breaks on another machine, or on this one after a reboot.

**R4. One goroutine owns the device.** Both interfaces, one owner, a single
slot mailbox where a newer request replaces a waiting one -- the pattern
lighting already uses. The Python needed a priority queue because every call
was a separate process contending for one node; hotaru holds two handles and
does not.

**R5. A write is not finished when the device accepts it.** Where the screen is
concerned, the bucket must also be selected, and the result says which bucket
is being displayed rather than that a transfer completed.

**R6. Replies are matched to their commands.** A reply carries the command's
prefix with the first byte incremented, and the cooler streams unsolicited
status reports besides, so reading the next report is reading noise. Every
exchange matches, and a read that finds no matching reply is an error rather
than a value.

**R7. A declared transfer is delivered in full.** The payload is padded to the
packet count the device was given.

**R8. Buckets are not reclaimed while the panel is showing one.** Releasing the
previous bucket stops the screen updating while every write still reports
success. Memory is cleared wholesale when it needs clearing, not incrementally
underneath a live display.

**R9. Degradation is per capability.** No cooler, no telemetry and no screen.
No `nvidia-smi`, no GPU temperature and everything else unaffected. A cooler
with no screen has no screen; the panel's size comes from the device.

**R10. The screen can always be given back.** `0x38 0x01 0x02 0x00` returns it
to the firmware readout, and that is the recovery whenever hotaru cannot put
something sensible on it -- including on shutdown, so a machine that stops
running hotaru does not keep a stale dashboard.

## Acceptance Criteria

- [ ] AC1. Status is read over hidraw and matches `liquidctl --json status`
      for coolant temperature, pump speed and duty, fan speed and duty.
- [ ] AC2. A reading costs single-digit milliseconds, asserted by a test that
      fails if a process is spawned on the status path.
- [ ] AC3. The cooler is located by USB ids through sysfs; no device path is
      written down anywhere in the source or the rules file.
- [ ] AC4. The bulk interface is claimed while hidraw stays open, in one
      process, and released on shutdown.
- [ ] AC5. A single-frame GIF reaches the screen and is displayed.
- [ ] AC6. An animated GIF reaches the screen and plays.
- [ ] AC7. Brightness and orientation are settable.
- [ ] AC8. The screen returns to the firmware readout on request and on
      service shutdown.
- [ ] AC9. Every command's reply is matched by prefix, and a test feeds an
      interleaved status report through the fake to prove a mismatched reply is
      never read as a result.
- [ ] AC10. The bytes delivered equal the packet count declared, with a test.
- [ ] AC11. A dashboard pushed once a second lands every update, verified on
      the machine with a value that changes on every tick.
- [ ] AC12. CPU, board and PSU temperatures are read from hwmon by label
      rather than by hwmon index, which is not stable across boots.
- [ ] AC13. Every capability degrades alone: unplugging the cooler leaves
      lighting, telemetry and the API working.
- [ ] AC14. The fake cooler models the device's unsolicited status reports, so
      reply matching is exercised without hardware.
- [ ] AC15. Verified on the development machine with somebody watching the
      screen: status, a still image, an animation, brightness, orientation,
      and the return to the firmware readout.

## Risks & Assumptions

- **We own the protocol now.** liquidctl's bugs stop being inherited and start
  being ours, including whatever liquidctl#774 turns out to be. That is the
  price of the 50x, and it is also the only way the bucket behaviour above
  could have been measured at all.
- **One cooler, one firmware.** Everything here was learned from an Elite V2 on
  1.2.0. The Kraken family differs by product id, and hotaru should decline to
  drive a cooler it does not recognise rather than guess.
- **Writing to a device nobody else is writing to.** The Python's queue existed
  partly because the OpenRGB server also holds a handle to this cooler -- for
  its lighting, on a *different* interface and a different hidraw node
  (`hidraw14` against `hidraw7` here). They do not collide, and this should be
  re-checked rather than assumed on other hardware.
- **Rollback** is a revert to shelling out, which is why R1 is a requirement
  about behaviour and not a rewrite of the interface the rest of hotaru sees.

## Alternatives Considered

Considered keeping liquidctl for the LCD and going native only for status;
rejected because the LCD path is where the 105 ms hurts most and because two
backends for one device is how the Python ended up with a colour path that
could not work (its spec 023 removed it).

Considered the RGB565 streaming path as the primary mechanism; rejected for now
because it does not latch a single frame on this firmware, and a dashboard that
must re-push forever to stay visible is worse than one that occasionally has to
retry an install.
