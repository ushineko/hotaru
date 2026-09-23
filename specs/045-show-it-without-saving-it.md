# Spec 045: show it without saving it

**Issue**: [#120](https://github.com/ushineko/hotaru/issues/120)

## Status: INCOMPLETE

## Context

Three things found by editing a dashboard on the panel rather than by reading
the editor.

### Two buttons that both save

`Save` and `Save and show it` differ in one thing: whether the dashboard is
made the active one afterwards. Both write it, both close the editor. So the
second is the first with a step added, and the word somebody reaches for it
with -- *show* -- is not the thing it mainly does.

The wanted button is **Show**: draw what is being edited, now, and change
nothing on disk. Saving is `Save`, and it stays the only thing that makes an
edit stick.

**And it does not leave the editor.** Somebody who asks to see their work is
still editing it; taking them back to the list is answering a question they
did not ask, and costs them their place in a long form.

What goes to the panel is the frame the preview is already showing -- the
editor renders the draft on every change, so the bytes are in hand. It goes by
the same route a picture does: hold the panel, write the frame, and the panel
holds it until somebody asks for the dashboard back.

**That makes it a still.** The numbers on it do not tick, because it is one
frame rather than a dashboard the loop is drawing. That is honest for what it
is -- a look at an unsaved draft -- and the editor says so rather than leaving
somebody to wonder why the temperature stopped.

### The readings size does nothing on `stacked`

Spec 044 wrote down the risk and this is it.

A stacked row is a label column and a number column. The label column is as
wide as the widest label *at the labels' own scale*, and the numbers get what
is left. With labels at 150% that is 253 pixels of words and 123 pixels for
`40.0 · 2714`, which does not fit at any size the fit will accept -- so the
numbers come out at the 16 point floor, and they come out at the floor at 50%,
100% and 150% alike. The Readings slider is connected to nothing.

**The numbers ask first.** The column is measured at the size the author
asked for, and the labels take the room that is left. A label with nowhere to
go is drawn in what it has and may be clipped by the number beside it, which
is a thing somebody can see and fix; a slider that does nothing is not.

The label column keeps a floor, because a row whose words are squeezed to
nothing is not a row. Between the floor and the widest label the numbers win.

### A unit touching its number

Spec 044 packs a value and its unit as one run so that nothing sits between
them. One run turned out to be a shade *too* close: `12%` with no space reads
as one token, and the eye separates the figure from the unit more slowly than
it should.

A hair of space, scaled with the text so it is the same gap at every size.

## Requirements

**R1. The editor's second button is `Show`**, and it does not save.

**R2. `Show` leaves the editor open** with the draft as it was.

**R3. `Show` puts the previewed frame on the panel**, and says it is a still.

**R4. A stacked row sizes its numbers at the size the dashboard asked for**,
and gives the labels what is left.

**R5. The label column keeps a floor** it is not squeezed below.

**R6. A unit is drawn a small distance from its number**, scaled with it.

## Acceptance Criteria

- [x] AC1. The editor has `Save`, `Show` and `Cancel`; `Show` does not call
      `SaveDashboard`.
- [x] AC2. After `Show`, the editor is still open and the draft is unchanged.
- [x] AC3. `Show` sends the previewed frame to `POST /v1/screen`, and with no
      frame yet rendered it says so rather than sending nothing.
- [x] AC4. In a stacked dashboard with labels at 150%, raising the readings
      size raises the point size the numbers are drawn at.
- [x] AC5. The label column is never narrower than its floor, whatever the
      readings ask for.
- [x] AC6. A unit is drawn further from its number than it was, and the gap
      grows with the point size.
- [x] AC7. The shipped dashboards' golden frame is not regenerated: units
      default off, so the gap draws nothing that was not drawn before, and
      none of the shipped dashboards is stacked.
- [ ] AC8. Verified on the development machine, on the panel.

## Alternatives Considered

- **A live draft on the panel.** The service would hold the unsaved dashboard
  and the loop would keep drawing it with live readings. Rejected for now: it
  is new service state with its own rules about when it clears, for a button
  whose job is "let me look at this".
- **Shrinking labels and numbers together to keep their ratio.** Rejected: the
  sliders are sizes, not a proportion, and a slider that changes both is
  harder to explain than one that wins.
- **Capping the label column at a fixed share of the row.** Rejected: it
  ignores what either slider says.

## Risks & Assumptions

- **A long label can now be run into.** The numbers take their room first, so
  a label wider than what is left is drawn under the numbers' left edge. The
  floor limits how far this goes; beyond it the answer is a shorter label or a
  smaller readings size, and both are in the editor.

- **The still can be left on the panel.** `Show` holds the panel the way a
  picture does, and the panel keeps the frame until somebody asks for the
  dashboard back. The editor's flash says so, and the dashboards list's
  `Show it` gives it back.

- **The gap is a guess**, like the unit's size in spec 044: a fraction of the
  point size, picked by eye and named as a constant.

- **Rollback** is a revert. Nothing is written to disk that an earlier version
  reads differently: the change is to what the editor does and to how a frame
  is drawn.
