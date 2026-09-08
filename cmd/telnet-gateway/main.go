package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tylerhardison/race-condition-kingdom/internal/auth"
	"github.com/tylerhardison/race-condition-kingdom/internal/database"
	"github.com/tylerhardison/race-condition-kingdom/internal/events"
	"github.com/tylerhardison/race-condition-kingdom/internal/game"
	"github.com/tylerhardison/race-condition-kingdom/internal/telnet"
	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
)

func main() {
	// Initialize logger
	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	logger.SetFormatter(&logrus.JSONFormatter{})

	logger.Info("Starting SID64 Quest Telnet Gateway")

	// Load configuration
	cfg := config.LoadFromEnv()
	logger.WithFields(logrus.Fields{
		"telnet_port":  cfg.Server.TelnetPort,
		"petscii_port": cfg.Server.PETSCIIPort,
		"http_port":    cfg.Server.HTTPPort,
		"host":         cfg.Server.Host,
	}).Info("Configuration loaded")

	// Initialize database connections
	db, err := database.New(cfg, logger)
	if err != nil {
		logger.WithError(err).Fatal("Failed to initialize database")
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.WithError(err).Error("Failed to close database connections")
		}
	}()

	// Test database health
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	health := db.Health(ctx)
	for service, err := range health {
		if err != nil {
			logger.WithError(err).WithField("service", service).Fatal("Database health check failed")
		} else {
			logger.WithField("service", service).Info("Database connection healthy")
		}
	}

	// Initialize event bus
	eventBus := events.NewEventBus(db.GetRedisClient(), logger)
	defer eventBus.Close()

	// Initialize authentication service
	authService := auth.NewAuthService(db.GetPostgreSQLDB(), db.GetRedisClient(), cfg, logger)
	worldService := game.NewWorldService(db.GetPostgreSQLDB())
	if err := worldService.EnsureStarterWorld(context.Background()); err != nil {
		logger.WithError(err).Fatal("Failed to initialize starter world")
	}

	// Initialize telnet server
	telnetServer := telnet.NewServer(cfg, authService, eventBus, worldService, logger)
	petsciiServer := telnet.NewPETSCIIServer(cfg, authService, eventBus, worldService, logger)

	petsciiServer.SharePlayers(telnetServer)

	// Start background services
	var wg sync.WaitGroup

	// Start event bus subscriber for global events
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := eventBus.SubscribeToGlobal(handleGlobalEvent(logger)); err != nil {
			logger.WithError(err).Error("Event bus subscription failed")
		}
	}()

	// Start periodic cleanup routines
	shutdownCtx, stopBackground := context.WithCancel(context.Background())
	defer stopBackground()

	wg.Add(1)
	go func() {
		defer wg.Done()
		runCleanupRoutines(shutdownCtx, authService, logger)
	}()

	// Start telnet server
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := telnetServer.Start(); err != nil {
			logger.WithError(err).Error("Telnet server failed")
		}
	}()

	// Start the dedicated PETSCII listener for raw Commodore terminal clients.
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := petsciiServer.Start(); err != nil {
			logger.WithError(err).Error("PETSCII telnet server failed")
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	logger.Info("Telnet Gateway is running. Press Ctrl+C to stop.")

	// Block until signal received
	sig := <-sigChan
	logger.WithField("signal", sig).Info("Shutdown signal received")

	// Graceful shutdown
	logger.Info("Shutting down services...")

	// Stop telnet server
	if err := telnetServer.Stop(); err != nil {
		logger.WithError(err).Error("Error stopping telnet server")
	}
	if err := petsciiServer.Stop(); err != nil {
		logger.WithError(err).Error("Error stopping PETSCII telnet server")
	}

	// Release the background workers before waiting on them: the cleanup loop
	// watches this context and the Redis subscriber only returns once the bus
	// is closed.
	stopBackground()
	eventBus.Close()

	// Wait for background goroutines to finish
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// Wait for graceful shutdown or timeout
	select {
	case <-done:
		logger.Info("All services stopped gracefully")
	case <-time.After(30 * time.Second):
		logger.Warn("Shutdown timeout reached, forcing exit")
	}

	logger.Info("Telnet Gateway stopped")
}

// handleGlobalEvent returns an event handler for global events
func handleGlobalEvent(logger *logrus.Logger) func(*events.Event) {
	return func(event *events.Event) {
		logger.WithFields(logrus.Fields{
			"event_id":   event.ID,
			"event_type": event.Type,
			"player_id":  event.PlayerID,
			"room_id":    event.RoomID,
			"timestamp":  event.Timestamp,
		}).Debug("Received global event")

		// Handle specific event types
		switch event.Type {
		case events.EventPlayerConnect:
			if characterName, ok := event.Data["character_name"].(string); ok {
				logger.WithFields(logrus.Fields{
					"player_id":      event.PlayerID,
					"character_name": characterName,
				}).Info("Player connected")
			}

		case events.EventPlayerDisconnect:
			if reason, ok := event.Data["reason"].(string); ok {
				logger.WithFields(logrus.Fields{
					"player_id": event.PlayerID,
					"reason":    reason,
				}).Info("Player disconnected")
			}

		case events.EventPlayerMove:
			if fromRoom, ok := event.Data["from_room"].(string); ok {
				if toRoom, ok := event.Data["to_room"].(string); ok {
					if direction, ok := event.Data["direction"].(string); ok {
						logger.WithFields(logrus.Fields{
							"player_id": event.PlayerID,
							"from_room": fromRoom,
							"to_room":   toRoom,
							"direction": direction,
						}).Debug("Player moved")
					}
				}
			}

		case events.EventAdminAction:
			if action, ok := event.Data["action"].(string); ok {
				logger.WithFields(logrus.Fields{
					"admin_id": event.PlayerID,
					"action":   action,
					"target":   event.Data["target"],
				}).Warn("Admin action performed")
			}

		case events.EventWorldEdit:
			if editType, ok := event.Data["edit_type"].(string); ok {
				if objectID, ok := event.Data["object_id"].(string); ok {
					logger.WithFields(logrus.Fields{
						"builder_id": event.PlayerID,
						"edit_type":  editType,
						"object_id":  objectID,
					}).Info("World edit performed")
				}
			}
		}
	}
}

// runCleanupRoutines runs periodic cleanup tasks
func runCleanupRoutines(ctx context.Context, authService *auth.AuthService, logger *logrus.Logger) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			logger.Debug("Running cleanup routines")

			// Clean up expired tokens
			if err := authService.CleanupExpiredTokens(); err != nil {
				logger.WithError(err).Error("Failed to cleanup expired tokens")
			}

			// Clean up expired sessions
			if err := authService.CleanupExpiredSessions(); err != nil {
				logger.WithError(err).Error("Failed to cleanup expired sessions")
			}

			logger.Debug("Cleanup routines completed")
		}
	}
}
