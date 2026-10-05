# Spec 062: drop onto screen and scenes

**Issue**: [#179](https://github.com/ushineko/hotaru/issues/179)

## Status: COMPLETE

## Executive Summary

A picture dropped on Screen or Scenes is kept in Pictures as before, and then
offered the saved screens as layouts, each rendered with the picture behind
it. Screen opens its editor on the copy; Scenes opens Make a scene, and
"Make it" saves the screen and a scene that shows it, with the lights from the
picture. The library records the SHA-256 of each file it converts, so a file
it already holds is used rather than kept twice.

Look first at `internal/gui/dropped.go` (routing, the chooser, both flows),
the `onKept` continuation through `internal/gui/pictures.go`, and
`SceneFromImageOn` in `internal/service/scenefromimage.go`.

## Context

A file dropped on the window goes to Pictures, whatever is on screen
(`App.dropped` in `internal/gui/app.go`). The library converts it, asks for a
name and keeps it (`PicturesSection.Dropped`, `preview`, `keep`, `store`).

What somebody does next with a wallpaper is usually one of two things, both by
hand today:

- **A screen with the picture behind it.** Open Screen, copy a saved screen,
  set its background to the picture, save. The maintainer's own `custom1` to
  `custom4`, `forest1` and `planet1` are this: a saved screen's layout with a
  picture behind it.
- **A scene from the picture.** Pictures' "Make a scene from it" builds the
  lights from the picture and puts the picture alone on the panel
  (`SceneFromImage`, `internal/service/scenefromimage.go`). A scene that puts a
  screen with the picture behind it on the panel needs the screen made first,
  then the scene made from the screen's rendered frame (`SceneFromDashboard`).
  That frame takes its colours from the dimmed picture and the readings drawn
  over it, not from the picture.

The library has no idea whether a dropped file is already in it. Stored
pictures are converted GIFs, and the conversion is not repeatable byte for
byte: `popular` in `internal/images/images.go` sorts histogram cells with
`sort.Slice` over a map, so cells with equal counts come out in any order and
the palette differs between two runs. A hash of the stored file cannot say
"this is the same picture".

## Requirements

### R1. The source's hash

- R1.1 `images.Library.Add` records the SHA-256 of the source bytes beside the
  stored picture, as `<name>.sha256` holding the lowercase hex digest.
  `AddSlideshow` records none: a slideshow has no single source.
- R1.2 `Library.All` and `Library.Rename` report it as `Image.Source`, empty
  when there is no record. `Rename` moves the record with the picture.
  `Remove` deletes it. Replacing a picture under the same name replaces it.
- R1.3 The API's `Image` carries it as `source_sha256`, omitted when empty, in
  `GET /v1/images` and in the answer to `PUT /v1/images/{name}`.
- R1.4 Pictures stored before this change have no record, so they never match.
  This is stated in `docs/api.md`.

### R2. A scene from a picture, with a screen

- R2.1 `SceneFromImageRequest` gains `screen`: what the scene puts on the
  panel instead of the picture. Empty keeps today's behaviour (the picture).
- R2.2 The service accepts `dashboard:<name>` for a saved screen, and refuses
  a screen that does not exist by name. The lights still come from the
  picture.
- R2.3 The CLI's `hotaru image scene` takes `--screen <name>` for a saved
  screen.

### R3. One import step

- R3.1 Every picture the window imports goes through one step: read the file,
  hash it, and look for a stored picture with that `source_sha256`. A match
  is used: nothing is converted or stored, and the window says
  "Already in Pictures as <name>." Otherwise the step is today's: convert,
  show the result, ask for a name, keep it.
- R3.2 This applies to a drop on any tab and to "Add a picture".
- R3.3 A slideshow is not checked, and keeps its own dialog.

### R4. Where a drop goes

- R4.1 A drop while Create shows Screen or Scenes, with no editor open in that
  part, stays on that part and runs R5 or R6.
- R4.2 Anywhere else, including an open editor in Screen or Scenes, a drop
  goes to Pictures as today.
- R4.3 Several files dropped on Screen or Scenes ask the existing stack
  question. "One slideshow" continues with R5 or R6 for the slideshow.
  "Keep them separately" keeps each in Pictures, one question each as today,
  and says that one picture at a time makes a screen or a scene.

### R5. A drop on Screen

- R5.1 After the import, a chooser offers the saved screens as layouts: each
  with its thumbnail and its name, shipped ones included, the active one
  selected. The chooser has the shape of the scene editor's screen chooser.
- R5.2 Choosing one opens the screen editor on a new draft: a copy of that
  screen with `shipped` cleared and its background set to the picture
  (`kind: picture`), keeping the copied screen's dim. The name field holds
  the picture's name, or `<name>-2`, `<name>-3` and on when a screen of that
  name exists.
- R5.3 Nothing is saved until the editor's Save. Cancel in the chooser or in
  the editor leaves no screen behind. The picture stays in Pictures.

### R6. A drop on Scenes

- R6.1 After the import, the same chooser opens, with one more option first:
  "the picture alone", which is what Pictures' "Make a scene from it" puts on
  the panel.
- R6.2 Choosing opens the Make a scene dialog (`makeScene`) for the picture,
  with the same name, separation and effect fields. The scene's name is the
  picture's name.
- R6.3 "Make it" with a saved screen chosen first saves a new screen, as R5.2
  builds it under the unique name, then makes the scene with
  `screen: dashboard:<that name>`. With "the picture alone" it makes the scene
  as Pictures does.
- R6.4 Cancel in the chooser or in the dialog leaves no screen and no scene.
  The flash after "Make it" names both the scene and any screen made.

### R7. Documentation

- R7.1 `docs/api.md` for `source_sha256` and the `screen` field.
- R7.2 README changelog under `### Unreleased`, with the spec number and the
  issue link.

## Acceptance Criteria

- [x] `images` tests: `Add` records the source hash and `All` reports it; a
      second `Add` under the name replaces it; `Rename` moves it; `Remove`
      deletes it; `AddSlideshow` and an older picture without a record report
      none.
- [x] `GET /v1/images` carries `source_sha256` for a picture added through
      `PUT /v1/images/{name}`, against a real `images.Library` in a temp dir.
- [x] `POST /v1/images/{name}/scene` with `screen: dashboard:<name>` saves a
      scene whose screen is that dashboard. An unknown dashboard is refused
      and saves nothing.
- [x] `hotaru image scene --screen <name>` sends `dashboard:<name>`.
- [x] GUI: a dropped file whose hash matches a stored picture converts and
      stores nothing (no `POST /images/preview`, no `PUT /images/...`).
- [x] GUI: a drop on Screen opens the layout chooser and stays on Screen;
      choosing a screen opens the editor with a draft whose background is the
      picture, whose layout is the chosen screen's, and whose name is unique.
- [x] GUI: a drop on Scenes opens the chooser with "the picture alone" first;
      "Make it" after a saved screen saves a screen with the picture behind it
      and a scene whose screen is that screen.
- [x] GUI: a drop on Screen with the editor open still goes to Pictures.
- [x] `make test`, `go test -tags migrated_fynedo ./internal/gui/...` and
      `make lint` pass.
- [x] Live: with the local build installed, a wallpaper dropped on Screen and
      on Scenes produces the screen and the scene described, a second drop of
      the same file reports it is already stored, and the panel shows the new
      screen when the scene is applied.

## Risks & Assumptions

- **Exact files only.** The hash is of the file as dropped. The same picture
  saved again by an editor, or resized, is a different file and is stored
  again. Pictures stored before this change are never matched (R1.4).
- **Sidecar files.** The library directory gains `<name>.sha256` files.
  `All` lists only `.gif` files, so an older hotaru ignores them. A picture
  removed by an older hotaru leaves its record behind; a later `Add` under the
  name overwrites it.
- **A scene's screen comes from the request.** `screen` is checked against
  the saved screens, so a typo is refused rather than recorded.
- **No device is touched.** Importing, saving a screen and saving a scene
  write files; none of them applies anything. The inert rule holds.
- **Shared code.** `makeScene` is unchanged in shape: the Scenes flow saves
  the screen inside the `build` callback it already takes. Its flash now also
  names a saved screen the scene shows, when that is not what the scene was
  made from.
- **Recolouring.** `hotaru scene recolour` on a scene that shows a dashboard
  takes its colours from the dashboard's frame (spec 061 behaviour). A scene
  made by R6 is therefore recoloured from the dimmed picture with readings
  over it, not from the picture. Left as it is; a recolour that read the
  dashboard's background picture would be a separate change.
- **Rollback**: revert the branch. Scenes saved with a dashboard screen work
  on older builds, which already read `dashboard:<name>`. Sidecar files are
  ignored by older builds.

## Alternatives Considered

- Considered hashing the stored GIF; rejected because the conversion is not
  repeatable byte for byte (Context).
- Considered making the scene from the new screen's frame
  (`SceneFromDashboard`); rejected because that frame is the dimmed picture
  with readings over it, which shifts the colours away from the picture.
- Considered checking the hash in the service on `PUT /images`; rejected
  because a `PUT` under a name means "keep this under that name", and the CLI
  relies on that.

## Verification

### Checks

Run in the worktree on 2026-10-04.

```
$ make test                      # go test ./...
ok  ... (every package)
$ go test -tags migrated_fynedo ./internal/gui/...
ok  	github.com/ushineko/hotaru/internal/gui	2.884s
$ make lint
0 issues.
$ govulncheck ./...              # govulncheck v1.8.0
No vulnerabilities found.
```

The new tests, by criterion:

| Criterion | Test |
|---|---|
| Library records, renames, removes the hash | `TestAPictureRemembersTheFileItCameFrom`, `TestAPictureWithNoRecordHasNoSource` (`internal/images`) |
| `source_sha256` through the socket | `TestTheLibraryReportsWhereAPictureCameFrom` (`internal/cli`, real service and library) |
| `screen` on a scene from a picture, and refusal | `TestASceneFromAPictureCanShowASavedScreen` (`internal/cli`, real service, library, scene and dashboard stores) |
| A held file is not converted or stored | `TestADroppedFileTheLibraryHoldsIsNotKeptAgain` |
| Drop on Screen | `TestAPictureDroppedOnScreenOpensTheEditorWithItBehind`, `TestANewPictureIsKeptBeforeTheLayoutIsAsked` |
| Drop on Scenes | `TestAPictureDroppedOnScenesMakesAScreenAndAScene`, `TestAPictureAloneMakesASceneAndNoScreen` |
| Open editor goes to Pictures | `TestADropOnAnOpenScreenEditorGoesToPictures` |

The two Screen and Scenes drop tests were run once with the routing in
`App.dropped` disabled, and both failed, so they test the routing.

### Live run

Installed with `make install` (the service runs `~/go/bin/hotaru` through the
existing `dev-build.conf` drop-in) and restarted with
`systemctl --user restart hotaru`. Run by the maintainer in the window on
2026-10-04, after the fix under Gaps found:

- A wallpaper dropped on Scenes kept the Scenes tab in front and showed the
  conversion preview ("This is what the panel will show") with the name
  prompt. A screenshot of that step is the evidence recorded here.
- After "Keep it", the layout chooser and Make a scene followed. The
  maintainer reported the flow as working ("works") and approved the change.

### Gaps found

- **The layout chooser closed as it opened** (first live drop). Keeping a
  new picture ran through `sh.Perform`, and the chooser was opened from inside
  that work, above Perform's busy popup. Removing the popup removed every
  overlay above it, so the drop looked like a drop on Pictures. The headless
  test shell is never on screen, so Perform runs its work inline with no
  popup, and the tests could not see this. Fixed in `PicturesSection.keeping`:
  a keep that something follows runs as a goroutine, as `preview` already
  does. No automated test covers it; the live run does.
