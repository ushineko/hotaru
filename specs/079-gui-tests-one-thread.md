# Spec 079: the window's tests draw on one thread

**Issue**: [#219](https://github.com/ushineko/hotaru/issues/219)

## Status: COMPLETE

## Executive Summary

The window's tests drew from several goroutines at once, because Fyne's test
driver runs `fyne.Do` in place where the window's driver queues it; the race
detector found 217 races and the text shaper panicked now and then. The test
build now queues `onScreen` and draws it on the test's goroutine with
`gui.Settle()`; the poll hands back through `onScreen` too. Three tests settle
while they wait, the three skipped drop tests run again, and the package
passes ten runs of ten with no race. Reviewers should look at
`internal/gui/export_test.go` and `internal/gui/app.go`.

## Context

The window's tests fail now and then with a panic inside the text shaper
(`go-text/typesetting/harfbuzz`), and `go test -race` reports some two hundred
races in `internal/gui`. Found on 2026-11-17 (#219):

- The window hands work from its workers back to the screen with `onScreen`,
  which calls `fyne.Do`. Fyne's own driver queues that onto the UI thread.
  Fyne's test driver runs it in place, on whichever goroutine calls it
  (`fyne.io/fyne/v2@v2.8.1/test/driver.go:53`). So in the tests, workers'
  callbacks (a dashboard's redraw, a picture's thumbnail, a drop's dialog)
  draw beside each other and beside the test, and the shared shaper panics.
  The window itself is not affected.
- Three picture-drop tests were skipped for 2.0.0 because every panic in
  twelve runs came from them. They drop a file, choose a layout, build a
  dashboard and a scene, and check all of it at once.
- Some sections have a `Settle` that waits for their own workers, a fix for
  the same cause in two places.
- Queuing the callbacks onto a goroutine of their own was tried and reverted:
  they then drew beside the test instead, and tests that expect a drop's
  dialog at once failed.

## Requirements

- R1 Every hand-back to the screen goes through `onScreen`, the poll's too.
  The window's build calls `fyne.Do`, as now.
- R2 The tests' build queues `onScreen`'s work, and `gui.Settle()` runs the
  queue on the calling goroutine, the test's, until it is empty. Nothing a
  worker hands back draws until a test settles, and then on the test's
  goroutine, as the window's driver draws on its one thread.
- R3 Tests that wait for a worker's drawing settle while they wait. A test
  that finishes leaves nothing queued for the next.
- R4 The three skipped drop tests run again. Pared down if they still fail;
  with R2 they pass every run, and are kept as they were.
- R5 The sections' own `Settle`s are kept where they wait for workers that
  are not yet done, and draw through R2 once they are.

## Acceptance Criteria

- [x] `make test` and `make lint` pass, and the GUI tests pass ten runs of
      ten with nothing skipped for #219.
- [x] `go test -race -tags migrated_fynedo ./internal/gui/` reports no race
      with `onScreen` or the poll on either side.
- [x] Tests: a callback handed to `onScreen` does not draw until `Settle`,
      and draws on the caller's goroutine; the three drop tests, re-enabled.
- [x] The window opens and works on the desk as before (no change for the
      window's driver).

## Risks & Assumptions

- **Tests only.** The window's driver path is `fyne.Do`, unchanged; the
  queue exists only in the test build (`export_test.go`).
- **A test that forgets to settle** sees nothing drawn and fails plainly,
  rather than drawing beside itself and panicking now and then.
- **Rollback:** revert.

## Verification

2026-11-17, on `fix/gui-test-thread`:

- `make test` and `make lint` (0 issues) pass.
- `go test -tags migrated_fynedo ./internal/gui/`: ten runs, ten passes,
  nothing skipped. Before: about one run in five failed, with three tests
  skipped.
- `go test -race -tags migrated_fynedo ./internal/gui/`: no race. Before: 217,
  all but one through `onScreen`.
- Only three tests waited on a worker's drawing without settling
  (`TestANewPictureIsKeptBeforeTheLayoutIsAsked`,
  `TestMakeASceneShowsThePicturesColoursBeforeMakingIt`,
  `TestShowingADraftDrawsItWithoutSavingIt`); they settle while they wait.
- `TestAHandBackDrawsOnlyWhenSettledOnTheSettlersGoroutine` holds the queue
  to R2, under `-race`.
- The window built from the branch, on the desk: the maintainer clicked
  through it and it worked as before.
