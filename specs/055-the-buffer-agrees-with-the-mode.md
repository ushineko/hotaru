# Spec 055: the buffer agrees with the mode

**Issue**: [#159](https://github.com/ushineko/hotaru/issues/159), superseded in reasoning by [#160](https://github.com/ushineko/hotaru/issues/160)

## Status: SUPERSEDED by [spec 057](057-the-gap-between-the-two-packets.md)

## What this was

An attempt at the fault spec 057 finally measured: an effect's colour lost
when a scene arrives from an all-lit one. It was implemented, released and did
not fix it.

## Why it is superseded

Spec 057 isolated the sequence by sending hotaru's exact packets from outside
hotaru, one variable at a time, and looking at the keyboard after each. The
gap between the two mode packets is the whole fix. Everything this spec
changed was ruled out by that isolation and has been reverted:

- **the buffer written in the mode's colour** (spec 055) -- the effect's
  colour wins with the scene's own mixed colours in the buffer, so the buffer
  is not what the device displays and never was
- **the frame moved ahead of both mode packets** (spec 056) -- the original
  order works once the gap is there, so the frame's position never mattered
- **the second read when the device reports the old mode** (spec 056) -- with
  the gap the device reports correctly, measured five times out of five

The reasoning in both is kept in git history rather than here. What is worth
carrying forward is in spec 057, including why each of these readings looked
right at the time.

## Status of the code

None of it remains. See `writeFrame` in `internal/service/service.go`.
