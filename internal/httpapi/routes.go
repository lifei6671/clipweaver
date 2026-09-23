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
