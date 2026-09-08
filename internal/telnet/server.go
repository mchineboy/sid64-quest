package telnet

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/tylerhardison/race-condition-kingdom/internal/ansi"
	"github.com/tylerhardison/race-condition-kingdom/internal/auth"
	"github.com/tylerhardison/race-condition-kingdom/internal/events"
	"github.com/tylerhardison/race-condition-kingdom/internal/game"
	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
	"github.com/tylerhardison/race-condition-kingdom/pkg/models"
)

// errQuit reports that the player asked to leave. It travels the same return
// path as a failure, so the connection loop must distinguish it from one and
// close the session without complaining to the player.
var errQuit = errors.New("client requested disconnect")

// ConnectionState represents the state of a telnet connection
type ConnectionState int

const (
	StateConnected ConnectionState = iota
	StateAwaitingUsername
	StateAwaitingAuth
	StateAuthenticated
	StateInGame
	StateDisconnected
)

// Connection represents a telnet client connection
type Connection struct {
	ID           string
	Conn         net.Conn
	State        ConnectionState
	Username     string
	User         *models.User
	Character    *models.Character
	Room         *models.Room
	Session      *models.Session
	AuthToken    string
	PairingCode  string
	LastActivity time.Time
	IdleTimeout  time.Duration
	Reader       *bufio.Reader
	Writer       *bufio.Writer
	Formatter    *ansi.Formatter
	TerminalType string
	Presentation Presentation
	Context      context.Context
	Cancel       context.CancelFunc
	mutex        sync.RWMutex
	writeMutex   sync.Mutex
	afterCR      bool
	ScriptEditor *scriptEditor
}

// Server represents the telnet server
type Server struct {
	hub          *playerHub
	config       *config.Config
	authService  *auth.AuthService
	eventBus     *events.EventBus
	world        *game.WorldService
	logger       *logrus.Logger
	port         int
	forcePETSCII bool
	listener     net.Listener
	connections  map[string]*Connection
	connMutex    sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// NewServer creates a new telnet server
func NewServer(cfg *config.Config, authService *auth.AuthService, eventBus *events.EventBus, world *game.WorldService, logger *logrus.Logger) *Server {
	return newServer(cfg, authService, eventBus, world, logger, cfg.Server.TelnetPort, false)
}

// NewPETSCIIServer starts a raw-friendly listener for Commodore clients that
// do not advertise a terminal type through telnet negotiation.
func NewPETSCIIServer(cfg *config.Config, authService *auth.AuthService, eventBus *events.EventBus, world *game.WorldService, logger *logrus.Logger) *Server {
	return newServer(cfg, authService, eventBus, world, logger, cfg.Server.PETSCIIPort, true)
}

func newServer(cfg *config.Config, authService *auth.AuthService, eventBus *events.EventBus, world *game.WorldService, logger *logrus.Logger, port int, forcePETSCII bool) *Server {
	ctx, cancel := context.WithCancel(context.Background())

	return &Server{
		hub:          newPlayerHub(),
		config:       cfg,
		authService:  authService,
		eventBus:     eventBus,
		world:        world,
		logger:       logger,
		port:         port,
		forcePETSCII: forcePETSCII,
		connections:  make(map[string]*Connection),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Start starts the telnet server
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Server.Host, s.port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to start telnet server: %w", err)
	}

	s.listener = listener
	s.logger.WithField("address", addr).Info("Telnet server started")

	// Start connection cleanup routine
	s.wg.Add(1)
	go s.cleanupRoutine()

	// Accept connections
	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return nil
			default:
				s.logger.WithError(err).Error("Failed to accept connection")
				continue
			}
		}

		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

// Stop stops the telnet server
func (s *Server) Stop() error {
	s.cancel()

	if s.listener != nil {
		s.listener.Close()
	}

	// Close all connections
	s.connMutex.Lock()
	for _, conn := range s.connections {
		conn.Close()
	}
	s.connMutex.Unlock()

	s.wg.Wait()
	s.logger.Info("Telnet server stopped")
	return nil
}

// handleConnection handles a new telnet connection
func (s *Server) handleConnection(netConn net.Conn) {
	defer s.wg.Done()

	connID := uuid.New().String()
	ctx, cancel := context.WithCancel(s.ctx)

	idleTimeout := s.idleTimeout()

	conn := &Connection{
		ID:           connID,
		Conn:         netConn,
		State:        StateConnected,
		LastActivity: time.Now(),
		IdleTimeout:  idleTimeout,
		Reader:       bufio.NewReader(netConn),
		Writer:       bufio.NewWriter(netConn),
		Formatter:    ansi.NewFormatter(!s.forcePETSCII),
		Presentation: PresentationANSI,
		Context:      ctx,
		Cancel:       cancel,
	}

	// Set connection timeouts
	netConn.SetReadDeadline(time.Now().Add(idleTimeout))
	netConn.SetWriteDeadline(time.Now().Add(time.Duration(s.config.Server.WriteTimeout) * time.Second))

	// Add to connections map
	s.connMutex.Lock()
	if s.config.Server.MaxConnections > 0 && len(s.connections) >= s.config.Server.MaxConnections {
		s.connMutex.Unlock()
		conn.Close()
		return
	}
	s.connections[connID] = conn
	s.connMutex.Unlock()

	// Remove from connections map when done
	defer func() {
		s.hub.remove(connID)
		_ = s.authService.DeleteSession(connID)
		s.connMutex.Lock()
		delete(s.connections, connID)
		s.connMutex.Unlock()
		conn.Close()
	}()

	s.logger.WithFields(logrus.Fields{
		"connection_id": connID,
		"remote_addr":   netConn.RemoteAddr().String(),
	}).Info("New telnet connection")

	if s.forcePETSCII {
		conn.SetTerminalType("PETSCII (dedicated port)")
	} else if terminalType, err := conn.NegotiateTerminalType(250 * time.Millisecond); err != nil {
		s.logger.WithError(err).Debug("Terminal type negotiation failed")
	} else if terminalType != "" {
		conn.SetTerminalType(terminalType)
		s.logger.WithFields(logrus.Fields{
			"connection_id": connID,
			"terminal_type": terminalType,
			"presentation":  conn.Presentation.String(),
		}).Info("Terminal type detected")
	}
	if conn.Presentation == PresentationPETSCII && !s.forcePETSCII {
		if err := conn.EnableServerEcho(); err != nil {
			s.logger.WithError(err).Debug("Failed to negotiate server echo")
		}
	}

	// Send the greeting and immediately show the username prompt.
	if err := conn.SendWelcome(); err != nil {
		s.logger.WithError(err).Debug("Failed to send welcome message")
		return
	}
	if err := s.handleInitialConnection(conn); err != nil {
		s.logger.WithError(err).Debug("Failed to initialize connection")
		return
	}

	// Handle the connection
	s.connectionLoop(conn)
}

// connectionLoop handles the main connection loop
func (s *Server) connectionLoop(conn *Connection) {
	type readResult struct {
		input string
		err   error
	}
	reads := make(chan readResult)
	go func() {
		for {
			input, err := conn.ReadLine()
			select {
			case reads <- readResult{input: input, err: err}:
			case <-conn.Context.Done():
				return
			}
			if err != nil && !errors.Is(err, errLineTooLong) {
				return
			}
		}
	}()

	// Browser authentication happens out of band. Polling here lets the
	// terminal advance as soon as the browser links its pending session while
	// keeping reads, polling, and state transitions serialized in this loop.
	authTicker := time.NewTicker(time.Second)
	defer authTicker.Stop()

	for {
		select {
		case <-conn.Context.Done():
			return
		case <-authTicker.C:
			if conn.State == StateAwaitingAuth {
				if err := s.checkAuthentication(conn, false); err != nil {
					s.logger.WithError(err).Error("Error polling authentication")
					if conn.SendError("An error occurred checking authentication.") != nil {
						return
					}
				}
			}
		case result := <-reads:
			input, err := result.input, result.err
			if err != nil {
				if errors.Is(err, errLineTooLong) {
					if conn.SendError("Command too long (maximum 1024 bytes). Please try again.") != nil {
						return
					}
					continue
				}
				if err.Error() != "EOF" {
					s.logger.WithError(err).Debug("Connection read error")
				}
				return
			}

			conn.UpdateActivity()

			// Process input based on connection state
			if err := s.processInput(conn, input); err != nil {
				if errors.Is(err, errQuit) {
					return
				}
				s.logger.WithError(err).Error("Error processing input")
				conn.SendError("An error occurred processing your input.")
			}
		}
	}
}

// processInput processes input based on the connection state
func (s *Server) processInput(conn *Connection, input string) error {
	defer s.hub.update(conn)
	if conn.State == StateInGame && conn.Session != nil {
		active, err := s.authService.UserActive(conn.Context, conn.Session.UserID)
		if err != nil || !active {
			conn.SendMessage("Your session has ended. Please reconnect.")
			return errQuit
		}
	}

	if conn.State == StateInGame && conn.ScriptEditor != nil {
		return s.editScript(conn, input)
	}
	input = strings.TrimSpace(input)
	parts := strings.Fields(input)
	if len(parts) > 0 && strings.EqualFold(parts[0], "terminal") {
		return s.handleTerminalCommand(conn, parts[1:])
	}

	switch conn.State {
	case StateConnected:
		return s.handleInitialConnection(conn)
	case StateAwaitingUsername:
		return s.handleUsernameInput(conn, input)
	case StateAwaitingAuth:
		return s.handleAuthWaiting(conn, input)
	case StateAuthenticated:
		return s.handleCharacterSelection(conn, input)
	case StateInGame:
		return s.handleGameCommand(conn, input)
	default:
		return fmt.Errorf("unknown connection state: %d", conn.State)
	}
}

// handleInitialConnection handles the initial connection state
func (s *Server) handleInitialConnection(conn *Connection) error {
	conn.State = StateAwaitingUsername
	if conn.Presentation == PresentationPETSCII {
		return conn.SendPrompt("Username>")
	}
	conn.SendMessage("Display: terminal ansi or terminal petscii")
	return conn.SendPrompt("Enter your username: ")
}

// handleUsernameInput handles username input
func (s *Server) handleUsernameInput(conn *Connection, username string) error {
	username = strings.ToLower(username)

	switch username {
	case "quit", "q", "exit":
		conn.SendMessage("Goodbye!")
		return errQuit
	}

	if len(username) < 3 || len(username) > 20 {
		conn.SendError("Username must be between 3 and 20 characters.")
		conn.SendPrompt("Enter your username: ")
		return nil
	}

	// Validate username format (alphanumeric + underscore)
	for _, r := range username {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			conn.SendError("Username can only contain letters, numbers, and underscores.")
			conn.SendPrompt("Enter your username: ")
			return nil
		}
	}

	conn.Username = username

	// Generate a long internal token and a short code suitable for a
	// 40-column terminal and phone entry.
	token, pairingCode, err := s.authService.GenerateAuthChallenge(conn.ID)
	if err != nil {
		return fmt.Errorf("failed to generate auth challenge: %w", err)
	}

	conn.AuthToken = token
	conn.PairingCode = pairingCode
	conn.State = StateAwaitingAuth

	pairingURL := s.authService.GetPairingURL(pairingCode)
	conn.SendAuthInstructions(pairingURL, s.authService.GetPairingEntryURL(), pairingCode)

	return nil
}

// handleAuthWaiting handles input while waiting for authentication
func (s *Server) handleAuthWaiting(conn *Connection, input string) error {
	input = strings.ToLower(input)

	switch input {
	case "qr":
		if conn.Presentation == PresentationPETSCII {
			screen, err := petsciiQRScreen(s.authService.GetPairingURL(conn.PairingCode))
			if err != nil {
				return conn.SendMessage("QR does not fit this screen. Type HELP for the URL.")
			}
			return conn.sendPETSCII(screen)
		}
		return conn.SendAuthInstructions(s.authService.GetPairingURL(conn.PairingCode), s.authService.GetPairingEntryURL(), conn.PairingCode)
	case "help", "h":
		pairingURL := s.authService.GetPairingURL(conn.PairingCode)
		conn.SendAuthInstructions(pairingURL, s.authService.GetPairingEntryURL(), conn.PairingCode)
	case "quit", "q", "exit":
		conn.SendMessage("Goodbye!")
		return errQuit
	case "renew":
		return s.handleUsernameInput(conn, conn.Username)
	case "check", "c":
		return s.checkAuthentication(conn, true)
	default:
		conn.SendMessage("Waiting for authentication... Type 'help' for instructions or 'quit' to exit.")
	}

	return nil
}

// checkAuthentication checks if the user has completed authentication
func (s *Server) checkAuthentication(conn *Connection, notifyPending bool) error {
	// The browser marks a token as used when it links the pending session. The
	// session, rather than the now-used token, is the source of truth here.
	session, err := s.authService.GetSession(conn.ID)
	if err != nil {
		if notifyPending {
			conn.SendMessage("Authentication not yet complete. Please visit the authentication URL.")
		}
		return nil
	}

	// Check if session has been authenticated
	if session.UserID == uuid.Nil || session.AuthToken != conn.AuthToken {
		if notifyPending {
			conn.SendMessage("Authentication not yet complete. Please visit the authentication URL.")
		}
		return nil
	}

	// Authentication complete!
	conn.Session = session
	conn.State = StateAuthenticated

	conn.SendSuccess("Authentication successful! Welcome back!")

	// Get user characters
	characters, err := s.authService.GetUserCharacters(session.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user characters: %w", err)
	}

	if len(characters) == 0 {
		// The player is signed in but has nothing to play. Say so and leave the
		// session open so they can create a character in the browser and retry.
		conn.SendMessage("You don't have any characters yet. Create one in the browser, then type 'check' again.")
		return nil
	}

	conn.SendCharacterList(characters)
	return nil
}

// handleCharacterSelection handles character selection
func (s *Server) handleCharacterSelection(conn *Connection, input string) error {
	switch strings.ToLower(input) {
	case "quit", "q", "exit":
		conn.SendMessage("Goodbye!")
		return errQuit
	case "check", "c":
		return s.checkAuthentication(conn, true)
	}
	characters, err := s.authService.GetUserCharacters(conn.Session.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user characters: %w", err)
	}

	if len(characters) == 0 {
		conn.SendError("No characters available.")
		return nil
	}

	selection, err := strconv.Atoi(input)
	if err != nil || selection < 1 || selection > len(characters) {
		conn.SendError(fmt.Sprintf("Choose a character number between 1 and %d.", len(characters)))
		conn.SendPrompt("Choose a character: ")
		return nil
	}

	conn.Character = characters[selection-1]
	if conn.Character.CurrentRoomID == nil {
		return fmt.Errorf("character has no current room")
	}
	loaded, err := s.world.LoadCharacter(conn.Context, conn.Character.ID)
	if err != nil {
		return fmt.Errorf("load character: %w", err)
	}
	conn.Character = loaded
	room, err := s.world.GetRoom(conn.Context, *conn.Character.CurrentRoomID)
	if err != nil {
		return fmt.Errorf("load character room: %w", err)
	}
	if !s.hub.claim(conn.Character.ID, conn.ID) {
		conn.Character = nil
		return conn.SendError("That character is already online. Quit the other connection, then choose again.")
	}
	conn.Room = room
	conn.State = StateInGame
	s.hub.update(conn)

	// Publish player connect event
	event := events.PlayerConnectEvent(conn.Character.ID, conn.Character.Name)
	if err := s.eventBus.Publish(event); err != nil {
		s.logger.WithError(err).Warn("Failed to publish player connect event")
	}

	if err := conn.SendGameWelcome(); err != nil {
		return err
	}
	if err := s.sendLook(conn); err != nil {
		return err
	}
	if _, err := s.runScriptHook(conn, "room", conn.Room.ID, "on_enter", "login", ""); err != nil {
		return err
	}
	return conn.SendPrompt(conn.formatPrompt())
}

// handleGameCommand handles in-game commands
func (s *Server) handleGameCommand(conn *Connection, input string) error {
	if input == "" {
		return conn.SendPrompt(conn.formatPrompt())
	}

	if err := s.handleGameCommandWithoutPrompt(conn, input); err != nil {
		return err
	}

	return conn.SendPrompt(conn.formatPrompt())
}

func (s *Server) handleGameCommandWithoutPrompt(conn *Connection, input string) error {
	if input == "" {
		return nil
	}

	// Parse command
	parts := strings.Fields(input)
	command := strings.ToLower(parts[0])
	args := parts[1:]

	// Handle basic commands
	switch command {
	case "script":
		return s.scriptCommand(conn, args)
	case "quit", "q", "exit":
		conn.SendMessage("Goodbye!")
		return errQuit
	case "help", "h":
		conn.SendHelp()
	case "who":
		s.sendWhoList(conn)
	case "say":
		if len(args) > 0 {
			message := strings.Join(args, " ")
			s.broadcastToRoom(conn.Room.ID, fmt.Sprintf("%s says: %s", conn.Character.Name, message))
			_, err := s.runScriptHook(conn, "room", conn.Room.ID, "on_say", command, message)
			return err
		} else {
			conn.SendError("Say what?")
		}
	case "where":
		return conn.SendMessage(fmt.Sprintf("You are in %s. Exits: %s.", conn.Room.Name, strings.Join(game.SortedExitNames(conn.Room.Exits), ", ")))
	case "look", "l":
		if len(args) == 0 {
			if err := s.sendLook(conn); err != nil {
				return err
			}
			_, err := s.runScriptHook(conn, "room", conn.Room.ID, "on_look", command, "")
			return err
		}
		return s.examineItem(conn, strings.Join(args, " "))
	case "north", "n", "south", "s", "east", "e", "west", "w":
		return s.moveCharacter(conn, command)
	case "take", "get":
		if len(args) == 0 {
			conn.SendError("Take what?")
			return nil
		}
		return s.takeItem(conn, strings.Join(args, " "))
	case "drop":
		if len(args) == 0 {
			conn.SendError("Drop what?")
			return nil
		}
		return s.dropItem(conn, strings.Join(args, " "))
	case "use", "drink":
		if len(args) == 0 {
			conn.SendError("Use what?")
			return nil
		}
		if target, err := s.world.ScriptTarget(conn.Context, conn.Character.ID, conn.Room.ID, "item", strings.Join(args, " ")); err == nil {
			if handled, err := s.runScriptHook(conn, "item", target, "on_use", command, strings.Join(args, " ")); err != nil || handled {
				return err
			}
		}
		return s.useItem(conn, strings.Join(args, " "))
	case "equip", "wear", "wield":
		if len(args) == 0 {
			conn.SendError("Equip what?")
			return nil
		}
		message, err := s.world.EquipItem(conn.Context, conn.Character.ID, strings.Join(args, " "))
		if err != nil {
			conn.SendError(err.Error())
			return nil
		}
		return conn.SendMessage(message)
	case "unequip", "remove":
		if len(args) == 0 {
			conn.SendError("Unequip what?")
			return nil
		}
		message, err := s.world.UnequipItem(conn.Context, conn.Character.ID, strings.Join(args, " "))
		if err != nil {
			conn.SendError(err.Error())
			return nil
		}
		return conn.SendMessage(message)
	case "talk":
		if target, err := s.world.ScriptTarget(conn.Context, conn.Character.ID, conn.Room.ID, "npc", strings.Join(args, " ")); err == nil {
			if handled, err := s.runScriptHook(conn, "npc", target, "on_talk", command, strings.Join(args, " ")); err != nil || handled {
				return err
			}
		}
		message, err := s.world.Talk(conn.Context, conn.Character.ID, conn.Room.ID, strings.Join(args, " "))
		if err != nil {
			conn.SendError(err.Error())
			return nil
		}
		return conn.SendMessage(message)
	case "give":
		if len(args) == 0 {
			conn.SendError("Give what?")
			return nil
		}
		return s.giveItem(conn, args)
	case "rest":
		message, character, err := s.world.Rest(conn.Context, conn.Character.ID, conn.Room.ID)
		if err != nil {
			conn.SendError(err.Error())
			return nil
		}
		conn.Character = character
		return conn.SendSuccess(message)
	case "inventory", "inv", "i":
		items, err := s.world.ListInventory(conn.Context, conn.Character.ID)
		if err != nil {
			return err
		}
		return conn.SendInventory(items)
	case "stats", "st":
		character, err := s.world.LoadCharacter(conn.Context, conn.Character.ID)
		if err != nil {
			return err
		}
		conn.Character = character
		return conn.SendStats()
	default:
		if handled, err := s.runScriptHook(conn, "room", conn.Room.ID, "on_command", command, strings.Join(args, " ")); err != nil || handled {
			return err
		}
		conn.SendError(fmt.Sprintf("Unknown command: %s", command))
	}

	return nil
}

func (s *Server) moveCharacter(conn *Connection, direction string) error {
	if conn.Character == nil || conn.Room == nil {
		return fmt.Errorf("no character room loaded")
	}
	from, to, stamina, err := s.world.MoveCharacter(conn.Context, conn.Character.ID, direction)
	if err != nil {
		conn.SendError(err.Error())
		return nil
	}
	direction = game.NormalizeDirection(direction)

	s.broadcastToRoom(from.ID, fmt.Sprintf("%s leaves %s.", conn.Character.Name, direction))
	conn.Character.CurrentRoomID = &to.ID
	conn.Character.Stamina = stamina
	conn.Room = to
	s.hub.update(conn)
	if err := s.sendLook(conn); err != nil {
		return err
	}
	s.broadcastToRoom(to.ID, fmt.Sprintf("%s arrives.", conn.Character.Name))

	if err := s.eventBus.Publish(events.PlayerMoveEvent(conn.Character.ID, from.ID, to.ID, direction)); err != nil {
		s.logger.WithError(err).Warn("Failed to publish player movement event")
	}
	_, err = s.runScriptHook(conn, "room", to.ID, "on_enter", direction, "")
	return err
}

func (s *Server) sendLook(conn *Connection) error {
	npcs, err := s.world.ListRoomNPCs(conn.Context, conn.Room.ID)
	if err != nil {
		return err
	}
	ground, err := s.world.ListRoomItems(conn.Context, conn.Room.ID)
	if err != nil {
		return err
	}

	npcNames := make([]string, 0, len(npcs))
	for _, npc := range npcs {
		npcNames = append(npcNames, npc.Name)
	}
	itemNames := make([]string, 0, len(ground))
	for _, item := range ground {
		if item.Quantity > 1 {
			itemNames = append(itemNames, fmt.Sprintf("%s x%d", item.Name, item.Quantity))
		} else {
			itemNames = append(itemNames, item.Name)
		}
	}

	others := make([]string, 0)
	for _, other := range s.hub.snapshots() {
		if other.conn.ID != conn.ID && other.roomID == conn.Room.ID {
			others = append(others, other.player.Name)
		}
	}
	sort.Strings(others)

	return conn.SendRoomDescription(npcNames, itemNames, others)
}

func (s *Server) examineItem(conn *Connection, query string) error {
	inventory, err := s.world.ListInventory(conn.Context, conn.Character.ID)
	if err != nil {
		return err
	}
	for _, entry := range inventory {
		if game.MatchesName(entry.Item.Name, query) {
			return conn.SendMessage(entry.Item.Description)
		}
	}
	ground, err := s.world.ListRoomItems(conn.Context, conn.Room.ID)
	if err != nil {
		return err
	}
	for _, item := range ground {
		if game.MatchesName(item.Name, query) {
			return conn.SendMessage(item.Item.Description)
		}
	}
	conn.SendError(fmt.Sprintf("You do not see %q here.", query))
	return nil
}

func (s *Server) takeItem(conn *Connection, query string) error {
	if strings.EqualFold(strings.TrimSpace(query), "all") {
		items, err := s.world.TakeAll(conn.Context, conn.Character.ID, conn.Room.ID)
		if err != nil {
			return conn.SendError("You couldn't gather the items. Please try again.")
		}
		if len(items) == 0 {
			ground, err := s.world.ListRoomItems(conn.Context, conn.Room.ID)
			if err != nil {
				return err
			}
			if len(ground) == 0 {
				return conn.SendMessage("There is nothing here to take. Even your optimism won't fit in a pocket.")
			}
			return conn.SendMessage("Nothing you can take right now. Your pack may be full, or you already have the quest items.")
		}
		for _, item := range items {
			name := item.Name
			if item.Quantity > 1 {
				name = fmt.Sprintf("%s x%d", name, item.Quantity)
			}
			s.broadcastToRoom(conn.Room.ID, fmt.Sprintf("%s takes %s.", conn.Character.Name, name))
		}
		remaining, err := s.world.ListRoomItems(conn.Context, conn.Room.ID)
		if err != nil {
			return err
		}
		if len(remaining) > 0 {
			return conn.SendMessage("Some items remain: your pack is full or you already carry that quest item.")
		}
		return nil
	}
	item, err := s.world.TakeItem(conn.Context, conn.Character.ID, conn.Room.ID, query)
	if err != nil {
		if errors.Is(err, game.ErrNoMatch) {
			if message := sceneryTakeReply(conn.Room.Name, query); message != "" {
				return conn.SendMessage(message)
			}
			return conn.SendError(fmt.Sprintf("You do not see %q here.", query))
		}

		conn.SendError(err.Error())
		return nil
	}
	s.broadcastToRoom(conn.Room.ID, fmt.Sprintf("%s takes %s.", conn.Character.Name, item.Name))
	return nil
}

func (s *Server) dropItem(conn *Connection, query string) error {
	item, err := s.world.DropItem(conn.Context, conn.Character.ID, conn.Room.ID, query)
	if err != nil {
		conn.SendError(err.Error())
		return nil
	}
	s.broadcastToRoom(conn.Room.ID, fmt.Sprintf("%s drops %s.", conn.Character.Name, item.Name))
	return nil
}

func (s *Server) useItem(conn *Connection, query string) error {
	message, character, err := s.world.UseItem(conn.Context, conn.Character.ID, query)
	if err != nil {
		conn.SendError(err.Error())
		return nil
	}
	conn.Character = character
	return conn.SendMessage(message)
}

func (s *Server) giveItem(conn *Connection, args []string) error {
	words := make([]string, 0, len(args))
	for _, arg := range args {
		if !strings.EqualFold(arg, "to") {
			words = append(words, arg)
		}
	}
	npcs, err := s.world.ListRoomNPCs(conn.Context, conn.Room.ID)
	if err != nil {
		return err
	}
	itemQuery := strings.Join(words, " ")
	npcQuery := ""
	if len(words) >= 2 {
		last := words[len(words)-1]
		for _, npc := range npcs {
			if game.MatchesName(npc.Name, last) {
				npcQuery = last
				itemQuery = strings.Join(words[:len(words)-1], " ")
				break
			}
		}
	}
	message, character, err := s.world.GiveItem(conn.Context, conn.Character.ID, conn.Room.ID, itemQuery, npcQuery)
	if err != nil {
		conn.SendError(err.Error())
		return nil
	}
	conn.Character = character
	return conn.SendMessage(message)
}

func (s *Server) sendWhoList(requester *Connection) {
	players := []OnlinePlayer{}
	for _, p := range s.hub.snapshots() {
		players = append(players, p.player)
	}
	_ = requester.SendWhoList(players)
}
func (s *Server) broadcastToRoom(roomID uuid.UUID, message string) {
	for _, p := range s.hub.snapshots() {
		if p.roomID == roomID {
			if err := p.send(message); err != nil {
				p.conn.Close()
			}
		}
	}
}

// cleanupRoutine periodically cleans up inactive connections
func (s *Server) cleanupRoutine() {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cleanupInactiveConnections()
		}
	}
}

// idleTimeout is how long a connection may sit without input before it is
// dropped. Sessions are interactive: a player reading a room description or a
// Commodore user typing a pairing code on a phone must not be disconnected, so
// this is deliberately much longer than the per-request Server.ReadTimeout.
func (s *Server) idleTimeout() time.Duration {
	seconds := s.config.Server.IdleTimeout
	if seconds <= 0 {
		seconds = 900
	}
	return time.Duration(seconds) * time.Second
}

// cleanupInactiveConnections removes inactive connections
func (s *Server) cleanupInactiveConnections() {
	now := time.Now()
	timeout := s.idleTimeout()

	s.connMutex.Lock()
	defer s.connMutex.Unlock()

	for id, conn := range s.connections {
		conn.mutex.RLock()
		lastActivity := conn.LastActivity
		conn.mutex.RUnlock()
		if now.Sub(lastActivity) > timeout {
			s.logger.WithField("connection_id", id).Info("Cleaning up inactive connection")
			conn.Close()
			delete(s.connections, id)
		}
	}
}

// GetActiveConnections returns the number of active connections
func (s *Server) GetActiveConnections() int {
	s.connMutex.RLock()
	defer s.connMutex.RUnlock()
	return len(s.connections)
}

// BroadcastMessage broadcasts a message to all connected players
func (s *Server) BroadcastMessage(message string) {
	for _, p := range s.hub.snapshots() {
		if err := p.send(message); err != nil {
			p.conn.Close()
		}
	}
}

// handleTerminalCommand is available throughout login and play, so a client
// with a missing or incorrect terminal report can recover without reconnecting.
func (s *Server) handleTerminalCommand(conn *Connection, args []string) error {
	if len(args) != 1 || (!strings.EqualFold(args[0], "ansi") && !strings.EqualFold(args[0], "petscii")) {
		return conn.SendMessage("Usage: terminal ansi | petscii")
	}
	presentation := presentationForTerminalType(args[0])
	if presentation != conn.Presentation && !s.forcePETSCII {
		command := byte(252) // WONT ECHO: return ANSI input to local echo.
		if presentation == PresentationPETSCII {
			command = telnetWILL
		}
		if err := conn.write(string([]byte{telnetIAC, command, telnetECHO})); err != nil {
			return err
		}
	}
	conn.SetTerminalType(args[0])
	if err := conn.SendWelcome(); err != nil {
		return err
	}
	switch conn.State {
	case StateConnected, StateAwaitingUsername:
		return s.handleInitialConnection(conn)
	case StateAwaitingAuth:
		return conn.SendAuthInstructions(s.authService.GetPairingURL(conn.PairingCode), s.authService.GetPairingEntryURL(), conn.PairingCode)
	case StateAuthenticated:
		return conn.SendPrompt("Type 'check' for characters, or 'quit': ")
	case StateInGame:
		if err := s.sendLook(conn); err != nil {
			return err
		}
		return conn.SendPrompt(conn.formatPrompt())
	}
	return nil
}
