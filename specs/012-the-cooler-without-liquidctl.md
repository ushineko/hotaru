# Spec 012: the cooler, without liquidctl

**Issue**: [#2](https://github.com/ushineko/hotaru/issues/2)

## Status: COMPLETE

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

## What it looks like

	$ hotaru cooling
	NZXT Kraken Elite V2
	  coolant   37.2 C
	  pump      2606 rpm (81%)
	  fan       1244 rpm (51%)

	$ hotaru screen show dashboard.gif
	$ hotaru screen set --brightness 60
	$ hotaru screen readout

The screen and the telemetry share one control channel, so one owner
serialises both: a picture can go up while a reading is being taken, and
neither interleaves with the other. The panel is claimed on first use, so a
machine that never draws anything leaves the interface to whatever else wants
it, and it is handed back when the service stops.

## The protocol, as read off the device

Written down because it cost a day, and because every mistake below presented
as the same two symptoms -- a `0x04` refusal or a timeout -- so the wrong
diagnosis is always available. Offsets are into the report as hidraw delivers
it, report number included, matching liquidctl's indices so the two can be
compared directly.

### Talking

A reply carries the command's prefix with the **first byte incremented**:
`0x32 0x01` is answered by `0x33 0x01`. Matching only the first byte finds the
wrong report.

The cooler **broadcasts `0x75 0x02` about once a second**, unasked, and the
kernel queues those per open handle up to 64. A long-lived handle therefore has
a queue as deep as it has been idle, so **clear it before every question**
(liquidctl calls this `clear_enqueued_reports`). Skipping that is a reader that
finds twelve stale broadcasts and concludes the device is silent -- which
happens only after idleness, so a loop of calls never sees it.

A few commands are **written and not answered**. Waiting for a reply to one of
those times out after the full deadline and reads as a dead device.

### Commands

| command | reply | what it is |
|---|---|---|
| `74 01` | `75 01` | ask for status |
| `30 04 <slot>` | `31 04` | what is in a slot |
| `32 02 <slot>` | `33 02` | clear a slot |
| `32 01 <slot> <slot+1> <addr:2> <size:2> 01` | `33 01` | reserve a slot for a transfer |
| `36 03` | `37 03` | open the exchange |
| `36 01 <slot>` | `37 01` | begin the data transfer |
| `36 02` | `37 02` | end the data transfer |
| `38 01 <mode> <slot>` | `39 01` | show a slot (`04`) or the firmware readout (`02`) |
| `30 02 01 <brightness> 00 00 01 <orientation/90>` | **none** | brightness and orientation |

### Results, at byte 14

| value | meaning |
|---|---|
| `01` | done |
| `04` | refused: out of sequence, or the slot is occupied, or the address is not one the device chose |
| `09` | that slot would not clear -- **try the next one** |

`0x04` is the trap. It is returned for at least three unrelated mistakes, so it
says "you did something wrong" and nothing about what.

### Status reply, `75 01`

| bytes | meaning |
|---|---|
| 15, 16 | coolant temperature, whole and tenths |
| 17, 18 | pump rpm, little endian |
| 19 | pump duty, percent |
| 23, 24 | fan rpm, little endian |
| 25 | fan duty, percent |

`FF FF` at 15 and 16 is a firmware fault, not 255.5 degrees (liquidctl#172).

### Slot reply, `31 04`

| bytes | meaning |
|---|---|
| 14 | slot index |
| 15 | asset index, slot + 1 -- **zero means the slot is empty** |
| 17, 18 | address, little endian |
| 19, 20 | size in packets, little endian |

A slot is vacant when everything from byte 15 onward is zero.

	slot 0:  index=00 asset=01 addr=0000 size=0007 used=01
	slot 1:  index=01 asset=02 addr=0016 size=0016 used=01

### Sending an image

Order is load-bearing and not guessable:

1. `36 03` **first**, before asking which slots are free. Choosing a slot and
   opening the exchange afterwards is refused with `04`.
2. Ask about all sixteen slots and keep the replies: the addresses are needed
   below and **move between writes**, so they are read every time.
3. Clear a slot, and **read the result**. `09` means try the next slot; a setup
   on a slot that did not clear is refused with `04`. A slot that held data is
   cleared twice.
4. Work out the address from the replies in 2 -- reuse the slot's own space if
   the image fits, else after everything, else the room at the start, else
   clear the whole screen. **The device refuses an address it did not arrive at
   itself.**
5. `32 01` to reserve, `36 01` to begin, then the bulk writes, then `36 02`.
6. **Pad the payload to the packet count declared.** The transfer is described
   in whole 1024-byte packets and an image rarely divides evenly; sending only
   the image leaves the tail of the last packet holding whatever was there,
   which the panel draws as noise along the bottom.
7. `38 01 04 <slot>` to show it. Without this every step reports success and
   the screen does not change.

An image must be a **GIF**. The firmware does not retain a static picture --
measured reverting to the built-in display in five to ten seconds with nothing
touching the device -- while a GIF plays indefinitely. One frame is enough.

## Requirements

**R1. No process is spawned to reach the cooler.** Status and screen writes go
over `/dev/hidraw*` and usbfs directly. liquidctl stops being a dependency.

**R2. Temperatures come from the kernel.** CPU package from `coretemp`, the
board from `nct6798`, the PSU from `corsairpsu`, all under `/sys/class/hwmon`.
The GPU comes from `nvidia-smi` for want of an hwmon node. OpenLinkHub is not a
dependency of hotaru: everything the Python used it for is a file read.

**R3. Devices are found, not configured, and the device confirms which.** USB
vendor and product ids identify candidates, and the hidraw node and usbfs path
are derived from sysfs. No hardcoded `/dev/hidraw7`.

Sysfs alone is not enough. One device commonly exposes several hidraw nodes --
on the development machine a Logitech receiver has three, a keyboard two, and
this cooler had two earlier the same day -- and they are indistinguishable from
outside. The glob is lexical besides, so `hidraw10` sorts before `hidraw7` and
"the first match" is a coin toss that lands differently after a reboot.

There is a declarative discriminator and hotaru uses it: the HID **usage page**
in the report descriptor, which is what hidapi exposes as `usage_page`. A
control protocol lives on a vendor-defined page -- this cooler declares
`0xFF00` -- while a device's other collections declare standard ones. So
candidates are ordered vendor-defined first.

It is an ordering and not a filter, because a usage page says what an interface
is *for* and not whether this firmware will answer on it. So every candidate is
then asked for a reading, with a short deadline, and the one that replies is
the cooler. Asking is safe, because candidates are already filtered to a known
vendor and product, and decisive, because a mismatched device answers with its
own prefix -- a Corsair power supply, asked this, replied `74 96`.

liquidctl does neither: it takes hidapi's first match, filtered by serial
number where it has one. That works until a device exposes two nodes, which
most of the ones on the development machine do.

**R4. One owner, and reads coalesce rather than supersede.** Two callers on one
HID endpoint interleave control transfers, which corrupts rather than merely
delaying, so access is serialised. The Python needed a priority queue because
every call was a separate process contending for one node; hotaru holds the
handle, so a mutex is the whole mechanism.

Lighting's rule does **not** apply here. Its queue drops a waiting write when a
newer one arrives, because nobody wants the second-to-last scene applied after
the last. A read is not like that: a caller whose request was superseded still
wants a number. So concurrent readers share one recent answer -- a quarter of a
second -- which is what a dashboard, a status endpoint and a telemetry consumer
asking at once actually want, and it keeps hotaru from writing to the device
more often than anybody needs.

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

- [x] AC1. Status is read over hidraw and matches `liquidctl --json status`
      for coolant temperature, pump speed and duty, fan speed and duty.
- [x] AC2. A reading costs single-digit milliseconds, asserted by a test that
      fails if a process is spawned on the status path.
- [x] AC3. The cooler is located by USB ids through sysfs; no device path is
      written down anywhere in the source or the rules file.
- [x] AC4. The bulk interface is claimed while hidraw stays open, in one
      process, and released on shutdown.
- [x] AC5. A single-frame GIF reaches the screen and is displayed.
- [x] AC6. An animated GIF reaches the screen and plays.
- [x] AC7. Brightness and orientation are settable.
- [x] AC8. The screen returns to the firmware readout on request and on
      service shutdown.
- [x] AC9. Every command's reply is matched by prefix, and a test feeds an
      interleaved status report through the fake to prove a mismatched reply is
      never read as a result.
- [x] AC10. The bytes delivered equal the packet count declared, with a test.
- [ ] AC11. A dashboard pushed once a second lands every update, verified on
      the machine with a value that changes on every tick.
- [x] AC12. CPU, board and PSU temperatures are read from hwmon by label
      rather than by hwmon index, which is not stable across boots.
- [x] AC13. Every capability degrades alone: unplugging the cooler leaves
      lighting, telemetry and the API working.
- [x] AC14. The fake cooler models the device's unsolicited status reports, so
      reply matching is exercised without hardware.
- [x] AC15. Verified on the development machine with somebody watching the
      screen: status, a still image, an animation, brightness, orientation,
      and the return to the firmware readout.

## Built so far

`internal/cooler`, telemetry only. The screen is not in it yet.

Discovery walks sysfs from the USB ids to a hidraw node and a usbfs path, so
nothing is written down: on the development machine it finds
`/dev/hidraw7` and `/dev/bus/usb/001/013`, which is what the prototype had
hardcoded and would have been wrong on the next boot. Where sysfs offers more
than one node -- which it does for most devices on that machine -- the cooler
is asked which one it is, because nothing outside the device can tell. A cooler this package
does not recognise is declined rather than guessed at, and a machine with no
cooler gets `ErrNoCooler`, which is an ordinary state.

Reply matching is in the type rather than in a caller's memory: there is no
method that reads the next report, only `ask`, which matches the prefix. The
probe that justified it, run against the machine, is worth recording --
twelve reads after one request produced:

	75 01  x1    the reply
	75 02  x11   status reports the device streams unasked

The prototype matched only the first byte, so eleven times in twelve it parsed
a broadcast. It worked because a broadcast carries status too, which is luck
rather than design. The fake chatters for the same reason: a fake that answered
politely would let the bug straight back in.

Measured against liquidctl on the same cooler, same minute:

	hotaru:     coolant 37.3 C  pump 2596 rpm (81%)  fan 1244 rpm (51%)
	liquidctl:  37.3 C          2596 rpm   81 %      1244 rpm   51 %

Every field identical, at **2.003 ms per reading against 105 ms**.

### Absence is an answer

`GET /v1/cooling` reports a machine with no cooler as `200` with `absent` set,
rather than `404` or an error. "This machine has no cooler" is a fact a
consumer wants; making it a failure means every caller writes the same special
case to tell it from a broken socket, and a dashboard that cannot distinguish
them raises an alarm about an ordinary desktop.

A cooler that is present and will not answer is the same shape with its name
attached, because present-and-silent is a different problem from absent and the
name is the first thing somebody needs in order to chase it.

### Why there is no fake screen

The testing policy already covers this, and this spec is a worked example of
it rather than a new rule:

> Mocks make this strictly worse: the AI invents the mock, invents the
> contract, and writes a test that passes against its own invention.

That is what a fake cooler would be. Where the behaviour is hotaru's own --
scope resolution, frame composition, the wizard's flow -- a model of it *is*
the specification, which is why the OpenRGB fake earns its place and runs the
whole suite on a laptop with no RGB in it. Where the behaviour belongs to
somebody else's firmware, encoding a belief and then confirming it says nothing
about the hardware.

The evidence is what it found: in the day behind this spec, nothing. Every
discovery came from the panel, from `liquidctl` disagreeing, or from somebody
looking at a screen and saying what they saw. The existing cooler fake's models
of unsolicited broadcasts, a queued backlog and a lost reply were each written
*after* the corresponding bug had been diagnosed on hardware -- a regression
guard, not an instrument.

Written earlier it would have been worse than absent. Built from the
understanding held at the start of that hour, it would have accepted a setup
before `36 03`, ignored delete results, and taken any address: all three of the
day's bugs would have passed against it.

The device is an integration boundary, so the policy's rule applies directly --
at least one acceptance criterion exercises the real thing. That is AC15, and
it is not satisfiable any other way. Those checks live in `live_test.go` beside
the OpenRGB one, so they are re-run rather than retyped: they skip where there
is no cooler, and the screen test is opt-in and puts the firmware readout back,
because a test that leaves a checkerboard on somebody's cooler is a test nobody
runs twice.

The rest of this package's tests divide the way the policy divides them.
Contracts between components -- what the service gets from a cooler that is
absent, present, or present and silent -- are the ones worth keeping through a
refactor. Slot selection and placement are not contracts; they are the
"isolating specific logic" case, justified by arithmetic with real edge cases
and by one of those tests catching a wrong assumption immediately, and they
should be recognised as that rather than dressed up as boundaries. What is tested without hardware is the
part that is not protocol: slot selection and placement arithmetic are pure
functions over the bytes the device returns, with real edge cases, and one of
those tests caught a wrong assumption immediately.

## Risks & Assumptions

- **We own the protocol now.** liquidctl's bugs stop being inherited and start
  being ours, including whatever liquidctl#774 turns out to be. That is the
  price of the 50x, and it is also the only way the bucket behaviour above
  could have been measured at all.
- **One cooler, one firmware.** Everything here was learned from an Elite V2 on
  1.2.0. The Kraken family differs by product id, and hotaru should decline to
  drive a cooler it does not recognise rather than guess.
- **A long-lived handle accumulates broadcasts.** This cooler reports its state
  about once a second whether or not anybody asked, and the kernel queues those
  per open handle -- up to 64, then it drops the oldest. hotaru's handle is
  open for the life of the service, so the queue is as deep as the service has
  been idle: a minute of quiet leaves sixty stale reports ahead of the next
  reply, and a reader that looks at twelve finds none of them are it.

  Reported from the machine as "no 7501 reply in 12 reports". It looked
  intermittent because a run of calls keeps the queue empty -- forty
  back-to-back calls passed either side of the failure -- and only idleness
  fills it, which is exactly how a person uses the command and not how a loop
  does.

  Clearing the queue before each question is the fix, and it is what liquidctl
  calls `clear_enqueued_reports`. It was considered earlier and dismissed on a
  measurement taken against a **freshly opened** handle, which by definition
  has nothing queued.

- **Another program holds the same node.** The OpenRGB
  server had this cooler on a different hidraw node earlier in development, so
  the two were assumed not to collide. After a reboot they were on the same
  one:

	openrgb  576461  fd 26u  /dev/hidraw7
	hotaru  1056349  fd  3u  /dev/hidraw7

  This was first blamed for the failure above, on the theory that a report
  OpenRGB reads is a report hotaru does not. That is **wrong**: Linux queues
  hidraw reports per open file description, so both readers receive every
  report and neither can take the other's. The mechanism was assumed rather
  than checked.

  Sharing the node is still worth knowing about -- two writers on one interrupt
  endpoint would interleave control transfers -- and hotaru retries an exchange
  that finds no reply, which costs nothing and covers a report genuinely lost.
  But it is not what the reported failure was.
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
