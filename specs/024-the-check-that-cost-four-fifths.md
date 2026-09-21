# Spec 024: the check that cost four fifths

**Issue**: [#55](https://github.com/ushineko/hotaru/issues/55)

## Status: COMPLETE

## Context

The window was to be profiled before any more features, and the first profile
answered a question nobody had asked.

A 25-second CPU profile taken while the window was dragged, About open:

	11.14s of 13.55s   82%   runtime.Stack
	                          <- async.goroutineID
	                          <- async.IsMainGoroutine
	                          <- async.EnsureMain
	                          <- common.(*Canvas).Refresh

Fyne runs `EnsureMain` on **every canvas refresh** to warn about touching the
interface from the wrong goroutine. To find out which goroutine it is on, it
calls `runtime.Stack` into a thirty-byte buffer -- which formats a whole
traceback, symbolising frame after frame, and then keeps the first thirty
bytes to read the ID out of. Every text segment update, every `canvas.Text`
resize, every image refresh pays for a stack walk.

It is why the profile looked as though the program were panicking:
`recordForPanic`, `printlock`, `gwrite`, `printhexopts`, `pcvalue`, `step`,
`textAddr` are all traceback-printing machinery, and between them they were a
third of the samples before the walk itself was counted.

### Why this window may turn it off

The check exists for programs that have not moved to `fyne.Do`. This one has,
and not as an intention: `internal/gui/thread_test.go` walks the AST of every
`Perform` callback and fails the build when one touches the interface without
`onScreen` or `fyne.Do`. That test was written after a crash inside harfbuzz
(spec 017) and is the reason the tag is safe here.

So the tag and the guard are one decision. Dropping either means dropping
both, and this spec is where that is written down.

### What it bought

Same drag, same window, same document:

| | before | after |
|---|---|---|
| samples in 25 s | 13.55 s | 9.82 s |
| `markdown.Pane.Resize` | 11.40 s (84%) | 3.03 s (31%) |
| `runtime.Stack` | 11.14 s (82%) | gone |

The window's own resize work fell by 3.8x. What is at the top now is
`runtime.cgocall` -- GL and GLFW, Fyne actually drawing, which is where the
time should go.

### What it did not buy

A third profile, taken while the Scenes editor was resized, came to 18.84 s of
samples -- the heaviest of the three -- with the tag already on. Its cost was
never the thread check: 76% is `scrollContainerRenderer.Layout` laying out
every widget it holds, 28% is interface-keyed lookups in Fyne's renderer and
text caches, and 14% is `buttonRenderer.MinSize`. That is a widget-count
problem and is not this spec's.

## Requirements

**R1. Every way of building the window carries the tag.** A build that misses
it is a window four fifths of whose refresh time is a debug check, and the
difference is invisible from the outside.

**R2. A developer's install carries it too.** `go install ./cmd/...` does not,
which is what the whole profiling session was run against until this was
found.

**R3. The tag is guarded by a test**, in the same file as the thread guard
that makes it safe.

## Acceptance Criteria

- [x] AC1. `make gui` and `make install` build the window with
      `-tags migrated_fynedo`.
- [x] AC2. A test reads the Makefile and fails when any line that builds the
      window loses the tag, verified by removing it.
- [x] AC3. The existing thread guard still passes, since the tag rests on it.
- [x] AC4. Verified on the development machine by profiling the same drag
      before and after.

## Risks & Assumptions

- **The tag removes a safety net.** What replaces it is a test that fails the
  build rather than a warning nobody reads at runtime -- but only for the
  paths that test covers, which are `Perform` callbacks. Code that touches the
  interface from a goroutine started some other way is now unguarded, and the
  `-race` build is where that would show.
- **It is a Fyne internal.** `internal/build.DisableThreadChecks` is set by
  this build tag today; a later Fyne could rename it, at which point the
  window gets slower and nothing fails. The guard test checks the Makefile,
  not the effect.
- **Rollback** is deleting one variable from the Makefile.

## Alternatives Considered

Considered the `Migrations["fyneDo"]` key in `FyneApp.toml`, which Fyne also
honours; rejected because it still costs a `sync.Once` and a map lookup per
refresh where the tag is a constant the compiler folds away.
