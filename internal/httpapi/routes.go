package httpapi

import (
	"log/slog"

	"github.com/gofiber/fiber/v2"
)

func Register(app *fiber.App, assets assetService, logger *slog.Logger) {
	h := handlers{assets: assets, logger: logger}
	app.Post("/api/assets/videos", h.videos)
	app.Post("/api/assets/audio", h.audio)
	app.Get("/api/assets", h.list)
}

func RegisterMixes(app *fiber.App, mixes mixService, files mixFiles, logger *slog.Logger) {
	h := mixHandlers{service: mixes, files: files, logger: logger}
	app.Post("/api/mixes", h.create)
	app.Get("/api/mixes/:id/file", h.preview)
	app.Get("/api/mixes/:id/download", h.download)
}
