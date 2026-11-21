# Credits

Every device hotaru drives was understood through someone else's open-source
work before a line of this program was written. This page says whose.

## The protocols

| Project | Licence | What it taught |
|---|---|---|
| [liquidctl](https://github.com/liquidctl/liquidctl) | GPL-3.0 | The NZXT Kraken 2023 / Elite protocol: the status report, the LCD's bucket commands and bulk header, the memory placement, draining queued reports before an ask. Spec 012 read the protocol off liquidctl and wrote hotaru's own driver in Go; spec 059 moved that driver to [sanshoku](https://github.com/ushineko/sanshoku), whose [credits](https://github.com/ushineko/sanshoku/blob/main/docs/credits.md) carry the detail. Its raw-frame path is the lead for streaming to the panel (sanshoku #27). |
| [PawnIO.Modules](https://github.com/namazso/PawnIO.Modules) | LGPL-2.1 | `LpcIO.bin`, `SmbusPIIX4.bin` and `SmbusI801.bin`, carried unchanged from release 0.2.12 with their licence in `third_party/pawnio` and installed beside the program, for the PawnIO helper (spec 083). They run in [PawnIO](https://pawnio.eu/)'s driver, which the person installs. |
| [OpenRGB](https://openrgb.org/) | GPL-2.0 | Every lit device, until spec 076. hotaru drove OpenRGB's server over its SDK protocol until sanshoku drew every device itself (specs 060, 075 and 076). sanshoku learned most of its lighting protocols from OpenRGB's controllers; its [credits](https://github.com/ushineko/sanshoku/blob/main/docs/credits.md) carry the detail. Device and zone names follow OpenRGB's, so records and scenes made through it still find their devices. |
| The Linux kernel | GPL-2.0 | hidraw and usbfs, hwmon by label, `/proc/stat` and `/proc/meminfo`, the DRM busy counter. |

## Built on

| Library | Licence | For |
|---|---|---|
| [sanshoku](https://github.com/ushineko/sanshoku) | MIT | Every device hotaru touches: the cooler's telemetry and LCD, the hwmon tables, and the lighting of every device hotaru draws (`lighting.Canvas`). |
| [Fyne](https://github.com/fyne-io/fyne) | BSD-3-Clause | The window. |
| [fynedesygn](https://github.com/ushineko/fynedesygn) | MIT | The window's shell, theme and widgets. |
| [Cobra](https://github.com/spf13/cobra) | Apache-2.0 | The command line. |

## Where it came from

The Python prototype that preceded hotaru ran liquidctl per call and read the
cooler in 105 ms of interpreter start-up; spec 012 is the record of why the
program reads the device itself.
