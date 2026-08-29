# Roadmap to a playable private alpha

The project is a working local vertical slice, not yet a game people can meaningfully inhabit. This plan deliberately earns complexity: a shared world comes before extra services, dashboards, or a Kubernetes deployment.

## 1. Foundation — complete

- Local PostgreSQL, Redis, and MongoDB stack starts with Compose.
- Browser login is linked to the originating telnet session.
- Tests, vetting, and builds run cleanly; the auth tests use an in-process Redis.
- Documentation describes the real state of the project instead of the original wish list.

## 2. Terminal compatibility — in progress

- Negotiate the telnet `TERMINAL-TYPE` option.
- Recognize PETSCII/Commodore terminal identifiers and render a 40-column PETSCII experience with native color and graphics bytes.
- Keep ANSI as the default, and add an explicit player override for terminals that do not report their type.
- Test with a C64-capable client such as CCGMS/UltimateTerm and SyncTerm configured for C64 mode.

**Exit condition:** an ANSI terminal and a PETSCII terminal can both complete login and read the core screens correctly.

## 3. A shared world — complete

- Add self-service account and character creation, with validation and a safe starting room.
- Load rooms and exits from PostgreSQL; implement `look`, cardinal movement, and a `where` command.
- Track online players by room, broadcast `say`, and make `who` reflect real connections.
- Give the game a small authored starter area (five to eight rooms), not a procedurally broad empty map.

**Exit condition met:** two people can create characters, meet in a room, move around, and talk.

## 4. Persistent player loop — complete

- Persist location, inventory, health, stamina, and activity safely.
- Implement items, take/drop/use, simple equipment, and a clear save-on-change policy.
- Add a minimal NPC interaction and one repeatable non-combat objective.
- Cover commands and database mutations with unit and integration tests.

**Exit condition met:** a player has a reason to log back in, and their progress survives a restart.

## 5. Private-alpha hardening

The first host is the Raspberry Pi 5 Tailscale machine **symptom-pi** (modeburner). Invited play goes over the tailnet. When the MUD is ready, it gets a public DNS name; Commodore/PETSCII remains a client connecting to that host, not a deployment target.

- Replace tracked development credentials with documented bootstrap/secrets handling on the Pi.
- Add migrations instead of relying on a Compose init script.
- Keep hostnames in configuration (`AUTH_BASE_URL`, bind addresses). Use MagicDNS for the tailnet alpha; switching to public DNS should not require a rewrite.
- Keep the Pi image small: PostgreSQL, Redis, auth HTTP, and the telnet/PETSCII listeners. Do not require MongoDB or the monitoring Compose file.
- Add HTTP rate limits, session handling, audit logging, health/readiness, TLS for the login page, and `pg_dump` backups that survive a power loss.
- Run a small invited playtest over the tailnet, then fix disconnect/reconnect and abuse cases.

**Exit condition:** a small invited group can play on the Pi without the operator babysitting every login or restart.

## 6. Expand only after the loop is fun

- Combat and NPC behavior.
- Economy, shops, banks, and auctions.
- Builder tooling, moderation, and scripting.
- Optional service separation and monitoring once a measured need exists.

The original architecture document remains a source of ideas, not an implementation order.
