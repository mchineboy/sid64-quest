package telnet

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tylerhardison/race-condition-kingdom/pkg/models"
)

const (
	// Leave one column spare: C64 terminals may advance automatically at column 40.
	petsciiTextWidth = 39
	petWhite         = "\x05"
	petCyan          = "\x9f"
	petYellow        = "\x9e"
	petGreen         = "\x1e"
	petRed           = "\x1c"
)

// petsciiText wraps printable text before sending it. Native controls are added
// by the screen renderer, never counted as visible columns or re-encoded.
func petsciiText(text string) string {
	text = encodePETSCII(text)
	var out []string
	for _, paragraph := range strings.Split(text, "\r") {
		line := ""
		for _, word := range strings.Fields(paragraph) {
			if len(line) > 0 && len(line)+1+len(word) > petsciiTextWidth {
				out = append(out, line)
				line = ""
			}
			for len(word) > petsciiTextWidth {
				out = append(out, word[:petsciiTextWidth])
				word = word[petsciiTextWidth:]
			}
			if line != "" {
				line += " "
			}
			line += word
		}
		out = append(out, line)
	}
	return strings.Join(out, "\r")
}

func petLine(text string) string { return petsciiText(text) + "\r" }
func petHeading(title string) string {
	return "\x93\x0e" + petCyan + "\x12" + petLine(title) + "\x92" + petCyan + strings.Repeat("\x60", petsciiTextWidth) + "\r" + petWhite
}

func (c *Connection) petsciiCharacters(characters []*models.Character) string {
	var b strings.Builder
	b.WriteString(petHeading("Choose your character"))
	for i, ch := range characters {
		b.WriteString(petYellow + petLine(fmt.Sprintf("%d. %s", i+1, ch.Name)))
		b.WriteString(petWhite + petLine(fmt.Sprintf("Level %d   HP %d/%d", ch.Level, ch.Health, ch.MaxHealth)) + "\r")
	}
	b.WriteString(petCyan + petLine("Enter a number, or QUIT.") + petWhite + encodePETSCII("Character> "))
	return b.String()
}

func (c *Connection) petsciiRoom(npcs, items, others []string) string {
	if c.Room == nil {
		return petLine("The world is still loading.")
	}
	var b strings.Builder
	b.WriteString(petHeading(c.Room.Name))
	b.WriteString(petLine(c.Room.Description) + "\r")
	dirs := make([]string, 0, len(c.Room.Exits))
	for d := range c.Room.Exits {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	exits := "none"
	if len(dirs) > 0 {
		exits = strings.Join(dirs, " / ")
	}
	b.WriteString(petCyan + petLine("Exits: "+exits))
	for _, group := range []struct {
		label   string
		entries []string
	}{{"People", npcs}, {"Players", others}, {"Items", items}} {
		if len(group.entries) > 0 {
			b.WriteString(petYellow + petLine(group.label+": ") + petWhite + petLine(strings.Join(group.entries, ", ")))
		}
	}
	if c.Character != nil {
		ch := c.Character
		b.WriteString("\r" + petGreen + petLine(fmt.Sprintf("HP %d/%d  SP %d/%d", ch.Health, ch.MaxHealth, ch.Stamina, ch.MaxStamina)))
	}
	b.WriteString(petWhite)
	return b.String()
}

func petsciiHelp() string {
	return petHeading("Commands") + petLine(`LOOK / L       Room or LOOK <item>
N S E W        Move
WHERE          Location and exits
TAKE <item|all> Pick up
DROP <item>    Put down
USE <item>     Drink potion
EQUIP <item>   Wear or wield
UNEQUIP <item> Remove equipment
TALK [name]    Speak to an NPC
GIVE <item> [name]
REST           Recover at the inn
SAY <message>  Talk to the room
WHO            Online players
ANNOUNCE <msg> Server message (admin)
STATS          Character details
INV / I        Inventory
TERMINAL ANSI / PETSCII
SCRIPT         Builder scripting
QUIT / Q       Leave the game`)
}

func (c *Connection) petsciiInventory(items []*models.InventoryItem) string {
	b := petHeading("Inventory")
	if len(items) == 0 {
		b += petLine("Your pack is empty.")
	}
	for _, entry := range items {
		name := entry.Item.Name
		if entry.Quantity > 1 {
			name += fmt.Sprintf(" x%d", entry.Quantity)
		}
		if entry.Equipped {
			name += " (equipped)"
		}
		b += petLine(name)
	}
	return b + "\r" + petYellow + petLine(fmt.Sprintf("Gold: %d", c.Character.Gold)) + petWhite
}

func (c *Connection) petsciiStats() string {
	ch := c.Character
	b := petHeading(ch.Name)
	b += petLine(fmt.Sprintf("Level %d   XP %d", ch.Level, ch.Experience))
	b += petGreen + petLine(fmt.Sprintf("Health:  %d/%d", ch.Health, ch.MaxHealth))
	b += petLine(fmt.Sprintf("Stamina: %d/%d", ch.Stamina, ch.MaxStamina))
	b += petYellow + petLine(fmt.Sprintf("Gold: %d", ch.Gold)) + petWhite
	lawful, good := "Neutral", "Neutral"
	if ch.AlignmentLawful > 25 {
		lawful = "Lawful"
	} else if ch.AlignmentLawful < -25 {
		lawful = "Chaotic"
	}
	if ch.AlignmentGood > 25 {
		good = "Good"
	} else if ch.AlignmentGood < -25 {
		good = "Evil"
	}
	b += petLine("Alignment: " + lawful + " / " + good)
	b += petLine(fmt.Sprintf("Harbor deliveries: %d", ch.Deliveries))
	return b
}

func petsciiWho(players []OnlinePlayer) string {
	b := petHeading("Players online")
	for _, p := range players {
		b += petYellow + petLine(fmt.Sprintf("%s / Level %d", p.Name, p.Level))
		b += petWhite + petLine(p.Location)
	}
	return b + "\r" + petCyan + petLine(fmt.Sprintf("%d player(s) online", len(players))) + petWhite
}
