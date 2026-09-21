# Spec 021: the window says what this is

**Issue**: [#49](https://github.com/ushineko/hotaru/issues/49)

## Status: COMPLETE

## Context

The window opens on Service, System, Scenes, Pictures and Cooling, and nowhere
in it does it say what hotaru is, what version is running, what licence it
carries, or where it came from. Somebody who opens it without having read
anything has five control surfaces and no page that introduces them.

### The README, not a summary of it

The obvious move is a paragraph in an About pane. It is the wrong one, and
fynedesygn says so in the doc comment on `shell.AboutSection`: long-form
documentation belongs in the README, and a shortened restatement here goes
stale. A program that describes itself twice keeps one description and forgets
the other, and the forgotten one is the one nobody's tests read.

So the front page is embedded verbatim and rendered -- 448 lines, headings,
tables, command blocks and the architecture diagram -- under the standard
About header, with the project link beside it.

`go:embed` cannot read above the directory it is written in, so the embed
lives in a package at the repository root. That is where README.md is.

### The diagram is drawn at build time

fynedesygn's rule, carried whole: mermaid is a build-time step, never a runtime
dependency. `make generate` renders every fence in the repository to a light
and a dark PNG keyed by a hash of the diagram's own text, and those are
committed and embedded. A program that drew a diagram at runtime would need
node, a browser and the network to draw a box with an arrow in it.

A fence whose image is missing does not fail anything: it renders as its own
source with a caption saying it was not rendered. That is a failure mode with
no alarm on it, so two tests carry one: `mermaid.Check` over the repository,
and one that renders the README's fence and asserts what it became.

### What this found in fynedesygn

`shell.AboutSection` wrapped its page in a scroller, and the shell already puts
what a section builds inside its own. `About.Extra` -- documented as receiving
the shell "so it can follow the content scroller" -- therefore handed the
document pane the outer scroller while the pane sat in the inner one. The pane
renders only what is near its viewport and never heard the viewport move: one
screenful of README over the blank height of the rest of it.

Fixed there (fynedesygn spec 020, #19) rather than worked around here. A
consumer that works around a library bug is a consumer that keeps it.

### And then the page jumped

Reported on the first read: scrolling down, the page leaps back to the top on
the way past the diagram. Two faults, one symptom.

`markdown.Pane` asked each block for its height and then told it how wide it
was, so a paragraph measured as one line and a diagram as its natural size:
the document reserved about two thirds of the height it draws as. Only the
*first* measurement is wrong -- any later width change gets it right, because
the blocks have been resized by then -- so it took a program that rebuilds a
document under somebody's scroll position to make it visible (fynedesygn spec
021, #21).

Which is the second fault, and it is this window's. The poll rebuilds whatever
section is on screen when anything it watches moves, and a section that says
nothing about what it watches is rebuilt whenever anything does -- on a
machine with a running pump, every two seconds. The README does not follow the
machine, so it now says so, the way the Scenes list had to.

## Requirements

**R1. An About section, and it is the README.** Embedded verbatim, rendered,
never restated.

**R2. The project link is in it**, along with the version, the licence and the
socket the window is talking to.

**R3. Diagrams are rendered at build time** and embedded. `make generate` is
the step, and a test fails when a diagram and its image have drifted apart.

**R4. One scroller.** The document follows the shell's content scroller rather
than bringing its own, as fynedesygn's rule requires.

## Acceptance Criteria

- [x] AC1. The window's sections end with About, and a test names the list.
- [x] AC2. The About section's text carries the project link.
- [x] AC3. What the section builds contains no scroller of its own.
- [x] AC4. The README's mermaid fence renders as a diagram rather than as its
      source with an apology.
- [x] AC5. Every mermaid fence in the repository has a current image, checked
      by a test rather than by remembering to run the generator.
- [x] AC6. `make generate` renders them.
- [x] AC7. The section is not rebuilt by the poll: it declares that nothing on
      the machine changes it, with a test.
- [x] AC8. Verified on the development machine: the About section read
      top to bottom without losing the reader's place, diagram drawn, link
      followed.

## Verified on hardware

Read in the window on the development machine: the page scrolls as one and
keeps its place past the diagram, which is drawn rather than apologised for,
and the link opens the project.

The diagram is the one thing that reads better on GitHub than in the window --
it is a 623x1093 flowchart at 1x, taller than the pane, which is what
fynedesygn's own guidance warns about ("a diagram that needs zooming belongs in
the README"). It is kept because the README is embedded verbatim and the link
to the readable copy is two inches above it.

## Risks & Assumptions

- **The window carries the README's diagrams.** 1.2 MB of PNG for four
  diagrams, of which the About page uses one pair; the rest are the docs'. The
  generator writes one directory and the embed takes it whole, which is the
  cheaper trade than a hand-maintained list of hashes -- the thing the
  hash-keyed design exists to avoid.
- **`make generate` needs mmdc.** Only on a machine that changes a diagram;
  the images are committed.
- **Rollback** is removing one section from the list.

## Alternatives Considered

Considered a written About paragraph and a link; rejected for the reason in
fynedesygn's own doc comment, which is that the paragraph rots.

Considered copying README.md into `internal/gui` so the embed could live beside
the section; rejected because then the documentation is in two places and the
copy is the one the window shows.
