# Authoring the world in Starlark

The bundled world contains 83 rooms across the town, six outland routes, and six scripted dungeons. All room descriptions, topology, puzzle clues, and dungeon behavior live in `internal/game/content/*.star`. Go supplies validation, transactional installation, movement, loot, resurrection, and the event APIs.

## Playing

Type `trails` in Town Square to see directions. The original dock delivery route remains unchanged.

| Destination | Directions from Town Square | Objective | Reward |
|---|---|---|---|
| Bellkeeper Crypt | west, west, down | Read the side chambers; `answer <word>` in the reliquary | 30 gold |
| Silvervein Mine | north, east, north, north, north, north, east, down | Read the survey diagram; `crank <label>` at the pumps | 40 gold |
| Tideglass Grotto | north, north, east, east, down | Read the shell archive; `align <symbol>` at the lens | 50 gold |
| Tithe Vault | north, east, north, east, east, east, north, east, down | Read the ledger; `press <plate>` in the strongroom | 45 gold |
| Tideworn Hulk | south, south, south, south, south, east, south, south, down | Read the galley table; `rig <pin>` at the capstan | 55 gold |
| Frostfall Deep | north, east, north, north, north, east, east, up, north, north, east, north, down | Read the ice verse; `stoke <lever>` at the forge | 60 gold |

Every passage has a return exit. Rest works at six inns and shelters. Dungeon rewards are once per character; sequence progress survives reconnects and restarts. These are shared dungeons with optional PvE encounters, without instances or locked movement.

Use the [editor packages](../editors/README.md) for `.star` highlighting and world/monster snippets.

## Building a map

`world.star` runs in the existing disposable Starlark worker, with three world-specific functions:

```python
def build():
    room("square", "Town Square", "A fountain and a road north.", kind="safe")
    room("cellar", "Old Cellar", "Steps return to the square.", kind="dungeon", script="cellar")
    link("square", "down", "cellar")

build()
```

- `room(key, name, description, kind="normal", script="")` declares a room. Keys and script names use lowercase letters, digits, and underscores, start with a letter, and have at most 40 characters. Names allow 100 bytes; descriptions allow 512 bytes. Terminal control characters are rejected.
- `link(from_room, direction, to_room)` creates both exits automatically. Directions are `north`, `south`, `east`, `west`, `up`, and `down`. Declare each connection once.
- Room kinds are `normal`, `safe`, `shop`, `inn`, and `dungeon`. Only `inn` grants ordinary rest. PvE combat is permitted in `normal` and `dungeon` rooms; all other room types prohibit it.
- `script="cellar"` references `content/cellar.star`, an ordinary room-hook script using the [event API](SCRIPTING.md). It is validated before installation.

Put loops inside functions, as in the shipped `build()` function. The interpreter permits no filesystem, network, imports, or database access. World sources allow 32 KiB; event scripts retain the 16 KiB limit. Both have a 50,000-step, two-second execution budget. Maps are capped at 96 rooms, 192 links, and 32 monsters. Validation rejects duplicate keys, unknown destinations, conflicting exits, self-links, and unreachable rooms.

## Installation and updates

Rebuild and restart the game core (or standalone gateway) after changing bundled files. Startup applies migrations through `007_loot_and_resurrection.sql` and installs content under the existing world advisory lock and transaction. A failed build or installation leaves the world unchanged.

The installer adopts existing starter rooms by name, retaining their UUIDs, characters, items, and delivery progress. Stable content keys are then mapped to room UUIDs in `world_content_rooms`. Never rename an installed content key to change a room's title. On first adoption, bundled descriptions and direction slots are installed; unrelated exits and existing script attachments are retained.

Existing content is seeded once. Subsequent startups preserve administrator edits, disabled scripts, detached hooks, room descriptions, exits, and player puzzle state. Adding new room keys installs the rooms and their connections, including entrances from existing rooms. If a new entrance would replace an occupied exit, installation fails instead of overwriting it. Removing or changing an existing declaration does not delete or rewrite live rooms: ship an explicit data migration for existing topology/description changes. Source edits apply automatically to fresh installations; updating an existing published script requires the normal admin edit/review/publish workflow. Keep script identity stable to preserve puzzle progress.

Bundled hooks have deterministic UUIDs and no player owner. Admins can find the eight `world_*` scripts with `script list`, and edit, test, publish, disable, or detach them normally. Initial installation is recorded in `script_audit`. Non-admin builders cannot edit these release-owned scripts.

State is scoped to script, target room, and character. Each dungeon's puzzle and reward live in its finale room, so no shared cross-room state is needed. Reward and completion state commit together; simultaneous answers cannot claim the reward twice.

## Validation

`go test ./internal/scripting ./internal/game ./internal/telnet` exercises map validation, the published dungeon routes, reciprocal exits, puzzle sequences, incorrect answers, independent player state, and reward repeatability. With a disposable PostgreSQL database configured, `TestWorldContentUpgradeAndPersistence` also creates an isolated database to test starter-room adoption, movement into/out of the crypt, concurrent rewards, admin edits, and restart persistence. Never run database tests against production.

## PvE encounters

Declare hostile NPCs in `world.star` alongside the rooms:

```python
monster("tunnel_rat", "mine_gallery", "Tunnel Rat", "A rat guards its nest.",
        health=18, attack=4, gold=5, experience=10, respawn=120,
        loot="Reinforced Hide", drop=800)
```

`monster(...)` uses stable monster keys. Optional `loot` names an installed item and `drop` is its chance in basis points (800 = 8%). Both must be present together. Other bounds remain health 1–1000, attack 1–100, defense 0–50, gold 0–100, experience 0–1000, and respawn 30–3600 seconds.

Twelve opponents are shipped, two per dungeon. Use `attack <name>` to fight and `loot <name>` after landing the killing blow. All encounters are opt-in; injured monsters retain health when players leave.

Each successful attack spends 2 stamina. Damage is `max(1, 5 + equipped weapon damage - monster defense)`. A surviving creature retaliates for `max(1, monster attack - equipped armor defense)`. Actual damage cannot exceed remaining health. Only the strongest equipped bonus of each type counts, bounded to 100 weapon damage and 50 armor defense. Killing blows receive no counterattack. Equipment in the pack has no combat effect.

The killing blow grants experience and creates a killer-owned corpse for 30 minutes. Every corpse contains gold plus non-zero silver and copper. It may also contain Reinforced Hide (8%), Warden Mail (3%), Runed Plate (1%), or Crownward Aegis (0.25%), depending on the monster. Coin uses one copper-unit balance: 100 copper = 1 silver and 100 silver = 1 gold. Looting is transactional and cannot be duplicated by concurrent commands.

Death is not permanent and does not remove items, coin, or experience. The dead move to the Hall of Returning at 0 HP and cannot leave or use ordinary gameplay commands. `resurrect pay` spends 100 gold immediately; otherwise `resurrect` succeeds after ten real minutes. Resurrection restores half health and full stamina. The monster keeps the damage it dealt that round.
