package server

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/requestid"
)

func New(cfg Config, logger *slog.Logger) (*fiber.App, error) {
	if _, err := os.Stat(filepath.Join(cfg.WebDistDir, "index.html")); err != nil {
		return nil, err
	}
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		BodyLimit:             cfg.MaxUploadMB * 1024 * 1024,
	})
	app.Use(requestid.New())
	app.Use(func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		logger.Info("request",
			"requestId", c.GetRespHeader(fiber.HeaderXRequestID),
			"method", c.Method(),
			"path", c.Path(),
			"status", c.Response().StatusCode(),
			"elapsedMs", time.Since(start).Milliseconds(),
		)
		return err
	})
	app.Get("/api/health", func(c *fiber.Ctx) error {
		ffmpegVersion, ffmpegErr := binaryVersion("ffmpeg")
		ffprobeVersion, ffprobeErr := binaryVersion("ffprobe")
		if ffmpegErr != nil || ffprobeErr != nil {
			logger.Error("media tools unavailable", "ffmpegError", ffmpegErr, "ffprobeError", ffprobeErr)
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"status":  "unhealthy",
				"ffmpeg":  fiber.Map{"available": ffmpegErr == nil, "version": ffmpegVersion},
				"ffprobe": fiber.Map{"available": ffprobeErr == nil, "version": ffprobeVersion},
			})
		}
		return c.JSON(fiber.Map{
			"status":  "ok",
			"ffmpeg":  fiber.Map{"available": true, "version": ffmpegVersion},
			"ffprobe": fiber.Map{"available": true, "version": ffprobeVersion},
		})
	})
	app.Static("/assets", filepath.Join(cfg.WebDistDir, "assets"))
	app.Get("/*", func(c *fiber.Ctx) error {
		if strings.HasPrefix(c.Path(), "/api/") || filepath.Ext(c.Path()) != "" {
			return fiber.ErrNotFound
		}
		return c.SendFile(filepath.Join(cfg.WebDistDir, "index.html"))
	})
	return app, nil
}

func binaryVersion(name string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, name, "-version").Output()
	if err != nil {
		return "", err
	}
	line, _, _ := strings.Cut(string(output), "\n")
	return strings.TrimSpace(line), nil
}
