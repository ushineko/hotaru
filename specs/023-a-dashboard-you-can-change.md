# Spec 023: a dashboard you can change

**Issue**: [#52](https://github.com/ushineko/hotaru/issues/52)

## Status: COMPLETE

## Context

The panel draws a coolant headline with a severity ring and three columns
underneath, over a starfield, and every one of those is a constant in
`internal/dashboard`. They are the Python's choices, ported deliberately and
correctly (spec 013), and they are somebody else's machine's answers.

Spec 022 made the machine say nine numbers instead of four. This spec is what
lets somebody choose which of them they see.

### A layout you pick, not a canvas you drag

The panel is 640x640 behind a round bezel in a case, and the interesting
positions on it are few. Free placement would let somebody put a number where
the bezel cuts it off, and the editor for it is a canvas with handles and a
grid and its own set of bugs.

So: a few **arrangements**, each drawn against the panel, and the choice is
which reading fills each of their slots. Nothing can land somewhere bad, the
editor is a form, and the arrangements can be added to without anybody's saved
dashboard changing meaning.

### A dashboard is a name

Like a scene. Saved, listed, switched, and nameable by a scene -- so "evening"
can dim the lights *and* put the quiet dashboard up, which is the thing a
scene could not do before.

The shipped set is code rather than a file, exactly as the shipped scenes are:
a fresh install has dashboards before anybody has saved one, saving over a
shipped name replaces it, and deleting the replacement brings the original
back.

### Text over a photograph

Reported before it was built, from someone who has looked at this panel a lot:
a picture behind the numbers will sooner or later put a light region under
light text, and the number disappears. A dark outline around every glyph costs
eight offset draws per string and makes the text legible over anything.

It is not switched on for the starfield, which was designed with the numbers
in mind and would only get heavier. It is the picture that is unpredictable,
because it is somebody's photograph.

### What the editor previews

The frame itself, rendered by the service, not an approximation drawn by the
window. The panel takes a 640x640 GIF and the service is what makes them; a
second renderer in the GUI would be a second set of answers about what the
screen looks like, and the one nobody checks is the one on screen.

## Requirements

**R1. Named dashboards, stored and shipped.** Save, list, switch, delete; a
shipped set in code; replacing and restoring as the scene store already does.

**R2. Four arrangements.** `ring` (today's), `grid` (headline over four in a
2x2), `stacked` (headline over four rows, no ring) and `big` (the headline
alone). Each says how many slots it has.

**R3. A slot names a reading, and may rename it.** Any of spec 022's nine,
with an optional label and unit of the dashboard's own; empty means the
reading's own.

**R4. A caption.** One line of the author's text, drawn where the arrangement
puts it. Empty draws nothing and takes no space.

**R5. Themes.** A small collection, each a set of colours. The coolant grade
-- green, amber, red -- is not themed: it is the one colour on this screen
that carries meaning rather than style, and it has to agree with what the
alerts say.

**R6. Backgrounds: starfield, plain, or a picture** from the image library,
dimmed by an amount the author sets.

**R7. Text over a picture is outlined.** Automatically, because the author
cannot know what is under every glyph and the failure is unreadable rather
than ugly.

**R7a. Rings are a list, and how many fit is the arrangement's business.**
Each tracks a reading of its own; inner ones are drawn progressively thinner,
which is what keeps several legible at arm's length.

**R8. The default is what the panel draws today.** A machine that upgrades and
touches nothing sees no change. The shipped `coolant` dashboard is spec 013's
layout, constant for constant.

**R9. A scene can name a dashboard.** `screen: dashboard` keeps working and
means the active one; `screen: dashboard:load` names one.

**R10. Both shells, one renderer.** `hotaru dashboard list|show|use|delete`
and an editor in the window, over one API, with the preview rendered by the
service.

## Acceptance Criteria

- [x] AC1. A machine with no dashboards file has the shipped set, and
      `coolant` is the active one.
- [x] AC2. The frame drawn for the shipped `coolant` dashboard is identical to
      what spec 013's renderer drew, asserted by content hash against the
      previous implementation's output.
- [x] AC3. Saving over a shipped name replaces it; deleting that replacement
      brings the shipped one back.
- [x] AC4. Each arrangement draws its own number of slots, and a dashboard
      with more slots than the arrangement has drops the extra rather than
      overflowing the panel.
- [x] AC5. A slot with no label of its own draws the reading's label; one with
      a label draws that.
- [x] AC6. An empty caption draws nothing and moves nothing.
- [x] AC7. A themed dashboard differs from the default by colour and not by
      geometry, and the coolant grade is the same colour in every theme.
- [x] AC8. A picture background is drawn, dimmed, and the text over it is
      outlined; the starfield is not outlined.
- [x] AC9. A picture that has been removed from the library costs the
      background and not the frame: it falls back to the theme's plain colour.
- [x] AC10. `hotaru dashboard use <name>` changes what the panel is being
      pushed, and the parity test covers every route.
- [x] AC11. A scene naming `dashboard:load` applies that dashboard; a scene
      naming `dashboard` applies the active one; a scene written before this
      spec is unchanged in meaning.
- [x] AC12. The window lists dashboards, edits one as a form, previews the
      rendered frame, and saves.
- [x] AC13. Rendering is safe to do from two goroutines at once, with a
      canary that reports data races under `-race` when the guard is removed.
- [x] AC14. Verified on the development machine: a dashboard built in the
      window, previewed, put on the panel, and read across the room --
      including one over a photograph, which is the case the outline exists
      for.

## Verified on hardware

Built in the window and put on the panel, plus every shipped arrangement from
the terminal. A scene naming `dashboard:quiet` changed the lights and the
screen together.

**Three faults, and two of them were one.** The font cache is a package-level
map and an `opentype.Face` is not safe for concurrent use. Until the preview
route existed exactly one goroutine ever rendered -- the push loop, for the
life of the service -- and the editor renders once per change while that loop
is still drawing. Two goroutines shaping text through one face corrupted it,
which showed up three ways at once: garbled frames in the preview, `EOF`
answers where the HTTP server recovered from the panic, and **the service
exiting**, because the push loop's goroutine had nothing to recover it.

One lock, held across the drawing rather than across the map lookup, since the
unsafe thing is the face and not the map that found it.

The third fault was the editor's: every finished preview invalidated the
section, which rebuilds every widget in it and takes the control out from
under whoever is typing. The picture and its caption are updated in place, and
only the two choices that change the form's *shape* -- the arrangement and the
background kind -- rebuild it.

And a Fyne trap found on the way: a decoded image is cached against its
resource's name, so a second `preview.gif` can draw the first one back. Each
frame gets a name of its own.

**The grid's first row spacing was wrong** and only visible by rendering it:
stepping 112 where a column is 126 tall put one row's unit through the next
row's label.

## Risks & Assumptions

- **Frame size buys settling time.** A photograph behind the numbers is a
  bigger GIF than a starfield, and the push floor scales with it (spec 013):
  a picture background means a slower refresh, not a slower machine. The
  editor says so where the background is chosen.
- **More arrangements is more geometry nobody has looked at.** Each one is a
  set of constants that can only be judged on the panel, which is why there
  are four and not twelve.
- **Rollback** is a revert. The shipped set is code and the file is additive;
  a machine that never saved a dashboard has nothing to roll back.

## Alternatives Considered

Considered free placement of blocks on the panel; rejected because the round
bezel makes most of the canvas a trap and the editor is a bigger program than
the thing it edits.

Considered drawing the preview in the window from the dashboard description;
rejected because it is a second renderer, and two answers about what the panel
shows is one answer too many.
