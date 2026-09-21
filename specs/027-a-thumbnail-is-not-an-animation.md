# Spec 027: a thumbnail is not an animation

**Issue**: [#60](https://github.com/ushineko/hotaru/issues/60)

## Status: COMPLETE

## Context

The window was reported as reaching a 948 MB high-water mark -- "larger than
an electron app" -- and as climbing without plateauing while sections were
switched. Sampling the live heap after a forced collection, every five
seconds:

	secs   live_MB   sys_MB   RSS_MB
	   0        94      167      388
	  30       227      336      593
	  60       405      530      752
	  95       444      579      764

**It is a bounded working set, not a leak.** Fyne expires a cached object a
minute after it was last touched, and only sweeps on a canvas refresh -- so an
idle window holds everything it built and a busy one holds a minute of it. The
number came back to 158 MB once the switching stopped and the window was
touched again.

The bound is visits per minute times decoded bytes per visit, and the second
number was indefensible:

	128.70 MB  image.NewNRGBA     ~12 copies of one 1246x2186 diagram
	 73.49 MB  image.NewPaletted  ~179 GIF frames
	 14.46 MB  the raw GIF bytes this window cached for thumbnails

### A correction worth recording

The first reading of this was wrong. The heap sat at exactly 324 MB through
two minutes of idle, and that was called retention -- when it was the opposite:
Fyne sweeps its caches only on a canvas refresh, so an idle window collects
nothing. A flat line meant nothing was running, not that something was held.

The second round is what said so: the moment the window was touched again the
number fell to 158 MB. **A measurement taken while nothing is happening
measures nothing.**

### What was ours

`PicturesSection` cached each picture's *file bytes* and handed them to a
`canvas.Image`, which decodes from its resource -- and decodes again on every
refresh. Handing it a GIF decodes the **whole animation**, at the panel's own
640x640: `berserk-slide` is sixty frames, so one visit to that section decoded
about 25 MB of paletted images to draw squares ninety-six pixels across.

Two changes. The first frame is decoded once and scaled to twice the drawn
size -- 147 KB rather than 25 MB. And the result goes in fynedesygn's shared
cache (its spec 025) rather than a map of this section's, so it survives the
section being rebuilt and is bounded when it does not.

The key carries the picture's size and the time it was added, because a
picture replaced under the same name is a different picture.

### What was not ours

The diagram, which `markdown.Pane` re-decoded per measure, is fixed in
fynedesygn by the same cache. **And the diagram stays the size it is**: one
copy of a 10 MB picture is a reasonable thing for a program to hold, and
twelve were not. The cache is what makes the resolution affordable rather than
something to apologise for.

### What it bought

Same window, same switching, same sampling:

| | before | after |
|---|---|---|
| peak live heap | 444 MB | **271 MB** |
| live heap, sampled | 314 MB | **179 MB** |
| `image.NewNRGBA` | 128.70 MB | **21.63 MB** |
| `image.NewPaletted` | 73.49 MB | **not in the top forty** |
| raw bytes held for thumbnails | 14.46 MB | **none** |

What is left at the top is font tables at 39.91 MB, which are a one-off, and
Fyne's own text and renderer caches -- widgets from trees this window rebuilds
on every navigation. That is the next thing, and it is structural.

## Requirements

**R1. A thumbnail is decoded once, small.** The first frame, at the size this
list draws it, not the animation at the panel's size.

**R2. Through the shared cache**, so it survives a rebuild of the section and
is bounded when it does not.

**R3. The key changes when the picture does.**

**R4. The measurement is a profile**, before and after, same machine, same
thing being done.

## Acceptance Criteria

- [x] AC1. The section holds no pictures of its own; the cache holds them.
- [x] AC2. A picture is decoded with `gif.Decode` -- the first frame -- and
      scaled once.
- [x] AC3. Removing a picture forgets its thumbnail; replacing one produces a
      different key.
- [x] AC4. A profile of the same switching shows the numbers above.
- [x] AC5. Verified on the development machine.

## Risks & Assumptions

- **A thumbnail no longer moves.** An animated picture shows its first frame
  in the list. `Show it` puts the animation on the panel, which is two inches
  to the right and is where an animation belongs.
- **Twice the drawn size** is a guess at what a scaled desktop needs. It is
  147 KB against 25 MB, so the direction has room in it.
- **The remaining growth is Fyne's widget caches**, which is a question about
  rebuilding trees rather than about pictures.
- **Rollback** is a revert; the cache it uses is additive in fynedesygn.

## Alternatives Considered

Considered shrinking the architecture diagram to fit fynedesygn's own mermaid
guidance; rejected because the cache is what the guidance's size limit was
protecting against, and with it a document can carry a diagram at the
resolution it deserves.
