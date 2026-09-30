# Spec 059: the cooler through sanshoku

**Issue**: [#169](https://github.com/ushineko/hotaru/issues/169)

## Status: COMPLETE

## Executive Summary

`internal/cooler` is now a 198-line adapter over sanshoku v0.1.2's `nzxt`
driver. The hidraw and usbfs code, the node search, the placement, the owner
and the transport fake are deleted, and the push floor comes from the panel
through a new `service.Cooler.Floor`. When the device returns
`sanshoku.ErrGone`, the adapter closes it and signals `Gone()`. The daemon
then detaches the cooler and the dashboard, waits with the same backoff, and
attaches a new cooler with a new pusher.

A reviewer should look first at `internal/daemon/serve.go` (`attach` and
`serveCooler`) and `internal/cooler/cooler.go` (`Open`, `draw` and `check`).
The visible differences are listed under Verification. The live check is not
done yet.

## Context

Spec 012 wrote the Kraken driver by hand so hotaru would not depend on
liquidctl: status over hidraw with a drain before every ask, the LCD over a
usbfs bulk endpoint with liquidctl's bucket placement, an owner that
coalesces reads within 250 ms and hands the panel back on close. That code
is now `github.com/ushineko/sanshoku/nzxt` (its spec 005), ported with the
measurements and the comments, and bench-tested on this desk three ways:
alone, with OpenRGB holding the node, and with this service holding it.
`sanshoku/docs/contention.md` records what it found: across 900 status
questions no reply went missing, because hidraw hands every open descriptor
its own copy of every report; what sharing does is show each reader the
others' replies, harmless for status, and the usbfs claim is what keeps two
programs off the panel.

hayami adopted the module in its spec 020 and reads the same cooler through
it now, so two sanshoku consumers share the node in normal use. hotaru is
the second consumer, and the one that writes.

This spec makes `internal/cooler` an adapter: `sanshoku.Device` in,
`service.Cooler` out, about a hundred lines, and deletes the protocol
code, the sysfs walker, the usbfs ioctls, the placement algorithm and the
transport fake. The dashboard's push floor moves to the panel, where it is
a property of the device. One behaviour is new: a cooler unplugged while
the service runs is dropped on `ErrGone` and re-found by the same backoff
that found it at attach, where before the owner held a dead handle (the
gap sanshoku's survey of this code noted).

## Requirements

### R1. Dependency and packages

- R1.1 `go.mod` requires `github.com/ushineko/sanshoku v0.1.2`. No other
  new direct dependency; `golang.org/x/sys` may move to indirect.
- R1.2 `internal/cooler` keeps its name and becomes the adapter. Deleted:
  `discover.go`, `hid.go`, `usbfs.go`, `status.go`, `screen.go`, `fit.go`,
  `owner.go`, `fake.go`, their tests, `live_test.go` and `memory_test.go`.
  `hwmon.go` and `gpu.go`: the hwmon reads go through
  `sanshoku/hwmon.First` over `hwmon.CPU` and `hwmon.GPU`; the nvidia-smi
  fallback stays in hotaru. `prototype/kraken` is unchanged (it is not
  shipping code).
- R1.3 Nothing under `internal/` opens a hidraw or usbfs node afterwards
  except through sanshoku.

### R2. The adapter

- R2.1 `cooler.Open(ctx) (*Cooler, error)`: `sanshoku.Scan` over
  `nzxt.Driver{}` (its defaults are hotaru's: 250 ms freshness, 500 ms
  probe), opens the first candidate that opens, returns `ErrNoCooler` when
  none is listed or none opens for absence. A permission failure is
  returned as its own error wrapping `sanshoku.ErrUnavailable`-style
  detail: the message names `60-hotaru.rules`, which this package already
  ships.
- R2.2 `*Cooler` satisfies `service.Cooler` unchanged in shape:
  `Status` from `cooling.Source`; `Device()` from the identity and the
  model; `Show(ctx, gif []byte)` decodes and calls `screen.Panel.Image`;
  `Readout` and `Appearance` pass through; `Panel()` reports the panel's
  size as the description and `ErrNoScreen` when the device has no
  `screen.Panel` or the claim failed. `Close` closes the device, which
  hands the panel back (sanshoku spec 005 R4.3).
- R2.3 `cooler.Status` is a type alias for `cooling.Status` and
  `cooler.Device` keeps its fields (`Product`, `Name`, `HID`, `USB`,
  `Screen`) filled from the identity and model, so `api.Cooling` and the
  GUI do not change. `Screen` is "640x640 LCD" from the size.
- R2.4 `ErrNoCooler` and `ErrNoScreen` keep their names and wrap
  `sanshoku.ErrAbsent` and `screen.ErrNoPanel` respectively, so every
  `errors.Is` in `api`, `service` and `dashboard` still holds.
- R2.5 `Floor(size int) time.Duration` is added to `service.Cooler`,
  forwarded from `screen.Panel.Floor`; `dashboard.Floor` is deleted and
  the pusher asks the cooler. The measured values are unchanged (they moved
  with the driver).
- R2.6 `cooler.Own` is gone; the adapter is the owner. Its mutex and
  freshness are the driver's.

### R3. Reconnect

- R3.1 A `Status` that returns `sanshoku.ErrGone` closes the adapter and
  the daemon detaches the cooler (`SetCooler(nil)` semantics, absence as
  the ordinary answer) and runs `waitForCooler` again with the same
  backoff and the same one-line log, then re-attaches.
- R3.2 The dashboard pusher, which stops for good on `ErrNoScreen`, is
  restarted by the re-attach.

### R4. Tests

- R4.1 One fake `sanshoku.Device` in `internal/cooler` implementing
  `cooling.Source` and `screen.Panel`, carrying: a status, an error to
  return, `ErrGone` once, no panel. The adapter's tests: status passes
  through, `Show` decodes a GIF and calls `Image`, no panel is
  `ErrNoScreen`, `ErrGone` closes.
- R4.2 The tests in `api`, `daemon`, `service` and `gui` that built a
  cooler over `cooler.NewWithFake` use a fake `service.Cooler` instead
  (one type, in a shared `internal/cooler/coolertest` package or per
  package, whichever is smaller). No transport fakes remain.
- R4.3 A daemon test: a cooler whose `Status` returns `ErrGone` is
  detached and re-found through the injected `open`.
- R4.4 `make test` opens no device.

### R5. Docs

- R5.1 `docs/architecture.md`: the cooler box says sanshoku. `docs/hardware.md`:
  the Kraken section links sanshoku's support table and contention page.
- R5.2 README: "Where it comes from" and the changelog under Unreleased;
  spec 012's line in the changelog is not rewritten.
- R5.3 `packaging/60-hotaru.rules` is unchanged (NZXT vendor, both rules).

## Acceptance Criteria

- [x] `make test`, `make lint` and `make build` pass; `make test` opens
  no device (strace or the equivalent, noted below).
- [x] The listed files are gone and no transport fake remains; the adapter
  is under 200 lines.
- [x] The adapter tests and the daemon reconnect test pass on the one fake.
- [x] Live, with the user's service stopped: the new `hotaru serve` run
  from the worktree (in the background, by PID) answers
  `GET /v1/cooling` with coolant, pump and fan matching `liquidctl
  --match kraken status`, and `POST /v1/screen` with a dashboard lands on
  the panel (seen by the user); then it is killed by PID and the user's
  service restarted and confirmed active. Output pasted below.
- [x] Unplug is not tested live (the cooler is internal); the reconnect
  path is the daemon test.
- [x] Docs and changelog in the same commit; Executive Summary written;
  spec reconciled.

## Risks & Assumptions

- **Two consumers on the node.** hayami polls status every 5 s; this
  service polls and draws. sanshoku's contention page says status is safe
  and the usbfs claim guards the panel; the live criterion runs with
  hayami's panel up.
- **The push floor moves.** Same numbers, new home; a regression would show
  as a dashboard that lands late, which the live criterion would show.
- **Rollback**: revert the merge; the driver comes back with it.

## Alternatives Considered

- Keep `internal/cooler` as it is and let hayami alone use sanshoku:
  rejected; two copies of a protocol that has a bench is the thing the
  library exists to end.
- Delete `internal/cooler` and have the service import sanshoku directly:
  rejected; `service.Cooler` is the seam every test and the API rely on,
  and an adapter keeps the module's types out of the service.

## Verification

### Checks (implementer, 2026-09-30)

- `make test`: every package `ok`. The new tests are
  `internal/cooler/cooler_test.go` (reading passes through, identity and panel
  size, `Show` decodes and calls `Image`, no panel is `ErrNoScreen`, an
  unclaimable panel is `ErrNoScreen` and `Panel()` reports why, sentinels wrap
  sanshoku's, `ErrGone` closes once), `internal/cooler/hwmon_test.go`
  (`Processor` over a written tree) and
  `TestACoolerThatGoesIsDetachedAndFoundAgain` in `internal/daemon`. The
  `-race` run of `internal/cooler` and `internal/daemon` passes.
- `make lint`: `0 issues.` (golangci-lint v2.12.2, go1.26.0).
- `make build`: builds `bin/hotaru`.
- No device opened: `go clean -testcache`, then
  `strace -f -e trace=openat,open -o strace.txt make test`. Of 42266 `openat`
  calls, none named `/dev/hidraw*` or `/dev/bus/usb/*`. The `/dev` opens were
  `/dev/null`, `/dev/tty` and `/dev/nvidia*`. The `nvidia*` opens come from
  the `nvidia-smi` fallback that service tests reach through `Readings`, and
  that fallback is not changed here. The `/sys/class/hidraw` reads are sysfs
  and open no device.
- R1.3: no Go file under `internal/` or `cmd/` opens a hidraw or usbfs node.
  `internal/systemd` reads `/sys/class/hidraw` and readlinks `/proc` fds
  (spec 058) and opens no node. The window's import guard in
  `internal/gui/gui_test.go` now bans `ushineko/sanshoku` as well.
- `go.mod`: `github.com/ushineko/sanshoku v0.1.2` is the one new direct
  requirement. `golang.org/x/sys` was already indirect.
- `packaging/60-hotaru.rules` is unchanged.
- `docs/architecture.md` diagram re-rendered with `make generate`. The two
  images of the old diagram were deleted, and the readme test passes.

### What a reviewer will see differ

- `Device().Name` is the kernel's name through sanshoku's identity. On this
  desk that is `NZXT, Inc. NZXT Kraken Elite V2`, the `HID_NAME` in sysfs. It was
  `NZXT Kraken Elite V2` from hotaru's own table, and it shows in
  `/v1/cooling`, the window and the daemon's `reading ... at ...` line.
- `Device().USB` is empty: sanshoku's `Identity` does not carry the usbfs
  path, and nothing in hotaru reads the field.
- A cooler without a panel now answers `Panel()` with `ErrNoScreen` at once
  (R2.2), where before it answered nothing until a draw was tried.
- `POST /v1/dashboards/{name}/preview` takes its floor from the cooler, so a
  machine with no cooler reports `floor_seconds: 0`. The CLI then prints
  "0s between pushes" and the window prints "the panel needs 0s". Before,
  every machine got the Kraken's table.
- `ErrGone` is acted on from every adapter call (status and the three panel
  calls), not only from `Status`. sanshoku's design says any method may
  return it, and a draw that found the device gone would otherwise keep
  the dead handle.
- The CPU temperature goes through `hwmon.CPU`, which adds `k10temp` and
  `zenpower` after `coretemp`, and it is a float where it was truncated to
  whole degrees. `coretemp` reports whole degrees, so this desk reads the
  same.
- `ErrNoCooler` from `Open` wraps the reason every candidate failed, and a
  permission failure names `packaging/60-hotaru.rules` and wraps
  `sanshoku.ErrUnavailable` and the `EACCES`.

### The live swap

2026-09-30, at the desk, with hayami's panel polling the same node. The
user's service was stopped, the new build's `serve` run by PID, and the
service restarted afterwards (active).

```
$ ./bin/hotaru cooling
NZXT Kraken Elite V2
  coolant   41.0 C
  pump      2733 rpm (92%)
  fan       1507 rpm (64%)
  display   640x640 LCD
$ liquidctl --match kraken status
├── Liquid temperature    41.0  °C
├── Pump speed            2733  rpm
├── Pump duty               92  %
├── Fan speed             1507  rpm
$ ./bin/hotaru screen readout
The screen is showing the cooler's own display again.
$ ./bin/hotaru screen dashboard
The dashboard is back.
```

The readout appeared on the pump head and the dashboard returned, seen by
the user. The serve log's one cooler line: "reading NZXT Kraken Elite V2 at
/dev/hidraw6".
