# Authoring the world in Starlark

The bundled world contains 41 rooms: the five original town rooms, 18 new overworld locations, and three six-room dungeons. All room descriptions, topology, puzzle clues, and dungeon behavior live in `internal/game/content/*.star`. Go supplies validation, transactional installation, movement, and the existing event APIs.

## Playing

Type `trails` in Town Square to see directions. The original dock delivery route remains unchanged.

| Destination | Directions from Town Square | Objective | Reward |
|---|---|---|---|
| Bellkeeper Crypt | west, west, down | Read the side chambers; `answer <word>` in the reliquary | 30 gold |
| Silvervein Mine | north, east, north, north, north, north, east, down | Read the survey diagram; `crank <label>` at the pumps | 40 gold |
| Tideglass Grotto | north, north, east, east, down | Read the shell archive; `align <symbol>` at the lens | 50 gold |

Every passage has a return exit. Use `up`/`u` and `down`/`d` for stairs and the mine lift. Rest restores health and stamina at the Prancing Pony Inn, Forester's Lodge, or Keeper's Cottage. Dungeon rewards are once per character. Partial pump/lens progress survives reconnects and restarts; an incorrect valid control resets its sequence. These are shared dungeons with optional PvE encounters in side chambers, without instances or locked movement.

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

Put loops inside functions, as in the shipped `build()` function. The interpreter permits no filesystem, network, imports, or database access. World scripts have the same 16 KiB source, 50,000-step, two-second execution and bounded worker-output limits as event scripts, plus caps of 64 rooms, 192 links, and 32 monsters per map. Validation rejects duplicate keys, unknown destinations, conflicting exits, self-links, and rooms unreachable from the first room. World-building functions are unavailable to runtime room/NPC/item hooks.

## Installation and updates

Rebuild and restart the game core (or standalone gateway) after changing bundled files. Startup applies migrations `005_world_content.sql` and `006_pve_combat.sql` and installs content under the existing world advisory lock and transaction. A failed build or installation leaves the world unchanged. No production deployment is performed by editing these files.

The installer adopts existing starter rooms by name, retaining their UUIDs, characters, items, and delivery progress. Stable content keys are then mapped to room UUIDs in `world_content_rooms`. Never rename an installed content key to change a room's title. On first adoption, bundled descriptions and direction slots are installed; unrelated exits and existing script attachments are retained.

Existing content is seeded once. Subsequent startups preserve administrator edits, disabled scripts, detached hooks, room descriptions, exits, and player puzzle state. Adding new room keys installs the rooms and their connections, including entrances from existing rooms. If a new entrance would replace an occupied exit, installation fails instead of overwriting it. Removing or changing an existing declaration does not delete or rewrite live rooms: ship an explicit data migration for existing topology/description changes. Source edits apply automatically to fresh installations; updating an existing published script requires the normal admin edit/review/publish workflow. Keep script identity stable to preserve puzzle progress.

Bundled hooks have deterministic UUIDs and no player owner. Admins can find `world_guide`, `world_crypt`, `world_mine`, and `world_grotto` with `script list`, and edit, test, publish, disable, or detach them normally. Initial installation is recorded in `script_audit`. Non-admin builders cannot edit these release-owned scripts.

State is scoped to script, target room, and character. Each dungeon's puzzle and reward live in its finale room, so no shared cross-room state is needed. Reward and completion state commit together; simultaneous answers cannot claim the reward twice.

## Validation

`go test ./internal/scripting ./internal/game ./internal/telnet` exercises map validation, the published dungeon routes, reciprocal exits, puzzle sequences, incorrect answers, independent player state, and reward repeatability. With a disposable PostgreSQL database configured, `TestWorldContentUpgradeAndPersistence` also creates an isolated database to test starter-room adoption, movement into/out of the crypt, concurrent rewards, admin edits, and restart persistence. Never run database tests against production.

## PvE encounters

Declare hostile NPCs in `world.star` alongside the rooms:

```python
monster("tunnel_rat", "mine_gallery", "Tunnel Rat", "A rat guards its nest.",
        health=18, attack=4, defense=0, gold=5, experience=10, respawn=120)
```

`monster(key, room, name, description, health=20, attack=5, defense=0, gold=0, experience=0, respawn=300)` uses stable monster keys. Bounds: health 1–1000, attack 1–100, defense 0–50, gold 0–100, experience 0–1000, and respawn 30–3600 seconds. Monsters must reference a normal or dungeon room. Duplicate keys, invalid rooms, control characters, and excessive stats are rejected before installation. Definitions cannot create player targets. Their deterministic NPC identities are seeded once; changing a live creature's stats requires a data migration. Startup preserves living injuries and pending respawns.

The six shipped opponents are Bronze Sentinel, Lantern Wraith, Tunnel Rat, Quartz Golem, Glassback Crab, and Storm Eel. Use `look <name>` to inspect a creature and `attack <name>` (aliases `hit` and `kill`) to fight. Commands are case-insensitive; ambiguous names must be clarified. All encounters are opt-in: nothing attacks on entry, while idle, or after retreat. Leaving uses ordinary movement and is always possible, even with no stamina. Injured monsters retain their health when players leave.

Each successful attack spends 2 stamina. Damage is `max(1, 5 + equipped weapon damage - monster defense)`. A surviving creature retaliates for `max(1, monster attack - equipped armor defense)`. Actual damage cannot exceed remaining health. Only the strongest equipped bonus of each type counts, bounded to 100 weapon damage and 50 armor defense. Killing blows receive no counterattack. Equipment in the pack has no combat effect.

The player landing the killing blow receives the creature's gold and experience. Experience is stored and shown in `stats`; automatic leveling is not yet implemented. Dead monsters disappear and return at full health after their configured delay, when the room is next observed or attacked. Rewards are repeatable per respawn. Concurrent attacks serialize, and rewards and damage share the terminal command transaction, including replay protection in the persistent core.

Defeat returns the player to the Prancing Pony Inn with 1 HP and unchanged gold, inventory, and experience. Use `rest` to recover. There is no permanent death or item loss. The monster keeps the damage it received that round. Players, friendly NPCs, and remote targets cannot be damaged, including in legacy rooms labeled `pvp`.
