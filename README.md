# SID64 Quest

SID64 Quest is a small Go MUD for Commodore and modern terminals, with a browser-based login step. Visit [sid64.quest](https://sid64.quest) to create an account, then connect on port 6464 for PETSCII or 2323 for ANSI. It is an early playable foundation, not a finished online world: you can enter a small shared map, move between rooms, and talk with other players.

The project began as an experiment in mixing the old telnet MUD experience with a normal web login. The current server should not be treated as a production service yet.

## What works today

- PostgreSQL and Redis power the alpha. MongoDB is optional.
- The telnet gateway presents a short-lived browser pairing code. ANSI clients also get a scannable QR code; a 40x25 PETSCII screen cannot hold one, so it shows the code in large reverse video instead.
- The auth service validates the link and attaches the browser login to the telnet session.
- New players can create an account and their first character from that same browser handoff.
- A persistent five-room starter area supports `look`, cardinal movement, `say`, and a real `who` list.
- Items persist on the ground and in inventory. You can `take`, `drop`, `use` potions, and `equip` a weapon or armor.
- The Town Crier will pay you for returning the misplaced harbor ledger from the Moonlit Docks; that job repeats.
- Resting at the inn restores health and stamina. Movement spends stamina. Those values are saved immediately.
- PETSCII-aware telnet clients get a native Commodore presentation; raw Commodore callers can use the dedicated PETSCII port.
- New installations start without default accounts or passwords.
- In-game commands: `look`, cardinal movement (`n`/`s`/`e`/`w`), `where`, `take`/`drop`/`use`/`equip`, `talk`, `give`, `rest`, `say`, `who`, `stats`, `inventory`, `help`, `terminal ansi|petscii`, and `quit`.

## What is still a stub

Combat, private messages, shops, banks, auctions, builders, moderation, and production monitoring are either placeholders or schema/design work. The database has seed data for some of those ideas, but the gateway does not implement them.

That distinction matters: this repository is ready for local development and for turning into a real game, but it is not ready to be exposed as a public MUD.

## Run it locally

You need Go 1.23 (the version in `go.mod`) and Docker Desktop or another Docker-compatible runtime with Compose v2.

```bash
git clone https://github.com/tylerhardison/race-condition-kingdom.git
cd race-condition-kingdom

./scripts/dev-setup.sh
docker compose -f docker-compose.simple.yml up -d
go test ./...
make build
```

Start the two applications in separate terminals:

```bash
make run-auth-service
```

```bash
make run-telnet-gateway
```

Then connect from a third terminal:

```bash
telnet localhost 2323
```

Commodore 64/128 callers should connect their PETSCII terminal program to port `6464` instead. At any login prompt or in game, use `terminal ansi` or `terminal petscii` to override the display mode for your connection. On the normal telnet port, the server also uses terminal-type negotiation and switches automatically when a client identifies itself as PETSCII, C64, C128, CCGMS, CGTerm, NovaTerm, or UltimateTerm.

At the username prompt, enter a name. Scan the QR code on an ANSI terminal, or open `/pair` on the auth service and enter the displayed pairing code. Create an account and character, or sign in with your existing account. New installations have no seeded admin account. Type `check` in telnet after the browser confirms the login, then enter `1` to select a character.

Stop the backing services with:

```bash
make dev-stop
```

To discard local database data completely, use `docker compose -f docker-compose.simple.yml down -v`.

For emulated C64 testing, see [VICE and CCGMS setup](docs/VICE-TESTING.md).

## Web accounts

Open `/account` on the auth service (port 8080) to sign in without a terminal.
`/signup` creates an account and first character; `/login` signs in existing players.
The account page shows saved characters, locations, health, stamina, and gold.
Players can create up to five characters, rename their own characters, update
email with their current password, and change their password. Renames appear in
open terminal sessions after reconnecting. There are no web gameplay controls.

Browser sessions expire after 12 hours; sign-out revokes the current browser
session and a password change invalidates all browser sessions. Existing terminal
connections are unaffected. Forms use CSRF tokens, authentication attempts are
rate-limited, and session cookies are HttpOnly/SameSite with Secure enabled when
`AUTH_BASE_URL` uses HTTPS. Set that URL to a host players can reach.

Email verification, emailed password recovery, character retirement/deletion,
and account deletion are not implemented. For now, forgotten passwords need
operator assistance. The existing terminal pairing signup flow remains available.

## Useful commands

```bash
make test                 # run Go tests
make build                # build the auth service and telnet gateway
make dev-start            # start PostgreSQL, Redis, and MongoDB
make dev-stop             # stop those services
make dev-start-full       # start databases plus the optional admin/monitoring tools
```

The service defaults live in [`pkg/config/config.go`](pkg/config/config.go). The `make run-*` commands export the local `.env` file before starting a service. If you run a binary directly, export that file yourself first (for example, `set -a; . ./.env; set +a`).

## A practical next milestone

The next useful step is private-alpha hardening: secrets handling, migrations, TLS, health checks, and a small invited playtest. Splitting it into more services, Kubernetes, Discord, scripts, or an auction system will make operating it harder before it makes the game more playable.

`ARCHITECTURE.md` records the original, much larger idea. It is useful as a backlog, not a description of the running system.

## Private alpha on the Pi

See [the deployment and operator runbook](docs/ALPHA-RUNBOOK.md) and
[the tester guide](docs/ALPHA-TESTERS.md). The Pi uses a separate, fresh world;
local development players are not imported.
