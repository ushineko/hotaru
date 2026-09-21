# Spec 037: the words and the numbers

**Issue**: [#88](https://github.com/ushineko/hotaru/issues/88)

## Status: COMPLETE

## Context

The panel's text was the arrangement's decision and nobody else's: spec 013's
sizes, the theme's colours, and an outline that appeared over a picture and
nowhere else. Every one of those is right as a default and none of them is
right for every desk -- a panel two feet away wants smaller numbers than one
across a room, and a photograph with a bright corner wants a thicker edge than
two pixels.

So a dashboard carries its own lettering: a face, and a size, a colour and an
edge for each of the two kinds of text on it.

### Two kinds, because they are read differently

A label is a word somebody learned the shape of months ago and glances past. A
reading is what they are actually looking at. Making one bigger is usually a
reason to leave the other alone, which is why this is two settings and not one
"text size".

### A percentage, not points

The arrangement still decides the sizes; this scales them. The headline being
three times its label is a relationship arrived at by looking at a panel in a
case, and it is worth keeping when somebody makes everything bigger. The range
is 50 to 150: below that the text is unreadable at arm's length, which is the
job, and above it the text leaves the space the arrangement gave it and lands
on its neighbour.

### The colour that means something is not a colour to choose

Coolant is green, amber or red because those are the alert thresholds, and a
pump at zero is the most alarming thing this screen can say. A screen showing
calm while a notification says critical is worse than either alone (spec 013),
so a dashboard's own colour applies to the readings that carry no grade and
the ones that carry one keep it. The editor says so rather than leaving
somebody to notice.

### "Automatic" is not "none"

The edge has always been two pixels over a picture and nothing over a flat
colour. That default is worth keeping, and it is a different answer from "no
outline, whatever the background" -- which is why the field is a pointer and
the chooser lists both.

### And a GIF holds 256 colours

Found while adding this. The palette was the interface's 33 plus up to 236 of
the picture's, which is 269, and `gif.EncodeAll`'s error is discarded -- so a
photograph with enough distinct colours came back as a **zero-byte frame**:
the panel showed nothing and nothing said why. It needed a busy picture to
reach, which is why it survived until a dashboard could add colours of its
own.

The palette is built to the limit now. The interface's colours come first and
are kept, including the ones a dashboard chose, because text drawn in the
nearest available colour to the one somebody picked is text in a colour nobody
picked. What is dropped is the tail of the picture's, which are its least
common colours.

## Requirements

**R1. A dashboard names a face**, from the ones compiled in. An unknown one
draws in the default, so a dashboard from a later version still lights up.

**R2. Labels and readings each carry a size, a colour and an edge.**

**R3. A graded reading keeps its grade**, whatever colour the dashboard names.

**R4. The settings survive the file and the wire**, because the editor sends
back what it drew the form from.

**R5. The palette never exceeds what a GIF holds.**

## Acceptance Criteria

- [x] AC1. A size, a face and an edge each change what is drawn, and an
      unknown face draws the default.
- [x] AC2. A size outside the range is held to it.
- [x] AC3. A named colour is the colour on the panel, and something that is
      not a colour leaves the theme's.
- [x] AC4. A critical coolant is still drawn critical when the dashboard
      names a value colour, and the ungraded readings take it.
- [x] AC5. An edge of none is none over a picture too; unset is two pixels
      over a picture and nothing over a flat colour.
- [x] AC6. Lettering written to the dashboards file comes back from it.
- [x] AC7. The editor offers all of it, and moving a control changes the
      draft.
- [x] AC8. A picture with more colours than the palette holds still encodes.
- [x] AC9. Verified on the development machine, on the panel.

## Risks & Assumptions

- **A size within the range can still collide** on a crowded arrangement --
  the bound is what fits the space, not what fits every string. The preview is
  next to the form for that reason.
- **The faces are compiled in.** A panel drawn in a font somebody installed
  would draw differently on the next machine, and the frame is built by the
  service rather than by the desktop.
- **Smallcaps has one weight**, so a value drawn in it is the same weight as
  its label. It is a shape rather than a weight and the family has no bold.
- **Rollback** is a revert. A dashboard with no lettering draws exactly as it
  did, which is what every dashboard saved before this has.
