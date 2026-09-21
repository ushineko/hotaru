# Spec 034: a picture is a picture

**Issue**: [#77](https://github.com/ushineko/hotaru/issues/77)

## Status: COMPLETE

## Context

The library was a column of cards. Each card gave the picture a square an inch
across and the rest of the line to its size, its frame count and the directory
every picture in the library is in -- so eighteen pictures were eighteen
screens of mostly nothing, and finding one meant scrolling past the same path
eighteen times.

What tells two pictures apart is what they look like. So: a grid of tiles, the
picture above its name, the three things worth doing with it as icons under
it, and the path gone -- it is the library's directory, which is the one thing
every row agreed on.

`GridWrap` reflows to the width it is given, so a wide window shows a row of
six and a narrow one a row of two, which is what somebody resizing a window
full of pictures expects.

### And it took seconds to appear

The thumbnails were decoded inline. Spec 027 made each one cheap -- the first
frame, scaled once, kept in the shared cache -- but the first visit still read
eighteen 640x640 GIFs from disk on the thread drawing the window, and so did
any visit after the cache had been trimmed.

A tile now asks the cache and draws what it has. What it does not have is
decoded in a goroutine and set in place, which is what the dashboard
thumbnails already did (spec 027). Arriving at the section warms them off the
UI thread as well, because arriving is the moment they are about to be drawn.

A picture the library lists and the disk does not have says so in the tile
rather than leaving a blank square.

### And the seconds were not the decoding

Said with a profile rather than from the code, after the first fix did not
make the section appear any faster.

Listing the library decoded every picture in it. `Image.Frames` is what says
whether a picture moves, and it was counted with `gif.DecodeAll`, which undoes
the LZW compression of every frame of every file to tell you how many there
are. This machine's library is 110 MB of animation, so `hotaru image list`
took **1.28 seconds** -- and the window asks for that list whenever the
Pictures section is drawn, and again for the screen chooser.

A frame is an image descriptor, which is a byte. Walking the file's blocks --
fixed-size headers, colour tables whose size is in the byte before them,
chains of length-prefixed sub-blocks -- counts them without decoding a pixel:
**0.019 seconds** for the same eighteen files, a factor of sixty-seven.

The cheap count has to agree with the expensive one, so a test asserts it does
against `gif.DecodeAll` for a still, a two-frame and a seventeen-frame
picture, and that a file that is not a GIF counts as one rather than failing
the listing.

## Requirements

**R1. Pictures are a grid** that reflows to the window's width.

**R2. A tile is the picture, its name, what it costs, and its three actions.**

**R3. Nothing is decoded on the thread drawing the window.**

**R4. A picture that cannot be read says so.**

**R5. Listing the library does not decode it.**

## Acceptance Criteria

- [x] AC1. Two pictures are side by side at a wide window, not in a column.
- [x] AC2. A tile carries the size and the frame count, and the frame count
      only when there is more than one.
- [x] AC3. A built section's thumbnails arrive afterwards, and a warmed
      section's are there on the first build.
- [x] AC4. The frame count read off the blocks equals the one from
      `gif.DecodeAll`, and a file that is not a GIF counts as one.
- [x] AC5. Verified on the development machine, with eighteen pictures and
      110 MB of animation: `hotaru image list` went from 1.28 s to 0.019 s.

## Risks & Assumptions

- **The tile size is fixed**, so a long name is truncated. The picture is
  what somebody is choosing between; the name says which one they chose.
- **Icons rather than words** for the three actions, because three labelled
  buttons do not fit a tile this size. Each carries a tip, which is the only
  thing that says what an icon alone is.
- **Rollback** is a revert; nothing is stored.
