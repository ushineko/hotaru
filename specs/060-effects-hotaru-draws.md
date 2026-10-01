# Spec 060: effects hotaru draws

**Issue**: [#172](https://github.com/ushineko/hotaru/issues/172)

## Status: INCOMPLETE

## Context

Every effect hotaru offers today is a firmware mode: a scene names "Breathing"
or "Solid Splash", and `writeFrame` switches the device into it through
OpenRGB. That works because almost every lit device carries its own effects.

The SteelSeries Apex Pro TKL Wireless Gen 3 does not. Through OpenRGB it
offers two modes, Direct and Onboard (spec 014 recorded this). sanshoku's spec
010 measured why. No command on the board changes its effect, and a usbmon
capture of SteelSeries GG showed GG drawing every effect and preset on the PC.
GG streamed the result as frames, about 18 a second, each acknowledged by the
keyboard. The board holds the last frame it was sent until it is rebooted.
sanshoku v0.1.5 carries that stream as `lighting.Canvas`:

```go
type Canvas interface {
    Keys() []Key                                 // the lights, in the device's order
    Frame(ctx context.Context, px []Pixel) error // one frame, acknowledged
    Release(ctx context.Context) error           // hand back to the firmware (the Apex reboots)
    Floor() time.Duration                        // shortest interval that shows
}
```

So hotaru can give the Apex a colour and nothing more, and the Effects field
of `scenes.Scene` already anticipates the gap (scene.go:64-79): "Effects
hotaru renders itself ... are their own spec." This is that spec.

It is written for any device that offers a Canvas, not for the Apex. The
Apex is the first such device; it will not be the last. On njv-cachyos the
NZXT Kraken Elite V2 is next for a capture of NZXT CAM in the same vein as
sanshoku 010's GG capture. Anything that turns out to be frame-streamed there
becomes a sanshoku canvas, and this spec's code drives it unchanged. Nothing
in hotaru's renderer, its device model or its tests names a product.

There is also a constraint that comes from the reason these programs exist.
On Windows every vendor ships its own always-on service with its own effect
engine, and together they cost a steady share of the CPU. hotaru drawing
frames must not become that, so its cost is a criterion here, measured, not
hoped for.

## Requirements

### R1. Canvas devices beside OpenRGB devices

- R1.1 hotaru depends on sanshoku v0.1.5 and finds canvas devices with
  `sanshoku.Scan` over the drivers that can produce one (today `steelseries`),
  keeping each candidate that satisfies `lighting.Canvas`. The scan runs at
  service start and again on the existing rescan.
- R1.2 A canvas device appears in hotaru's device list as a `devices.Device`.
  Its name is the kernel's, through sanshoku. It has one zone, "Keys", with
  one LED per `Keys()` entry, named by `Key.Name`. Its modes are hotaru's
  renderers (R2), not firmware modes. Scenes, assignments, targets, named
  segments, the mapping wizard and `light list` treat it as any other device.
- R1.3 The same physical device is also an OpenRGB device. While a canvas is
  attached, hotaru does not write to its OpenRGB twin: no `SetMode`, no
  `SetFrame`, no restore, no reassert. The twin is matched by OpenRGB's
  location carrying the same hidraw path as sanshoku's `Identity.Path`. If a
  device's location does not carry it, a config key names the twin
  explicitly. The twin stays visible in `light list` marked as handed to
  the canvas, so the user can see why it is not written.
  *Amended after measuring (2026-10-01, cachyos):* OpenRGB does report the
  path (`HID: /dev/hidraw5`), but it is the node OpenRGB found when it
  started, and it is not updated when the board re-enumerates. The board's
  0xFFC0 node was `/dev/hidraw4` (input3) at the time and `/dev/hidraw5` no
  longer existed. So the location match is kept, compared as a whole path,
  and the config key (`twin` on the canvas device's rule, a substring of
  OpenRGB's name) is the dependable path. A rule that names a twin decides
  alone; the location is not consulted for that canvas.
- R1.4 A canvas device is attached and detached like the cooler (spec 059):
  `attach` with the 1/2/5/15/30 s backoff, an adapter whose `check` closes the
  device once and signals `Gone()` on `sanshoku.ErrGone`, and a new
  attachment re-applies the desired state. OpenRGB stopping reboots the Apex
  (sanshoku `docs/contention.md`), so this path is ordinary, not exceptional.
- R1.5 `packaging/60-hotaru.rules` gains the SteelSeries hidraw rule from
  sanshoku's `docs/udev.md`. A permission error names it, as the cooler's does.

### R2. The renderer

- R2.1 A new package, `internal/render`, holds the effects. An effect is a
  pure function of time and the device's keys, returning one frame:
  `func(t time.Duration, keys []lighting.Key, p Params) []lighting.Pixel`.
  `Params` carries the colours and the speed a scene's `Effect` already has
  (`Colour`, `Speed`). No I/O, no goroutines, no clock in the package.
- R2.2 The first effects are a deliberate short list: **Static** (one colour
  or the scene's assignments), **Breathing** (the colour scaled by a sine),
  **Spectrum** (every key one hue, cycling), and **Rainbow Wave** (hue by key
  position, moving). Names match OpenRGB's where they mean the same, so a
  scene that says `Breathing` means the same thing on every device. Effects
  that need input (reactive, ripple on key press) are out of scope: they read
  the keyboard's events, which is a permissions and privacy question of its
  own.
- R2.3 Key position for Wave comes from the key's name on a standard US
  (ANSI) layout where the name is one the layout knows, and from the canvas's
  key order otherwise. The wizard is not extended here. (Amended in review:
  the first canvas lists A to Z and then the digits, so key order alone
  waved across the alphabet, not the board.)
- R2.4 Brightness is a multiply in the renderer, as it is in GG. The device
  rule's `Brightness` applies to canvas devices that way.

### R3. The animator

- R3.1 One animator per attached canvas, owned by the daemon, started and
  stopped with the attachment. It is the only writer to its canvas.
- R3.2 **A static scene sends one frame and then nothing.** The board holds
  its last frame, so Static, a solid colour, an image scene and Off are one
  `Frame` each and no ticker runs.
- R3.3 An animated effect ticks at the scene's interval, default **56 ms**
  (GG's median in the capture), never shorter than `Floor()`, and a config
  key may set it per device. The default is a measured choice, not the floor:
  the floor is 16 ms and the default is the vendor's own pace.
- R3.4 A frame identical to the last one sent is not sent, as
  `dashboard.Pusher` skips identical panel frames.
- R3.5 A frame that is not acknowledged (`hidraw.ErrSilent`) is logged once
  per run and the stream continues; `ErrGone` ends the attachment (R1.4).
- R3.6 Previews, leases and reconcile apply to canvas devices as to others.
  Reconcile sends the recorded frame once; reassert is a no-op for a canvas,
  because the board does not drift. A full restore redraws each canvas once
  (amended after the live run: behind a receiver a reboot is invisible).

### R4. Stopping

- R4.1 When the service stops, or a scene leaves a canvas device without an
  effect, the animator stops and the board keeps its last frame. hotaru
  never calls `Release` on its own, because on the Apex it reboots the board.
- R4.2 `hotaru light release <device>` and a matching GUI action call
  `Release`, for a user who wants the firmware's own lighting back. The
  output says the device will re-enumerate.

### R5. Parity and the interface

- R5.1 The effect picker in the GUI and `scene` in the CLI offer a canvas
  device's renderers as its modes. A new API route for R4.2 is reachable from
  both shells, and `parity_test.go` covers it.
- R5.2 `light list` and `light health` show a canvas device's frame rate
  while an effect runs and "holding" while it does not.

### R6. Cost

- R6.1 The cost of drawing is measured on cachyos with the Apex through its
  receiver and recorded in Verification: the service's CPU (from
  `/proc/<pid>/stat` over 10 minutes) for no scene, a static scene, and
  Rainbow Wave at the default interval and at the floor.
- R6.2 The battery cost is recorded the same way: the Apex's level, read
  through hayami or `sanshoku-bench read`, over two hours of Rainbow Wave
  through the receiver beside two hours of a static scene. The reading moves
  in steps of five, so this is an order of magnitude, written as one.

### R7. Documentation

- R7.1 docs/architecture.md: canvas devices, the twin rule and the animator.
  docs/hardware.md: the Apex as a canvas device and what Release does.
- R7.2 README: the changelog under `### Unreleased` with spec 060 and #172;
  "Where it comes from" names sanshoku's lighting capability.

## Acceptance Criteria

- [x] `make test`, `make lint` and the style and README canaries pass; `make
      test` opens no device.
- [x] `internal/render` has unit tests per effect as pure functions (a static
      effect gives the same frame at every t; Breathing at t=0 and at half
      its period; Wave moves by one key per step at a known speed;
      brightness scales every channel), and none of them names a product.
- [x] The animator is tested against a fake canvas carrying only measured
      behaviours (acknowledges, holds the last frame, `ErrSilent`, `ErrGone`):
      a static scene sends exactly one frame; an animated one ticks at the
      configured interval and never under the floor; an identical frame is
      skipped; `ErrGone` ends the attachment and a new one re-applies.
- [x] A canvas device's OpenRGB twin receives no write while the canvas is
      attached, tested against `openrgb.Fake` through `Apply`, `Reconcile`
      and reassert.
- [ ] **Live, on cachyos, with someone watching the Apex:** `hotaru scene
      apply` of a scene with Breathing and then Rainbow Wave on the Apex
      shows both. A static scene shows and the stream stops (frames counted
      on the canvas: one). Restarting OpenRGB reboots the board and hotaru
      reattaches and restores the scene within the backoff. This is the
      integration-boundary criterion: a real keyboard through sanshoku.
- [ ] R6.1 and R6.2 are measured and in Verification. A static scene costs no
      CPU above the no-scene baseline, and the stream's CPU at the default
      interval is reported as a number.
- [ ] `hotaru light release` hands the Apex back to its onboard effect and
      hotaru reattaches afterwards without writing to it until a scene asks.
- [ ] Every other lit device behaves as before (`light list`, a scene apply
      and a reconcile on cachyos and njv-cachyos).
- [x] Docs per R7.

## Risks & Assumptions

- **The twin match.** Assumes OpenRGB reports the hidraw path in the
  device's location (hidapi's path on Linux); `light list --json` does not
  show location today, so this is checked first. The config key in R1.3 is
  the fallback, and the spec is amended to whichever was needed.
  *Measured:* the location carries a path and it goes stale (R1.3). On
  cachyos the user's rule needs `twin: Apex Pro TKL Gen 3 Wireless` on the
  rule matching `Apex Pro TKL Wireless Gen 3` until OpenRGB restarts and
  records the current node. A stale location can also name a node the
  kernel has since given to another device, which is one more reason the
  rule wins.
- **OpenRGB still holds the node.** OpenRGB keeps the keyboard's hidraw
  handle open whether hotaru writes to it or not. Idle, it sends nothing
  (sanshoku E5 measured a clean stream beside it); a profile applied by
  `openrgb-profile.service` would. The user's "purp" profile should leave the
  Apex out; this spec documents that, it does not edit the profile.
- **Every OpenRGB restart reboots the Apex.** R1.4 makes that a reattach.
  The board shows its onboard rainbow for the length of the backoff.
- **Battery.** Streaming through the receiver spends the battery faster; R6
  measures it, and a static scene costs nothing.
- **A second canvas consumer.** hayami reads the Apex's battery through the
  same node. sanshoku drains before each ask, so frames do not starve it;
  verified in sanshoku 010 on one handle, assumed for two processes.
- **Rollback**: revert. With no canvas device attached, hotaru behaves as
  before; the udev rule addition is inert without the hardware.

## Alternatives Considered

Considered OpenRGB's Effects plugin, which draws about sixty effects on any
Direct-mode device and can be started by name over the SDK; rejected because
OpenRGB 1.0 loads plugins only from its GUI window, which would replace the
headless server with a GUI in the session, put effect settings in OpenRGB's
UI rather than scenes, and move away from the Go-native design goal (sanshoku
010, Alternatives Considered).

Considered drawing effects in hotaru and sending them through OpenRGB's
Direct mode, keeping one lighting path; rejected because OpenRGB's frames
carry no acknowledgement back, its exit reboots the board regardless, and
sanshoku already holds the node for the battery.

Considered putting the effects in sanshoku; rejected there (sanshoku 010)
because effects are a product decision and sanshoku is device access.

## Verification

The live criteria (watching the Apex, the reattach after an OpenRGB restart,
the CPU and battery measurements of R6, release on the board, and every other
device on cachyos and njv-cachyos) are for the live run and are not recorded
yet.

### Checks

Run in the worktree on 2026-10-01, no hardware opened:

- `make test`: every package passes, including `internal/render`,
  `internal/canvas`, the canvas tests in `internal/service`,
  `internal/daemon`, `internal/cli` and `internal/gui`, and the style and
  README canaries at the root.
- `go test -race ./internal/canvas/... ./internal/service/ ./internal/daemon/
  ./internal/render/`: pass.
- `go test -tags migrated_fynedo ./internal/gui/`: pass.
- `make lint` (golangci-lint v2.12.2): 0 issues.
- `go vet ./...`: clean.
- `make generate`, then the stale diagram images pruned: the architecture
  diagram is rendered.
- `make build`, `make gui`, and `CGO_ENABLED=0 go build ./cmd/hotaru`:
  build.
- The OpenRGB location reading in R1.3: a read-only listing over the SDK on
  cachyos (no mode or colour set, no service touched), beside
  `/sys/class/hidraw/*/device/uevent` and the first bytes of each node's
  report descriptor.

### Gaps found

- **Rainbow Wave by key order crossed the alphabet, not the board.**
  sanshoku lists the Apex's keys in GG's frame order, A to Z, the digits,
  then the rest by HID usage. Fixed in review: `internal/render/layout.go`
  places a key by its name on a standard US layout, and a light whose name
  the layout does not know keeps its place in the canvas's order. R2.3 is
  amended.
- **Found in the live run, fixed:** a scene's "N of M device(s) lit"
  counted the skipped twin as an unlit device (results now carry
  `handed_to`, and the count leaves it out); a restore after OpenRGB
  restarted waited for the twin, which the restarted server did not list
  because the board was still rebooting (reconcile now skips a recorded
  twin named by a rule); and the look straight after a loss attached to the
  rebooting board's old node, which the kernel keeps for about two seconds,
  and lost it again (nodes still carrying a lost device's kernel name are
  skipped). The last reads sysfs and is verified live, not by a unit test.
- **Through the receiver, a reboot changes nothing the kernel shows.** The
  receiver's hidraw node and its kernel name stay the same while the
  keyboard behind it reboots, so the dying-node skip above blocked the
  device for good after a release (found live, fixed: the skip lasts four
  seconds), and `Present` cannot see a reboot while a static scene holds.
  A full restore now redraws every canvas once, because OpenRGB
  reconnecting is when its exit has most likely rebooted the board; a
  reboot from any other cause while a static scene holds behind the
  receiver shows the firmware's effect until the next scene or restore.
- **A static frame that went unacknowledged was never sent again.** A
  static scene has no ticker, so the board would hold the previous scene.
  Fixed in review: the animator retries at the show's interval until a frame
  lands, then stops ticking.
- **"The scene's interval" (R3.3) has no field in a scene.** It is the
  device rule's `frame_interval`, default 56 ms, never under `Floor()`. A
  per-scene interval would be a field on `scenes.Effect`, and nothing asked
  for one.
- **A canvas device is listed only while the OpenRGB server is connected.**
  Every service operation starts from the OpenRGB client, and a canvas device
  is listed beside it. OpenRGB's exit reboots the Apex anyway, so the canvas
  is not usable without the server today; a canvas without OpenRGB is a
  separate change to how the service reports an unreachable server.
- **Off is a renderer.** R2.2 lists four effects; Off (one black frame) is a
  fifth mode so that `light off` and an Off scene reach a canvas device
  through the ordinary off path. `never_blank` applies to it as to any
  device.
- **An apply reports a canvas device as applied when the animator has the
  frame**, not when the board acknowledges it. The animator is the only
  writer and sends asynchronously; an unacknowledged frame is logged once per
  run, and `ErrGone` ends the attachment and the reattachment puts the scene
  back.
- **A holding canvas sends nothing, so it cannot see `ErrGone`.** The daemon
  reads `/sys/class/hidraw/<node>/device` every two seconds and compares the
  kernel's HID device name (`0003:VVVV:PPPP.NNNN`), whose sequence number
  moves on every enumeration even when the node path comes back the same.
  That is how a board rebooted by OpenRGB's exit is reattached while showing
  a static scene.
- **`light health`'s stale verdict (spec 058) may name the twin**, because
  OpenRGB does hold a descriptor for a node the kernel removed. That is true
  of OpenRGB and harmless to hotaru, which no longer writes to the twin; it
  is left as it is.
- **The user's OpenRGB "purp" profile** is not touched; docs/hardware.md says
  to leave the keyboard out of any profile applied at login.
