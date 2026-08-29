# Race Condition Kingdom

Race Condition Kingdom is a small Go MUD with a telnet front door and a browser-based login step. It is an early playable foundation, not a finished online world: you can create an account, enter a small shared map, move between rooms, and talk with other players.

The project began as an experiment in mixing the old telnet MUD experience with a normal web login. The name is a joke; the current server should not be treated as a production service yet.

## What works today

- PostgreSQL, Redis, and MongoDB are started with Docker Compose.
- The telnet gateway accepts connections and hands each player a short-lived browser login link.
- The auth service validates the link and attaches the browser login to the telnet session.
- New players can create an account and their first character from that same browser handoff.
- A persistent five-room starter area supports `look`, cardinal movement, `say`, and a real `who` list.
- PETSCII-aware telnet clients get a native Commodore presentation; raw Commodore callers can use the dedicated PETSCII port.
- A local development account has a character, **The Steward**, and can enter the game.
- In-game commands: `look`, `say`, `who`, `stats`, `inventory`, `help`, and `quit`.

## What is still a stub

Combat, inventory persistence, private messages, shops, banks, auctions, builders, moderation, and production monitoring are either placeholders or schema/design work. The database has seed data for some of those ideas, but the gateway does not implement them.

That distinction matters: this repository is ready for local development and for turning into a real game, but it is not ready to be exposed as a public MUD.

## Run it locally

You need Go 1.23 (the version in `go.mod`) and Docker Desktop or another Docker-compatible runtime with Compose v2.

```bash
git clone https://github.com/tylerhardison/race-condition-kingdom.git
cd race-condition-kingdom

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

Commodore 64/128 callers should connect their PETSCII terminal program to port `6464` instead. On the normal telnet port, the server also uses terminal-type negotiation and switches automatically when a client identifies itself as PETSCII, C64, C128, CCGMS, CGTerm, NovaTerm, or UltimateTerm.

At the username prompt, enter a name, then open the URL printed by the game. Sign in with `admin` / `admin123` or follow the link to create your own account and first character. Do not deploy the development account, its password, or the compose-file credentials anywhere public. Type `check` in telnet after the browser confirms the login, then enter `1` to select a character.

Stop the backing services with:

```bash
make dev-stop
```

To discard local database data completely, use `docker compose -f docker-compose.simple.yml down -v`.

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

The next useful step is a persistent player loop: items, inventory, health/stamina, and one small repeatable objective. Splitting it into more services, Kubernetes, Discord, scripts, or an auction system will make operating it harder before it makes the game more playable.

`ARCHITECTURE.md` records the original, much larger idea. It is useful as a backlog, not a description of the running system.
