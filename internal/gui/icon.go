package gui

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

/*
The window's icon, compiled in.

Also shipped as a file, because three separate things have to agree before a
Fyne window shows an icon on KDE under Wayland, and this is only the first:

 1. this resource, which is the in-app icon and the X11 window icon;
 2. a desktop entry whose basename is the Wayland app_id, since the compositor
    resolves the *titlebar* icon by matching app_id to a desktop file of that
    name, and its Icon= then resolves through the icon theme;
 3. StartupWMClass in that entry, which is what the task manager matches on --
    without it the titlebar has an icon and the taskbar does not.

And then the trap: with all three right the taskbar can still be blank, because
plasmashell caches "no icon" for an app_id it has already failed to resolve.
Every run made while the entry was missing poisons it, `kbuildsycoca6` does not
clear it, and restarting plasmashell does. See docs/packaging.md.
*/
//go:embed assets/hotaru.svg
var iconSVG []byte

// Icon is the bulb, as a Fyne resource.
func Icon() fyne.Resource { return fyne.NewStaticResource("hotaru.svg", iconSVG) }
