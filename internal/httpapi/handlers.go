package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/lifei6671/clipweaver/internal/domain"
	"github.com/lifei6671/clipweaver/internal/service"
)

type assetService interface {
	UploadVideo(context.Context, string, io.Reader) (domain.Asset, error)
	UploadAudio(context.Context, string, io.Reader) (domain.Asset, error)
	ListAssets() ([]domain.Asset, error)
	DeleteVideo(string) (string, error)
	OpenPoster(string) (*os.File, error)
}

type handlers struct {
	assets assetService
	logger *slog.Logger
}

type assetResponse struct {
	ID         string            `json:"id"`
	Kind       domain.AssetKind  `json:"kind"`
	Name       string            `json:"name"`
	DurationUS domain.DurationUS `json:"durationUs"`
	Width      int               `json:"width"`
	Height     int               `json:"height"`
	PosterURL  string            `json:"posterUrl,omitempty"`
}

func publicAsset(asset domain.Asset) assetResponse {
	response := assetResponse{
		ID: asset.ID, Kind: asset.Kind, Name: asset.Name,
		DurationUS: asset.DurationUS, Width: asset.Width, Height: asset.Height,
	}
	if asset.Kind == domain.AssetKindVideo && asset.HasPoster {
		response.PosterURL = "/api/assets/" + asset.ID + "/poster"
	}
	return response
}

func (h handlers) poster(c *fiber.Ctx) error {
	file, err := h.assets.OpenPoster(c.Params("id"))
	if errors.Is(err, os.ErrNotExist) {
		return writeError(c, service.ErrAssetNotFound, h.logger)
	}
	if err != nil {
		return writeError(c, err, h.logger)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return writeError(c, err, h.logger)
	}
	c.Set(fiber.HeaderContentType, "image/jpeg")
	return c.SendStream(file, int(info.Size()))
}

type uploadItem struct {
	Filename string         `json:"filename"`
	Status   string         `json:"status"`
	Asset    *assetResponse `json:"asset,omitempty"`
	Error    *apiError      `json:"error,omitempty"`
}

func (h handlers) videos(c *fiber.Ctx) error {
	form, err := c.MultipartForm()
	if err != nil {
		return writeError(c, errInvalidRequest, h.logger)
	}
	defer form.RemoveAll()
	files := form.File["files"]
	if len(files) == 0 || len(form.File) != 1 {
		return writeError(c, errInvalidRequest, h.logger)
	}
	items := make([]uploadItem, 0, len(files))
	for _, header := range files {
		item := uploadItem{Filename: header.Filename}
		file, openErr := header.Open()
		var asset domain.Asset
		if openErr == nil {
			asset, err = h.assets.UploadVideo(c.UserContext(), header.Filename, file)
			closeErr := file.Close()
			if closeErr != nil {
				h.logger.Error("close multipart video", "error", closeErr)
			}
		} else {
			err = openErr
		}
		if err != nil {
			_, public := mapError(err)
			item.Status, item.Error = "failed", &public
			if public.Code == "INTERNAL_ERROR" || public.Code == "FFPROBE_FAILED" {
				h.logger.Error("video upload failed", "error", err)
			}
		} else {
			result := publicAsset(asset)
			item.Status, item.Asset = "ready", &result
		}
		items = append(items, item)
	}
	return c.JSON(fiber.Map{"items": items})
}

func (h handlers) audio(c *fiber.Ctx) error {
	form, err := c.MultipartForm()
	if err != nil {
		return writeError(c, errInvalidRequest, h.logger)
	}
	defer form.RemoveAll()
	files := form.File["file"]
	if len(files) != 1 || len(form.File) != 1 {
		return writeError(c, errInvalidRequest, h.logger)
	}
	asset, err := h.uploadAudio(c, files[0])
	if err != nil {
		return writeError(c, err, h.logger)
	}
	result := publicAsset(asset)
	return c.JSON(uploadItem{Filename: files[0].Filename, Status: "ready", Asset: &result})
}

func (h handlers) uploadAudio(c *fiber.Ctx, header *multipart.FileHeader) (domain.Asset, error) {
	file, err := header.Open()
	if err != nil {
		return domain.Asset{}, err
	}
	asset, uploadErr := h.assets.UploadAudio(c.UserContext(), header.Filename, file)
	closeErr := file.Close()
	if closeErr != nil {
		h.logger.Error("close multipart audio", "error", closeErr)
	}
	if uploadErr != nil {
		return domain.Asset{}, uploadErr
	}
	return asset, nil
}

func (h handlers) list(c *fiber.Ctx) error {
	assets, err := h.assets.ListAssets()
	if err != nil {
		return writeError(c, err, h.logger)
	}
	items := make([]assetResponse, 0, len(assets))
	for _, asset := range assets {
		items = append(items, publicAsset(asset))
	}
	return c.JSON(fiber.Map{"items": items})
}

func (h handlers) deleteVideo(c *fiber.Ctx) error {
	id, err := h.assets.DeleteVideo(c.Params("id"))
	if err != nil {
		return writeError(c, err, h.logger)
	}
	return c.JSON(fiber.Map{"id": id, "deleted": true})
}
