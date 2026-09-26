# Spec 053: a name somebody can change

**Issue**: [#157](https://github.com/ushineko/hotaru/issues/157)

## Status: INCOMPLETE -- AC12 is a look at the machine, and is the author's

## Context

Every named thing in hotaru is named once, at the moment it is made, and never
again. A scene, a screen and a picture are all keyed by their name -- the map
key in `scenes.yml`, the map key in `dashboards.yml`, the filename in the image
directory -- and the only way to change one is to make a second thing and
delete the first.

Doing that by hand loses whatever pointed at the old name. A key bound to
`evening` still fires `evening`. A scene saying `screen: dashboard:load` still
says `load`. A dashboard whose background is `wallpaper` still asks for
`wallpaper`, and the panel quietly falls back to the theme's plain colour,
because a missing picture costs the background and not the frame. None of that
reports anything. The machine keeps working and shows the wrong thing.

So a rename is not a rename of one entry. It is a rename plus every reference
to it, and the references are what make it worth writing.

### What points at what

| Renamed | Kept as | Named by |
|---|---|---|
| A scene | a key under `scenes:` | the key bindings, and the KWin script built from them |
| A screen | a key under `dashboards:` | `active:`, and every scene saying `screen: dashboard:<name>` |
| A picture | `<name>.gif` in the image directory | every scene saying `screen: <path>`, and every screen whose background is that picture |

The window adds one more: a thumbnail is cached under a key built from the
picture's name, so a rename that did not drop the old key would leave a
thumbnail nobody can reach taking up the cache.

### Shipped things are code

The nine shipped scenes and the shipped screens are carried in the binary, not
in anybody's file. There is nothing to rename: the name is a string literal,
and a file entry that renamed one would be an override of a name plus a
shipped entry that came back on the next release.

The honest answer is to refuse, and to offer the thing somebody actually
wanted. For a screen that is a clone.

### The same screen with another picture

Cloning is the second half of this spec, and it is where the request started.
A dashboard is a dozen decisions -- the arrangement, the theme, four rings,
the lettering, the caption -- and wanting the same dozen with a different
photograph behind them is not an unusual thing to want. Today it is rebuilding
all twelve by eye in a second editor and hoping they match.

A clone is one call: the same screen under a new name, not shipped, not shown.
The picture is then one field in the editor, and a scene naming the new screen
is the existing "make a scene from this screen" button.

Cloning a shipped screen is allowed, and is the one way to start from one.
That is what it always meant to save a shipped screen under another name; this
says so in a verb.

## Requirements

**R1. A saved scene, screen or picture can be renamed**, and everything that
named it names the new one. The list in "What points at what" is the list.

**R2. A rename reports what else it changed**: how many scenes, how many
screens, how many keys, and whether the screen the panel draws was one of
them. A rename that touched nothing else says so by counting nothing.

**R3. A shipped scene or screen cannot be renamed.** The refusal names the
thing to do instead.

**R4. A name that is taken is refused, and nothing is written.** Taken
includes a shipped name, because saving over one is a different decision from
renaming into one.

**R5. A screen can be cloned**, shipped ones included. The copy is not
shipped, is not made active, and is refused the same way on a name that is
taken.

**R6. Rename and clone are on every surface**: the service, the API, the CLI
and the window. The parity rule, which is already asserted by a test.

**R7. What the window remembers about a renamed thing is dropped** -- the
thumbnail under the old name, and the chooser's kept list of screens -- so the
next look is of what is there now.

**R7a. A picture's tile is widened to hold the button put on it**, from 150
to 168. Four icons centred in a tile of 150 hang over both edges, and the ones
at the ends are the scene and the delete.

**R8. What the service reports it is showing follows a rename**, so status
after renaming the applied scene is not the name of a scene that no longer
exists.

## Acceptance Criteria

- [x] AC1. Renaming a scene moves it, and a key bound to the old name fires
      the new one (`TestRenamingASceneTakesItsKeysWithIt`).
- [x] AC2. Renaming a screen rewrites every scene naming it, and follows the
      panel when it was the active one
      (`TestRenamingAScreenTakesTheScenesThatNameItWithIt`).
- [x] AC3. Renaming a picture moves the file, rewrites every scene naming its
      path and every screen using it as a background
      (`TestRenamingAPictureTakesItsScenesAndBackgroundsWithIt`).
- [x] AC4. A rename reports the counts, and a rename of something nothing
      points at reports none (`TestARenameSaysWhatElseItChanged`).
- [x] AC5. Renaming a shipped scene or screen is refused and nothing is
      written (`TestAShippedThingCannotBeRenamed`).
- [x] AC6. Renaming onto a taken name is refused and neither entry changes
      (`TestARenameOntoATakenNameChangesNothing`).
- [x] AC7. Cloning a screen copies every field under the new name, leaves the
      original alone, and does not make the copy active
      (`TestCloningAScreenCopiesItWithoutShowingIt`).
- [x] AC8. Cloning a shipped screen produces one that is not shipped
      (`TestAClonedShippedScreenIsNotShipped`).
- [x] AC9. Every new route is reachable from the CLI
      (`TestEveryRouteTheServiceServesIsReachableFromTheCommandLine`, the
      existing parity test, extended).
- [x] AC10. Renaming a picture in the window drops the thumbnail cached under
      the old name (`TestRenamingAPictureForgetsItsThumbnail`).
- [x] AC11. The applied scene's name follows a rename
      (`TestTheAppliedSceneFollowsARename`).
- [ ] AC12. Verified on the development machine: rename a picture a scene and
      a screen both use, and both still show it; clone a screen, change its
      picture, and the original is unchanged.
- [x] AC14. Taking the name the copy dialog offers copies the screen, rather
      than closing and doing nothing (`TestTakingTheNameACopyOffersIsAnAnswer`,
      `TestARenameThatChangesNothingAsksForNothing`). Found by copying a
      screen in the window and watching nothing happen: the dialog was given
      one name for both what to fill the field with and what means "nothing
      was asked for", which is the same name for a rename and two different
      names for a copy.
- [x] AC13. The window's rows and tiles still hold the buttons put on them:
      a screen's row fits the section at the default window size
      (`TestAScreenRowFitsTheWindowAtItsDefaultSize`) and a picture's tile
      fits its four icons (`TestAPictureTileHoldsItsOwnButtons`).

## Alternatives Considered

- **Refusing a rename while anything points at it**, and making somebody fix
  the references first. It is the safest rule and the wrong one here: a
  picture used by five scenes is exactly the picture whose name somebody got
  wrong, and five manual edits is how a feature goes unused.
- **A rename as a `PUT` to the new name with the old one in the body.** The
  routes here already spell a verb that is not CRUD as a sub-path -- `/use`,
  `/apply`, `/capture` -- and a rename is one of those.
- **Cloning scenes and pictures too.** A scene is already cloneable by saving
  a previewed one under another name, and a picture's clone is a second copy
  of a file with no use case behind it. Left out until one turns up.

## Risks & Assumptions

- **A rename is several writes and is not atomic.** The scene file, the
  dashboard file and the image directory are three stores, and a machine that
  loses power between them has a reference pointing at a name that has moved.
  The cost is a scene drawing the theme's plain colour instead of a
  photograph, which is what a deleted picture already costs, and the fix is to
  rename it back. Not worth a journal.
- **Rewriting somebody's file on their behalf.** The references written are
  the ones the rename made stale and nothing else, and the count printed is
  what says so. A rename that cannot rewrite them all still renames: a
  half-updated file is recoverable and a refusal halfway through is not.
- **A picture's rename checks for a name that is taken and then moves the
  file**, which is two operations and not one. Another process creating that
  name in between would have it overwritten: Go has no portable atomic
  rename-if-absent, and the library has no lock anywhere else either. The
  window is a few microseconds wide, both writers would be the person's own
  hotaru, and the cost is one picture. Left as it is rather than given a lock
  the rest of the type does not have.
- **A picture's name is cleaned** before it becomes a filename, so the name
  asked for and the name given are not always the same string. The result
  carries the name as stored.
- **Rollback** is a revert. Nothing is stored differently: the files keep the
  shape they have, and a hotaru without this feature reads a renamed file
  exactly as it reads any other.

## Executive Summary

Scenes, screens and pictures can be renamed, and a screen can be copied under
another name. A rename carries its references with it -- the keys bound to a
scene, the scenes naming a screen, the screens drawing a picture behind their
numbers, the screen the panel is set to -- and reports how many it moved.
Shipped entries are refused, because their names are in the binary; cloning is
what to do with one instead.

Reviewers should look at `internal/service/rename.go` first: it is the whole
of the reference-rewriting, and the table in the Context section is the list
it has to be complete against.
