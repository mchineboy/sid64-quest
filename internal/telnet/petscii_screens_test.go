package telnet

import (
	"github.com/google/uuid"
	"github.com/tylerhardison/race-condition-kingdom/pkg/models"
	"strings"
	"testing"
)

func assertPETSCIIScreen(t *testing.T, screen string, maxRows int) {
	t.Helper()
	col, rows := 0, 1
	for _, b := range []byte(screen) {
		switch b {
		case 13:
			rows++
			col = 0
		case 0x93:
			rows = 1
			col = 0
		case 0x05, 0x9f, 0x9e, 0x1e, 0x1c, 0x12, 0x92, 0x0e:
		default:
			if b < 32 || (b > 126 && (b < 0xc1 || b > 0xda)) {
				t.Fatalf("unexpected byte %02x", b)
			}
			col++
			if col > 39 {
				t.Fatalf("row %d exceeds safe width", rows)
			}
		}
	}
	if rows > maxRows {
		t.Fatalf("screen takes %d rows; budget %d", rows, maxRows)
	}
	if strings.ContainsAny(screen, "?\x1b\n") {
		t.Fatal("replacement glyph, ANSI escape or LF in screen")
	}
}

func TestPETSCIIScreensFitC64(t *testing.T) {
	ch := &models.Character{Name: "Testchar", Level: 1, Health: 100, MaxHealth: 100, Stamina: 97, MaxStamina: 100, Gold: 130}
	c := &Connection{Character: ch, Room: &models.Room{Name: "Market Lane", Description: "Canvas awnings snap overhead. A baker, a tinker, and a suspiciously cheerful potion seller compete for your attention.", Exits: map[string]uuid.UUID{"west": uuid.New()}}}
	items := []*models.InventoryItem{{Item: &models.Item{Name: "Leather armor"}, Quantity: 1, Equipped: true}}
	for name, screen := range map[string]string{
		"selection": c.petsciiCharacters([]*models.Character{ch}),
		"room":      c.petsciiRoom(nil, []string{"Leather armor", "Rusty sword", "Stamina potion"}, nil),
		"help":      petsciiHelp(), "inventory": c.petsciiInventory(items), "stats": c.petsciiStats(),
		"who": petsciiWho([]OnlinePlayer{{Name: "Testchar", Level: 1, Location: "Market Lane"}}),
	} {
		t.Run(name, func(t *testing.T) { assertPETSCIIScreen(t, screen, 23) })
	}
	room := c.petsciiRoom(nil, nil, nil)
	if !strings.Contains(room, "POTION") || !strings.Contains(room, encodePETSCII("HP 100/100 SP 97/100")) {
		t.Fatal("room missing text or status")
	}
}

func TestPETSCIIWrapPreservesWordsAndBreaks(t *testing.T) {
	input := "A suspiciously cheerful potion seller competes for your attention.\r\n\r\nExits: west"
	got := petsciiText(input)
	assertPETSCIIScreen(t, got, 8)
	if strings.Contains(got, "POT\rION") || !strings.Contains(got, encodePETSCII("\r\rExits: west")) {
		t.Fatalf("bad wrapping: %q", got)
	}
	long := petsciiText(strings.Repeat("a", 90))
	assertPETSCIIScreen(t, long, 3)
	if strings.ReplaceAll(long, "\r", "") != strings.Repeat("A", 90) {
		t.Fatal("long token lost data")
	}
}

func TestPETSCIIRoomDispatchUsesNativeRenderer(t *testing.T) {
	c, collect := drainedConnection(t)
	c.SetTerminalType("petscii")
	c.Room = &models.Room{Name: "Market Lane", Description: "A quiet market."}
	if err := c.SendRoomDescription(nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.SendPrompt(c.formatPrompt()); err != nil {
		t.Fatal(err)
	}
	got := collect()
	if !strings.HasPrefix(got, "\x93\x0e") || !strings.Contains(got, petCyan) {
		t.Fatal("native screen controls missing")
	}
	assertPETSCIIScreen(t, got, 23)
}

func TestPETSCIILoginScreensFitWithoutAutoWrap(t *testing.T) {
	c, collect := drainedConnection(t)
	c.SetTerminalType("petscii")
	if err := c.SendWelcome(); err != nil {
		t.Fatal(err)
	}
	if err := (&Server{}).handleInitialConnection(c); err != nil {
		t.Fatal(err)
	}
	welcome := collect()
	assertPETSCIIScreen(t, welcome, 23)
	if !strings.HasSuffix(welcome, encodePETSCII("Username>")+petWhite+" ") {
		t.Fatal("missing username prompt")
	}
	if strings.Contains(welcome, "DETECTED") {
		t.Fatal("redundant terminal detection notice")
	}
	for _, url := range []string{"http://192.168.1.81:8080/pair", "https://symptom-pi.example-tailnet.ts.net/pair"} {
		pairing := petsciiAuthInstructions(url, "9MDJ-XNAH")
		assertPETSCIIScreen(t, pairing, 23)
		if !strings.Contains(pairing, encodePETSCII("9MDJ-XNAH")) || !strings.HasSuffix(pairing, encodePETSCII("Check> ")) {
			t.Fatal("missing pairing instructions")
		}
	}
}
