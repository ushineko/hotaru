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

## Requirements

**R1. Pictures are a grid** that reflows to the window's width.

**R2. A tile is the picture, its name, what it costs, and its three actions.**

**R3. Nothing is decoded on the thread drawing the window.**

**R4. A picture that cannot be read says so.**

## Acceptance Criteria

- [x] AC1. Two pictures are side by side at a wide window, not in a column.
- [x] AC2. A tile carries the size and the frame count, and the frame count
      only when there is more than one.
- [x] AC3. A built section's thumbnails arrive afterwards, and a warmed
      section's are there on the first build.
- [x] AC4. Verified on the development machine, with eighteen pictures.

## Risks & Assumptions

- **The tile size is fixed**, so a long name is truncated. The picture is
  what somebody is choosing between; the name says which one they chose.
- **Icons rather than words** for the three actions, because three labelled
  buttons do not fit a tile this size. Each carries a tip, which is the only
  thing that says what an icon alone is.
- **Rollback** is a revert; nothing is stored.
