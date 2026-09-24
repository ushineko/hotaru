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

The cooler is the exception, and the only place where hotaru carries a driver
of its own. It writes NZXT's protocol directly over `/dev/hidraw` and usbfs,
with no Python, no subprocess and no cgo. That gives a short device list
rather than a borrowed one. See
[spec 012](../specs/012-the-cooler-without-liquidctl.md).

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

**Cooler telemetry: hotaru itself.** The kernel has no driver for recent NZXT
coolers. `nzxt-kraken3` matches 2007, 2014, 3008, 300C and 300E, so a Kraken
Elite V2 has no hwmon node and `sensors` reports nothing at all. hotaru reads
the device over `/dev/hidraw`, which costs about two milliseconds and starts no
processes.

**The panel: hotaru itself, on coolers that have one.** The Kraken Z and Elite
families store images in sixteen buckets of their own memory. Most coolers
have nothing of the kind. The firmware does not retain a still picture, so
hotaru sends one-frame GIFs.

**Temperatures: the kernel.** The CPU package comes from `coretemp`, read by
label. The graphics card comes from `amdgpu` or `nouveau` where they are
present. NVIDIA's own driver registers no hwmon, so that card's number comes
from `nvidia-smi`. This is why `nvidia-utils` is an `optdepends` rather than a
dependency: a machine without it shows a placeholder rather than an error.

**Permissions.** Both cooler nodes need a udev rule that tags the device
`uaccess`: the `/dev/hidraw*` node, and the `/dev/bus/usb/BBB/DDD` node behind
which the panel's bulk endpoint lives. Without that rule `systemd-logind` puts
no ACL on them, and hotaru finds a cooler it cannot open. The package ships the
rule. See [docs/packaging.md](packaging.md).

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
```

If `hotaru light list` shows nothing while `openrgb --list-devices` shows
devices, the server is not running, or it enumerated late. `health` reports
which. OpenRGB detects devices once, at server start, so a device connected
afterwards stays invisible until the server restarts.
