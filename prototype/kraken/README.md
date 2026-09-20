# Kraken prototype

Throwaway code that answered one question: can hotaru drive an NZXT Kraken
Elite V2 without liquidctl? It can, and this is what proved it.

**Not shipping code.** Device paths are hardcoded, errors are printed and the
whole thing is one `package main`. The real implementation lives in
`internal/cooler` and is written from the findings, not from this.

`panel.go` and `panel2.go` are peripheral-battery-monitor's LCD dashboard
ported from Qt to `x/image`, constant for constant. It is the only part with a
dependency (`golang.org/x/image`, for a font rasteriser the standard library
does not have) and the only part worth reading for design rather than for
protocol.

Kept because the protocol work behind it cost an evening at the machine, with
somebody watching the screen to say what each experiment did, and the useful
part is not the code but what it measured. See
`specs/012-the-cooler-without-liquidctl.md`.

    go run . [status]      read coolant, pump and fan over /dev/hidraw
    go run . lcd [file]    push a GIF (a generated test card if no file given)
    go run . anim          push a 12-frame animated GIF
    go run . liquid        return the screen to the firmware readout
    go run . stress [mode] push repeatedly at falling intervals
    go run . fb            the framebuffer path liquidctl reserves for 0x300E
    go run . dash [s] [iv] a live dashboard, default 30s at 1 Hz
    go run . count         a big incrementing number, to prove updates land
    go run . panel [s] [iv] the monitor's dashboard, ported; default 60s at 1 Hz
    go run . sizes         what each frame weighs, which decides the push floor
