# Spec 063: a server that dies mid-exchange

**Issue**: [#181](https://github.com/ushineko/hotaru/issues/181)

## Status: COMPLETE

## Executive Summary

When the OpenRGB server died between a request and its answer, hotaru waited
for that answer forever. It held the connection's lock while it waited, so
health, status and the reconnect of spec 058 hung behind it until hotaru was
restarted. Every request now has a five-second deadline of its own. One that
runs out marks the connection gone, and the service redials and restores.

Look first at `Conn.exchange` in `internal/openrgb/conn.go`. It explains why a
caller's cancellation no longer ends a request: in this SDK, abandoning one
leaves the connection unusable. Then look at `Close`, which closes the socket
rather than the SDK client for the same reason.

## Context

The OpenRGB server crashed while hotaru was applying a scene. The crash was
OpenRGB's own: a double free inside hidapi's `hid_read_timeout`. systemd
restarted the server five seconds later. hotaru never came back:
`/v1/health`, `/v1/devices` and `/v1/status` hung, the journal said nothing,
and only restarting hotaru recovered it.

A goroutine dump of the stuck service showed the shape:

```
1  go-openrgb-sdk.(*Client).SendPacketAndExpectPacketCtx   <- RequestControllerCount
     openrgb.(*Conn).catalogue <- find <- SetFrame          (holds Conn.mu)
     service.settle <- writeFrame <- queue job
87 openrgb.(*Conn).Devices      waiting for Conn.mu        (health, status, devices)
1  openrgb.(*Conn).Gone         waiting for Conn.mu        (the reconciler)
```

Spec 058 made an OpenRGB restart survivable, but only for a server that dies
between exchanges, where the next write fails with EPIPE or EOF. A server
that dies *during* one is a different path, and `go-openrgb-sdk` v1.0.1 (the
latest release) has three traps on it:

1. **A dead socket does not wake a waiting exchange.** The read loop returns
   on EOF and leaves every pending reply channel as it was. The exchange
   waits until its context ends.
2. **The context had no deadline.** The queue's context ends at shutdown, so
   the wait never ended. `Conn.mu` stayed held. `Gone()` takes the same
   mutex, so the reconciler never learned the socket was dead, and
   `supervise` never redialled.
3. **An abandoned exchange poisons the connection.** When an exchange gives
   up on its context, its reply channel stays registered. A reply that
   arrives later blocks the read loop forever on that unbuffered channel, so
   every later exchange on the connection waits for nothing. `Client.Close`
   then blocks too, sending `nil` to the same channel. An API request whose
   client disconnects mid-exchange can trigger this against a healthy server.

## Requirements

- **R1** Every exchange that waits for an answer has a deadline of its own,
  `exchangeTimeout` (5 s). A listing takes under a millisecond against a
  local server, so 5 s is a dead server, not a slow one.
  - **R1.1** The deadline is the only thing that ends an exchange. A caller
    that cancels (a client that hung up, a queue shutting down) does not
    abandon an exchange halfway, because of trap 3. The caller's values carry
    through (`context.WithoutCancel`).
  - **R1.2** Dial's version negotiation is bounded the same way and also
    honours the caller's context, so a server that accepts and never answers
    cannot hang a redial.
  - **R1.3** The deadline is a field on `Conn`, so a test can shorten it
    instead of waiting 5 s.
- **R2** An exchange that runs out of time marks the connection gone. The
  SDK reports it as `ResponseTimeoutError`, and `isGone` counts it. Under R1.1
  only the deadline produces one, so this means a server that stopped
  answering.
- **R3** `Gone()` does not wait for an exchange in flight. It reads an atomic
  flag rather than taking `Conn.mu`, so the reconciler and health learn of a
  dead server even while another goroutine holds the mutex.
- **R4** `Close()` closes the socket itself, not the SDK client, and returns
  promptly whatever state the SDK is in. `sdk.Client.Close` can block forever
  after trap 3.
- **R5** With R1–R4, the reconnect path of spec 058 takes over: the stuck
  exchange returns within `exchangeTimeout`, the reconciler sees `Gone()`,
  and `supervise` redials and restores.

## Acceptance Criteria

- [x] An exchange against a server that reads the request and never answers
      returns an error within the deadline, and `Gone()` is then true (test).
- [x] An exchange against a server that reads the request and closes the
      socket does the same (test). This is the crash in #181.
- [x] A caller that cancels its context mid-exchange does not end the
      exchange: a reply that arrives later is still delivered, and the
      connection is not marked gone (test).
- [x] `Gone()` answers while another goroutine is blocked in an exchange
      (test).
- [x] `Close()` returns after an exchange has timed out (test).
- [x] Dial against a server that accepts and never answers returns an error
      (test).
- [x] `make test` and `make lint` pass.
- [x] Live: with the development build running as the user's service,
      killing the OpenRGB server while scenes are being applied leaves
      hotaru answering `hotaru status`, and the journal shows "the OpenRGB
      server ... came back" and a restore.

## Risks & Assumptions

- **A slow server is now a dead one after 5 s.** OpenRGB answers from data it
  already holds, and the measured listing is under 1 ms. A server busy for
  5 s would be redialled, which costs a reconnect and a restore. That is
  visible in the journal and recovers by itself.
- **A late reply leaks one goroutine.** After a timeout, a reply that still
  arrives blocks the SDK's read loop on the abandoned channel. Closing the
  socket does not free it, because it is blocked on a channel, not a read.
  That costs one goroutine and one SDK client per timeout. Timeouts happen
  only when a server stops answering, so this is bounded by how often
  OpenRGB dies. The fix is upstream, in the SDK's read loop.
- **Cancellation is weaker.** A caller that gives up waits up to 5 s for the
  exchange in flight to finish. Exchanges take under a millisecond, so this
  matters only when the server is already dying.
- **Rollback**: revert the commit. The behaviour goes back to that of 0.1.23.

## Alternatives Considered

- Fix the SDK and use a `replace` directive or a fork. Rejected for this
  change: hotaru would carry a fork, and the hotaru-side deadline is needed
  either way, because a server that stops answering without closing the
  socket is not a read-loop problem. An upstream report is worth filing.
- Put a deadline on the socket (`SetReadDeadline`). Rejected: the SDK reads
  in a background loop that is idle most of the time, so any read deadline
  would close a healthy idle connection.

## Verification

Tests, in `internal/openrgb/exchange_internal_test.go`, against a server on
one end of a socket pair:

| Criterion | Test |
|---|---|
| Server stops answering | `TestAServerThatStopsAnsweringIsGoneWithinTheDeadline` |
| Server hangs up mid-exchange | `TestAServerThatDiesMidExchangeIsGoneWithinTheDeadline` |
| Caller cancels mid-exchange | `TestACallerThatGivesUpDoesNotPoisonTheConnection` |
| `Gone()` during a stuck exchange | `TestGoneAnswersWhileAnExchangeIsStuck` |
| `Close()` after a timeout | `TestCloseReturnsAfterAnExchangeTimedOut` |
| Dial against a silent server | `TestDialGivesUpOnAServerThatDoesNotAgree` |

Each was checked against a mutation of the fix:

- Restoring the caller's cancellation fails the caller test.
- Restoring `sdk.Client.Close` hangs the suite.
- Dropping `ResponseTimeoutError` from `isGone` fails both "gone" tests.

The tests use a socket pair, not `net.Pipe`. A pipe blocks every write until
the reader takes it, and the SDK writes a header and a body apart. So a pipe
fails the client's second write instead of leaving the request waiting.

`make test`, `go test -tags migrated_fynedo ./internal/gui/...`, `go test
-race -count=5 ./internal/openrgb/` and `make lint` pass. Lint ran with a
fresh `GOLANGCI_LINT_CACHE`: the shared cache held results for a deleted
worktree and failed on `main` too. `govulncheck ./...`: no vulnerabilities.

Live, the development build ran as the user's service. Eight clients
requested `/v1/devices` in a loop, and the OpenRGB server was killed with
SIGKILL three times:

```
round 1: killed OpenRGB at 13:56:57
round 2: killed OpenRGB at 13:57:17
round 3: killed OpenRGB at 13:57:37

13:57:06 the OpenRGB server at 127.0.0.1:6742 came back (protocol 3); putting the lights back
13:57:07 restored 6 devices to what they were showing
13:57:22 the OpenRGB server at 127.0.0.1:6742 came back (protocol 3); putting the lights back
13:57:31 restored 6 devices to what they were showing
13:57:46 the OpenRGB server at 127.0.0.1:6742 came back (protocol 3); putting the lights back
13:57:55 restored 6 devices to what they were showing
```

`hotaru status` answered after every round. In round 3 one request was in
flight when the server died, which is the case in #181:

```
"error": "read device 4 from 127.0.0.1:6742: response timeout"
```

It returned at the deadline, and the service redialled. The other failed
requests in those windows were `broken pipe`, the between-exchanges case
spec 058 already handles.
