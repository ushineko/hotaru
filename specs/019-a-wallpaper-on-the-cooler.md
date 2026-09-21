# Spec 019: a wallpaper on the cooler

**Issue**: [#5](https://github.com/ushineko/hotaru/issues/5)

## Status: COMPLETE

## Context

The panel takes 640x640 GIFs and nothing else. Spec 016 found out what happens
when it is handed anything else: the transfer succeeds, the bucket switch
succeeds, and the screen goes blank -- no error on either side. A person with a
wallpaper they like has no way in.

What they have is a JPEG, four thousand pixels wide, and an expectation that a
program showing pictures can show it.

### The conversion belongs to the service

It could live in the window. It should not: the CLI would then be unable to do
something the GUI can, which is the parity rule falling over the first time it
is tested. And a library of converted images is a directory somebody's two
shells both write into, which is the one-writer question again with a worse
answer.

So `hotaru image add wallpaper.jpg` and the window's **Add an image** are the
same request, the service converts and stores, and both shells list what is
there.

### What conversion means here

Scaled to the panel's square, then written as a GIF, because a still picture is
not retained by this firmware and one frame of a GIF is (spec 012). An animated
GIF keeps its frames and its timing.

**Cropped to fill, not letterboxed.** A wallpaper is a wide picture and a panel
is a circle in a case: bars top and bottom of somebody's photograph look like a
mistake, where a centre crop looks like a photograph. The alternative is
offering a choice nobody wants to make twice.

### The library is data, not configuration

`$XDG_DATA_HOME/hotaru/images`. Not the config directory, because these are
files somebody added rather than settings they chose, and not the state
directory, because losing them would lose work rather than a cache.

A scene refers to one by its path, which is what a scene already does with a
GIF. Nothing new is needed in the scene format, and a scene written before this
existed keeps working.

## Requirements

**R1. Any image the standard library reads.** JPEG, PNG and GIF in; a GIF the
panel takes out.

**R2. Converted once, stored, and listed.** Adding the same name twice replaces
it. The list says what each one is: its name, its size on disk, and whether it
moves.

**R3. Both shells.** `hotaru image add|list|remove`, and the same three things
in the window, over one API.

**R4. The window can preview one on the panel** and put back what was there
afterwards, the way the scene editor's preview does.

**R5. A scene can name a stored image**, through the same `screen` field a
scene already has.

**R6. An image that cannot be read says so**, naming the file. A directory of
holiday photographs contains one thing that is not an image, and the message is
the only way anybody finds out which.

## Acceptance Criteria

- [x] AC1. A JPEG of any size is converted to a 640x640 GIF and stored.
- [x] AC2. A PNG and an animated GIF convert too, the GIF keeping its frames.
- [x] AC3. A wide image is cropped to the square rather than letterboxed.
- [x] AC4. The stored file is a GIF the panel accepts, asserted against the
      same check the screen path uses.
- [x] AC5. `hotaru image list` names what is stored, and `remove` removes it.
- [x] AC6. Adding a name that exists replaces it.
- [x] AC7. Something that is not an image is refused, by name.
- [x] AC8. The window lists the library, adds to it, and previews one on the
      panel, restoring what was showing afterwards.
- [x] AC9. A scene naming a stored image applies it.
- [x] AC10. Verified on the development machine: a wallpaper on the cooler.
- [x] AC11. A picture dragged onto the window from a file manager is converted,
      wherever on the window it lands.
- [x] AC12. The conversion is shown before anything is kept, because the crop
      is the decision being asked for.

## Verified on hardware

Development machine, with somebody looking at the panel and at the window.

A 3840x2160 JPEG became a 198 KB GIF in a third of a second and appeared on the
cooler. A wallpaper dragged from the file manager onto the window did the same
thing without the file chooser being opened at all.

### The palette had to come from the picture

The first conversion used Plan 9's fixed palette and a picture of a red storm
on a blue planet came back speckled: half of those 256 colours are greens and
greys that image had no use for, so the dithering was spending its budget
avoiding colours that were not there. Choosing the 256 from a histogram of the
picture itself is a different photograph -- smooth clouds, and the storm still
red. It costs 143 KB to 198 KB, which is well inside the panel's floor.

### Two things the window got wrong, and how they were found

**A dialog shown from inside `Perform` disappears.** The busy indicator is a
modal popup, and Fyne's overlay stack removes everything above an overlay when
it goes -- so the preview dialog was built, shown, and taken down by the
operation that built it finishing. Every step logged success and the window did
nothing. It was found by a line of diagnostics per step, the same instrument
that made the LCD's silence legible in spec 013, and those lines are still
there.

**A `FileDialog` cannot be resized before it is shown.** Its `Resize`
dereferences the window `Show` builds, so sizing one first is a nil pointer --
a crash rather than a small dialog, and it went in front of somebody. There is
a test now that calls the real type.

## Risks & Assumptions

- **640x640 is this panel.** The size comes from the same constant the
  dashboard uses, and a cooler with a different screen would need it measured
  rather than assumed -- the caution spec 013 already carries.
- **Conversion is lossy and one-way.** The original is not kept: hotaru stores
  what it can show, and the person still has their wallpaper.
- **A big animation is a big file.** The panel's memory is about 24 MB and the
  push floor scales with frame size, so a long animation converts to something
  slow to send. It is stored anyway and the floor does the rest.
- **Rollback** is deleting the directory; scenes naming a missing file already
  report it and apply their colours.
