package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lifei6671/clipweaver/internal/server"
)

func main() {
	cfg, err := server.LoadConfig()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		logger.Error("create data directory", "error", err)
		os.Exit(1)
	}
	root, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app, err := server.New(cfg, logger, root)
	if err != nil {
		logger.Error("initialize server", "error", err)
		os.Exit(1)
	}
	logger.Info("listening", "address", cfg.ListenAddr)
	listenResult := make(chan error, 1)
	go func() { listenResult <- app.Listen(cfg.ListenAddr) }()
	select {
	case err := <-listenResult:
		if err != nil {
			logger.Error("server stopped", "error", err)
			os.Exit(1)
		}
	case <-root.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := app.ShutdownWithContext(shutdown); err != nil {
			logger.Error("server shutdown failed", "error", err)
			os.Exit(1)
		}
	}
}
