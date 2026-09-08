package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/tylerhardison/race-condition-kingdom/internal/telnet"
	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
)

func main() {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	target := os.Getenv("CORE_TARGET")
	if target == "" {
		target = "unix:/tmp/rck-core.sock"
	}
	edge := &telnet.Edge{Config: config.LoadFromEnv(), Logger: logger, Target: target, TargetFile: os.Getenv("CORE_TARGET_FILE"), Token: os.Getenv("CORE_TOKEN")}
	if err := edge.Run(ctx); err != nil {
		logger.WithError(err).Fatal("Terminal edge stopped")
	}
}
