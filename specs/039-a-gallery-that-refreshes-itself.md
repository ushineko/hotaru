# Spec 039: a gallery that refreshes itself

**Issue**: [#97](https://github.com/ushineko/hotaru/issues/97)

## Status: COMPLETE

## Context

The window had no pictures anywhere. Everything about it was described in
words, including to somebody deciding whether to install it.

The capture is scripted rather than done by hand, because screenshots taken by
hand are screenshots nobody refreshes: `tools/screenshot.sh` starts a window
per image, opens it on the section it wants, waits for it to have focus, grabs
it, and stops it again. A whole set is one command with nothing to click,
which is the difference between images that track the interface and images
that quietly go stale.

Ported from fynedesygn's, which came from angou's. What is hotaru's own:

- **`--current`**, for the states a flag cannot reach. The scene editor with a
  light selected and a chooser mid-dialog are worth showing and are reached by
  clicking; somebody sets the state up and this grabs the window as it stands.
- **`$XDG_RUNTIME_DIR` is left alone** while HOME and the XDG config and data
  directories are thrown away. That is where the Wayland socket *and* hotaru's
  own socket live: the window is a client, and a capture of "the service is
  not running" is a picture of a machine nobody has.
- **The window learned `--section` and `--scheme`**, which is the flag the
  script needs and a thing somebody might want anyway: `--section Pictures`
  opens straight there. Create's parts are named, because "open on Pictures"
  is how a person says it.

### A separate document

`docs/gallery.md`, not the README, because the README *is* the About section:
a gallery of the window inside the window is a program showing pictures of
itself, and 2 MB of them in a file the window parses on every visit.

### Somebody's photographs are not the program

The images show this desk's real devices and scenes, which is the point. The
picture library is the exception: it is a dog, a family in a car and a
screenshot of an employer's dashboard, and this is a public repository. That
one image is captured against a library of stock wallpapers, swapped in for
the length of the capture -- the service reads the directory per request, so
nothing restarts.

The tidier version of that is a throwaway service with its own library, and
`--socket` exists for it. It is not used: a second service on a machine with a
cooler is a second writer to the same hardware, which is the one thing this
program does not allow.

### And it found a bug in its first run

The status bar read **`0 of 0 devices · 0.0 °C`** under a grid of eighteen
pictures, with the service healthy the whole time.

It is repainted when a section is rebuilt, and a section that watches little
is never rebuilt -- Pictures watches only whether the service is there. So the
bar kept whatever it said when the window opened, which on a new window is
nothing. A window somebody has had open for an hour corrects itself the first
time anything changes, which is why this had never been seen: a screenshot
opens a new window every time.

The poll repaints it now, whatever the section says. A handful of labels
against a poll that already asked the service four questions.

## Requirements

**R1. One command refreshes the set**, with nothing clicked.

**R2. The window can be asked which section to open on**, including Create's
parts by name.

**R3. A capture cannot read this desk's settings** -- a throwaway HOME -- and
can reach the running service.

**R4. The published images contain no personal photographs.**

**R5. The gallery is its own document**, linked from the README rather than in
it.

**R6. The status bar follows the machine**, on every section.

## Acceptance Criteria

- [x] AC1. `make screenshots` writes the seven section images.
- [x] AC2. `--section` and `--scheme` work on `hotaru-gui`, and a part name
      opens the group with that part in front.
- [x] AC3. An unknown section name opens the first, rather than failing.
- [x] AC4. The capture finds its own window by pid, not the developer's copy
      of the program that is already running.
- [x] AC5. The Pictures image is a library of stock wallpapers, and the real
      library is back afterwards.
- [x] AC6. The status bar reads the machine in every image.
- [x] AC7. Verified on the development machine: seven images, and the bar
      going from "0 of 0 devices" to "healthy · 6 of 6 devices · 37.9 °C" in
      the same shot.

## Risks & Assumptions

- **The capture needs the desk.** It raises windows and takes over the screen
  for a minute; it cannot run while the machine is in use, and it cannot run
  on a machine with no session at all. That is a property of screenshots.
- **KDE and Wayland**, through kdotool and spectacle. The images are of this
  desktop; the window is the same everywhere but its decorations are not.
- **The status-bar fix is verified by capture** rather than by a test: the
  bar is repainted by the shell on a window, and a headless shell has no
  window to repaint. What a test can reach is already covered.
- **Rollback** is a revert; the images are a directory of PNGs.
