# Supported hardware

Two things, and they are different claims:

1. **What has actually been tested** — hardware someone ran hotaru against and
   watched. That list is short and it is below.
2. **What is expected to work** — every lit device OpenRGB supports, because
   hotaru contains no lighting drivers of its own. That list belongs to OpenRGB
   and is linked rather than copied.

hotaru does not maintain a lighting device list. If OpenRGB can set your
keyboard's colour, so can hotaru; if it cannot see the device, neither can
hotaru, and no configuration will change it. This is also the answer to why
installing hotaru installs OpenRGB: that is where the lighting support lives.

The cooler is the exception, and the only place hotaru carries a driver of its
own. It speaks NZXT's protocol directly over `/dev/hidraw` and usbfs — no
Python, no subprocess, no cgo — which is a short device list rather than a
borrowed one. See [spec 012](../specs/012-the-cooler-without-liquidctl.md).

## Tested

"Works" here means observed on the hardware, not inferred from a support list.

### The development machine

| Device | Through | Notes |
|---|---|---|
| NZXT Kraken 2024 Elite (`1e71:3012`) | both | Lighting via OpenRGB — the radiator fans' RGB daisy-chains into it, and OpenRGB exposes **only** the colour channels. Telemetry and the 640x640 LCD are hotaru's own, over `/dev/hidraw` and usbfs. The two Hue 2 channels are separate controllers and are written separately (spec 011); the bucket being displayed is never written into, or the panel blanks for the length of the transfer (spec 013) |
| MSI GeForce RTX 4090 Suprim Liquid X | OpenRGB | Accepts `direct/breathing/flashing/off`; **rejects `static`** and goes dark if sent it |
| ASUS ROG Maximus Z790 Hero | OpenRGB | Onboard LEDs plus four addressable headers. Static drives only the onboard LED — the headers need Direct |
| Corsair MM700 | OpenRGB | Its logo has no blue channel, so a request for purple shows as dim red. Not a failed write, and nothing in the protocol says so — OpenRGB's own command line does the same (spec 009) |
| Logitech G502 X PLUS | OpenRGB | Wireless: restores onboard state on wake, so its colour is re-asserted on a timer. Solaar cannot set its colour at all — its CLI silently drops the colour argument |
| Keychron K4 HE | OpenRGB | Advertises no Off mode, so `off` would resolve to Direct with black and kill the backlight. Coloured, never blanked |
| Intel Core i9-14900K | kernel | CPU package temperature from `coretemp`, read by label. Never by hwmon index: the numbers are assigned in probe order and move between boots |
| MSI GeForce RTX 4090 | `nvidia-smi` | NVIDIA's driver registers no hwmon, so the dashboard's GPU reading comes from the tool — 18 ms, once per update. AMD and nouveau are read from `/sys/class/hwmon` like anything else |

### Test systems

| Machine | Status |
|---|---|
| CachyOS, unrelated hardware, OpenRGB already running in service mode | **Pending.** The acceptance test: installing hotaru there and using it must take no extra steps, with no configuration written by hand. Results recorded here |

Every quirk in the first table is a correction hotaru applies by *discovering*
it — reading back what a write actually did — rather than by matching a device
name against a table. The table records what was learned, not what the program
requires.

## Everything else

Lighting comes from one upstream project. Its device list is the authoritative
answer for hardware not tested above:

| Package | Version developed against | What it provides | Its device list |
|---|---|---|---|
| `openrgb` | 1.0 | Every lit device: GPUs, motherboards, RAM, keyboards, mice, mousepads, cases, coolers, strips | [openrgb.org/devices](https://openrgb.org/devices_1.0rc3.html) |
| `nvidia-utils` | — | Optional. The GPU temperature on the dashboard, where the kernel exposes none | — |

`liquidctl` and `openlinkhub` were dependencies and are not any more, which
took Python off machines that may have no liquid cooler at all. Either can
still be installed alongside hotaru; nothing in hotaru calls them.

Minimum versions are not pinned artificially. hotaru speaks OpenRGB's SDK
protocol, negotiated at connect and reported by `hotaru light health`. A server
too old for your device is a reason to upgrade it, not hotaru — and a device
missing from `openrgb --list-devices` is an upstream matter, not a hotaru one.

### What each contributes

**Lighting — OpenRGB.** Colours on any device it enumerates, per device, per
zone, or per LED. Per-LED control usually requires the device's Direct mode;
devices that render Static as a single colour cannot show a multi-colour frame,
and hotaru reports that rather than approximating it. hotaru talks to the
running OpenRGB *server*, so lighting needs `openrgb` installed **and** its
server running.

**Cooler telemetry — hotaru itself.** The kernel has no driver for recent NZXT
coolers: `nzxt-kraken3` matches 2007/2014/3008/300C/300E, so a Kraken Elite V2
has no hwmon node and `sensors` reports nothing at all. hotaru asks the device
over `/dev/hidraw`, which costs about two milliseconds and no processes.

**The LCD — hotaru itself, on coolers that have a screen.** The Kraken Z and
Elite families take images into sixteen buckets of their own memory; most
coolers have nothing of the kind. A still picture is not retained by the
firmware, so hotaru sends one-frame GIFs.

**Temperatures — the kernel.** CPU package from `coretemp` by label, and the
graphics card from `amdgpu` or `nouveau` where they are there. NVIDIA's own
driver registers no hwmon, so that one card's number comes from `nvidia-smi`,
which is why `nvidia-utils` is an `optdepends` rather than a dependency: a
machine without it shows a placeholder, not an error.

**Permissions.** Both cooler nodes — the `/dev/hidraw*` and the
`/dev/bus/usb/BBB/DDD` the screen's bulk endpoint lives behind — need a udev
rule tagging the device `uaccess`, or `systemd-logind` puts no ACL on them and
hotaru finds the cooler it cannot open. The package ships that rule; see
[docs/packaging.md](packaging.md).

## Not supported, deliberately

- **Fan and pump duty control.** The Commander ST firmware silently discards
  duty writes — it did so from liquidctl and from OpenLinkHub alike, which is
  where this was learned. A control would report success and change nothing, so
  there is none.
- **Device lighting persistence.** OpenRGB sets volatile state. Devices that
  restore onboard colour on wake are handled by re-asserting on a timer, not by
  writing device profiles.
- **Coolers other than the NZXT models above.** The protocol hotaru speaks is
  NZXT's. Another vendor's cooler is not a configuration away; it is a driver,
  and the honest answer is that it is absent.
- **Anything needing a vendor's own daemon.**

## Checking your own machine

```bash
openrgb --list-devices        # what OpenRGB sees
hotaru light list             # what hotaru sees, with modes and scope
hotaru light health           # why, if it sees nothing
hotaru light probe            # what each device can actually do
hotaru cooling                # the cooler, or the fact that there is not one
```

`hotaru light list` showing nothing while `openrgb --list-devices` shows devices
means the server is not running, or enumerated late — `health` says which.
OpenRGB detects devices once, at server start, so a device connected afterwards
is invisible until it is restarted.
