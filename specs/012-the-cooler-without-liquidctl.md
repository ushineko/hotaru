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

### What repeated writes really do

peripheral-battery-monitor's specs 021 and 022 are built on "write as rarely as
possible", to avoid the bucket-switch failures of liquidctl#774. Its spec 028
then found that the screen needs *regular* writes or the image expires, and
recorded the tension as unresolved.

Measured here, by pushing at falling intervals from 2 s to flat out:

| interval | pushes | refusals |
|---|---|---|
| 2 s | 4 | 2 |
| 1 s | 5 | 4 |
| 500 ms | 8 | 2 |
| 250 ms | 10 | 2 |
| 100 ms | 15 | 3 |
| flat out | 20 | **0** |

**Rate is not the variable.** The refusals carry `reply[14] = 0x05` and the
address in each one climbs: 2620, 2625, 2637, 2643, and later 23428 against a
24320-byte ceiling. It is memory exhaustion. liquidctl allocates each image
after the last one and never reclaims, so a program that pushes repeatedly
walks to the end of the device's memory and stays there.

Two things were tried against it:

- **Reclaiming the previous bucket** after switching away from it: refusals
  halved, 12 to 6. Not a fix, because placement still climbs to `maxOccupied`.
- **Choosing the addresses ourselves**, two fixed slots alternating: every
  setup refused, with a *different* code, `reply[14] = 0x04`. The allocator
  belongs to the device. The bucket query is not a handshake to be skipped --
  it is how the device says where a write may go. A build that skipped it
  failed 100% of the time.

So `0x05` is "no room" and `0x04` is "not yours to place", and the honest
reading is that this protocol is meant for storing a handful of images, not for
refreshing one.

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

**R6. Bucket memory is managed, and its exhaustion is reported honestly.** The
previously displayed bucket is released once the new one is shown. When the
device still refuses with `0x05`, hotaru clears all sixteen buckets and retries
once; if that fails, it says the screen could not be updated rather than
reporting success.

**R7. Degradation is per capability.** No cooler, no telemetry and no screen.
No `nvidia-smi`, no GPU temperature and everything else unaffected. A cooler
with no screen has no screen; the panel's size comes from the device.

**R8. The screen can always be given back.** `0x38 0x01 0x02 0x00` returns it
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
- [ ] AC9. A bucket setup refused with `0x05` triggers a full clear and one
      retry, and a second failure is reported as a failure.
- [ ] AC10. The previously displayed bucket is released after a successful
      switch, and a test says so.
- [ ] AC11. CPU, board and PSU temperatures are read from hwmon by label
      rather than by hwmon index, which is not stable across boots.
- [ ] AC12. Every capability degrades alone: unplugging the cooler leaves
      lighting, telemetry and the API working.
- [ ] AC13. The fake cooler used in tests models a refused bucket setup, so
      the retry path is exercised without hardware.
- [ ] AC14. Verified on the development machine with somebody watching the
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
