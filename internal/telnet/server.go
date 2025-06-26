package telnet

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/tylerhardison/race-condition-kingdom/internal/ansi"
	"github.com/tylerhardison/race-condition-kingdom/internal/auth"
	"github.com/tylerhardison/race-condition-kingdom/internal/events"
	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
	"github.com/tylerhardison/race-condition-kingdom/pkg/models"
)

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
	ID          string
	Conn        net.Conn
	State       ConnectionState
	Username    string
	User        *models.User
	Character   *models.Character
	Session     *models.Session
	AuthToken   string
	LastActivity time.Time
	Reader      *bufio.Reader
	Writer      *bufio.Writer
	Formatter   *ansi.Formatter
	Context     context.Context
	Cancel      context.CancelFunc
	mutex       sync.RWMutex
}

// Server represents the telnet server
type Server struct {
	config      *config.Config
	authService *auth.AuthService
	eventBus    *events.EventBus
	logger      *logrus.Logger
	listener    net.Listener
	connections map[string]*Connection
	connMutex   sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

// NewServer creates a new telnet server
func NewServer(cfg *config.Config, authService *auth.AuthService, eventBus *events.EventBus, logger *logrus.Logger) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Server{
		config:      cfg,
		authService: authService,
		eventBus:    eventBus,
		logger:      logger,
		connections: make(map[string]*Connection),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start starts the telnet server
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.TelnetPort)
	
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
	
	conn := &Connection{
		ID:           connID,
		Conn:         netConn,
		State:        StateConnected,
		LastActivity: time.Now(),
		Reader:       bufio.NewReader(netConn),
		Writer:       bufio.NewWriter(netConn),
		Formatter:    ansi.NewFormatter(true), // Enable ANSI by default
		Context:      ctx,
		Cancel:       cancel,
	}
	
	// Set connection timeouts
	netConn.SetReadDeadline(time.Now().Add(time.Duration(s.config.Server.ReadTimeout) * time.Second))
	netConn.SetWriteDeadline(time.Now().Add(time.Duration(s.config.Server.WriteTimeout) * time.Second))
	
	// Add to connections map
	s.connMutex.Lock()
	s.connections[connID] = conn
	s.connMutex.Unlock()
	
	// Remove from connections map when done
	defer func() {
		s.connMutex.Lock()
		delete(s.connections, connID)
		s.connMutex.Unlock()
		conn.Close()
	}()
	
	s.logger.WithFields(logrus.Fields{
		"connection_id": connID,
		"remote_addr":   netConn.RemoteAddr().String(),
	}).Info("New telnet connection")
	
	// Send welcome message
	conn.SendWelcome()
	
	// Handle the connection
	s.connectionLoop(conn)
}

// connectionLoop handles the main connection loop
func (s *Server) connectionLoop(conn *Connection) {
	for {
		select {
		case <-conn.Context.Done():
			return
		default:
			// Read input from client
			input, err := conn.ReadLine()
			if err != nil {
				if err.Error() != "EOF" {
					s.logger.WithError(err).Debug("Connection read error")
				}
				return
			}
			
			conn.UpdateActivity()
			
			// Process input based on connection state
			if err := s.processInput(conn, input); err != nil {
				s.logger.WithError(err).Error("Error processing input")
				conn.SendError("An error occurred processing your input.")
			}
		}
	}
}

// processInput processes input based on the connection state
func (s *Server) processInput(conn *Connection, input string) error {
	input = strings.TrimSpace(input)
	
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
	conn.SendPrompt("Enter your username: ")
	return nil
}

// handleUsernameInput handles username input
func (s *Server) handleUsernameInput(conn *Connection, username string) error {
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
	
	// Generate authentication token
	token, err := s.authService.GenerateAuthToken(conn.ID)
	if err != nil {
		return fmt.Errorf("failed to generate auth token: %w", err)
	}
	
	conn.AuthToken = token
	conn.State = StateAwaitingAuth
	
	// Send authentication URL
	authURL := s.authService.GetAuthURL(token)
	conn.SendAuthInstructions(authURL)
	
	return nil
}

// handleAuthWaiting handles input while waiting for authentication
func (s *Server) handleAuthWaiting(conn *Connection, input string) error {
	input = strings.ToLower(input)
	
	switch input {
	case "help", "h":
		authURL := s.authService.GetAuthURL(conn.AuthToken)
		conn.SendAuthInstructions(authURL)
	case "quit", "q", "exit":
		conn.SendMessage("Goodbye!")
		return fmt.Errorf("user quit")
	case "check", "c":
		// Check if authentication is complete
		return s.checkAuthentication(conn)
	default:
		conn.SendMessage("Waiting for authentication... Type 'help' for instructions, 'check' to verify, or 'quit' to exit.")
	}
	
	return nil
}

// checkAuthentication checks if the user has completed authentication
func (s *Server) checkAuthentication(conn *Connection) error {
	// Try to validate the token (this will fail if not used yet)
	sessionID, err := s.authService.ValidateAuthToken(conn.AuthToken)
	if err != nil {
		conn.SendMessage("Authentication not yet complete. Please visit the authentication URL.")
		return nil
	}
	
	// Get the session to see if it's been linked
	session, err := s.authService.GetSession(sessionID)
	if err != nil {
		conn.SendMessage("Authentication not yet complete. Please visit the authentication URL.")
		return nil
	}
	
	// Check if session has been authenticated
	if session.UserID == uuid.Nil {
		conn.SendMessage("Authentication not yet complete. Please visit the authentication URL.")
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
		conn.SendMessage("You don't have any characters yet. Character creation coming soon!")
		return fmt.Errorf("no characters available")
	}
	
	conn.SendCharacterList(characters)
	return nil
}

// handleCharacterSelection handles character selection
func (s *Server) handleCharacterSelection(conn *Connection, input string) error {
	// For now, just select the first character
	// TODO: Implement proper character selection
	characters, err := s.authService.GetUserCharacters(conn.Session.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user characters: %w", err)
	}
	
	if len(characters) == 0 {
		conn.SendError("No characters available.")
		return fmt.Errorf("no characters available")
	}
	
	conn.Character = characters[0]
	conn.State = StateInGame
	
	// Publish player connect event
	event := events.PlayerConnectEvent(conn.Character.ID, conn.Character.Name)
	if err := s.eventBus.Publish(event); err != nil {
		s.logger.WithError(err).Warn("Failed to publish player connect event")
	}
	
	conn.SendGameWelcome()
	return nil
}

// handleGameCommand handles in-game commands
func (s *Server) handleGameCommand(conn *Connection, input string) error {
	if input == "" {
		return nil
	}
	
	// Parse command
	parts := strings.Fields(input)
	command := strings.ToLower(parts[0])
	args := parts[1:]
	
	// Handle basic commands
	switch command {
	case "quit", "q":
		conn.SendMessage("Goodbye!")
		return fmt.Errorf("user quit")
	case "help", "h":
		conn.SendHelp()
	case "who", "w":
		conn.SendWhoList()
	case "say":
		if len(args) > 0 {
			message := strings.Join(args, " ")
			conn.SendMessage(fmt.Sprintf("You say: %s", message))
			// TODO: Broadcast to room
		} else {
			conn.SendError("Say what?")
		}
	case "look", "l":
		conn.SendRoomDescription()
	case "inventory", "inv", "i":
		conn.SendInventory()
	case "stats", "st":
		conn.SendStats()
	default:
		conn.SendError(fmt.Sprintf("Unknown command: %s", command))
	}
	
	return nil
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

// cleanupInactiveConnections removes inactive connections
func (s *Server) cleanupInactiveConnections() {
	now := time.Now()
	timeout := 10 * time.Minute
	
	s.connMutex.Lock()
	defer s.connMutex.Unlock()
	
	for id, conn := range s.connections {
		if now.Sub(conn.LastActivity) > timeout {
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
	s.connMutex.RLock()
	defer s.connMutex.RUnlock()
	
	for _, conn := range s.connections {
		if conn.State == StateInGame {
			conn.SendMessage(message)
		}
	}
}