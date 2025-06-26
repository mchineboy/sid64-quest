package telnet

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/tylerhardison/race-condition-kingdom/internal/ansi"
	"github.com/tylerhardison/race-condition-kingdom/pkg/models"
)

// ReadLine reads a line of input from the connection
func (c *Connection) ReadLine() (string, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	// Set read deadline
	c.Conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	
	line, err := c.Reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	
	// Remove telnet control characters and trim
	line = strings.TrimSpace(line)
	line = strings.ReplaceAll(line, "\r", "")
	line = strings.ReplaceAll(line, "\n", "")
	
	return line, nil
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
	welcome := c.Formatter.Box(`
    ╔═══════════════════════════════════════════════════════════╗
    ║                                                           ║
    ║              🏰 RACE CONDITION KINGDOM 🏰                ║
    ║                                                           ║
    ║         A Modern MUD with Classic Telnet Gameplay        ║
    ║                                                           ║
    ║  Welcome, adventurer! Prepare for a nostalgic journey    ║
    ║  through a world of magic, combat, and community.        ║
    ║                                                           ║
    ╚═══════════════════════════════════════════════════════════╝
`, 65)
	
	if err := c.send(welcome + "\r\n\r\n"); err != nil {
		return err
	}
	
	return c.SendInfo("Connection established. Initializing...")
}

// SendAuthInstructions sends authentication instructions
func (c *Connection) SendAuthInstructions(authURL string) error {
	instructions := fmt.Sprintf(`
%s

To complete your login, please visit the following URL in your web browser:

%s

%s

Once you've authenticated, type 'check' to continue, or 'help' for more options.
Waiting for authentication...
`,
		c.Formatter.Colorize("🔐 AUTHENTICATION REQUIRED", ansi.UIWarning),
		c.Formatter.Colorize(authURL, ansi.UIInfo),
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
			c.Formatter.Colorize(char.Name, ansi.UIPrompt),
			fmt.Sprintf("%d", char.Level),
			healthColored,
			location,
		}
	}
	
	table := c.Formatter.Table(headers, rows)
	message.WriteString(table + "\r\n\r\n")
	
	// For now, auto-select the first character
	message.WriteString(c.Formatter.Colorize("Auto-selecting first character...", ansi.UISecondary) + "\r\n")
	
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
	
	// Show current room
	message.WriteString(c.formatRoomDescription())
	message.WriteString("\r\n")
	
	// Show prompt
	message.WriteString(c.formatPrompt())
	
	return c.send(message.String())
}

// SendHelp sends the help message
func (c *Connection) SendHelp() error {
	help := `
🆘 AVAILABLE COMMANDS:

Movement & Exploration:
  look, l          - Look around the current room
  north, n         - Go north (if exit exists)
  south, s         - Go south (if exit exists)
  east, e          - Go east (if exit exists)
  west, w          - Go west (if exit exists)

Communication:
  say <message>    - Say something to everyone in the room
  tell <player>    - Send a private message to a player
  who, w           - See who's online

Character & Inventory:
  stats, st        - View your character statistics
  inventory, inv, i - View your inventory
  equipment, eq    - View your equipped items

Game Information:
  time             - Check the current game time
  weather          - Check the weather
  help, h          - Show this help message
  quit, q          - Quit the game

Type any command to get started!
`
	
	return c.SendMessage(help)
}

// SendWhoList sends the list of online players
func (c *Connection) SendWhoList() error {
	// TODO: Implement actual who list from active connections
	whoList := c.Formatter.Colorize("👥 PLAYERS ONLINE", ansi.UIInfo) + "\r\n\r\n"
	whoList += fmt.Sprintf("%-20s %-10s %s\r\n", "Name", "Level", "Location")
	whoList += strings.Repeat("-", 50) + "\r\n"
	
	if c.Character != nil {
		whoList += fmt.Sprintf("%-20s %-10d %s\r\n", 
			c.Character.Name, 
			c.Character.Level, 
			"Town Square")
	}
	
	whoList += "\r\nTotal: 1 player online"
	
	return c.SendMessage(whoList)
}

// SendRoomDescription sends the current room description
func (c *Connection) SendRoomDescription() error {
	return c.send(c.formatRoomDescription() + "\r\n" + c.formatPrompt())
}

// SendInventory sends the character's inventory
func (c *Connection) SendInventory() error {
	if c.Character == nil {
		return c.SendError("No character selected")
	}
	
	inventory := c.Formatter.Colorize("🎒 INVENTORY", ansi.UIInfo) + "\r\n\r\n"
	
	// TODO: Get actual inventory from database
	inventory += "Your inventory is empty.\r\n"
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
func (c *Connection) formatRoomDescription() string {
	// TODO: Get actual room from database
	roomName := c.Formatter.Colorize("🏛️  Town Square", ansi.UIPrompt)
	
	description := `The heart of the kingdom, a bustling square where adventurers gather. 
Cobblestone paths lead in all directions, and a magnificent fountain sits in 
the center. This is a safe haven where no violence is permitted.

A cheerful Town Crier stands near the fountain, eager to share the latest news.

Obvious exits: north, south, east, west`
	
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
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
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