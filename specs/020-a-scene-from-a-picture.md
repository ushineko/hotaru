# Spec 020: a scene from a picture

**Issue**: [#47](https://github.com/ushineko/hotaru/issues/47)

## Status: COMPLETE

## Context

Spec 019 gave the panel a picture. It did nothing for the other two hundred and
twenty-five lights in the case, and the two are in the same room: a wallpaper of
Pluto on the cooler with the fans still purple looks like two machines.

Making them agree by hand means opening the editor, picking a colour per zone,
looking at the case, and picking again. Six or seven times, for one theme. The
picture already knows what its colours are, and hotaru is already holding it.

### Across the picture, not an average of it

An average of a red storm on a blue sky is mud, and a machine lit with mud
looks broken rather than themed. So a run of lights is read as a run across the
picture: light *i* of *n* gets the colour of the *i*th vertical slice, and a
twenty-four light ring carries the same left-to-right sweep the image has. A
single-light zone -- a logo -- gets the whole picture's colour, which is the
only honest answer when there is one light to say it with.

**And a slice is not its average either.** That was the first version, and on
the picture it was first tried with -- a rust planet on black space -- every
light came out the same grey-olive. Half of every vertical slice through a
photograph is background, and averaging the background in is averaging the
subject away: the same mistake as averaging the whole picture, one level down.
Weighting each pixel by how much colour it carries asks the slice what colour
it *has* rather than what it works out to, and the rust came back.

A region with no colour at all weighs nothing and falls back to the plain
average, so a grey picture lights grey rather than an invented hue.

### A photograph is mostly shadow

An LED given a shadow is an LED that is off. What somebody means by "match this
picture" is the picture's *colours*, at the brightness a light works at -- so
the hue and the relative saturation are kept and the value is lifted to a
floor. Black is the exception: it has no hue to keep, and a dim white is the
honest reading of "this part of the picture has nothing to give".

### A stack of files is a stack of files

Dropping four photographs on the window processed the first and discarded the
other three, silently. That is the third silent discard this project has
shipped, and the rule from the panel applies here too: the ones that say no by
doing nothing are the expensive ones.

Dropped files queue and are handled one at a time. A stack -- more than one at
once -- is a question rather than an assumption, because two things are
plausible and hotaru cannot tell which: four separate pictures, or one reel.
The stored animations are the evidence that the second is wanted; a reel of
stills, cross-faded, is what somebody already has on that panel and likes.

### What a reel costs

The panel holds about 24 MB per bucket, and a cross-fade is frames nobody asked
for individually. So one palette for the whole reel, chosen from the pictures
themselves, and the frames delta-encode against each other; the fade is capped
at a step count that fits the budget rather than a fixed smoothness.

Choosing the palette from the pictures rather than from a fixed table is spec
012's finding, and the LUT is the consequence of the first working version
taking thirteen seconds for three photographs: Go's paletted draw does a linear
search through 256 colours per pixel, and a cube lookup does not.

## Requirements

**R1. A scene from a stored picture, in both shells.** `hotaru image scene
<picture> <name>` and the same in the window, over one API call.

**R2. Every light gets a colour, and adjacent lights that agree share a rule.**
A scene of 225 separate assignments is unreadable; adjacent equal colours merge
into one rule using spec 018's range grammar.

**R3. The scene carries the picture too.** The panel shows the image the scene
was built from, so applying the scene makes the whole machine agree.

**R4. Colours are read across the picture and weighted by chroma.** Never one
average for the machine, and never a flat average within a slice.

**R5. Dropped files all arrive.** A drop of any size queues every file; none is
discarded without a word.

**R6. A stack asks.** Separate pictures or one slideshow, chosen by the person
dropping them. A single file asks nothing.

**R7. A slideshow fits the panel.** One palette for the reel, a hold and a
cross-fade, and a step count reduced until the encoded reel fits the bucket.

## Acceptance Criteria

- [x] AC1. `hotaru image scene` writes a scene covering every device with
      lighting, and the command exists in both shells (parity test).
- [x] AC2. A picture with a red left and a blue right produces a scene whose
      first lights are red and whose last are blue.
- [x] AC3. A saturated subject on a dark background keeps its colour rather
      than averaging to grey.
- [x] AC4. A grey region lights grey; black lights a dim white.
- [x] AC5. Adjacent lights of equal colour merge into one rule.
- [x] AC6. The written scene names the picture as the screen's image.
- [x] AC7. Dropping several files processes all of them, with a test on the
      queue rather than on a timer.
- [x] AC8. Dropping more than one file offers separate import or a slideshow;
      dropping one does not ask.
- [x] AC9. A slideshow of several photographs encodes under the panel's budget,
      with a test that the step count is reduced rather than the reel truncated.
- [x] AC10. Verified on the development machine: a scene built from a
      wallpaper, applied, and looked at.

## Verified on hardware

Built from `wallhaven-2kkreg` -- Pluto, rust on the left, pale teal on the
right, over black space -- and applied to the machine.

The first build lit every one of the 217 assignments somewhere between `#8c6f63`
and `#8c8c7f`: a olive-tan wash, the same colour everywhere, on a picture that
is plainly two colours. That is what a flat average within a slice does to a
subject on a dark background, and it is only visible by holding the picture
next to the case. Weighted by chroma the same picture gives `#a06250` at the
start of the ring, through `#c0ceb2`, to `#728c83` at the end -- the planet's
rust sweeping into its teal -- and the panel shows the picture it came from.

A note for the next person: the conversion runs in the service, so a rebuilt
binary changes nothing until `systemctl --user restart hotaru`. Two runs were
compared before that was noticed, and they were identical because they were the
same program.

## Risks & Assumptions

- **Chroma weighting is a judgement, not a measurement.** It suits photographs,
  which are what people have; a poster of flat colour blocks on white is a case
  where the plain average was arguably righter. The fallback covers only the
  fully grey region, not the mostly-grey one.
- **The slideshow budget is one panel's.** 23 MB with the bucket at 24; a
  firmware that holds less would fail the way spec 013 found -- silently.
- **Rollback** is a revert. Nothing written by this spec changes an existing
  scene, an existing picture, or the format either is stored in.

## Alternatives Considered

Considered a histogram per slice, taking the most popular colour rather than a
weighted mean; rejected because a smooth gradient has no popular colour and
would quantise to whichever bucket boundary fell inside it.

Considered doing the conversion in the window; rejected for spec 019's reason,
which is that the CLI would then be unable to do something the GUI can.
