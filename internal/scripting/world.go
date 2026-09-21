package scripting

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"go.starlark.net/starlark"
)

// WorldDefinition is a declarative build plan, never a live database capability.
type WorldDefinition struct {
	Rooms    []WorldRoom    `json:"rooms"`
	Monsters []WorldMonster `json:"monsters,omitempty"`
}
type WorldRoom struct {
	Key         string            `json:"key"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Kind        string            `json:"kind"`
	Script      string            `json:"script,omitempty"`
	Exits       map[string]string `json:"exits"`
}

// WorldMonster describes an NPC opponent; it cannot target player characters.
type WorldMonster struct {
	Key, Room, Name, Description                       string
	Health, Attack, Defense, Gold, Experience, Respawn int
	Loot                                               string
	Drop                                               int
}

var worldKey = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
var reverseDirection = map[string]string{"north": "south", "south": "north", "east": "west", "west": "east", "up": "down", "down": "up"}

func evaluateWorld(thread *starlark.Thread, source string) (Output, error) {
	world := &WorldDefinition{}
	indexes := map[string]int{}
	monsterKeys := map[string]bool{}
	type edge struct{ from, direction, to string }
	var edges []edge
	clean := func(value string, limit int) bool {
		return strings.TrimSpace(value) != "" && len(value) <= limit && strings.IndexFunc(value, unicode.IsControl) < 0
	}
	pre := starlark.StringDict{
		"monster": starlark.NewBuiltin("monster", func(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			m := WorldMonster{Health: 20, Attack: 5, Respawn: 300}
			if err := starlark.UnpackArgs(b.Name(), args, kwargs, "key", &m.Key, "room", &m.Room, "name", &m.Name, "description", &m.Description, "health?", &m.Health, "attack?", &m.Attack, "defense?", &m.Defense, "gold?", &m.Gold, "experience?", &m.Experience, "respawn?", &m.Respawn, "loot?", &m.Loot, "drop?", &m.Drop); err != nil {
				return nil, err
			}
			if !worldKey.MatchString(m.Key) || !worldKey.MatchString(m.Room) || !clean(m.Name, 100) || !clean(m.Description, 512) || (m.Loot != "" && !clean(m.Loot, 100)) {
				return nil, fmt.Errorf("invalid monster key, room or text")
			}
			if m.Health < 1 || m.Health > 1000 || m.Attack < 1 || m.Attack > 100 || m.Defense < 0 || m.Defense > 50 || m.Gold < 0 || m.Gold > 100 || m.Experience < 0 || m.Experience > 1000 || m.Respawn < 30 || m.Respawn > 3600 || m.Drop < 0 || m.Drop > 10000 || (m.Loot == "") != (m.Drop == 0) {
				return nil, fmt.Errorf("monster stats out of bounds")
			}
			if monsterKeys[m.Key] || len(world.Monsters) >= 32 {
				return nil, fmt.Errorf("duplicate monster or monster limit exceeded (32)")
			}
			monsterKeys[m.Key] = true
			world.Monsters = append(world.Monsters, m)
			return starlark.None, nil
		}),
		"room": starlark.NewBuiltin("room", func(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			r := WorldRoom{Kind: "normal", Exits: map[string]string{}}
			if err := starlark.UnpackArgs(b.Name(), args, kwargs, "key", &r.Key, "name", &r.Name, "description", &r.Description, "kind?", &r.Kind, "script?", &r.Script); err != nil {
				return nil, err
			}
			if !worldKey.MatchString(r.Key) || !clean(r.Name, 100) || !clean(r.Description, 512) || (r.Script != "" && !worldKey.MatchString(r.Script)) {
				return nil, fmt.Errorf("invalid room key, text or script")
			}
			if r.Kind != "normal" && r.Kind != "safe" && r.Kind != "shop" && r.Kind != "inn" && r.Kind != "dungeon" {
				return nil, fmt.Errorf("invalid room kind %q", r.Kind)
			}
			if _, exists := indexes[r.Key]; exists {
				return nil, fmt.Errorf("duplicate room %q", r.Key)
			}
			if len(world.Rooms) >= 96 {
				return nil, fmt.Errorf("world room limit exceeded (96)")
			}
			indexes[r.Key] = len(world.Rooms)
			world.Rooms = append(world.Rooms, r)
			return starlark.None, nil
		}),
		"link": starlark.NewBuiltin("link", func(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var e edge
			if err := starlark.UnpackArgs(b.Name(), args, kwargs, "from_room", &e.from, "direction", &e.direction, "to_room", &e.to); err != nil {
				return nil, err
			}
			if reverseDirection[e.direction] == "" || e.from == e.to {
				return nil, fmt.Errorf("invalid link direction or self-link")
			}
			if len(edges) >= 192 {
				return nil, fmt.Errorf("world link limit exceeded (192)")
			}
			edges = append(edges, e)
			return starlark.None, nil
		}),
	}
	if _, err := starlark.ExecFile(thread, "world.star", source, pre); err != nil {
		return Output{}, err
	}
	for _, m := range world.Monsters {
		i, ok := indexes[m.Room]
		if !ok {
			return Output{}, fmt.Errorf("monster references unknown room %s", m.Room)
		}
		if world.Rooms[i].Kind != "dungeon" && world.Rooms[i].Kind != "normal" {
			return Output{}, fmt.Errorf("monster must be in a normal or dungeon room")
		}
	}
	for _, e := range edges {
		a, okA := indexes[e.from]
		b, okB := indexes[e.to]
		if !okA || !okB {
			return Output{}, fmt.Errorf("link references unknown room: %s -> %s", e.from, e.to)
		}
		reverse := reverseDirection[e.direction]
		if world.Rooms[a].Exits[e.direction] != "" || world.Rooms[b].Exits[reverse] != "" {
			return Output{}, fmt.Errorf("link overwrites an exit: %s %s", e.from, e.direction)
		}
		world.Rooms[a].Exits[e.direction] = e.to
		world.Rooms[b].Exits[reverse] = e.from
	}
	if len(world.Rooms) == 0 {
		return Output{}, fmt.Errorf("world must contain rooms")
	}
	seen := map[string]bool{}
	var visit func(string)
	visit = func(key string) {
		if seen[key] {
			return
		}
		seen[key] = true
		for _, next := range world.Rooms[indexes[key]].Exits {
			visit(next)
		}
	}
	visit(world.Rooms[0].Key)
	if len(seen) != len(world.Rooms) {
		return Output{}, fmt.Errorf("world contains unreachable rooms")
	}
	return Output{World: world}, nil
}
