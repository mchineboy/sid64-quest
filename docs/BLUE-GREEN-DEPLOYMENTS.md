# Restarting the core without dropping terminals

`terminal-edge` owns the ANSI/PETSCII TCP sockets. `game-core` owns login,
gameplay and rendering. They communicate over authenticated HTTP on a private
TCP endpoint or Unix socket. Replacing the core leaves the edge and client
sockets running. `core-switch` checks readiness and protocol compatibility,
then atomically changes the endpoint file read by the edge.

```
ANSI / PETSCII clients -> terminal-edge -> game-core blue or green
                         owns sockets      |              |
                                           PostgreSQL + Redis
```

The supplied Compose configuration gives the edge no database credentials. It
keeps telnet negotiation, line framing, partial input and presentation locally.
Complete commands carry a connection UUID and increasing sequence number. Eight
complete lines can queue (plus the line being read and request in flight); TCP
backpressure bounds further input. The 1,024-byte line limit remains in effect.

During an outage, the edge says: “The realm is taking a breath. Stay connected;
we'll be right back.” After recovery: “The realm stirs again. Your journey
continues.” A longer outage gets reassurance at most once every 30 seconds.
Idle expiry pauses while retrying the core, with a fresh idle window on recovery.
Notices use native PETSCII wrapping and colors on Commodore terminals. New
terminals can also wait for the core while connected.

## State and delivery guarantees

Migration `004_terminal_sessions.sql` adds durable terminal checkpoints: login
state, pairing token/code, authenticated session, character/room, display mode,
unsaved script editor, command sequence and pending output. These are private
operational records, not client-supplied identities.

PostgreSQL serializes requests across cores with a transaction advisory lock.
Game operations support savepoints inside that transaction. A command's world
changes, checkpoint and output to affected players commit together. Death before
commit rolls everything back. Death after commit but before reply is recovered
by retrying the same sequence and returning saved output without executing again.
Output acknowledgements prevent repeated display of bytes the edge already wrote.
There is no guarantee that a human saw a successful TCP write. Edge crashes and
host restarts still lose sockets; these guarantees concern core replacement.

Both cores reconstruct presence from the same PostgreSQL checkpoints. `who`,
room occupants, broadcasts and duplicate-character claims span versions. A
partial unique index also enforces character ownership. Room messages use the
transactional checkpoint outbox rather than Redis Pub/Sub, which cannot replay
missed messages. Legacy connect/move telemetry publications are not emitted by
this core implementation.

Redis keeps the existing atomic browser pairing/session mechanism and adds
`terminal:presence:<connection UUID>` keys containing edge UUIDs with two-minute
TTLs. This directory is informational. PostgreSQL owns command deduplication,
presence and character claims; a Redis TTL is never used to lock game mutations.
A Redis outage pauses core requests.

Disconnects clear ownership promptly when a core is reachable; otherwise it
expires after two minutes without a core request. Checkpoints and closed-session
sequence tombstones expire after 24 hours of inactivity, cleaned on the next
request. Returning after that window keeps the socket but requires signing in
again. A character claimed elsewhere after lease expiry must be selected again.
An outbox exceeding 64 KiB retires that terminal session. Protect these private
session records through the existing database access controls and backups.

This implementation favors correctness for the current small alpha: requests
share one database command lane and reconstruct active presence per request.
Measure latency and database load before increasing concurrent player counts.

## Local test and interactive use

With local PostgreSQL and Redis running and `.env` configured for them:

```sh
make test-core-restart
```

The tests require local database hosts, create/drop disposable PostgreSQL
databases, and use UUID-scoped Redis sessions. They exercise actual core SIGKILL,
continued ANSI/PETSCII sockets, queued movement, partial input, editor recovery,
chat/presence, and a live blue/green switch. Additional cases cover lost replies,
duplicate command retries, checkpoint-write rollback, character ownership and
disabled accounts.

For interactive use, run `make build`. Set the same `CORE_TOKEN` (at least 32
random bytes; generate with `openssl rand -hex 32`) for all three executables.
Load `.env` for auth and cores and start auth as usual. In separate shells:

```sh
# Blue core:
CORE_LISTEN=127.0.0.1:8082 ./build/game-core

# Control shell, then the persistent edge:
./build/core-switch -target http://127.0.0.1:8082 -file "$PWD/build/core-target"
CORE_TARGET_FILE="$PWD/build/core-target" ./build/terminal-edge

# Green core:
CORE_LISTEN=127.0.0.1:8083 ./build/game-core

# Control shell, after green starts:
./build/core-switch -target http://127.0.0.1:8083 -file "$PWD/build/core-target"
```

Connect on 2323 or 6464. Blue can stop after switching; reversing the switch is
a rollback. Killing the only core also works: terminals wait until a core is
reachable again. Unix sockets use `CORE_LISTEN=unix:/absolute/path/blue.sock` and
the same `unix:` switch target. A listener lock handles stale files after SIGKILL
without replacing a live listener. Sockets are mode 0600: run edge and core as
the same user. Keep TCP cores on loopback/private networks, or behind TLS.

## Pi deployment boundary

Production migrated from `main` revision `6d0e7d0` on 2026-09-08. Public
ANSI/PETSCII sessions survived a live blue→green→blue switch; see the runbook for
the release record and rollback artifacts. `scripts/deploy-pi.sh` and `compose.yml`
describe the legacy gateway; the deploy script refuses to replace an installed
persistent edge. The installed `rck.service` boots `compose.edge.yml`. The legacy
gateway's `SIGUSR1` drain keeps the entire old process alive and is separate from the new
core replacement mechanism.

`deploy/pi/compose.edge.yml` is the production topology using the same `rck`
project and database volumes. Pin `EDGE_IMAGE`, `CORE_BLUE_IMAGE`,
`CORE_GREEN_IMAGE`, and `AUTH_IMAGE` to release tags in the Pi environment and
set `CORE_TOKEN`. The initial migration helper `configure-edge.py --release rck:REVISION`
preserves the Pi's existing secrets, backs up its environment, generates the
private core token, and pins these images. It sets `COMPOSE_FILE=compose.edge.yml`
so ordinary Compose, backup and restore-check commands select the new topology.
Build images with all binaries listed in `deploy/pi/Dockerfile`.
Never retag/recreate the edge as part of a routine core release.

For another installation, the initial move needs one announced disconnect: old sockets belong to the
monolithic gateway. After a backup, stop/remove only that gateway to free its
ports, create `/srv/rck/edge-control` owned by container UID/GID 65532 with mode
0755 (so the host operator can check for the target file), and run
`docker compose -f compose.edge.yml up -d --wait`. Use this Compose file for all
subsequent operations and update `rck.service` before rebooting. Mount the control
directory, not a single target file, so atomic renames are visible to the edge.

Initialize its target from the blue container:

```sh
docker compose -f compose.edge.yml exec -T core-blue /app/core-switch \
  -target http://core-blue:8082 -file /control/target
```

For a release, update only the inactive slot's pinned image, then:

```sh
docker compose -f compose.edge.yml up -d --no-deps --wait core-green
docker compose -f compose.edge.yml exec -T core-green /app/core-switch \
  -target http://core-green:8082 -file /control/target
```

After verifying gameplay, stop blue or keep it available for rollback. An old
request still executing during the switch is serialized with green's requests.
Migrations and checkpoint formats must support both releases throughout the
rollback window. Restarting the whole Compose project or `rck.service` still
restarts the edge and drops sockets. Do not run legacy gateways alongside the
new core against the same world: they bypass its ownership protocol.
