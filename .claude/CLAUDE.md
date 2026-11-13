# hotaru Project Guidelines

How this repository is worked on. It is written for any contributor and any
coding agent; nothing here assumes a particular workflow tool. Where a rule
is enforced by a test, the test is named.

---

## Project overview

- **What it is**: a service, CLI and desktop window for RGB lighting and an
  NZXT Kraken cooler, on Linux and Windows. `hotaru serve` is the only
  process that touches the hardware; `hotaru` (CLI), `hotaru-gui` (Fyne
  window) and KDE global shortcuts are clients of it.
- **Platforms**: Linux is complete. On Windows the service, CLI, window,
  OpenRGB lighting and the AULA F75 work (specs 067, 071, 072); the Kraken,
  readings, hotkeys and sleep/resume do not yet. Code shared by both is the
  default; a platform split is a `_windows.go`/`_unix.go` (or `_linux.go`)
  pair behind one API, not `runtime.GOOS` branches through the logic.
- **Module**: `github.com/ushineko/hotaru`. Public repository, MIT.
- **Lighting**: through an OpenRGB server for most devices. Devices that
  only accept streamed frames (a sanshoku `lighting.Canvas`, the SteelSeries
  Apex Pro TKL Gen 3 first) are drawn by hotaru itself (spec 060).
- **Device code lives in sanshoku** (`github.com/ushineko/sanshoku`): the
  Kraken's telemetry and LCD (spec 059) and canvas devices (spec 060). hotaru
  adapts what sanshoku returns; it does not talk to hidraw or usbfs itself.
  A device protocol change is a sanshoku change first.
- **Window**: built on `github.com/ushineko/fynedesygn`, whose
  `docs/design-system.md` governs the window's shell, theme and widgets. A
  shape the library lacks is a library change, not a local copy.
- **Source of record for decisions**: the specs in `specs/`, and
  `docs/architecture.md` for the current shape of the system.

---

## Architecture rules

- **One actor on the hardware.** Only `hotaru serve` writes to devices.
  Clients go through the HTTP/JSON API on the unix socket: in
  `$XDG_RUNTIME_DIR/hotaru/` on Linux; on Windows in
  `%ProgramData%\hotaru\run\` for the installed service, else
  `%LOCALAPPDATA%\hotaru\run\`. A client that opens a device is a bug.
- **One service layer.** `internal/host` is the only code that knows how
  hotaru was started (systemd, the Windows service manager, or a terminal).
  `internal/services` is the only code that asks a service manager about
  OpenRGB. Installing hotaru's own service is packaging's job (the AUR unit,
  the fynstall installer), never the program's.
- **Machine mode on Windows.** The installed service runs as
  `NT SERVICE\hotaru` and its files live in `%ProgramData%\hotaru`, with
  access lists set by `config.PrepareMachine`. Clients switch to those paths
  when `host.Installed` is true; the window's own `gui.yml` stays per user
  (`config.UserDir`).
- **CLI, window and API parity.** Every operation is an API route, reachable
  from both the CLI and the window. `internal/api` lists the routes
  explicitly; `internal/cli/parity_test.go` and `internal/api/guard_test.go`
  fail when a route has no CLI path. The window follows by convention and
  `internal/gui` tests. A CLI-only feature is the parity rule backwards.
- **The inert rule.** A fresh install with nothing recorded touches no
  device. hotaru writes what a person asked for, and puts back what it
  recorded; it never "corrects" lighting it was not asked to manage.
- **Latest write wins per device.** Each device has a single-slot write
  queue; a superseded write is not a failure.
- **Device code reads, never assumes.** A device that cannot do what was
  asked is reported (`skipped`, with why), not forced.
- **A canvas device is never handed back implicitly.** On the Apex, release
  reboots the keyboard, so it happens only when a person asks
  (`hotaru light release`). Stopping the service leaves the last frame.
- **Cost is a requirement.** hotaru runs all day. A static scene sends the
  least its device allows: one frame and then nothing where a frame stays
  (the Apex), and the same frame again, no sooner than the device needs it,
  where the device takes its lights back (a sanshoku `lighting.Fading`
  canvas, the AULA F75 first). Animated effects default to the vendor's
  pace, not the device's floor. New background work states and measures its
  cost, and a device that needs a resend has that cost measured in its spec.

---

## Issue tracking

GitHub Issues on this repository is the tracker. Labels: `bug`,
`enhancement`, `chore`, `docs`.

- Anything worth a spec gets an issue first. The issue says what is wrong or
  wanted; the spec says what will be done.
- Specs are `specs/0NN-short-lowercase-title.md`, numbered in sequence. The
  first lines are:

  ```
  # Spec 0NN: <lowercase title>

  **Issue**: [#NN](https://github.com/ushineko/hotaru/issues/NN)

  ## Status: INCOMPLETE
  ```

- A spec carries: Executive Summary (written last), Context, Requirements
  (R1, R1.1, ...), Acceptance Criteria (`- [ ]` checkboxes), Risks &
  Assumptions (including Rollback), Alternatives Considered when a choice
  was not obvious, and Verification. Recent examples: specs 058–060.
- A spec is COMPLETE only when every criterion is checked, and the spec is
  updated in the same branch as the code.
- The issue links the spec once it exists; the PR says `Closes #NN`. GitHub
  does not always close the issue from the PR body, so check after merging.

---

## Tests

- `make test` (`go test ./...`) **opens no device** and passes on a machine
  with no hardware. There are no live-test build tags.
- Fakes carry only measured behaviour: `internal/openrgb.Fake` for the
  OpenRGB server, `internal/cooler/coolertest` for the cooler,
  `internal/canvas/canvastest` for canvas devices. Prefer extending one of
  these to adding a mock.
- Live checks against real hardware are manual, run with someone watching
  where the result is visual, and pasted into the spec's Verification
  section. Work that touches a device has at least one live acceptance
  criterion.
- Time-dependent code takes an injectable clock; tests do not sleep through
  real intervals.
- Canaries that fail the build:
  - `style_test.go`: every file in `docs/` follows `docs/style.md`
    (sentences of 30 words or fewer, banned words such as "simply" and
    "just").
  - `readme_test.go`: the README's mermaid diagrams match their rendered
    images (`make generate`, needs `mmdc`), and the window embeds the
    README as it is.
- GUI tests run with the window's build tag:
  `go test -tags migrated_fynedo ./internal/gui/...`.

---

## Documentation changes with the code

In the same branch as the change, never as a follow-up:

- `docs/architecture.md` when the shape of the system changes (re-render
  its diagram with `make generate`).
- `docs/hardware.md` when a device's support or behaviour changes.
- `docs/api.md` for a new route or a new field in a response.
- `docs/credits.md` when a protocol or approach was learned from another
  project.
- README: the changelog entry under `### Unreleased` with the spec number
  and issue link; "Where it comes from" and the Roadmap when they change.

Prose follows `docs/style.md`: plain verbs, active voice, simple present, no
marketing language.

---

## Environment

- Go from `go.mod` (`go 1.26.0`). `make lint` runs golangci-lint v2.12.2
  under Go 1.26.0 with the config in `config/`, and expects the binary at
  `~/go/bin/golangci-lint-v2.12.2`; install that release there first.
- `make build` builds the service and CLI without cgo. `make gui` builds the
  window, which needs cgo, OpenGL and X11/Wayland headers. `make install`
  puts both in `~/go/bin`.
- `make generate` needs `mmdc` (mermaid-cli). `make screenshots` needs a KDE
  Wayland session, kdotool, spectacle and Pillow, and takes over the screen.
- The packaged service unit (`packaging/hotaru.service`) runs
  `/usr/bin/hotaru serve` under systemd hardening (`ProtectSystem=strict`,
  `ProtectHome=read-only`, and others). Code that reads or writes outside
  its config, state and runtime directories will fail there even when it
  works from a terminal.
- To run a development build under the user's service without replacing
  the package, use a drop-in that overrides `ExecStart` (for example
  `~/.config/systemd/user/hotaru.service.d/dev.conf` pointing at a binary in
  `~/.local/bin`), then `systemctl --user daemon-reload` and restart.
  Remove the drop-in to go back.

### Windows

- Run `make` from PowerShell, not Git Bash: MSYS2's make drops the
  environment Go needs. The window needs a C compiler on `PATH` (MSYS2
  UCRT64 gcc). `make lint-windows` lints the Windows build (without the window, which
  needs cgo) from any system.
- `make installer` builds the fynstall installer into `dist/` (fynstall
  pinned in the Makefile, run with `go run`). `fynstall.yaml`'s
  `app.version` must equal `.tag`; `installer_test.go` checks it.
- The installed service logs to `%ProgramData%\hotaru\state\hotaru.log`,
  and reports a failure to start to the Event Log. Control it with
  `Start-Service`/`Stop-Service`/`Restart-Service hotaru` from an
  administrator shell. A `hotaru serve` in a terminal while the service is
  installed uses the machine's paths too, so stop the service first.
- `make generate` works on Windows with mermaid-cli from npm and no
  Puppeteer browser download: set `PUPPETEER_EXECUTABLE_PATH` to Edge's
  `msedge.exe` first.
- OpenRGB must be 1.1 or later (1.0 misroutes writes, #191), and on MSI
  boards it must start after LibreHardwareMonitor (#197).
- Windows changes also run on Linux before merging (`make test`,
  `make lint`), since there is no CI.

---

## Public-repository rules

- No credentials, tokens or personal data in code, fixtures, docs or logs.
- No personal paths, hostnames, serial numbers or Bluetooth addresses in
  committed files. Fixtures use made-up identities.
- No third-party assets committed without a licence that allows it; credit
  sources in `docs/credits.md`.

---

## Git

None of this is enforced by GitHub; it is the convention across the
ushineko repositories.

- Work happens on a branch (`feat/`, `fix/`, `chore/`, `docs/` and a short
  slug) and lands on `main` through a PR. A worktree per branch is
  preferred when several pieces of work are in flight.
- Commit subjects: lowercase conventional prefix, imperative,
  sentence-like (`fix(canvas): count, restore and reattach as the live run
  showed`). The body says why; the diff says what.
- Stage files by name and check the staged list; never commit build output
  (`bin/`, release tarballs).
- A PR body says what changed, why, what a reviewer should look at first,
  and how it was verified, and links the spec.
- **No AI attribution** in commits or PRs: no `Co-Authored-By` trailers for
  tools, no "Generated with" footers.
- hotaru has no CI. Run `make test` and `make lint` before pushing; the PR
  shows only the external secret scan.

---

## Releases

The version of record is `.tag` (the Makefile stamps it into the binaries).
`.tag`, the `**Version**` line in `README.md`, the newest changelog heading,
`pkgver` in `packaging/PKGBUILD` and `app.version` in `fynstall.yaml` are the
same string, or the release is wrong. Ask the maintainer before bumping.

1. A commit `chore: release X.Y.Z` that sets `.tag`, the README Version
   line, turns `### Unreleased` into `### X.Y.Z (YYYY-MM-DD)`, and sets
   `pkgver` and `fynstall.yaml`'s version.
2. An annotated tag `vX.Y.Z` with the message `hotaru X.Y.Z`, pushed.
3. A GitHub Release for the tag whose notes are that version's changelog
   entry. Every tag gets one.
4. The AUR package: `updpkgsums` in `packaging/` (the tarball exists only
   once the tag is pushed), `namcap` on the built package, commit the
   checksum, then `scripts/update_aur.sh`.

Run each step only if the one before it succeeded.
