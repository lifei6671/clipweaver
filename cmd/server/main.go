package main

import (
	"log/slog"
	"os"

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
	app, err := server.New(cfg, logger)
	if err != nil {
		logger.Error("initialize server", "error", err)
		os.Exit(1)
	}
	logger.Info("listening", "address", cfg.ListenAddr)
	if err := app.Listen(cfg.ListenAddr); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
