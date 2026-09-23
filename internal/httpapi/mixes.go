package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/lifei6671/clipweaver/internal/domain"
	"github.com/lifei6671/clipweaver/internal/service"
	"github.com/lifei6671/clipweaver/internal/storage"
)

type mixService interface {
	Create(domain.MixRequest) (domain.MixMeta, error)
}
type mixFiles interface {
	OpenCompletedMixFile(id string) (*os.File, error)
}

type mixHandlers struct {
	service mixService
	files   mixFiles
	logger  *slog.Logger
}

type mixRequestJSON struct {
	VideoIDs []string        `json:"videoIds"`
	AudioID  string          `json:"audioId"`
	Seed     json.RawMessage `json:"seed"`
}

func (h mixHandlers) create(c *fiber.Ctx) error {
	var input mixRequestJSON
	if bytes.Equal(bytes.TrimSpace(c.Body()), []byte("null")) {
		return writeError(c, errInvalidRequest, h.logger)
	}
	if err := json.Unmarshal(c.Body(), &input); err != nil {
		return writeError(c, errInvalidRequest, h.logger)
	}
	var seed *int64
	if input.Seed != nil {
		var raw string
		if err := json.Unmarshal(input.Seed, &raw); err != nil {
			return writeError(c, errInvalidSeed, h.logger)
		}
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || raw == "" {
			return writeError(c, errInvalidSeed, h.logger)
		}
		seed = &parsed
	}
	meta, err := h.service.Create(domain.MixRequest{VideoIDs: input.VideoIDs, AudioID: input.AudioID, Seed: seed})
	if err != nil {
		return writeError(c, err, h.logger)
	}
	base := "/api/mixes/" + meta.ID
	return c.JSON(fiber.Map{
		"id": meta.ID, "status": meta.Status, "seed": strconv.FormatInt(meta.Seed, 10),
		"durationUs": meta.DurationUS, "previewUrl": base + "/file", "downloadUrl": base + "/download",
	})
}

type closingReader struct {
	io.Reader
	io.Closer
}

func (h mixHandlers) open(c *fiber.Ctx) (*os.File, error) {
	id := c.Params("id")
	if _, err := uuid.Parse(id); err != nil {
		return nil, storage.ErrInvalidID
	}
	file, err := h.files.OpenCompletedMixFile(id)
	if errors.Is(err, os.ErrNotExist) {
		return nil, service.ErrMixNotFound
	}
	return file, err
}

func (h mixHandlers) preview(c *fiber.Ctx) error {
	file, err := h.open(c)
	if err != nil {
		return writeError(c, err, h.logger)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return writeError(c, err, h.logger)
	}
	size := info.Size()
	if size == 0 {
		file.Close()
		return writeError(c, service.ErrMixNotFound, h.logger)
	}
	c.Set(fiber.HeaderContentType, "video/mp4")
	c.Set("Accept-Ranges", "bytes")
	raw := c.Get("Range")
	if raw == "" {
		return c.SendStream(file, int(size))
	}
	start, end, ok := parseSingleRange(raw, size)
	if !ok {
		file.Close()
		c.Set("Content-Range", "bytes */"+strconv.FormatInt(size, 10))
		return c.SendStatus(fiber.StatusRequestedRangeNotSatisfiable)
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		file.Close()
		return writeError(c, err, h.logger)
	}
	length := end - start + 1
	c.Status(fiber.StatusPartialContent)
	c.Set("Content-Range", "bytes "+strconv.FormatInt(start, 10)+"-"+strconv.FormatInt(end, 10)+"/"+strconv.FormatInt(size, 10))
	return c.SendStream(closingReader{Reader: io.LimitReader(file, length), Closer: file}, int(length))
}

func parseSingleRange(raw string, size int64) (int64, int64, bool) {
	if size <= 0 || !strings.HasPrefix(raw, "bytes=") || strings.Contains(raw, ",") {
		return 0, 0, false
	}
	parts := strings.Split(strings.TrimPrefix(raw, "bytes="), "-")
	if len(parts) != 2 || (parts[0] == "" && parts[1] == "") {
		return 0, 0, false
	}
	if parts[0] == "" {
		if !rangeDigits(parts[1]) {
			return 0, 0, false
		}
		suffix, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || suffix <= 0 {
			return 0, 0, false
		}
		if suffix >= size {
			return 0, size - 1, true
		}
		return size - suffix, size - 1, true
	}
	if !rangeDigits(parts[0]) {
		return 0, 0, false
	}
	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false
	}
	if parts[1] == "" {
		return start, size - 1, true
	}
	if !rangeDigits(parts[1]) {
		return 0, 0, false
	}
	end, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || end < start {
		return 0, 0, false
	}
	if end >= size {
		end = size - 1
	}
	return start, end, true
}

func rangeDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func (h mixHandlers) download(c *fiber.Ctx) error {
	file, err := h.open(c)
	if err != nil {
		return writeError(c, err, h.logger)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return writeError(c, err, h.logger)
	}
	if info.Size() == 0 {
		file.Close()
		return writeError(c, service.ErrMixNotFound, h.logger)
	}
	c.Set(fiber.HeaderContentType, "video/mp4")
	c.Set(fiber.HeaderContentDisposition, "attachment; filename=\"clipweaver-"+c.Params("id")+".mp4\"")
	return c.SendStream(file, int(info.Size()))
}
