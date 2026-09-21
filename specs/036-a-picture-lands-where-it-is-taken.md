# Spec 036: a picture lands where it is taken

**Issue**: [#84](https://github.com/ushineko/hotaru/issues/84)

## Status: COMPLETE

## Context

Dropping a picture on the window converted it, kept it, and then sent the
window to the service page.

The handler asked for the section by name:

	s.Window.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		s.Select("Pictures")
		a.pictures.Dropped(s, uris)
	})

Pictures stopped being a section in spec 030, when it became one of three
parts of Create. `Shell.Select` documented unknown titles as ignored and
navigated to the first section instead, so every drop went to Service. The
picture arrived; the window did not.

Two halves, in two repositories. fynedesygn's `Select` ignores a title no
section has (its spec 028, and `Options.Section` keeps the fallback, because
"open somewhere" and "navigate now" are different questions). This half asks
for the two things that do exist: the group, and the part inside it.

**A call that still compiles after the thing it names has moved.** Section
titles are strings on both sides of this, and nothing checks them -- which is
why the test is about where the window ends up rather than about what was
called.

## Requirements

**R1. A dropped picture shows the part that takes it**: Create, on Pictures.

**R2. The drop still works from anywhere**, whatever section is on screen.

## Acceptance Criteria

- [x] AC1. Dropping a file while another section is open leaves the window on
      Create, showing Pictures.
- [x] AC2. The picture reaches the library's drop handler.
- [x] AC3. Verified on the development machine.

## Risks & Assumptions

- **Titles are still strings.** A future rename breaks this again, silently on
  hotaru's side; what changed is that the shell no longer turns that into a
  jump to the front page. The test is the guard.
- **Rollback** is a revert.
