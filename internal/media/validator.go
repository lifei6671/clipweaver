package media

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"strings"

	"github.com/lifei6671/clipweaver/internal/domain"
)

var ErrRenderValidationFailed = errors.New("RENDER_VALIDATION_FAILED")

const renderToleranceUS domain.DurationUS = 100_000

type OutputInfo struct {
	VideoDurationUS  domain.DurationUS
	AudioDurationUS  domain.DurationUS
	FormatDurationUS domain.DurationUS
}

type ValidationError struct{ Reason string }

func (e *ValidationError) Error() string { return ErrRenderValidationFailed.Error() }
func (e *ValidationError) Unwrap() error { return ErrRenderValidationFailed }

func invalidRender(reason string) error { return &ValidationError{Reason: reason} }

type Validator struct {
	Binary string
	run    func(context.Context, string, ...string) ([]byte, error)
}

func NewValidator(binary string) Validator {
	if binary == "" {
		binary = "ffprobe"
	}
	return Validator{Binary: binary, run: runFFprobe}
}

func (v Validator) Validate(ctx context.Context, path string, target domain.DurationUS) (OutputInfo, error) {
	if target <= 0 {
		return OutputInfo{}, invalidRender("invalid target duration")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		return OutputInfo{}, invalidRender("output file missing or empty")
	}
	if v.Binary == "" {
		v.Binary = "ffprobe"
	}
	if v.run == nil {
		v.run = runFFprobe
	}
	data, err := v.run(ctx, v.Binary, "-v", "error", "-show_streams", "-show_format", "-of", "json", "-i", path)
	if err != nil || ctx.Err() != nil {
		return OutputInfo{}, invalidRender("ffprobe failed")
	}
	return parseRenderJSON(data, target)
}

type renderStream struct {
	CodecType    string          `json:"codec_type"`
	CodecName    string          `json:"codec_name"`
	PixFmt       string          `json:"pix_fmt"`
	Width        int             `json:"width"`
	Height       int             `json:"height"`
	AvgFrameRate string          `json:"avg_frame_rate"`
	RFrameRate   string          `json:"r_frame_rate"`
	DurationTS   json.RawMessage `json:"duration_ts"`
	TimeBase     string          `json:"time_base"`
	Duration     json.RawMessage `json:"duration"`
}

type renderDocument struct {
	Streams []renderStream `json:"streams"`
	Format  struct {
		FormatName string          `json:"format_name"`
		Duration   json.RawMessage `json:"duration"`
	} `json:"format"`
}

func streamDuration(ticks json.RawMessage, timeBase string, duration json.RawMessage) (domain.DurationUS, bool) {
	if n, ok := parsePositiveInteger(ticks); ok {
		if base, ok := parsePositiveRat(timeBase); ok {
			if us, ok := roundMicroseconds(new(big.Rat).Mul(new(big.Rat).SetInt(n), base)); ok {
				return us, true
			}
		}
	}
	if seconds, ok := parsePositiveRawRat(duration); ok {
		return roundMicroseconds(seconds)
	}
	return 0, false
}

func near30(avg, fallback string) bool {
	for _, raw := range []string{avg, fallback} {
		if rate, ok := parsePositiveRat(raw); ok {
			// Admit common 30000/1001 encodings while rejecting other frame rates.
			delta := new(big.Rat).Sub(rate, big.NewRat(30, 1))
			if delta.Sign() < 0 {
				delta.Neg(delta)
			}
			return delta.Cmp(big.NewRat(1, 20)) <= 0
		}
	}
	return false
}

func withinUS(a, b, tolerance domain.DurationUS) bool {
	if a >= b {
		return uint64(a)-uint64(b) <= uint64(tolerance)
	}
	return uint64(b)-uint64(a) <= uint64(tolerance)
}

func parseRenderJSON(data []byte, target domain.DurationUS) (OutputInfo, error) {
	if target <= 0 {
		return OutputInfo{}, invalidRender("invalid target duration")
	}
	var doc renderDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return OutputInfo{}, invalidRender("malformed ffprobe JSON")
	}
	mp4 := false
	for _, name := range strings.Split(doc.Format.FormatName, ",") {
		if name == "mp4" || name == "mov" {
			mp4 = true
		}
	}
	if !mp4 {
		return OutputInfo{}, invalidRender("unexpected container format")
	}
	var video, audio *renderStream
	for i := range doc.Streams {
		stream := &doc.Streams[i]
		switch stream.CodecType {
		case "video":
			if video != nil {
				return OutputInfo{}, invalidRender("multiple video streams")
			}
			video = stream
		case "audio":
			if audio != nil {
				return OutputInfo{}, invalidRender("multiple audio streams")
			}
			audio = stream
		default:
			return OutputInfo{}, invalidRender("unexpected stream")
		}
	}
	if video == nil || audio == nil {
		return OutputInfo{}, invalidRender("missing video or audio stream")
	}
	if video.CodecName != "h264" || video.PixFmt != "yuv420p" || video.Width != 1080 || video.Height != 1920 || !near30(video.AvgFrameRate, video.RFrameRate) || audio.CodecName != "aac" {
		return OutputInfo{}, invalidRender("invalid stream specification")
	}
	videoUS, vok := streamDuration(video.DurationTS, video.TimeBase, video.Duration)
	audioUS, aok := streamDuration(audio.DurationTS, audio.TimeBase, audio.Duration)
	formatRat, fok := parsePositiveRawRat(doc.Format.Duration)
	if !vok || !aok || !fok {
		return OutputInfo{}, invalidRender("missing duration")
	}
	formatUS, fok := roundMicroseconds(formatRat)
	if !fok || !withinUS(videoUS, target, renderToleranceUS) || !withinUS(audioUS, target, renderToleranceUS) || !withinUS(formatUS, target, renderToleranceUS) || !withinUS(videoUS, audioUS, renderToleranceUS) {
		return OutputInfo{}, invalidRender("duration outside tolerance")
	}
	return OutputInfo{VideoDurationUS: videoUS, AudioDurationUS: audioUS, FormatDurationUS: formatUS}, nil
}
