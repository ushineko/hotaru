# The window, in pictures

What hotaru's window looks like, one image per part of it. Kept here rather
than in the README because the README *is* the About section: a gallery of the
window, inside the window, is a program showing you pictures of itself.

The images are captured by `tools/screenshot.sh`, which starts a window per
image and grabs it. Nothing is clicked by hand for the set below.

## The set

Each image is 
[docs/img/window-&lt;section&gt;.png](img/), in Breeze Dark, at the window's
default size.

### Service

![The Service section: the connection to OpenRGB, the devices it reports, and
the remedies for a service that is not answering.](img/window-service.png)

Health, what it is connected to, and what to do about it when it is not. The
remedies name the command rather than the symptom.

### System

![The System section: the machine's devices drawn as rows of coloured blocks
proportional to their LED counts, with the cooler's readings and the scene
currently loaded.](img/window-system.png)

Every device, with its zones drawn proportionally and in the colours they are
showing. The cooler's numbers and the display it found are here, and so is
what is loaded: the scene last applied, and what the panel is drawing.

### Scenes

![The Scenes section: one row per scene with its shortcut key, a colour
swatch, what it lights and the buttons that apply, edit and delete
it.](img/window-scenes.png)

The bank, in the order of the keys the scenes sit on. The key is a button:
it is what somebody recognises a scene by, and it is where they reach to
change it.

### Pictures

![The Pictures section: a grid of thumbnails, each with its name, size and
frame count, and three icons under it.](img/window-pictures.png)

The library, as tiles. A picture is a picture: what tells two of them apart is
what they look like.

### Screen

![The Screen section: a table of saved dashboards, each with a thumbnail of
the frame it draws, its arrangement, theme and background.](img/window-screen.png)

The dashboards the cooler's panel can be asked to draw, each with a picture of
the frame the service would render for it.

### Appearance

![The Appearance section: colour scheme, font, text size and interface scale,
with the navigation's shape.](img/window-appearance.png)

The same Appearance section every program on fynedesygn has. Fyne draws its
own widgets, so this is the whole of what makes the window look like it
belongs on the desktop it is running on.

### About

![The About section: this project's README rendered in the window, with the
architecture diagram.](img/window-about.png)

The README, rendered, with the architecture diagram drawn at build time. One
description of hotaru rather than two.

## The ones that need a hand

Two states are worth showing and cannot be reached by a flag, because they are
reached by clicking: the **scene editor** with a light selected, and a chooser
mid-dialog. For those, open the state and capture the window as it stands:

```console
$ tools/screenshot.sh --current docs/img/window-editor.png
$ tools/screenshot.sh --current --with-dialog docs/img/window-effects.png
```

`--with-dialog` is for the second one: when a dialog is open it *is* the
active window, so an active-window grab returns the dialog floating on
nothing. That mode captures the desktop and crops to the window.

## Refreshing them

```console
$ make gui                      # the images are of the build, not of the package
$ tools/screenshot.sh --all
```

It takes about a minute, during which it raises windows and takes over the
screen. It cannot run while the machine is being used.

**The service must be running.** The window is a client; a capture of "the
service is not running" is a screenshot of a machine nobody has. The images
therefore show this desk's real devices and scenes, which is the point: a
gallery of invented data is a gallery of a program that was never run.

### Except the picture library

The Pictures image is the one exception, and it is a deliberate one. A picture
library is somebody's own photographs -- a dog, a family in a car, a
screenshot of an employer's dashboard -- and this file is published. The grid
is captured against a library of stock wallpapers instead:

```console
$ L=~/.local/share/hotaru/images
$ mv "$L/images" "$L/images.real" && cp -r <stock> "$L/images"
$ SETTLE=6 tools/screenshot.sh --section Pictures docs/img/window-pictures.png
$ rm -rf "$L/images" && mv "$L/images.real" "$L/images"
```

The service reads the directory per request, so nothing is restarted and the
swap lasts as long as the capture. `--socket` exists for the tidier version of
this -- a throwaway service with its own library -- and is not used here,
because a second service on a machine with a cooler is a second writer to the
same hardware, which is the one thing this program does not allow.

Everything visible in that image is a wallpaper: nebulae, a Mandelbrot set, a
spectrum. The scene *names* in the Scenes image are this desk's own, which is
a different thing from its photographs.

### When to refresh

- **A section changes shape.** A new control, a moved one, a different layout.
  Not a copy edit.
- **Before a release that changes the window.** The images are the first thing
  anybody sees of it, and a stale one is a promise the program does not keep.
- **When the theme moves.** fynedesygn's default scheme changing makes every
  image wrong at once.

And whenever an image changes, **read its alt text again**. That text is the
only description a screen-reader user gets; a wrong one is worse than none,
and it is the half of this that rots silently because nobody sees it.
