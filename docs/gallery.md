# The window, in pictures

What hotaru's window looks like, one image per part of it. These images live
here rather than in the README because the README *is* the About section. A
gallery of the window, inside the window, shows you pictures of the program
you are already looking at.

`tools/screenshot.sh` captures the images. It starts a window per image and
grabs it. Nobody clicks anything by hand for the set below.

## The set

Each image is
[docs/img/window-&lt;section&gt;.png](img/), in Breeze Dark, at the window's
default size.

### Service

![The Service section: the connection to OpenRGB, the devices it reports, and
the remedies for a service that is not answering.](img/window-service.png)

Health, what the service is connected to, and what to do when it is not
answering. Each remedy names the command to run rather than the symptom.

### System

![The System section: the machine's devices drawn as rows of coloured blocks
proportional to their LED counts, with the cooler's readings and the scene
currently loaded.](img/window-system.png)

Every device, with its zones drawn proportionally and in the colours they
show. The cooler's numbers and the display it found are here. So is the state:
the scene applied last, and what the panel draws now.

### Scenes

![The Scenes section: one row per scene with its shortcut key, a colour
swatch, what it lights and the buttons that apply, edit and delete
it.](img/window-scenes.png)

The bank, in the order of the keys the scenes sit on. The key is a button.
Somebody recognises a scene by its key, so that is where they reach to change
it.

### Pictures

![The Pictures section: a grid of thumbnails, each with its name, size and
frame count, and three icons under it.](img/window-pictures.png)

The library, as tiles. A picture is a picture. What tells two of them apart is
what they look like.

### Screen

![The Screen section: a table of saved dashboards, each with a thumbnail of
the frame it draws, its arrangement, theme and background.](img/window-screen.png)

The dashboards the cooler's panel can draw, each with a picture of the frame
the service renders for it.

### Appearance

![The Appearance section: colour scheme, font, text size and interface scale,
with the navigation's shape.](img/window-appearance.png)

The same Appearance section every program on fynedesygn has. Fyne draws its
own widgets, so this section is the whole of what makes the window match the
desktop it runs on.

### About

![The About section: this project's README rendered in the window, with the
architecture diagram.](img/window-about.png)

The README, rendered, with the architecture diagram drawn at build time. One
description of hotaru rather than two.

## The ones that need a hand

Two states are worth showing, and a flag cannot reach either one, because a
click reaches them: the **scene editor** with a light selected, and a chooser
part way through a dialog. Open the state yourself and capture the window as
it stands:

```console
$ tools/screenshot.sh --current docs/img/window-editor.png
$ tools/screenshot.sh --current --with-dialog docs/img/window-effects.png
```

`--with-dialog` is for the second one. An open dialog *is* the active window,
so a grab of the active window returns the dialog floating on nothing. That
mode captures the desktop and crops to the window.

## Refreshing them

```console
$ make gui                      # the images are of the build, not of the package
$ tools/screenshot.sh --all
```

It takes about a minute. It raises windows and takes over the screen for that
time, so it cannot run while somebody is using the machine.

**The service must be running.** The window is a client, and a capture of "the
service is not running" is a screenshot of a machine nobody has. The images
therefore show this desk's real devices and scenes, which is the point. A
gallery of invented data is a gallery of a program nobody ran.

### Except the picture library

The Pictures image is the one exception, and it is deliberate. A picture
library holds somebody's own photographs: a dog, a family in a car, a
screenshot of an employer's dashboard. This file is published. The capture
therefore uses a library of stock wallpapers:

```console
$ L=~/.local/share/hotaru/images
$ mv "$L/images" "$L/images.real" && cp -r <stock> "$L/images"
$ SETTLE=6 tools/screenshot.sh --section Pictures docs/img/window-pictures.png
$ rm -rf "$L/images" && mv "$L/images.real" "$L/images"
```

The service reads the directory on each request, so nothing restarts and the
swap lasts as long as the capture. `--socket` exists for the tidier version of
this, a throwaway service with its own library, and this procedure does not
use it. A second service on a machine with a cooler is a second writer to the
same hardware, which is the one thing hotaru does not allow.

Everything visible in that image is a wallpaper: nebulae, a Mandelbrot set, a
spectrum. The scene *names* in the Scenes image are this desk's own, which is
a different thing from its photographs.

### When to refresh

- **A section changes shape.** A new control, a moved one, a different layout.
  A copy edit does not count.
- **Before a release that changes the window.** The images are the first thing
  anybody sees of hotaru, and a stale one promises what the program does not
  do.
- **When the theme moves.** A change to fynedesygn's default scheme makes
  every image wrong at once.

Whenever an image changes, **read its alt text again**. That text is the only
description a screen-reader user receives. A wrong one is worse than none, and
it is the half of this that rots without anybody seeing it.
