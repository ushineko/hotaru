# Spec 040: one colour at a time

**Issue**: [#100](https://github.com/ushineko/hotaru/issues/100)

## Status: COMPLETE

## Context

The wheel reports every drag step, and that is deliberate: the argument for a
window rather than a configuration file is that the machine is right there, so
a colour can be *shown* rather than described. The lights follow the pointer.

What was between the wheel and the hardware was a throttle that did not
throttle.

`Picker.throttled` took the time at the moment a send *started*, then made the
send itself -- synchronously, on the UI thread, through an HTTP `/apply` that
ends in USB writes to every device in scope. So the 120 milliseconds it was
meant to space sends by were spent *inside* the send. A drag step arriving
after an apply that took 200ms found `time.Since(last)` already past the
window and went straight out. The next one did too. On hardware slow enough to
need spacing there was none: the wheel ran a continuous back-to-back request
loop, which is the case the throttle existed for and the only case it did not
cover.

It was measuring the wrong end.

And because it was all on the UI thread, the window was blocked for the whole
of every round trip. What somebody felt was the wheel sticking under the
pointer while the lights ran behind it.

The cost on the other side is not small either. `Service.Apply` enumerates the
OpenRGB devices and then writes each in-scope device in turn, waiting for each
one. The per-device queues coalesce latest-wins, but nothing ever reached them
to coalesce: the window only ever had one request outstanding, so every colour
the wheel produced was written to hardware.

### Measure the far end, not the near one

The gap belongs *after* the send, not around it. One apply in flight at a
time; every drag step that arrives while it is out replaces a single pending
colour rather than queueing behind it; when the apply comes back, the gap is
counted from there. Hardware that takes 300ms to write gets a colour every
420ms and the pointer stays smooth; hardware that takes 5ms gets one every
125ms. The rate limits itself to what the machine can take, which is the thing
a fixed number cannot do because the number cannot know.

Latest wins, and **the last one is never the dropped one**. A drag that ends
between two sends must not leave the hardware on the second-to-last colour:
that looks exactly like the picker being wrong, and it is the bug this shape
of code usually has.

### The picker is a control, not a rate limiter

The limiting moves out of `Picker` and into the editor. The picker's job is to
report what it is showing; the thing that knows the hardware is slow is the
thing that talks to it. That also picks up the two routes that were never
limited at all -- the Sat and Lum sliders step at 0.01, so a slider dragged
end to end is a hundred applies, and the R/G/B boxes send one per keystroke.
They all funnel through `OnPick`, so they are all covered once the limit is
behind it.

The draft is still updated on the UI thread, every step, because it is a map
write and the window reads it. What crosses to the worker is the built scene:
a value, made on the UI thread and handed over, so there is no shared state to
race on.

### Dismissal has to win

Moving the send off the UI thread creates a race that could not exist before:
the modal closes, the preview lease is released, and an apply that was already
in flight lands afterwards -- leaving the lights on a preview colour with
nothing holding it. So stopping waits for the sender to go quiet before the
lease is let go. It blocks the UI thread for at most one apply, once, at
dismissal, and that is exactly the ordering the guarantee needs.

### The sender belongs to the modal, not to the section

The first version of this kept the sender in a `ScenesSection` field and
stopped it from `Detach`, next to the picker field already there. That is a
trap, and `Detach`'s own comment records the last person to walk into it: **the
shell detaches a section before every rebuild**, not only when it is replaced,
and this window rebuilds every two seconds. Ending the preview there once made
a draft go up and come down within one frame. Stopping the sender there would
drop it under a pointer still dragging, twice a minute.

So the sender is a local that the modal's callback ends, which is the lifecycle
the editor already documents: a control that changes the lights has to have an
unmistakable end, and a modal has one.

And the `picker` field it would have sat beside turned out to be dead --
nothing ever assigned it, so `stopPicking` had been nilling a permanent nil and
`Detach` had been doing nothing at all since the field was added. It goes,
along with `stopPicking` and its four call sites, and `Detach` becomes an empty
method whose comment is the whole of its value. Two tests hold the shape:
`Detach` has no body, and the section holds no limiter.

## Requirements

**R1. At most one apply is in flight from the picker at any moment.**

**R2. Colours produced while one is in flight collapse to the latest.**

**R3. The gap between sends is measured from the previous send's completion.**

**R4. The last colour is always sent.**

**R5. The UI thread does not wait on the hardware while the pointer moves.**

**R6. Nothing lands on the hardware after the picker is stopped.**

**R7. Nothing a rebuild runs can stop the sending.**

## Acceptance Criteria

- [x] AC1. With a send that blocks, offering many colours during it produces
      exactly one further send, carrying the last colour offered.
- [x] AC2. The gap between the end of one send and the start of the next is at
      least `live`.
- [x] AC3. A single colour offered and then nothing is sent once, promptly.
- [x] AC4. Stopping waits for an in-flight send to return and sends nothing
      after it.
- [x] AC5. A colour offered after stopping is not sent.
- [x] AC6. The picker itself reports every change to `OnPick` at once, with
      nothing held back: four colours chosen is four reports, in order. The
      sliders and the number boxes report through the same call.
- [x] AC7. `Detach` has an empty body and the section holds no limiter, so a
      rebuild cannot reach the sending. Both asserted against the source, the
      way the thread guard is.
- [x] AC8. The thread guard still passes: nothing that draws is called from
      the sender.
- [x] AC9. Verified on the development machine by dragging the wheel across a
      scene covering every device: the pointer stays under the cursor and the
      lights follow it.

## Risks & Assumptions

- **The lights lag the pointer by up to one apply plus `live`.** That is the
  honest cost of hardware that takes that long, and it replaces a wheel that
  did not move at all while it happened.
- **`live` stays 120ms** and is now a floor between sends rather than a period
  sends are attempted on. On fast hardware the visible rate is unchanged.
- **Stopping can block the UI thread for one apply.** Bounded by the client's
  2-second dial timeout and the apply itself; it happens once, when a modal is
  dismissed.
- **The daemon's per-apply device enumeration is untouched.** It is the other
  half of the cost and belongs to the apply path every caller shares, so it
  wants its own spec and its own before-and-after numbers.
- **`Detach` is now empty**, and an empty method is a thing somebody deletes.
  Its comment is why it exists and the test holds it in place; if it does go,
  the two things that must not live there go with it.

- **Rollback** is a revert. Nothing is persisted and no API changed.
