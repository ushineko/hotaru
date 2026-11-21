# Supported hardware

Two things, and they are different claims:

1. **What has been tested.** Hardware somebody ran hotaru against and watched.
   That list is short, and it is below.
2. **What should work.** Every device
   [sanshoku](https://github.com/ushineko/sanshoku) drives. hotaru reaches
   every device through sanshoku, so its
   [support table](https://github.com/ushineko/sanshoku/blob/main/docs/devices.md)
   is hotaru's, and this page links to it rather than copying it.

hotaru drives each device itself, through sanshoku, with no daemon between
them: over `/dev/hidraw`, usbfs and I2C, with no subprocess and no cgo. A
device sanshoku does not drive is not lit, and no configuration changes that.
Support for another device is a sanshoku driver first. Until spec 076, hotaru
drove most lit devices through an OpenRGB server.

Every lit device is a *canvas device*: hotaru draws its effects and sends it
frames, and lists the effects its firmware has beside them. See
[spec 060](../specs/060-effects-hotaru-draws.md) and "Canvas devices" below.
The cooler's driver is the `nzxt` package of sanshoku, which was hotaru's own
code until spec 059.

## Tested

"Works" here means observed on the hardware. It does not mean inferred from a
support list.

### The development machine

| Device | Through | Notes |
|---|---|---|
| NZXT Kraken 2024 Elite (`1e71:3012`) | sanshoku | hotaru draws the ring and the fans itself, through sanshoku, on the same handle that reads the telemetry and draws the 640x640 panel (spec 073). The dashboard goes to the panel as full-colour frames (spec 080). The radiator fans' RGB daisy-chains into the cooler. hotaru never writes into the bucket the panel is displaying, because the panel blanks for the length of that transfer (spec 013) |
| MSI GeForce RTX 4090 Suprim Liquid X | sanshoku | Drawn through sanshoku over the card's I2C adapter (spec 075; sanshoku spec 029), on Linux only. One light: a colour is the card's static mode, at most about four a second, and its own effects (Breathing and the rest) carry it moving. **Its LEDs wash out.** A pale colour looks close to white, and its middle greens look too bright. A `colour` rule corrects it (spec 066) |
| ASUS ROG Maximus Z790 Hero | sanshoku | Drawn through sanshoku (spec 075; sanshoku spec 027): the onboard LED and four addressable headers. **The board cannot read a header's length**, so a `lengths` rule gives it. On this desk the accent strip has 18, the side fans 16 and the rear fan 20; OpenRGB had 16 for each |
| Corsair MM700 | sanshoku | Drawn through sanshoku (spec 075; sanshoku spec 026). It keeps a frame. Its logo has no blue channel, so a request for purple appears as dim red (spec 009). Its LEDs wash out like the graphics card's |
| Logitech G502 X PLUS | sanshoku | Wireless, drawn through sanshoku on its receiver's node (spec 075; sanshoku spec 030), whose name is the receiver's: a `formerly` rule maps "G502 X PLUS" onto it. A frame holds; asleep, the mouse answers nothing, and the next frame after it wakes takes it again |
| Keychron K4 HE | sanshoku | Drawn through sanshoku over VIA (spec 075; sanshoku spec 028). **The firmware takes each key's hue and saturation and ignores its value.** The board's one brightness follows the brightest key. A key cannot be dark beside a lit one: a black key shows white |
| Intel Core i9-14900K | kernel | CPU package temperature from `coretemp`, read by label. Never by hwmon index: the kernel assigns those numbers in probe order, and they move between boots |
| MSI GeForce RTX 4090 | `nvidia-smi` | NVIDIA's driver registers no hwmon, so the dashboard reads the GPU temperature from the tool. It costs 18 ms, once per update. hotaru reads AMD and nouveau cards from `/sys/class/hwmon` like anything else |

### Test systems

Both ran before spec 076, with lighting through OpenRGB, on 0.1.29. Since
2.0, hotaru lights what sanshoku has a driver for
([#231](https://github.com/ushineko/hotaru/issues/231)). sanshoku 0.6.0
has one for each device below, and spec 082 draws them:

| Machine | Device | Since 2.0 |
|---|---|---|
| CachyOS | SteelSeries Apex Pro TKL Gen 3 | lit |
| CachyOS | Gigabyte Z790 AORUS MASTER X, two controllers | lit, spec 082: the strips are on the 12 V header (LED_C), the board's lights on the other controller |
| CachyOS | Razer Mouse Dock Pro, and a Basilisk V3 Pro through it | lit, spec 082 |
| CachyOS | Corsair Dominator Platinum RGB DDR5, four modules | lit, spec 082, over the chipset's SMBus (`i2c-dev`) |
| CachyOS | EVGA GeForce RTX 3080 FTW3 Ultra LHR | lit, spec 082, over the NVIDIA driver's I2C adapter |
| Windows 11 | AULA F75 | lit |
| Windows 11 | MSI B450 Tomahawk Max strip | through the PawnIO helper, spec 083; the chip's own Breathing, Flashing and Color Cycle, spec 085 |
| Windows 11 | Corsair Vengeance RGB, two modules | through the PawnIO helper, spec 083; the modules' own Breathing and Fading, spec 085 |
| Windows 11 | Razer Basilisk Ultimate and its dock | not yet checked on 2.x |

On Windows, the board's header and the memory are reached through PawnIO, a
signed kernel driver installed on its own (`winget install namazso.PawnIO`),
as OpenRGB reaches them. PawnIO serves only SYSTEM and administrators, so
hotaru's PawnIO helper, a second service, holds them (spec 083). With
LibreHardwareMonitor running, its hold on the bus locks stalls them; hayami
no longer needs it (hayami spec 103).

What they found under OpenRGB:

| Machine | Status |
|---|---|
| CachyOS, unrelated hardware, OpenRGB already running in service mode, keyboard shared from another machine over [deskflow](https://github.com/deskflow/deskflow) | **Done**, at 0.1.0 and again at 0.1.1. hotaru found nine devices and drove them with nothing written by hand. Three faults appeared, and this desk could not have shown any of them. See below |
| Windows 11, MSI B450 Tomahawk Max, OpenRGB 1.1 as a service at boot | **Done** for lighting, after 0.1.29 (specs 067, 071 and 072). The installed service found six devices and set all of them: the board's strip, two Corsair Vengeance RGB modules, a Razer Basilisk Ultimate and its dock, and the AULA F75. The board's strip kept its colour while LibreHardwareMonitor ran, an OpenRGB fault ([#197](https://github.com/ushineko/hotaru/issues/197)) |

What that installation found, in the order it was found:

1. **The service could not create its own directories.** `ProtectHome=read-only`
   opens a `ReadWritePaths` entry only if the directory already exists. On a
   machine that had never run hotaru, the first picture failed with "read-only
   file system", and rules and scenes would not have saved either. Fixed in
   0.1.1: the unit creates those directories before systemd builds the sandbox.
2. **The window jumped to the service page** when somebody dropped a picture on
   it, because the handler asked for a section that had been folded into a
   group. Half of that fix is in fynedesygn.
3. **The numpad shortcuts do not fire.** Everything else about them works.

### The numpad, over a shared keyboard

The nine shipped shortcuts are `Ctrl+Alt+Num+1` to `Ctrl+Alt+Num+9`, which is
what the desk hotaru was written on has always used. On the test system they
do nothing, and every other link in the chain checks out:

- KWin has the script loaded. `isScriptLoaded hotaru-scenes` returns true.
- The ten actions are in `kglobalshortcutsrc`, with the right sequences.
- `kglobalaccel` lists them for the `kwin` component.
- `Component.invokeShortcut` applies the scene, so the script, the D-Bus door
  and the scene all work.

The keyboard is what differs. It is not attached to that machine. It arrives
from another machine over deskflow, as a virtual device (`Vendor=beef
Product=dead`, `/devices/virtual/input/input34`). That device claims every
numpad keycode, so nothing about it can be detected. **Non-numpad bindings on
the same machine work**, which is the measurement that says where the fault is
not.

So on a machine whose keys arrive over the network, bind something other than
the numpad. The window offers that from the scene's own row since 0.1.1 (spec
035), and `hotaru keys bind "Meta+Shift+L" evening` accepts any sequence KDE
spells.

This was not chased further. The remedy is one binding, and the cause is in
another project's key forwarding.

**This is a class of fault rather than one tool's bug.** Anything that carries
a keyboard from one machine to another rebuilds modifier and lock state at the
far end: deskflow, the synergy and barrier lineage it comes from, a hardware
or software KVM, and a VM's console. That rebuilt state is the part that most
often does not survive the trip. Modifiers, NumLock and the keypad are where
it shows, and a shortcut like `Ctrl+Alt+Num+1` uses all three.

The rule for a machine whose keys arrive over a wire is therefore: **bind the
simplest sequence that works there.** Do not assume that a shortcut which
works on the machine with the keyboard attached also works on the machine
receiving it. A laptop with no numpad needs the same advice for a simpler
reason.

hotaru applies every correction in the first table by *discovering* it. It
reads back what a write did, rather than matching a device name against a
table. The table above records what was learned. It is not a list the program
requires.

### Lights that wash out

Some LEDs show a smaller range of colour than others. On the development
machine, the RTX 4090 and the MM700 show a pale colour close to white. The
fans and the keyboards show it as written.

The scene `ice1` measured it. To look like the fans' `#80aad1`, the 4090 had
to be written `#003eff`. That is 39% saturation against 100%, and a hue 16°
toward blue.

A `colour` rule corrects a device, and a scene keeps the colour that was
meant (spec 066):

```yaml
devices:
  - match: geforce
    colour:
      saturation: 1         # 0 as written, 1 always 100%
      value: 1              # the same, for brightness
      curve: {green: 2.15}  # a gamma per channel, applied last
```

`saturation` and `value` push each colour toward 100% and keep its hue. Black
stays black, and a grey keeps no hue. `curve` raises a channel to a power, so
above 1 dims its middle levels and leaves off and full alone. With these
numbers, `#80aad1` is written `#003eff`.

The service corrects every write to the device: scenes, `light set`,
previews, reconciling and an effect's own colours. Everything it reports is
the colour that was meant. It keeps what it last asked for and what it wrote,
and reports a light the device shows in the written colour as the asked-for
one. After a restart it has no record until its first write to the device.

These numbers are one desk's, matched by eye. Tune them by looking: change
the rule, run `hotaru reload`, and apply the scene again.

### Effects that follow

Every device hotaru draws runs the Kraken's fourteen effects under the
Kraken's names, drawn by hotaru to match the cooler by eye (spec 077). A
device can follow another: its rule's `follow` names the leader, and it takes
the leader's effect, colours and speed whenever the leader is given one.

```yaml
devices:
  - match: maximus
    follow: kraken
    shapes: {"Addressable RGB Header 3": ring}
    pace: {Breathing: 2.2}
  - match: msi
    follow: kraken
```

- **What each device shows.** The Kraken runs its own firmware effect. A
  device hotaru draws shows hotaru's copy, per fan, along a chain of fans, or
  staggered fan to fan, as the Kraken does. A device with one light takes the
  closest effect of its own: on the RTX 4090, Breathing for Breathing, Pulse,
  Candle and Starry Night; Color Cycle for Fading; Rainbow Wave for the
  rainbows; Fade In for Alternating, Tai Chi and Water Cooler; Flowing for
  Covering Marquee and Loading.
- **Fans and strips.** A zone's fans are the device's own count where it has
  one, else the rule's segments that cover the zone end to end. `shapes` says
  whether a header holds fans (`ring`) or a strip (`line`), since the board
  cannot tell.
- **Colours.** An effect a scene gives with no colours of its own runs in the
  scene's colours on that device, in order. A scene with a red ring and blue
  fans gives the Kraken Tai Chi in red and blue, and its followers the same.
  hotaru takes the most different of the scene's colours. Where they sit
  close, as a picture's often do, it pushes them apart so the effect shows.
  Colours you choose for an effect are used as they are.
- **Following.** Each device's card in the window's System section has a
  Follows choice. `hotaru light follow maximus kraken` does the same, and
  `hotaru light follow maximus none` stops it. A device given a leader starts
  on the leader's effect at once.
- **Pace.** The copies run at the Kraken's pace as measured. `hotaru light
  match <device> <effect>` runs the effect on a device and its leader
  together. Keys f and s change it by a tenth, F and S by a fiftieth, and a
  number sets it. Return writes the device's `pace` for that effect. A
  follower's card in the window has Match pace, with a slider and a field.
  A shape that differs from the Kraken's, as Breathing's does, drifts apart
  over minutes whatever the pace.
- **A leader for the rest.** With no cooler to follow, any device can lead
  (spec 084): `lead: true` in its rule, its card's Leads the rest, or
  `hotaru light lead <device>`. Every device whose rule names no leader of
  its own then follows it. One device leads at a time.
- **In step.** The leader is written first and every effect starts again, on
  an apply and on a full restore. An apply or a scene that gives several
  devices an effect starts them all again too, leader or not, so a device
  already running the same effect does not carry on out of step.
- **Limits.** The K4 HE has one brightness for the board, so an effect that
  dims keys apart dims the board. The MM700's logo has no blue. The F75
  through its receiver shows a merged palette. A moving effect on the G502
  costs battery.
- **A header's length.** `hotaru light length <board>/<header>` lights it
  green with its last light red; + and - move the end until the red is on
  the strip's last light, and Return writes `lengths`. The window's Headers
  on the board's card does the same.

### Set by hand

`hotaru light map` once asked about two things no command sets now. Both
stay in the rules file:

```yaml
devices:
  - match: keychron
    never_blank: true                        # it cannot go dark
    solid_modes: [Solid Reactive Multinexus] # lit only where you type
  - match: mm700
    brightness: 40                           # turned down, of 100
```

`hotaru light set --brightness` dims a device for one scene. `hotaru reload`
reads the rules again.

## Everything else

hotaru's lighting and the cooler come from sanshoku, a Go module built into
the binary. Its device list is the answer for hardware that is not tested
above:

| Package | What it provides | Its device list |
|---|---|---|
| sanshoku | Every lit device, the cooler's telemetry and panel, and the hwmon tables | [docs/devices.md](https://github.com/ushineko/sanshoku/blob/main/docs/devices.md) |
| `nvidia-utils` | Optional. The GPU temperature on the dashboard, where the kernel exposes none | — |

`liquidctl`, `openlinkhub` and `openrgb` were dependencies and are not any
more. You can still install any of them alongside hotaru. Nothing in hotaru
calls them, but a second program writing the same device fights hotaru for
its lights.

### What each contributes

**Lighting: hotaru itself, through sanshoku.** Colours on every device
sanshoku drives, per device, per zone, or per light. Every device takes a
colour per light in hotaru's Static mode.

**Mode colours: one for a device's own effect.** A device's own effect, such
as the keyboard's Solid Reactive Multinexus, shows one stored colour. hotaru's
renderers take more: Breathing up to four, Spectrum and Rainbow Wave up to
eight.

**Cooler telemetry: hotaru itself, through sanshoku.** The kernel has no
driver for recent NZXT coolers. `nzxt-kraken3` matches 2007, 2014, 3008, 300C
and 300E, so a Kraken Elite V2 has no hwmon node and `sensors` reports nothing
at all. hotaru reads the device over `/dev/hidraw`, which costs about two
milliseconds and starts no processes. sanshoku's
[support table](https://github.com/ushineko/sanshoku/blob/main/docs/devices.md)
lists the coolers its `nzxt` driver speaks to. Its
[contention page](https://github.com/ushineko/sanshoku/blob/main/docs/contention.md)
records what sharing the node does. Status is safe to read from two programs at
once, and the usbfs claim keeps a second program off the panel.

**The panel: hotaru itself, on coolers that have one.** The Kraken Z and Elite
families store images in sixteen buckets of their own memory. Most coolers
have nothing of the kind. A GIF in a bucket stays on the panel, so pictures
from `hotaru screen show` go there. The Kraken Elite also takes streamed
frames in full colour, as NZXT CAM sends them (spec 080). The firmware returns
to its readout 30 s after the last frame, so hotaru sends the dashboard's frame
again every 25 s. Other panels receive the dashboard as a one-frame GIF. For a picture behind a
dashboard, the image library keeps a full-colour copy of each still picture
beside its GIF.

**Motion on the Kraken Elite.** The panel also takes motion frames: Q565,
16-bit colour, at about 20 a second (sanshoku 0.4.0). A dashboard on it can
melt, morph or roll its changing numbers and sweep its rings, its trail drifts
smoothly, and an animated picture behind it plays (spec 081). Every dashboard
frame on it is a motion frame, still or not.

**Temperatures: the kernel.** The CPU package comes from `coretemp`, read by
label. An AMD processor is read from `k10temp` or `zenpower`, in the order
sanshoku's `hwmon.CPU` gives. The graphics card comes from `amdgpu` or `nouveau` where they are
present. NVIDIA's own driver registers no hwmon, so that card's number comes
from `nvidia-smi`. This is why `nvidia-utils` is an `optdepends` rather than a
dependency: a machine without it shows a placeholder rather than an error.

**Canvas devices: hotaru itself, through sanshoku.** On the Apex Pro TKL
Wireless Gen 3, no command on the board changes its effect. SteelSeries GG draws every effect on the PC and
streams it as frames, and the board holds the last frame it was sent.
sanshoku's `steelseries` driver sends those frames, and each one is
acknowledged. hotaru lists the board as a device whose modes are its own
renderers: Static, Breathing, Spectrum, Rainbow Wave and Off. A scene names
them as it names any firmware mode. Breathing takes up to four colours of its
own, one breath each. Spectrum and Rainbow Wave take up to eight and move
through them. Without colours each draws as it did before.

- **A scene that does not move is one frame.** Static, a solid colour and Off
  send one frame, and then nothing.
- **A moving effect is a stream.** It sends a frame every 56 ms, the pace GG
  uses, and the board's floor is 16 ms. A `frame_interval` rule changes the
  pace for one device. Through the wireless receiver, a stream spends the
  battery faster than a scene that does not move.
- **Stopping hotaru leaves the last frame showing.** `hotaru light release
  <device>` hands the lighting back to the firmware. On the Apex that reboots
  the board, which then shows its onboard effect. hotaru attaches it again
  and sends it nothing until a scene asks.
**The Kraken's ring and fans: hotaru draws them through sanshoku.** Measured
on a Kraken 2024 Elite on firmware 1.2.12 (spec
[073](../specs/073-the-kraken-drawn-through-sanshoku.md), sanshoku spec 024).

- **One owner of the cooler.** sanshoku's driver reads the cooler's status,
  draws on its screen and lights its ring and fans, on one handle. Two
  programs writing that node cost specs 068 and 070, and #206.
- **The lights are two zones, Ring and Fan.** Each key is named as sanshoku
  names it (`ring 1` to `ring 24`, `fan 1` and on), and hotaru makes a zone
  of each word.
- **A still scene is one frame.** The cooler keeps it with nothing sent. A
  moving effect is a stream at 56 ms; the cooler's floor is 16 ms.
- **The screen's readout no longer turns a ring LED green.** The readout
  puts back what the cooler has stored, and sanshoku stores the frame before
  every switch to it. The 12 s resend of spec 070 is gone.
- **`hotaru light release` keeps the cooler attached.** It stores the frame
  in the cooler and stops drawing. The cooler does not reboot.
- **The cooler's own effects come first.** Fading, Tai Chi, Loading and the
  rest of its 14 are the firmware's (sanshoku spec 025). `hotaru light list`
  marks each with `*`, and a scene's `effects` runs one at no cost to the
  host. Where a name is both, the firmware's wins: Breathing is the cooler's.
  The cooler does not report which effect runs, so hotaru knows only what it
  set. A preview does not set one, as for the F75.
- **Scenes written for OpenRGB's listing still apply.** A rule's `formerly`
  names the cooler as OpenRGB listed it before spec 076, and maps its zones. Targets,
  segments and the recorded colours under the old name land on the same
  lights. No file is rewritten.

```yaml
devices:
  - match: kraken
    formerly:
      name: NZXT Kraken 2024 ELITE Series RGB
      zones: {"Hue 2 Channel 1": Ring, "Hue 2 Channel 2": Fan}
```

**The other lit devices: through sanshoku.** Since spec 075 hotaru draws the
mainboard, the mousepad, the keyboard, the mouse and the graphics card
through sanshoku too. Since spec 076 there is no other way.

- **Names are OpenRGB's.** Each driver names the device, its zones and its
  lights as OpenRGB did, so scenes, segments and recorded colours made through
  OpenRGB find them.
  A record in OpenRGB's Direct restores in Static. A record laid out for
  OpenRGB's header sizes is fitted to the lengths now: each header's colours
  are stretched over its strip.
- **A header's length is a rule.** The mainboard cannot read how many LEDs a
  strip has, so a rule gives it:

```yaml
devices:
  - match: maximus
    lengths: {"Addressable RGB Header 1": 18, "Addressable RGB Header 2": 16,
              "Addressable RGB Header 3": 20}
```

- **The mouse is drawn on its receiver's node**, which is named "Logitech USB
  Receiver". A `formerly` rule keeps scenes written for OpenRGB's "G502 X
  PLUS":

```yaml
devices:
  - match: logitech usb receiver
    formerly: {name: G502 X PLUS}
```

- **The keyboard's own effects** are its firmware's 22, named as OpenRGB names
  them (sanshoku spec 031), so a keyboard recorded in Solid Reactive
  Multinexus comes back in it.
- **Two rule keys mean nothing now.** `twin` named a device's OpenRGB
  listing, and is read and ignored, so a rules file with it still loads. A
  `solid_modes` entry such as `direct` names an OpenRGB mode, and matches no
  mode now; the next entry, or Static, is used.
- **Keep other programs off the devices hotaru draws.** OpenRGB, or a
  vendor's own software, writing the same device fights hotaru for its
  lights.

**The AULA F75: its own effects, and frames when asked.** Measured on Windows,
through the keyboard's 2.4 GHz receiver (spec
[071](../specs/071-the-aula-f75.md), sanshoku specs 019 and 023). hotaru lists
it with two kinds of mode.

- **Its own effects come first.** Ripples shining, Reaction, Fixed on and the
  rest are the firmware's own. `hotaru light list` marks each with `*`. The
  effect and each effect's colour are stored in the keyboard's flash, so
  choosing one writes the keyboard's storage.
- **`light set purple` keeps the running effect and makes it purple.** An
  effect that takes no colour (Off, Self-define) goes to Fixed on. `light
  off` is the firmware's Off.
- **hotaru writes the storage only when a person asks.** A scene, `light set`,
  `light off`, a key or a rule writes it. A reconcile and a preview never do.
  A preview of a colour draws the colour until the preview ends. Two writes
  are at least two seconds apart, and the log names each one.
- **hotaru's renderers draw frames over the effect.** The keyboard takes its
  lights back about a second after the last frame. So a still colour is sent
  again every 500 ms, and a moving effect runs at 120 ms or slower. At 80 ms a
  moving frame flickered, because one can take 110 ms to write.
- **A power cycle loads the factory settings.** The keyboard comes back on
  Neon stream, with every effect on its factory colours. hotaru asks the
  keyboard one short question every ten seconds while a stored effect is
  recorded. When it answers again on Neon stream, hotaru puts the recorded
  effect and colour back. Waking from sleep keeps the settings, and an effect
  chosen with the keyboard's own keys is kept.
- **AULA's own software writes the same storage.** hotaru cannot stop it, and
  the last write wins.

**Permissions.** Both cooler nodes need a udev rule that tags the device
`uaccess`: the `/dev/hidraw*` node, and the `/dev/bus/usb/BBB/DDD` node behind
which the panel's bulk endpoint lives. Without that rule `systemd-logind` puts
no ACL on them, and hotaru finds a cooler it cannot open. The package ships the
rule. See [docs/packaging.md](packaging.md). Every lit device needs the same
tag on its node: the package's rule covers the hidraw nodes of SteelSeries,
ASUS, Corsair, Keychron and Logitech devices, and the NVIDIA I2C adapters.

`uaccess` grants that ACL to an **active seat session**, not to the user
manager that `enable-linger` starts at boot. hotaru therefore starts before
its own cooler is openable, by sixteen seconds on the machine this was
measured on. It waits for the device rather than opening it once, so the panel
arrives when the login does.

## Not supported, deliberately

- **Fan and pump duty control.** The Commander ST firmware discards duty writes
  and reports nothing. It did so from liquidctl and from OpenLinkHub alike,
  which is where this was learned. A control would report success and change
  nothing, so hotaru offers none.
- **Device lighting persistence.** hotaru does not save a device's lighting
  to its profiles; it puts recorded lighting back when the device appears.
- **Coolers other than the NZXT models above.** hotaru speaks NZXT's protocol.
  Another vendor's cooler needs a driver rather than a configuration entry, and
  hotaru does not have one.
- **Anything that needs a vendor's own daemon**, or a lighting daemon such as
  OpenRGB.
- **Hardware sanshoku does not drive.**

## Checking your own machine

```bash
hotaru light list             # what hotaru sees, with modes and scope
hotaru light health           # why, if it sees nothing
hotaru light length maximus/"Addressable RGB Header 3"   # a header's length
hotaru light name kraken      # names for a device's zones and fans
hotaru cooling                # the cooler, or the fact that there is not one
hotaru light release apex     # a device's own lighting back
```

If `hotaru light list` shows nothing, `health` says what to check: that the
device is one sanshoku drives, and that hotaru's udev rules are installed.
hotaru looks for devices again every thirty seconds, so a device plugged in
later appears on its own.
