# In-game Starlark scripting

Privileged players can now author, test and publish Starlark scripts from either terminal. This implementation supports room events and custom commands, NPC conversations, and carried-item use. Scripts can send messages, store persistent per-player state, award gold and heal. Creating rooms, changing exits, spawning items, combat hooks and a separate editable world are still future builder features.

## Access and publication

An operator grants the first permissions with the existing admin binary and database environment:

```sh
rck-admin grant-builder USERNAME
rck-admin grant-admin USERNAME
```

`revoke-builder` and `revoke-admin` remove those individual permissions. Admin implies builder access. Permissions are checked against the database on every scripting command and editor operation; no reconnect is required after revocation. Builders can access their own drafts; admins can review all drafts. Admins alone can publish, attach, detach and disable scripts. An admin can publish their own work.

Deployment requires rebuilding the gateway and admin binary. Migration `003_scripting.sql` runs through the existing startup migrator. No additional service or executable is needed: the gateway starts a disposable copy of itself for each evaluation. Installing the migration alone does not publish any scripts or grant permissions.

## First script

In the game:

```text
script new room greeting
script list
script edit <script-uuid>
```

New scripts contain a small example. Inside the editor, `.set N <source>` replaces a line, `.insert N <source>` inserts before a line, and `.delete N` removes a line. Text entered without a leading dot appends a source line. Spaces after the line-number separator are preserved. Use spaces for indentation; terminal tabs are converted to one space.

Replace the sample with:

```text
.set 1 def on_enter(event):
.set 2     visits = int(get_state("visits", "0")) + 1
    set_state("visits", str(visits))
    tell("Welcome, " + event.player.name + "! Visit " + str(visits))
.save
```

`.list` shows numbered source and `.abort` discards unsaved edits. Drafts support 256 editor lines and 16 KiB of source. Unsaved changes are lost on disconnect. Saving an outdated editor is rejected instead of overwriting another session's changes.

```text
script show <script-uuid>
script test <script-uuid> on_enter
```

Tests use the draft, your current player and room, a synthetic target and empty state. Messages (including room broadcasts) appear only to the tester. The result reports planned state and reward/healing effects; nothing is persisted or applied to the world.

An admin reviews the numbered source and publishes its exact draft revision, then attaches it:

```text
script publish <script-uuid> 2
script attach <script-uuid> here
```

`here` means the current room UUID. `script targets` lists that room, nearby NPCs and carried items with their UUIDs. Use an explicit UUID to attach an NPC or item script. An item attachment applies to that item definition, including all inventory copies. Each object has one script attachment; attaching another replaces it.

Walk into the room or reconnect to trigger `on_enter`. Subsequent draft edits leave the approved source active until an admin publishes another revision.

## Hooks and event data

Each hook takes exactly one positional parameter, conventionally named `event`.

| Script type | Hook | When it runs |
|---|---|---|
| room | `on_enter(event)` | After movement into the room or login |
| room | `on_look(event)` | After a player explicitly uses `look` without an item name |
| room | `on_say(event)` | After the player's room chat is broadcast |
| room | `on_command(event)` | For an otherwise unknown command |
| npc | `on_talk(event)` | Before ordinary conversation with an unambiguously matched NPC in the room |
| item | `on_use(event)` | Before ordinary use of an unambiguously matched carried item |

Return `True` from `on_command`, `on_talk` or `on_use` to mark the command handled. Return `False` or `None` to allow normal processing. Existing core commands, login, `script` and `quit` cannot be intercepted by room scripts. Entry/look/say return values have no effect. Script failure falls back to ordinary gameplay and is recorded in gateway logs.

The event contains immutable records:

- `event.player.id` and `.name`: the acting character.
- `event.room.id` and `.name`: the current room.
- `event.target.id`, `.name`, `.kind`: the attached room, NPC or item.
- `event.command`: the lowercase command, movement direction, or `login`.
- `event.text`: command arguments, chat text, or an empty string for entry/look.

For `script test <id> <hook> [text]`, the supplied text becomes `event.text` and its first word becomes `event.command`. The synthetic target is named `Test target`.

## Game API

| Function | Behavior |
|---|---|
| `tell(text)` | Send one message to the acting player |
| `say(text)` | Send one message to everyone in the current room |
| `get_state(key, default="")` | Read a persistent string, or the supplied default |
| `set_state(key, value)` | Store a string for this script, target and character |
| `award_gold(amount)` | Award the acting player a nonnegative integer amount |
| `heal(amount)` | Restore health up to the character's maximum |

State is private to each character for each script/target pair. It survives reconnects, process restarts and script publication. It is not shared room state. Each pair allows 32 keys, 64 bytes per key, 1,024 bytes per value, and 8 KiB of JSON in total. Rewards and healing each allow at most 100 per invocation; approved authors are responsible for controlling repeatability.

For example, this room command grants a reward only once per character:

```python
def on_command(event):
    if event.command != "answer":
        return False
    if event.text.lower() != "a shadow":
        tell("The inscription remains dark.")
        return True
    if get_state("solved") == "yes":
        tell("You already solved this riddle.")
        return True
    set_state("solved", "yes")
    award_gold(10)
    tell("The inscription shines. You receive ten gold.")
    return True
```

Writes and rewards are committed together only after successful evaluation. Concurrent runs for the same state record serialize. A failed script commits neither state nor rewards and emits no script messages. Messages are delivered after commit; a disconnected terminal can miss a message even though the reward was saved.

## Review, recovery and limits

```text
script history <id>
script restore <id> <old-revision>
script disable <id>
script detach <id> <target-id|here>
```

History shows the last 30 audit actions. Create/save/restore retain source revisions in PostgreSQL. Restore copies an earlier source into a new draft revision; an admin must review and publish it. Disable stops execution on all attached objects. Detach removes only the selected object's attachment. State is retained across both operations. `script list` shows up to 100 scripts, and each author can create up to 100.

The language uses the [Go Starlark interpreter](https://pkg.go.dev/go.starlark.net/starlark). Functions, loops, conditionals, comprehensions, lists, dictionaries and ordinary built-ins are available. Recursion is disabled. No `load` resolver, filesystem/network APIs, environment access, imports or raw database access are exposed. Use `tell`/`say` instead of `print`; game effects must be inside hook functions.

Each invocation has a 50,000-step budget, a two-second wall deadline, a maximum of 16 messages of 512 bytes each, and rejects terminal control characters. There are at most four concurrent workers per gateway process. Workers receive only serialized event data, source and scoped state, with no database credentials in their environment. Linux additionally enforces a one-second CPU limit, 2 GiB virtual address-space ceiling and disabled core dumps. Go's 64 MiB memory target is a soft target, not an allocation quota; non-Linux development hosts do not have the kernel limits. Deployment container limits remain a useful outer bound.

Publication validates source and hook signatures. It cannot prove every branch works; test the relevant hooks and inputs before approval. Persistent audit history and scripts are included in ordinary PostgreSQL backups.
