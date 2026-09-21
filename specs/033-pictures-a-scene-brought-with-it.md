# Spec 033: pictures a scene brought with it

**Issue**: [#74](https://github.com/ushineko/hotaru/issues/74)

## Status: COMPLETE

## Context

A scene written before hotaru kept a picture library names a file wherever it
happened to be -- under `~/Pictures`, on another disk, in a directory that is
about to be tidied up. Nine scenes on the development machine did.

It works, until the file moves. And the window cannot offer it: the chooser
lists what hotaru keeps rather than every GIF on the machine, so a scene made
by hand could not be edited in the same place as one made in the window.

`hotaru scene adopt` copies the picture into the library and points the scene
at the copy, which is what a scene made today would have. The original is left
alone -- it is somebody's file, not hotaru's -- and nothing is adopted twice.

A command rather than a migration on startup: it rewrites somebody's saved
work, and that is a thing to run once, on purpose, and be able to see the
result of.

## Requirements

**R1. A scene pointing outside the library is adopted**: the file is copied
in and the scene points at the copy.

**R2. The original is untouched.**

**R3. Nothing else is**: a dashboard, the readout, a scene already inside the
library, and a file that is no longer there are all left alone.

**R4. A name already taken is not written over.**

## Acceptance Criteria

- [x] AC1. A scene naming a file outside the library points at a library
      picture afterwards, and the file is still where it was.
- [x] AC2. Running it again adopts nothing and says so.
- [x] AC3. A scene naming a missing file is reported and the run continues.
- [x] AC4. Verified on the development machine: nine scenes adopted, and they
      are selectable in the window's chooser.

## Risks & Assumptions

- **It rewrites scenes**, which is somebody's saved work. It is a command
  they run, it prints every change, and the pictures it copies are copies.
- **The library is one directory**, so adopting duplicates a file somebody
  already had. That is the point: the library is what hotaru can promise is
  still there.
- **Rollback** is editing the scenes file back, or re-pointing a scene with
  `hotaru scene write --screen`. The originals are untouched either way.
