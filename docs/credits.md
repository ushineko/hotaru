# Credits

Every device hotaru drives was understood through someone else's open-source
work before a line of this program was written. This page says whose.

## The protocols

| Project | Licence | What it taught |
|---|---|---|
| [liquidctl](https://github.com/liquidctl/liquidctl) | GPL-3.0 | The NZXT Kraken 2023 / Elite protocol: the status report, the LCD's bucket commands and bulk header, the memory placement, draining queued reports before an ask. Spec 012 read the protocol off liquidctl and wrote hotaru's own driver in Go; spec 059 moved that driver to [sanshoku](https://github.com/ushineko/sanshoku), whose [credits](https://github.com/ushineko/sanshoku/blob/main/docs/credits.md) carry the detail. Its raw-frame path is the lead for streaming to the panel (sanshoku #27). |
| [OpenRGB](https://openrgb.org/) | GPL-2.0 | Every lit device. hotaru does not speak to a lighting controller itself; OpenRGB's server does, and hotaru drives it over its SDK protocol. The one standing exception to the direct-access rule, for that breadth. The exception to the exception is a canvas device such as the SteelSeries Apex Pro TKL Wireless Gen 3, whose frames hotaru streams through sanshoku (spec 060); sanshoku learned that frame and the board's onboard command from OpenRGB's SteelSeries controller and a capture of SteelSeries GG. |
| The Linux kernel | GPL-2.0 | hidraw and usbfs, hwmon by label, `/proc/stat` and `/proc/meminfo`, the DRM busy counter. |

## Built on

| Library | Licence | For |
|---|---|---|
| [go-openrgb-sdk](https://github.com/csutorasa/go-openrgb-sdk) | MIT | The OpenRGB SDK protocol from Go. |
| [sanshoku](https://github.com/ushineko/sanshoku) | MIT | The cooler: telemetry and the LCD, and the hwmon tables. The frame stream a canvas device is drawn with (`lighting.Canvas`). |
| [Fyne](https://github.com/fyne-io/fyne) | BSD-3-Clause | The window. |
| [fynedesygn](https://github.com/ushineko/fynedesygn) | MIT | The window's shell, theme and widgets. |
| [Cobra](https://github.com/spf13/cobra) | Apache-2.0 | The command line. |

## Where it came from

The Python prototype that preceded hotaru ran liquidctl per call and read the
cooler in 105 ms of interpreter start-up; spec 012 is the record of why the
program reads the device itself.
