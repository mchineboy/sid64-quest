package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tylerhardison/race-condition-kingdom/internal/database"
	"github.com/tylerhardison/race-condition-kingdom/internal/game"
	"github.com/tylerhardison/race-condition-kingdom/internal/telnet"
	"github.com/tylerhardison/race-condition-kingdom/internal/terminalwire"
	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
)

func main() {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	token := os.Getenv("CORE_TOKEN")
	if len(token) < 32 {
		logger.Fatal("CORE_TOKEN must contain at least 32 bytes")
	}
	cfg := config.LoadFromEnv()
	db, err := database.New(cfg, logger)
	if err != nil {
		logger.WithError(err).Fatal("Core database initialization failed")
	}
	defer db.Close()
	if err = game.NewWorldService(db.PostgreSQL).EnsureStarterWorld(context.Background()); err != nil {
		logger.WithError(err).Fatal("Core world initialization failed")
	}
	address := os.Getenv("CORE_LISTEN")
	if address == "" {
		address = "unix:/tmp/rck-core.sock"
	}
	listener, err := terminalwire.Listen(address)
	if err != nil {
		logger.WithError(err).Fatal("Core listener failed")
	}
	defer listener.Close()
	core := telnet.NewCore(db.PostgreSQL, db.Redis, cfg, logger, token)
	server := &http.Server{Handler: core, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-ctx.Done()
		ctx, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		_ = server.Shutdown(ctx)
	}()
	logger.WithFields(logrus.Fields{"core_id": core.ID, "address": address}).Info("Game core ready")
	if err = server.Serve(listener); err != nil && err != http.ErrServerClosed {
		logger.WithError(err).Fatal("Core server failed")
	}
	<-stopped
}
