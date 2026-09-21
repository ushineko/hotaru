# Spec 029: colours far enough apart

**Issue**: [#70](https://github.com/ushineko/hotaru/issues/70)

## Status: COMPLETE

## Context

Spec 020 made a scene out of a picture: slice the image, average each slice,
light the LED that sits there with what is there. It works, and what came out
of a nebula was olive.

Two faults, one on top of the other.

### The average of a picture is mud

A slice of a starfield is mostly black with a few bright points, and the mean
of that is dark grey. A slice of a sunset is orange against blue, and the mean
of that is brown. Averaging every pixel equally asks "what colour is this
region" and answers with the colour nothing in it actually is.

So the mean is weighted by chroma: a pixel's distance from grey is how much it
counts. Grey stays grey -- a picture with no colour in it falls back to the
plain average, because the weights sum to zero and there is nothing to be
clever about.

### And even then they are all the same colour

A photograph is mostly one hue. Twenty-four lights showing twenty-four samples
of it is a machine lit one colour badly -- the differences are there, and they
are two per cent apart.

`Separate` takes the circular mean of the hues and scales each colour's
deviation from it. At 1 nothing moves. At 3 -- `MostDistance` -- a picture that
was "orange, orange, orange" is orange, red and yellow, which is what somebody
making a scene out of a picture is asking for: their machine to look like the
picture, not to be the picture.

The value is clamped to a floor, because pushing colours apart drags some of
them to black, and a light that is off is not a colour.

Order matters: `Lit` first, then `Separate`. Brightening a separated colour
undoes the separation.

### A dashboard is a picture too

The panel is showing a dashboard most of the time, and it is a 640x640 frame
like any other. `SceneFromDashboard` renders one and takes the colours out of
it, so "make my machine match what is on the screen" works for what is
actually on the screen.

### And the tweak has to be available afterwards

A separation somebody has to guess before the scene exists is a separation
they get wrong. `RecolourScene` re-reads the source and re-derives the
colours at a new distance, on a scene already saved -- in the window as a
slider, and as `hotaru scene recolour`.

## Requirements

**R1. The colour of a region is chroma-weighted**, and a region with no
colour in it is still its own average.

**R2. Separation is a number somebody sets**, from 1 (leave them alone) to
`MostDistance`, when the scene is made and afterwards.

**R3. A dashboard is a source** for a scene, the same as a picture.

**R4. A scene made this way says what it came from**, so recolouring it can
re-read the source.

## Acceptance Criteria

- [x] AC1. A picture of one hue yields colours at more than one hue when the
      distance is raised, and the same colours when it is 1.
- [x] AC2. A grey picture yields greys at every distance.
- [x] AC3. No colour comes out below the floor.
- [x] AC4. `hotaru image scene --distance` and `hotaru scene recolour` both
      work, and the window offers the same slider in both places.
- [x] AC5. A scene made from a dashboard names it, and recolouring re-renders
      it.
- [x] AC6. Verified on the development machine: the nebula scene, which was
      olive.

## Risks & Assumptions

- **Separation is a judgement, not a measurement.** There is no right answer
  for how far apart a machine's lights should be; the slider exists because
  the program cannot know.
- **Recolouring re-reads the source**, so a picture that has moved cannot be
  recoloured. It is reported, and the scene keeps the colours it has.
- **Rollback** is a revert. Scenes already saved carry ordinary colours and
  are unaffected.
