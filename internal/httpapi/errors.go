package httpapi

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/lifei6671/clipweaver/internal/domain"
	"github.com/lifei6671/clipweaver/internal/media"
	"github.com/lifei6671/clipweaver/internal/service"
	"github.com/lifei6671/clipweaver/internal/storage"
)

var errInvalidRequest = errors.New("invalid multipart request")
var errInvalidSeed = errors.New("INVALID_SEED")

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
	case errors.Is(err, errInvalidRequest), errors.Is(err, service.ErrInvalidMixRequest):
		result.Code, result.Message = "INVALID_REQUEST", "请求格式无效"
		return fiber.StatusBadRequest, result
	case errors.Is(err, errInvalidSeed):
		result.Code, result.Message = "INVALID_SEED", "seed 必须是 int64 十进制字符串"
		return fiber.StatusBadRequest, result
	case errors.Is(err, storage.ErrCorruptManifest):
		result.Code, result.Message = "INTERNAL_ERROR", "服务器处理失败"
		return fiber.StatusInternalServerError, result
	case errors.Is(err, storage.ErrInvalidID):
		result.Code, result.Message = "INVALID_ID", "ID 格式无效"
		return fiber.StatusBadRequest, result
	case errors.Is(err, service.ErrNoVideoSelected):
		result.Code, result.Message = "NO_VIDEO_SELECTED", "请选择至少一个视频"
		return fiber.StatusBadRequest, result
	case errors.Is(err, service.ErrAudioRequired):
		result.Code, result.Message = "AUDIO_REQUIRED", "请选择口播音频"
		return fiber.StatusBadRequest, result
	case errors.Is(err, service.ErrAssetNotFound):
		result.Code, result.Message = "ASSET_NOT_FOUND", "素材不存在"
		return fiber.StatusNotFound, result
	case errors.Is(err, service.ErrMixNotFound):
		result.Code, result.Message = "MIX_NOT_FOUND", "成片不存在"
		return fiber.StatusNotFound, result
	case errors.Is(err, domain.ErrInsufficientVideoDuration):
		result.Code, result.Message = "INSUFFICIENT_VIDEO_DURATION", "所选视频可用总时长不足"
		var shortage *service.InsufficientVideoError
		if errors.As(err, &shortage) {
			result.Details["missingDurationUs"] = shortage.MissingDurationUS
		}
		return fiber.StatusUnprocessableEntity, result
	case errors.Is(err, service.ErrMixBusy):
		result.Code, result.Message = "MIX_BUSY", "混剪任务繁忙"
		return fiber.StatusTooManyRequests, result
	case errors.Is(err, service.ErrMixTimeout):
		result.Code, result.Message = "MIX_TIMEOUT", "混剪处理超时"
		return fiber.StatusGatewayTimeout, result
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
	case errors.Is(err, media.ErrFFmpegFailed):
		result.Code, result.Message = "FFMPEG_FAILED", "视频渲染失败"
		return fiber.StatusInternalServerError, result
	case errors.Is(err, media.ErrRenderValidationFailed):
		result.Code, result.Message = "RENDER_VALIDATION_FAILED", "成片验收失败"
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
		logger.Error("API error", "error", err)
	}
	return c.Status(status).JSON(fiber.Map{"error": public})
}

func ErrorHandler(logger *slog.Logger) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error { return writeError(c, err, logger) }
}
