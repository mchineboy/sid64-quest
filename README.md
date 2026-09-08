# SID64 Quest

A Go MUD for Commodore and modern terminals, with a shared fantasy world, persistent characters, and browser-based account management. The public alpha is live at **[sid64.quest](https://sid64.quest/signup)**. Testers do not need Tailscale.

## Play the public alpha

| Connection | Address |
|---|---|
| Sign up | [sid64.quest/signup](https://sid64.quest/signup) |
| Manage your account | [sid64.quest/account](https://sid64.quest/account) |
| ANSI terminal | `sid64.quest:2323` |
| Commodore / PETSCII terminal | `sid64.quest:6464` |

1. Create an account and your first character on the website.
2. Connect your terminal, enter your username, and follow the browser-pairing instructions.
3. On a Commodore, type `QR` at the pairing screen to display a scannable login link. `HELP` restores the text instructions. ANSI clients also receive a QR code.
4. Sign in through the browser. The terminal detects completion automatically; choose a character to enter the world.

Pairing codes expire after five minutes; `renew` generates a fresh one. Passwords are entered in the HTTPS browser flow. Terminal gameplay and chat use plaintext TCP between the client and public gateway.

For modern terminals:

```sh
telnet sid64.quest 2323
```

For CCGMS or another native PETSCII client, dial `sid64.quest:6464` using your modem's connection command. No Mac relay is required for public access.

See the [tester guide](docs/ALPHA-TESTERS.md) for things to try and useful bug-report details.

## Implemented gameplay

- **Five persistent rooms:** Town Square, North Gate, Market Lane, The Prancing Pony Inn, and Moonlit Docks.
- **A shared world across both terminal ports:** ANSI and PETSCII players see one another, share room chat, and appear in the same online-player list.
- **Persistent progress:** location, inventory, equipment, health, stamina, gold, and delivery progress are stored in PostgreSQL.
- **Items and equipment:** take and drop items, drink potions, equip and unequip weapons or armor, and inspect your inventory.
- **Bulk pickup:** `take all` and `get all` collect available stacks up to the 50-item pack limit. Duplicate quest items stay behind. Pickups are transactional and protected against concurrent players taking the same items.
- **A repeatable objective:** return the misplaced harbor ledger from the docks to the Town Crier for a reward.
- **Rest and recovery:** movement spends stamina; resting at the inn restores health and stamina.
- **A little levity:** try taking the fountain, a lamppost, or other scenery. Actual portable items take precedence over scenery jokes.

| Commands | Purpose |
|---|---|
| `look`, `l`, `look <item>` | Inspect your surroundings or an item |
| `north`, `south`, `east`, `west` / `n`, `s`, `e`, `w` | Move |
| `where` | Show location and exits |
| `take <item>`, `get <item>`, `take all` | Pick up items |
| `drop <item>`, `use <item>` | Drop an item or use a potion |
| `equip <item>`, `unequip <item>` | Manage equipment |
| `talk [name]`, `give <item> [name]` | Interact with NPCs |
| `rest` | Recover at the inn |
| `say <message>`, `who` | Chat and see online players |
| `stats`, `inventory`, `inv`, `i` | Check your character and pack |
| `help`, `terminal ansi`, `terminal petscii` | Help and display preferences |
| `quit`, `q` | Leave the game |

A character can have only one active terminal connection. Idle sessions expire after 15 minutes; abrupt network loss may take time to clear.

## Commodore presentation

The dedicated PETSCII listener supports native Commodore clients without requiring telnet negotiation. The ANSI listener also recognizes compatible terminal identifiers, and display mode can be selected explicitly.

- Native colors and screen controls, with text wrapped to 39 columns to avoid accidental C64 auto-wrap.
- **Mixed-case text:** screens send `CHR$(14)` to select the lowercase/uppercase character set. Names, descriptions, URLs, pairing codes, and chat retain their case; keyboard input is decoded accordingly.
- Native character-selection, room, inventory, stats, help, and online-player screens.
- **PETSCII QR login:** four QR modules are packed into each character using solid quadrant glyphs. This preserves square modules and the quiet zone within a 40×25 screen. Use a black terminal background. Oversized URLs fall back to text instructions.

The QR render was checked against the C64 character ROM and independently decoded with macOS Vision. Scanning from a particular physical display still needs hardware validation. See [VICE, CCGMS, and hardware testing](docs/VICE-TESTING.md).

## Web accounts

The website handles account administration; gameplay stays in the terminal.

- Standalone signup and login, plus terminal-pairing signup.
- Up to five characters per account, character creation and renaming.
- Saved character location, health, stamina, and gold displayed on the account page.
- Email updates and password changes with current-password verification.
- CSRF-protected forms, rate limits, and HttpOnly/SameSite session cookies; production cookies use Secure.
- Twelve-hour browser sessions. Sign-out revokes the current session; changing the password invalidates existing browser sessions on subsequent requests.

Renamed characters appear in existing terminal sessions after reconnecting. Password changes do not disconnect active terminal sessions. Email verification, emailed password recovery, account deletion, and character deletion are not implemented. Operators can reset passwords and disable or enable accounts with `rck-admin`.

## Running deployment

```text
Browser / terminal
        |
        v
EC2 public gateway — sid64.quest
  Caddy: HTTPS + automatic certificates
  HAProxy: ANSI 2323 / PETSCII 6464
        |
        | Tailscale
        v
Raspberry Pi — symptom-pi
  Auth/account service + terminal gateway
  PostgreSQL + Redis
```

The EC2 gateway is a normal Tailscale device, not an exit node. Tailscale is used between servers; players connect through the public gateway. PostgreSQL and Redis have no published host ports. MongoDB is optional and is not part of the Pi deployment.

Operational work completed:

- Embedded, transactional, checksum-checked database migrations and serialized starter-world initialization.
- Fresh deployment secrets; no default accounts or seeded administrator password.
- Atomic, single-use browser pairing; shared terminal presence and duplicate-character-session prevention.
- HTTP readiness checks against PostgreSQL and Redis, container health checks, and service startup configuration.
- Daily PostgreSQL backups on the Pi, scheduled checksum-verified copies to the operator's Mac, and successful disposable-database restore rehearsals.
- An ARM64 deployment script that takes a backup and retains the previous application image.
- Operator account listing, disable/enable, and password-reset commands.

The deployed world is separate from local development data. Deployment interrupts active terminal sessions.

An opt-in persistent terminal edge now supports core restarts and blue/green
switches without dropping ANSI/PETSCII sockets, with durable session recovery
and command deduplication. See [the deployment and local testing guide](docs/BLUE-GREEN-DEPLOYMENTS.md).

Read the [operator runbook](docs/ALPHA-RUNBOOK.md), [Pi configuration](deploy/pi/compose.yml), and [EC2 gateway notes](deploy/ec2/README.md) before operating the deployment.

## Development

Requirements: **Go 1.23+** (see `go.mod`) and Docker with Compose v2.

```sh
git clone https://github.com/tylerhardison/race-condition-kingdom.git
cd race-condition-kingdom
cp .env.example .env
```

Edit `.env` before starting services. Supply your own PostgreSQL and MongoDB root passwords and a random `AUTH_SECRET`; `openssl rand -hex 32` can generate a secret. Keep `MONGODB_URI` empty unless using MongoDB. For local browser login, set `AUTH_BASE_URL=http://localhost:8080`; for testing from another device, use a hostname/address that device can reach.

```sh
docker compose -f docker-compose.simple.yml up -d --wait
make build
```

Start the applications in separate terminals:

```sh
make run-auth-service
```

```sh
make run-telnet-gateway
```

The `make run-*` targets load `.env`. Open [localhost:8080/signup](http://localhost:8080/signup), then connect to `localhost:2323` or `localhost:6464`.

For a binary or test command launched directly, export the environment first:

```sh
set -a
. ./.env
set +a
go test -race -count=1 ./...
go vet ./...
```

Database-backed tests require a reachable development PostgreSQL instance; they can skip when it is unavailable. Use a disposable development database, never production, for tests.

Stop local backing services with `make dev-stop`. Adding `-v` to a Compose `down` command deletes its database volumes. The optional full development stack includes additional dashboards and requires explicit dashboard passwords; it is not needed for the alpha.

`.env` is ignored. Historical development credentials remain in Git history and must not be reused. The repository/module path retains the original `race-condition-kingdom` name; the game is now SID64 Quest.

## In-game scripting

In-game Starlark scripting is deployed: authoring and testing from ANSI/PETSCII terminals, admin-reviewed publication, room/NPC/item hooks, persistent script state, and bounded reward/healing APIs. Use `script` in the gateway; see [the scripting guide](docs/SCRIPTING.md) for contributor permission setup, examples and limits.

## Validation and remaining work

The race-enabled Go suite covers account flows, authentication, migrations, gameplay persistence, PETSCII rendering/input, concurrent pickups, and session ownership. The public smoke test exercised HTTPS signup and pairing, ANSI/PETSCII chat, duplicate-login prevention, disconnect/reconnect, and saved location through the EC2 gateway.

This is a **small public alpha**, not a finished persistent-world game. Combat, private messages, a working shop/economy, banks, auctions, room/item creation tools, and comprehensive moderation/monitoring remain future work. Some schemas and architecture documents describe features that are not implemented.

The most immediate operational issue is Pi storage: a slow boot logged SD-card busy stalls. Services eventually recovered automatically, but the planned NVMe migration remains important. A successful service restart and backup restore are not proof of recovery from every power-loss scenario. Broader real-hardware testing and invited-player feedback are still needed.

See [ROADMAP.md](ROADMAP.md) for direction. [ARCHITECTURE.md](ARCHITECTURE.md) records the original larger design, rather than the exact deployed system.
