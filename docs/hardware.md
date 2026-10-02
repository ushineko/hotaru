# Supported hardware

Two things, and they are different claims:

1. **What has been tested.** Hardware somebody ran hotaru against and watched.
   That list is short, and it is below.
2. **What should work.** Every lit device OpenRGB supports, because hotaru
   contains no lighting drivers of its own. That list belongs to OpenRGB, and
   this page links to it rather than copying it.

hotaru does not maintain a lighting device list. If OpenRGB can set your
keyboard's colour, so can hotaru. If OpenRGB cannot see the device, neither
can hotaru, and no configuration changes that. This is also why installing
hotaru installs OpenRGB: the lighting support lives there.

The cooler is the exception. hotaru writes NZXT's protocol directly over
`/dev/hidraw` and usbfs, with no Python, no subprocess and no cgo. That gives a
short device list rather than a borrowed one. See
[spec 012](../specs/012-the-cooler-without-liquidctl.md). The driver is the
`nzxt` package of [sanshoku](https://github.com/ushineko/sanshoku), which was
hotaru's own code until spec 059.

A *canvas device* is the second exception. It has no lighting effects of its
own, so hotaru draws them and streams the frames through sanshoku. The first
canvas device is the SteelSeries Apex Pro TKL Wireless Gen 3. See
[spec 060](../specs/060-effects-hotaru-draws.md) and "Canvas devices" below.

## Tested

"Works" here means observed on the hardware. It does not mean inferred from a
support list.

### The development machine

| Device | Through | Notes |
|---|---|---|
| NZXT Kraken 2024 Elite (`1e71:3012`) | both | Lighting through OpenRGB. The radiator fans' RGB daisy-chains into the cooler, and OpenRGB exposes **only** the colour channels. Telemetry and the 640x640 panel are hotaru's own, over `/dev/hidraw` and usbfs. The two Hue 2 channels are separate controllers, so hotaru writes them separately (spec 011). hotaru never writes into the bucket the panel is displaying, because the panel blanks for the length of that transfer (spec 013) |
| MSI GeForce RTX 4090 Suprim Liquid X | OpenRGB | Accepts `direct/breathing/flashing/off`. It **rejects `static`** and goes dark when it receives it |
| ASUS ROG Maximus Z790 Hero | OpenRGB | Onboard LEDs and four addressable headers. Static drives the onboard LED only. The headers need Direct |
| Corsair MM700 | OpenRGB | Its logo has no blue channel, so a request for purple appears as dim red. The write does not fail, and nothing in the protocol reports the limit. OpenRGB's own command line behaves the same way (spec 009) |
| Logitech G502 X PLUS | OpenRGB | Wireless. It restores its onboard state on wake, so hotaru rewrites its colour on a timer. Solaar cannot set its colour at all, because its command line drops the colour argument without saying so |
| Keychron K4 HE | OpenRGB | The board advertises no Off mode, so `off` resolves to Direct with black and kills the backlight. hotaru colours the board instead, and never blanks it. **Per-key colour takes hue but not value.** `#ffffff` and `#101010` look the same on the board, and `#000000` is as lit as any other colour. A red row beside a green one draws correctly. OpenRGB's own command line reproduces this, so it is not hotaru's fault. Firmware `v1.1.1 2025-06-17`. **The lock keys belong to the firmware.** It paints Caps Lock and Num Lock white while they are engaged, over whatever hotaru wrote, and nothing on the host overrides that. The firmware does this deliberately: an indicator the host can switch off is an indicator that is off when it matters. Both findings are why spec 043 colours those keys rather than blanking them (#110) |
| Intel Core i9-14900K | kernel | CPU package temperature from `coretemp`, read by label. Never by hwmon index: the kernel assigns those numbers in probe order, and they move between boots |
| MSI GeForce RTX 4090 | `nvidia-smi` | NVIDIA's driver registers no hwmon, so the dashboard reads the GPU temperature from the tool. It costs 18 ms, once per update. hotaru reads AMD and nouveau cards from `/sys/class/hwmon` like anything else |

### Test systems

| Machine | Status |
|---|---|
| CachyOS, unrelated hardware, OpenRGB already running in service mode, keyboard shared from another machine over [deskflow](https://github.com/deskflow/deskflow) | **Done**, at 0.1.0 and again at 0.1.1. hotaru found nine devices and drove them with nothing written by hand. Three faults appeared, and this desk could not have shown any of them. See below |

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

## Everything else

Lighting comes from one upstream project. Its device list is the authoritative
answer for hardware that is not tested above:

| Package | Version developed against | What it provides | Its device list |
|---|---|---|---|
| `openrgb` | 1.0 | Every lit device: GPUs, motherboards, RAM, keyboards, mice, mousepads, cases, coolers, strips | [openrgb.org/devices](https://openrgb.org/devices_1.0rc3.html) |
| `nvidia-utils` | — | Optional. The GPU temperature on the dashboard, where the kernel exposes none | — |

`liquidctl` and `openlinkhub` were dependencies and are not any more, which
takes Python off machines that may have no liquid cooler at all. You can still
install either one alongside hotaru. Nothing in hotaru calls them.

hotaru does not pin minimum versions artificially. It speaks OpenRGB's SDK
protocol, negotiates the version at connect, and reports it in `hotaru light
health`. A server too old for your device is a reason to upgrade the server. A
device missing from `openrgb --list-devices` is an upstream matter.

### What each contributes

**Lighting: OpenRGB.** Colours on any device it enumerates, per device, per
zone, or per LED. Per-LED control usually needs the device's Direct mode.
Devices that render Static as a single colour cannot show a multi-colour
frame, and hotaru reports that rather than approximating it. hotaru talks to
the running OpenRGB *server*, so lighting needs `openrgb` installed **and** its
server running.

**Mode colours: as many as the mode reports.** OpenRGB gives each mode a
`colors_min` and a `colors_max`, and hotaru fills that many slots from a
scene's effect (spec 061). One colour fills every slot. Most modes take one.
Some take more:

| Device type | Mode | Colours |
|---|---|---|
| NVIDIA GPU (MSI) | Breathing, Fade In | 1 to 2 |
| NVIDIA GPU (MSI) | Color Cycle, Wave | 1 to 3 |
| NVIDIA GPU (EVGA) | Breathing | 1 to 2 |
| NVIDIA GPU (EVGA) | Color Cycle, Color Stack | 2 to 7 |
| NZXT Kraken | Fading, Breathing, Pulsing, Cover Marquee, Candle | 1 to 8 |
| NZXT Kraken | Alternating | 1 to 2 |
| Corsair Dominator DDR5 | Color Shift, Color Pulse, Color Wave, Visor, Rain | 2 |
| Razer mouse dock | Breathing | 1 to 2 |

These counts were read from OpenRGB on the two test machines on 2026-10-01.
A mode is written no more colours than it reports, and fewer than it needs
repeat the last one.

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
have nothing of the kind. The firmware does not retain a still picture, so
hotaru sends one-frame GIFs.

**Temperatures: the kernel.** The CPU package comes from `coretemp`, read by
label. An AMD processor is read from `k10temp` or `zenpower`, in the order
sanshoku's `hwmon.CPU` gives. The graphics card comes from `amdgpu` or `nouveau` where they are
present. NVIDIA's own driver registers no hwmon, so that card's number comes
from `nvidia-smi`. This is why `nvidia-utils` is an `optdepends` rather than a
dependency: a machine without it shows a placeholder rather than an error.

**Canvas devices: hotaru itself, through sanshoku.** The Apex Pro TKL Wireless
Gen 3 offers OpenRGB two modes, Direct and Onboard, and no command on the
board changes its effect. SteelSeries GG draws every effect on the PC and
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
- **OpenRGB's exit reboots the board.** hotaru notices within two seconds,
  attaches the board again and puts the scene back.
- **OpenRGB still lists the board.** hotaru calls that listing the board's
  *twin*, writes nothing to it, and shows it as handed over in `hotaru light
  list`. The twin is found by the hidraw path in OpenRGB's location. OpenRGB
  records that path when it starts and keeps it after the board moves to
  another node. On 2026-10-01 it listed the board at `/dev/hidraw5` after the
  board had moved to `/dev/hidraw4`. A rule names the twin where the path does
  not match:

```yaml
devices:
  - match: Apex Pro TKL Wireless Gen 3     # the kernel's name: the canvas device
    twin: Apex Pro TKL Gen 3 Wireless      # OpenRGB's name for the same board
```

- **An OpenRGB profile can still write to the board.** A profile applied at
  login that includes the keyboard sets OpenRGB's Direct mode, and two
  programs then send it frames in turn. Leave the keyboard out of any such
  profile. hotaru does not edit OpenRGB profiles.

**Permissions.** Both cooler nodes need a udev rule that tags the device
`uaccess`: the `/dev/hidraw*` node, and the `/dev/bus/usb/BBB/DDD` node behind
which the panel's bulk endpoint lives. Without that rule `systemd-logind` puts
no ACL on them, and hotaru finds a cooler it cannot open. The package ships the
rule. See [docs/packaging.md](packaging.md). A canvas device needs the same
tag on its hidraw node, and the package's rule covers every SteelSeries
device.

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
- **Device lighting persistence.** OpenRGB sets volatile state. hotaru handles
  devices that restore onboard colour on wake by rewriting the colour on a
  timer, rather than by writing device profiles.
- **Coolers other than the NZXT models above.** hotaru speaks NZXT's protocol.
  Another vendor's cooler needs a driver rather than a configuration entry, and
  hotaru does not have one.
- **Anything that needs a vendor's own daemon.**

## Checking your own machine

```bash
openrgb --list-devices        # what OpenRGB sees
hotaru light list             # what hotaru sees, with modes and scope
hotaru light health           # why, if it sees nothing
hotaru light probe            # what each device can actually do
hotaru cooling                # the cooler, or the fact that there is not one
hotaru light release apex     # a canvas device's own lighting back
```

If `hotaru light list` shows nothing while `openrgb --list-devices` shows
devices, the server is not running, or it enumerated late. `health` reports
which. OpenRGB detects devices once, at server start, so a device connected
afterwards stays invisible until the server restarts.
