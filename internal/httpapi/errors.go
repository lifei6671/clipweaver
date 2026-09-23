package httpapi

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/lifei6671/clipweaver/internal/media"
	"github.com/lifei6671/clipweaver/internal/service"
)

var errInvalidRequest = errors.New("invalid multipart request")

type apiError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

func mapError(err error) (int, apiError) {
	result := apiError{Details: map[string]any{}}
	switch {
	case errors.Is(err, fiber.ErrRequestEntityTooLarge):
		result.Code, result.Message = "UPLOAD_TOO_LARGE", "上传内容超过大小限制"
		return fiber.StatusRequestEntityTooLarge, result
	case errors.Is(err, errInvalidRequest):
		result.Code, result.Message = "INVALID_REQUEST", "上传请求格式无效"
		return fiber.StatusBadRequest, result
	case errors.Is(err, service.ErrCleanup):
		result.Code, result.Message = "INTERNAL_ERROR", "服务器处理失败"
		return fiber.StatusInternalServerError, result
	case errors.Is(err, service.ErrInvalidVideo):
		result.Code, result.Message = "INVALID_VIDEO", "未检测到有效视频流"
		return fiber.StatusBadRequest, result
	case errors.Is(err, service.ErrInvalidAudio):
		result.Code, result.Message = "INVALID_AUDIO", "未检测到有效音频流"
		return fiber.StatusBadRequest, result
	case errors.Is(err, media.ErrProbeFailed), errors.Is(err, media.ErrProbeTimeout), errors.Is(err, media.ErrProbeCanceled), errors.Is(err, media.ErrInvalidJSON):
		result.Code, result.Message = "FFPROBE_FAILED", "媒体探测失败"
		return fiber.StatusInternalServerError, result
	case errors.Is(err, fiber.ErrNotFound):
		result.Code, result.Message = "NOT_FOUND", "接口不存在"
		return fiber.StatusNotFound, result
	default:
		result.Code, result.Message = "INTERNAL_ERROR", "服务器处理失败"
		return fiber.StatusInternalServerError, result
	}
}

func writeError(c *fiber.Ctx, err error, logger *slog.Logger) error {
	status, public := mapError(err)
	if status >= 500 {
		logger.Error("asset API error", "error", err)
	}
	return c.Status(status).JSON(fiber.Map{"error": public})
}

func ErrorHandler(logger *slog.Logger) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error { return writeError(c, err, logger) }
}
