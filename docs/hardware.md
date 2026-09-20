# Supported hardware

Two things, and they are different claims:

1. **What has actually been tested** — hardware someone ran hotaru against and
   watched. That list is short and it is below.
2. **What is expected to work** — everything OpenRGB and liquidctl support,
   because hotaru contains no device drivers of its own. That list belongs to
   those projects and is linked rather than copied.

hotaru does not maintain a device list. If OpenRGB can set your keyboard's
colour, so can hotaru; if liquidctl cannot see your cooler, neither can hotaru,
and no configuration will change it. This is also the answer to why installing
hotaru installs those packages: they are where the hardware support lives.

## Tested

"Works" here means observed on the hardware, not inferred from a support list.

### The development machine

| Device | Through | Notes |
|---|---|---|
| NZXT Kraken 2024 Elite (`1e71:3012`) | both | Lighting via OpenRGB — the radiator fans' RGB daisy-chains into it. Telemetry and the 640x640 LCD via liquidctl. liquidctl exposes **no** colour channels for this model |
| MSI GeForce RTX 4090 Suprim Liquid X | OpenRGB | Accepts `direct/breathing/flashing/off`; **rejects `static`** and goes dark if sent it |
| ASUS ROG Maximus Z790 Hero | OpenRGB | Onboard LEDs plus four addressable headers. Static drives only the onboard LED — the headers need Direct |
| Corsair MM700 | OpenRGB | Also managed by OpenLinkHub; exclude it there so one controller owns it |
| Logitech G502 X PLUS | OpenRGB | Wireless: restores onboard state on wake, so its colour is re-asserted on a timer. Solaar cannot set its colour at all — its CLI silently drops the colour argument |
| Keychron K4 HE | OpenRGB | Advertises no Off mode, so `off` would resolve to Direct with black and kill the backlight. Coloured, never blanked |
| Corsair HX1000i | OpenLinkHub | Visible, but its "Probe" temperature channels are **not** coolant and must not be read as such |

### Test systems

| Machine | Status |
|---|---|
| CachyOS, unrelated hardware, OpenRGB already running in service mode | **Pending.** The acceptance test: installing hotaru there and using it must take no extra steps, with no configuration written by hand. Results recorded here |

Every quirk in the first table is a correction hotaru applies by *discovering*
it — reading back what a write actually did — rather than by matching a device
name against a table. The table records what was learned, not what the program
requires.

## Everything else

Support comes from three upstream projects. Their device lists are the
authoritative answer for hardware not tested above:

| Package | Version developed against | What it provides | Its device list |
|---|---|---|---|
| `openrgb` | 1.0 | Every lit device: GPUs, motherboards, RAM, keyboards, mice, mousepads, cases, coolers, strips | [openrgb.org/devices](https://openrgb.org/devices_1.0rc3.html) |
| `liquidctl` | 1.16.0 | Liquid coolers, some PSUs and fan controllers — coolant temperature, pump speed, and the LCD | [liquidctl supported devices](https://github.com/liquidctl/liquidctl#supported-devices) |
| `openlinkhub` | 0.9.1 | Corsair iCUE Link and Commander hardware. hotaru uses it for the CPU package temperature, and as a cooler fallback | [OpenLinkHub](https://github.com/jurkovic-nikola/OpenLinkHub) |

Minimum versions are not pinned artificially. hotaru speaks OpenRGB's SDK
protocol, negotiated at connect and reported by `hotaru light health`, and calls
liquidctl through its documented JSON output. A backend too old for your device
is a reason to upgrade that backend, not hotaru — and a device missing from
`openrgb --list-devices` or `liquidctl list` is an upstream matter, not a hotaru
one.

### What each contributes

**Lighting — OpenRGB.** Colours on any device it enumerates, per device, per
zone, or per LED. Per-LED control usually requires the device's Direct mode;
devices that render Static as a single colour cannot show a multi-colour frame,
and hotaru reports that rather than approximating it. hotaru talks to the
running OpenRGB *server*, so lighting needs `openrgb` installed **and** its
server running.

**Cooler telemetry — liquidctl.** For devices where the kernel has no hwmon
driver, which is the common case on recent NZXT coolers: `nzxt-kraken3` matches
2007/2014/3008/300C/300E, so a Kraken Elite V2 has no hwmon node and `sensors`
reports nothing at all.

**The LCD — liquidctl, on coolers that have a screen.** The Kraken Z and Elite
families expose `set screen`; most coolers expose nothing of the kind. The
panel's resolution is read from the device rather than assumed.

**CPU package temperature — OpenLinkHub.** One field. It is a hard dependency
because a user cannot be expected to know which field comes from which daemon,
not because it does much.

## Not supported, deliberately

- **Fan and pump duty control.** The Commander ST firmware silently discards
  duty writes from both liquidctl and OpenLinkHub — a control would report
  success and change nothing, so there is none.
- **Device lighting persistence.** OpenRGB sets volatile state. Devices that
  restore onboard colour on wake are handled by re-asserting on a timer, not by
  writing device profiles.
- **Anything needing a vendor's own daemon** beyond the three above.

## Checking your own machine

```bash
openrgb --list-devices        # what OpenRGB sees
liquidctl list                # what liquidctl sees
hotaru light list             # what hotaru sees, with modes and scope
hotaru light health           # why, if it sees nothing
hotaru light probe            # what each device can actually do
```

`hotaru light list` showing nothing while `openrgb --list-devices` shows devices
means the server is not running, or enumerated late — `health` says which.
OpenRGB detects devices once, at server start, so a device connected afterwards
is invisible until it is restarted.
