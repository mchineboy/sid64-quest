package telnet

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/tylerhardison/race-condition-kingdom/internal/ansi"
	"github.com/tylerhardison/race-condition-kingdom/pkg/models"
)

const (
	telnetIAC   = 255
	telnetWILL  = 251
	telnetDO    = 253
	telnetECHO  = 1
	telnetSB    = 250
	telnetSE    = 240
	telnetTTYPE = 24
	ttypeIS     = 0
	ttypeSEND   = 1
)

// ReadLine reads a line of input from the connection. The deadline bounds how
// long the player may stay idle, not how long a single read may take, so it
// must cover the time a user spends reading the screen or signing in elsewhere.
func (c *Connection) ReadLine() (string, error) {
	idle := c.IdleTimeout
	if idle <= 0 {
		idle = 15 * time.Minute
	}
	c.Conn.SetReadDeadline(time.Now().Add(idle))

	var line []byte
	for {
		character, err := c.readApplicationByte()
		if err != nil {
			return "", err
		}

		switch character {
		case '\n':
			if err := c.echoPETSCIIByte('\r'); err != nil {
				return "", err
			}
			return strings.TrimSpace(string(line)), nil
		case '\r':
			// Commodore clients commonly send CR without LF. Peeking an empty
			// socket would block until the full read deadline, so only consume
			// LF when it is already buffered.
			if c.Reader.Buffered() > 0 {
				if next, err := c.Reader.Peek(1); err == nil && next[0] == '\n' {
					_, _ = c.Reader.ReadByte()
				}
			}
			if err := c.echoPETSCIIByte('\r'); err != nil {
				return "", err
			}
			return strings.TrimSpace(string(line)), nil
		case '\b', 0x14, 0x7f:
			if len(line) > 0 {
				line = line[:len(line)-1]
				if err := c.echoPETSCIIByte(0x14); err != nil {
					return "", err
				}
			}
		default:
			line = append(line, c.petsciiInputByte(character))
			if err := c.echoPETSCIIByte(character); err != nil {
				return "", err
			}
		}
	}
}

// EnableServerEcho tells a negotiated telnet client that this server will echo
// typed characters. The dedicated PETSCII listener is raw-friendly and skips
// telnet negotiation, but still echoes bytes in ReadLine.
func (c *Connection) EnableServerEcho() error {
	c.writeMutex.Lock()
	defer c.writeMutex.Unlock()

	if err := c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	if _, err := c.Writer.Write([]byte{telnetIAC, telnetWILL, telnetECHO}); err != nil {
		return err
	}
	return c.Writer.Flush()
}

func (c *Connection) echoPETSCIIByte(character byte) error {
	if c.Presentation != PresentationPETSCII {
		return nil
	}

	c.writeMutex.Lock()
	defer c.writeMutex.Unlock()
	if err := c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	if err := c.Writer.WriteByte(character); err != nil {
		return err
	}
	return c.Writer.Flush()
}

// SendMessage sends a message to the connection
func (c *Connection) SendMessage(message string) error {
	return c.send(message + "\r\n")
}

// SendPrompt sends a prompt without a newline
func (c *Connection) SendPrompt(prompt string) error {
	return c.send(c.Formatter.Colorize(prompt, ansi.UIPrompt))
}

// SendError sends an error message
func (c *Connection) SendError(message string) error {
	formatted := c.Formatter.Colorize("ERROR: "+message, ansi.UIError)
	return c.send(formatted + "\r\n")
}

// SendSuccess sends a success message
func (c *Connection) SendSuccess(message string) error {
	formatted := c.Formatter.Colorize(message, ansi.UISuccess)
	return c.send(formatted + "\r\n")
}

// SendWarning sends a warning message
func (c *Connection) SendWarning(message string) error {
	formatted := c.Formatter.Colorize("WARNING: "+message, ansi.UIWarning)
	return c.send(formatted + "\r\n")
}

// SendInfo sends an info message
func (c *Connection) SendInfo(message string) error {
	formatted := c.Formatter.Colorize(message, ansi.UIInfo)
	return c.send(formatted + "\r\n")
}

// SendWelcome sends the initial welcome message
func (c *Connection) SendWelcome() error {
	if c.Presentation == PresentationPETSCII {
		if err := c.sendPETSCII(petsciiWelcome()); err != nil {
			return err
		}
		return c.SendInfo("COMMODORE DETECTED. PETSCII MODE ENABLED.")
	}

	welcome := c.Formatter.Colorize("RACE CONDITION KINGDOM", ansi.UIInfo) + "\r\n"
	welcome += "A small telnet world is waiting on the other side of a web login.\r\n\r\n"
	if err := c.send(welcome); err != nil {
		return err
	}

	return c.SendInfo("Connection established. Initializing...")
}

// SendAuthInstructions sends authentication instructions
func (c *Connection) SendAuthInstructions(authURL, entryURL, pairingCode string) error {
	if c.Presentation == PresentationPETSCII {
		return c.sendPETSCII(petsciiAuthInstructions(entryURL, pairingCode))
	}

	var scan string
	if c.Formatter.Enabled() {
		// A QR code is decoration if it fails to render; the URL and pairing
		// code below are the paths that must always work.
		if qr, err := ansiQRCode(authURL, 78); err == nil {
			scan = "\r\n" + qr
		}
	}

	instructions := fmt.Sprintf(`
%s
%s
Open this URL:

%s

Pairing code: %s

%s

Once you've authenticated, type 'check' to continue, or 'help' for more options.
Waiting for authentication...
`,
		c.Formatter.Colorize("🔐 AUTHENTICATION REQUIRED", ansi.UIWarning),
		scan,
		c.Formatter.Colorize(authURL, ansi.UIInfo),
		c.Formatter.Colorize(pairingCode, ansi.UIPrompt),
		c.Formatter.Colorize("This link will expire in 5 minutes for security.", ansi.UISecondary),
	)

	return c.send(instructions + "\r\n")
}

// SendCharacterList sends the character selection screen
func (c *Connection) SendCharacterList(characters []*models.Character) error {
	if len(characters) == 0 {
		return c.SendError("No characters found.")
	}

	var message strings.Builder
	message.WriteString(c.Formatter.Colorize("🎭 CHARACTER SELECTION", ansi.UIInfo) + "\r\n\r\n")

	// Create character table
	headers := []string{"#", "Name", "Level", "Health", "Location"}
	rows := make([][]string, len(characters))

	for i, char := range characters {
		health := fmt.Sprintf("%d/%d", char.Health, char.MaxHealth)
		healthColored := c.Formatter.Colorize(health, ansi.GetHealthColor(char.Health, char.MaxHealth))

		location := "Unknown"
		if char.CurrentRoomID != nil {
			location = "Town Square" // TODO: Get actual room name
		}

		rows[i] = []string{
			fmt.Sprintf("%d", i+1),
			char.Name,
			fmt.Sprintf("%d", char.Level),
			healthColored,
			location,
		}
	}

	table := c.Formatter.Table(headers, rows)
	message.WriteString(table + "\r\n\r\n")

	message.WriteString(c.Formatter.Colorize("Choose a character number: ", ansi.UISecondary))

	return c.send(message.String())
}

// SendGameWelcome sends the welcome message when entering the game
func (c *Connection) SendGameWelcome() error {
	if c.Character == nil {
		return fmt.Errorf("no character selected")
	}

	var message strings.Builder

	// Clear screen and show welcome
	message.WriteString(c.Formatter.ClearScreenAndHome())
	message.WriteString(c.Formatter.Colorize("🎮 ENTERING THE REALM", ansi.UISuccess) + "\r\n\r\n")

	// Character status
	message.WriteString(c.formatCharacterStatus())
	message.WriteString("\r\n")

	return c.send(message.String())
}

// SendHelp sends the help message
func (c *Connection) SendHelp() error {
	help := `
AVAILABLE COMMANDS

  look, l              Look around, or look <item>
  north, n             Move north when an exit exists
  south, s             Move south when an exit exists
  east, e              Move east when an exit exists
  west                 Move west when an exit exists
  take, get <item>     Pick up an item
  drop <item>          Drop an item in the room
  use <item>           Drink a potion
  equip <item>         Wear armor or wield a weapon
  unequip <item>       Stop using equipment
  talk [name]          Speak with someone here
  give <item> [name]   Hand an item to someone
  rest                 Recover health and stamina at an inn
  say <message>        Speak to everyone in the room
  who                  Show online players
  stats, st            Show your character stats
  inventory, inv, i    Show your inventory
  help, h              Show this help
  quit, q              Leave the game

Fetch the misplaced manifest from the Moonlit Docks and give it to the Town Crier.
`

	return c.SendMessage(help)
}

type OnlinePlayer struct {
	Name     string
	Level    int
	Location string
}

// SendWhoList sends the list of online players.
func (c *Connection) SendWhoList(players []OnlinePlayer) error {
	whoList := c.Formatter.Colorize("👥 PLAYERS ONLINE", ansi.UIInfo) + "\r\n\r\n"
	whoList += fmt.Sprintf("%-20s %-10s %s\r\n", "Name", "Level", "Location")
	whoList += strings.Repeat("-", 50) + "\r\n"

	for _, player := range players {
		whoList += fmt.Sprintf("%-20s %-10d %s\r\n", player.Name, player.Level, player.Location)
	}

	whoList += fmt.Sprintf("\r\nTotal: %d player(s) online", len(players))

	return c.SendMessage(whoList)
}

// SendRoomDescription sends the current room description
func (c *Connection) SendRoomDescription(npcs []string, items []string, others []string) error {
	return c.send(c.formatRoomDescription(npcs, items, others) + "\r\n")
}

// SendInventory sends the character's inventory
func (c *Connection) SendInventory(items []*models.InventoryItem) error {
	if c.Character == nil {
		return c.SendError("No character selected")
	}

	inventory := c.Formatter.Colorize("🎒 INVENTORY", ansi.UIInfo) + "\r\n\r\n"
	if len(items) == 0 {
		inventory += "Your inventory is empty.\r\n"
	} else {
		for _, entry := range items {
			name := entry.Item.Name
			if entry.Quantity > 1 {
				name = fmt.Sprintf("%s x%d", name, entry.Quantity)
			}
			if entry.Equipped {
				slot := "equipped"
				if entry.Item.IsWeapon() {
					slot = "wielded"
				} else if entry.Item.IsArmor() {
					slot = "worn"
				}
				name += " (" + slot + ")"
			}
			inventory += "  " + name + "\r\n"
		}
	}
	inventory += fmt.Sprintf("Gold: %s\r\n",
		c.Formatter.Colorize(fmt.Sprintf("%d", c.Character.Gold), ansi.ColorYellow))

	return c.SendMessage(inventory)
}

// SendStats sends the character's statistics
func (c *Connection) SendStats() error {
	if c.Character == nil {
		return c.SendError("No character selected")
	}

	var stats strings.Builder
	stats.WriteString(c.Formatter.Colorize("📊 CHARACTER STATISTICS", ansi.UIInfo) + "\r\n\r\n")

	// Basic info
	stats.WriteString(fmt.Sprintf("Name: %s\r\n",
		c.Formatter.Colorize(c.Character.Name, ansi.UIPrompt)))
	stats.WriteString(fmt.Sprintf("Level: %d\r\n", c.Character.Level))
	stats.WriteString(fmt.Sprintf("Experience: %d\r\n", c.Character.Experience))
	stats.WriteString("\r\n")

	// Health and Stamina with progress bars
	healthBar := c.Formatter.ProgressBar(c.Character.Health, c.Character.MaxHealth, 20,
		ansi.Style{Foreground: ansi.GetHealthColor(c.Character.Health, c.Character.MaxHealth)})
	stats.WriteString(fmt.Sprintf("Health: %s %d/%d\r\n",
		healthBar, c.Character.Health, c.Character.MaxHealth))

	staminaBar := c.Formatter.ProgressBar(c.Character.Stamina, c.Character.MaxStamina, 20,
		ansi.Style{Foreground: ansi.GetStaminaColor(c.Character.Stamina, c.Character.MaxStamina)})
	stats.WriteString(fmt.Sprintf("Stamina: %s %d/%d\r\n",
		staminaBar, c.Character.Stamina, c.Character.MaxStamina))
	stats.WriteString("\r\n")

	// Alignment
	alignment := c.Character.GetAlignment()
	lawfulText := "Neutral"
	if alignment.Lawful > 25 {
		lawfulText = "Lawful"
	} else if alignment.Lawful < -25 {
		lawfulText = "Chaotic"
	}

	goodText := "Neutral"
	if alignment.Good > 25 {
		goodText = "Good"
	} else if alignment.Good < -25 {
		goodText = "Evil"
	}

	stats.WriteString(fmt.Sprintf("Alignment: %s %s\r\n", lawfulText, goodText))
	stats.WriteString(fmt.Sprintf("Gold: %s\r\n",
		c.Formatter.Colorize(fmt.Sprintf("%d", c.Character.Gold), ansi.ColorYellow)))
	if c.Character.Deliveries > 0 {
		stats.WriteString(fmt.Sprintf("Harbor ledgers returned: %d\r\n", c.Character.Deliveries))
	}

	return c.SendMessage(stats.String())
}

// formatCharacterStatus formats the character status line
func (c *Connection) formatCharacterStatus() string {
	if c.Character == nil {
		return ""
	}

	health := c.Formatter.Colorize(
		fmt.Sprintf("HP: %d/%d", c.Character.Health, c.Character.MaxHealth),
		ansi.GetHealthColor(c.Character.Health, c.Character.MaxHealth))

	stamina := c.Formatter.Colorize(
		fmt.Sprintf("SP: %d/%d", c.Character.Stamina, c.Character.MaxStamina),
		ansi.GetStaminaColor(c.Character.Stamina, c.Character.MaxStamina))

	gold := c.Formatter.Colorize(fmt.Sprintf("Gold: %d", c.Character.Gold), ansi.ColorYellow)

	return fmt.Sprintf("[%s] [%s] [%s] Level %d %s",
		health, stamina, gold, c.Character.Level, c.Character.Name)
}

// formatRoomDescription formats the current room description
func (c *Connection) formatRoomDescription(npcs []string, items []string, others []string) string {
	if c.Room == nil {
		return "The world is still loading."
	}

	roomName := c.Formatter.Colorize(c.Room.Name, ansi.UIPrompt)
	directions := make([]string, 0, len(c.Room.Exits))
	for direction := range c.Room.Exits {
		directions = append(directions, direction)
	}
	sort.Strings(directions)
	exits := "none"
	if len(directions) > 0 {
		exits = strings.Join(directions, ", ")
	}

	var extra strings.Builder
	if len(npcs) > 0 {
		extra.WriteString("\r\nPeople here: " + strings.Join(npcs, ", "))
	}
	if len(others) > 0 {
		extra.WriteString("\r\nAlso here: " + strings.Join(others, ", "))
	}
	if len(items) > 0 {
		extra.WriteString("\r\nYou see: " + strings.Join(items, ", "))
	}

	description := fmt.Sprintf("%s\r\n\r\nObvious exits: %s%s", c.Room.Description, exits, extra.String())
	return fmt.Sprintf("%s\r\n%s", roomName, description)
}

// formatPrompt formats the command prompt
func (c *Connection) formatPrompt() string {
	if c.Character == nil {
		return "> "
	}

	prompt := fmt.Sprintf("[%s]> ", c.Character.Name)
	return c.Formatter.Colorize(prompt, ansi.UIPrompt)
}

// UpdateActivity updates the last activity time
func (c *Connection) UpdateActivity() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.LastActivity = time.Now()
}

// Close closes the connection
func (c *Connection) Close() error {
	c.Cancel()
	return c.Conn.Close()
}

// send sends raw data to the connection
func (c *Connection) send(data string) error {
	if c.Presentation == PresentationPETSCII {
		data = encodePETSCII(data)
	}
	return c.write(data)
}

// sendPETSCII writes a native PETSCII byte stream, including control codes and
// graphics glyphs that must not be transformed as ordinary text.
func (c *Connection) sendPETSCII(data string) error {
	return c.write(data)
}

func (c *Connection) write(data string) error {
	c.writeMutex.Lock()
	defer c.writeMutex.Unlock()

	// Set write deadline
	c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

	_, err := c.Writer.WriteString(data)
	if err != nil {
		return err
	}

	return c.Writer.Flush()
}

// IsConnected returns true if the connection is still active
func (c *Connection) IsConnected() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	select {
	case <-c.Context.Done():
		return false
	default:
		return true
	}
}

// GetRemoteAddr returns the remote address of the connection
func (c *Connection) GetRemoteAddr() string {
	return c.Conn.RemoteAddr().String()
}

// SetTerminalType selects the presentation that matches the advertised
// terminal. A client that does not answer the standard telnet negotiation
// remains on the ANSI/default path.
func (c *Connection) SetTerminalType(terminalType string) {
	c.TerminalType = terminalType
	c.Presentation = presentationForTerminalType(terminalType)
	c.Formatter = ansi.NewFormatter(c.Presentation == PresentationANSI)
}

// NegotiateTerminalType asks for RFC 930/RFC 884 terminal type information.
// It waits only briefly so old clients that never negotiate are not held at a
// blank screen. Any ordinary bytes received during that interval are retained
// in the buffered reader for ReadLine.
func (c *Connection) NegotiateTerminalType(timeout time.Duration) (string, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if err := c.Conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return "", fmt.Errorf("set negotiation deadline: %w", err)
	}
	defer c.Conn.SetReadDeadline(time.Time{})

	if _, err := c.Writer.Write([]byte{telnetIAC, telnetDO, telnetTTYPE}); err != nil {
		return "", fmt.Errorf("request terminal type: %w", err)
	}
	if err := c.Writer.Flush(); err != nil {
		return "", fmt.Errorf("flush terminal type request: %w", err)
	}

	for {
		character, err := c.Reader.ReadByte()
		if err != nil {
			if isTimeout(err) {
				return "", nil
			}
			return "", err
		}
		if character != telnetIAC {
			if err := c.Reader.UnreadByte(); err != nil {
				return "", fmt.Errorf("preserve client input: %w", err)
			}
			return "", nil
		}

		command, err := c.Reader.ReadByte()
		if err != nil {
			return "", err
		}
		switch command {
		case telnetWILL:
			option, err := c.Reader.ReadByte()
			if err != nil {
				return "", err
			}
			if option == telnetTTYPE {
				if _, err := c.Writer.Write([]byte{telnetIAC, telnetSB, telnetTTYPE, ttypeSEND, telnetIAC, telnetSE}); err != nil {
					return "", fmt.Errorf("request terminal type value: %w", err)
				}
				if err := c.Writer.Flush(); err != nil {
					return "", fmt.Errorf("flush terminal type value request: %w", err)
				}
			}
		case telnetDO, 252, 254:
			if _, err := c.Reader.ReadByte(); err != nil {
				return "", err
			}
		case telnetSB:
			option, data, err := readTelnetSubnegotiation(c.Reader)
			if err != nil {
				return "", err
			}
			if option == telnetTTYPE && len(data) > 1 && data[0] == ttypeIS {
				return string(data[1:]), nil
			}
		}
	}
}

func readTelnetSubnegotiation(reader *bufio.Reader) (byte, []byte, error) {
	option, err := reader.ReadByte()
	if err != nil {
		return 0, nil, err
	}

	var data []byte
	for {
		character, err := reader.ReadByte()
		if err != nil {
			return 0, nil, err
		}
		if character != telnetIAC {
			data = append(data, character)
			continue
		}

		next, err := reader.ReadByte()
		if err != nil {
			return 0, nil, err
		}
		switch next {
		case telnetSE:
			return option, data, nil
		case telnetIAC:
			data = append(data, telnetIAC)
		default:
			return 0, nil, fmt.Errorf("unexpected telnet subnegotiation command %d", next)
		}
	}
}

func isTimeout(err error) bool {
	if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
		return true
	}
	return err == io.EOF
}

func (c *Connection) readApplicationByte() (byte, error) {
	character, err := c.Reader.ReadByte()
	if err != nil || character != telnetIAC {
		return character, err
	}

	command, err := c.Reader.ReadByte()
	if err != nil {
		return 0, err
	}
	if command == telnetIAC {
		return telnetIAC, nil
	}
	if command == telnetSB {
		_, _, err := readTelnetSubnegotiation(c.Reader)
		return 0, err
	}

	// Negotiation commands are followed by an option byte and are not player
	// input. Keep reading until a printable byte or line ending arrives.
	if command == telnetWILL || command == telnetDO || command == 252 || command == 254 {
		if _, err := c.Reader.ReadByte(); err != nil {
			return 0, err
		}
	}
	return c.readApplicationByte()
}

func (c *Connection) petsciiInputByte(character byte) byte {
	if c.Presentation != PresentationPETSCII {
		return character
	}
	if character >= 0xc1 && character <= 0xda {
		return 'a' + (character - 0xc1)
	}
	return character
}
