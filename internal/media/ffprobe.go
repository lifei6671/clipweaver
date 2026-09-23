package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os/exec"
	"strings"
	"time"

	"github.com/lifei6671/clipweaver/internal/domain"
)

var (
	ErrProbeTimeout      = errors.New("PROBE_TIMEOUT")
	ErrProbeCanceled     = errors.New("PROBE_CANCELED")
	ErrProbeFailed       = errors.New("PROBE_FAILED")
	ErrInvalidJSON       = errors.New("INVALID_PROBE_JSON")
	ErrMissingStream     = errors.New("MISSING_MEDIA_STREAM")
	ErrInvalidDuration   = errors.New("INVALID_DURATION")
	ErrInvalidDimensions = errors.New("INVALID_VIDEO_DIMENSIONS")
)

const defaultProbeTimeout = 10 * time.Second

type Result struct {
	StreamIndex int
	DurationUS  domain.DurationUS
	Width       int
	Height      int
}

type Prober struct {
	Binary  string
	Timeout time.Duration
	run     func(context.Context, string, ...string) ([]byte, error)
}

func NewProber(binary string, timeout time.Duration) Prober {
	if binary == "" {
		binary = "ffprobe"
	}
	if timeout <= 0 {
		timeout = defaultProbeTimeout
	}
	return Prober{Binary: binary, Timeout: timeout, run: runFFprobe}
}

func runFFprobe(ctx context.Context, binary string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, binary, args...).Output()
}

func (p Prober) ProbeVideo(ctx context.Context, path string) (Result, error) {
	return p.probe(ctx, path, domain.AssetKindVideo)
}

func (p Prober) ProbeAudio(ctx context.Context, path string) (Result, error) {
	return p.probe(ctx, path, domain.AssetKindAudio)
}

func (p Prober) probe(ctx context.Context, path string, kind domain.AssetKind) (Result, error) {
	if p.Binary == "" || p.Timeout <= 0 {
		defaults := NewProber(p.Binary, p.Timeout)
		p.Binary, p.Timeout = defaults.Binary, defaults.Timeout
	}
	if p.run == nil {
		p.run = runFFprobe
	}
	ctx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()
	output, err := p.run(ctx, p.Binary, "-v", "error", "-show_streams", "-show_format", "-of", "json", "-i", path)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return Result{}, fmt.Errorf("%w: %v", ErrProbeTimeout, ctx.Err())
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return Result{}, fmt.Errorf("%w: %v", ErrProbeCanceled, ctx.Err())
	}
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrProbeFailed, err)
	}
	return parseProbeJSON(output, kind)
}

type probeDocument struct {
	Streams []struct {
		Index       *int            `json:"index"`
		CodecType   string          `json:"codec_type"`
		DurationTS  json.RawMessage `json:"duration_ts"`
		TimeBase    string          `json:"time_base"`
		Duration    json.RawMessage `json:"duration"`
		Width       int             `json:"width"`
		Height      int             `json:"height"`
		Disposition struct {
			Default int `json:"default"`
		} `json:"disposition"`
	} `json:"streams"`
	Format struct {
		Duration json.RawMessage `json:"duration"`
	} `json:"format"`
}

func parseProbeJSON(data []byte, kind domain.AssetKind) (Result, error) {
	var doc probeDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	selected := -1
	for i := range doc.Streams {
		stream := &doc.Streams[i]
		if stream.CodecType != string(kind) || stream.Index == nil || *stream.Index < 0 {
			continue
		}
		isDefault := stream.Disposition.Default == 1
		chosenDefault := selected >= 0 && doc.Streams[selected].Disposition.Default == 1
		if selected < 0 || (isDefault && !chosenDefault) ||
			(isDefault == chosenDefault && *stream.Index < *doc.Streams[selected].Index) {
			selected = i
		}
	}
	if selected < 0 {
		return Result{}, ErrMissingStream
	}
	stream := doc.Streams[selected]
	result := Result{StreamIndex: *stream.Index, Width: stream.Width, Height: stream.Height}
	if kind == domain.AssetKindVideo && (result.Width <= 0 || result.Height <= 0) {
		return Result{}, ErrInvalidDimensions
	}
	if ticks, ok := parsePositiveInteger(stream.DurationTS); ok {
		if base, ok := parsePositiveRat(stream.TimeBase); ok {
			seconds := new(big.Rat).Mul(new(big.Rat).SetInt(ticks), base)
			if us, valid := roundMicroseconds(seconds); valid && us > 0 {
				result.DurationUS = us
				return result, nil
			}
		}
	}
	if seconds, ok := parsePositiveRawRat(stream.Duration); ok {
		if us, valid := roundMicroseconds(seconds); valid && us > 0 {
			result.DurationUS = us
			return result, nil
		}
	}
	if seconds, ok := parsePositiveRawRat(doc.Format.Duration); ok {
		if us, valid := roundMicroseconds(seconds); valid && us > 0 {
			result.DurationUS = us
			return result, nil
		}
	}
	return Result{}, ErrInvalidDuration
}

func parsePositiveInteger(raw json.RawMessage) (*big.Int, bool) {
	value, ok := rawNumber(raw)
	if !ok {
		return nil, false
	}
	n, ok := new(big.Int).SetString(value, 10)
	return n, ok && n.Sign() > 0
}

func parsePositiveRawRat(raw json.RawMessage) (*big.Rat, bool) {
	value, ok := rawNumber(raw)
	if !ok {
		return nil, false
	}
	return parsePositiveRat(value)
}

func rawNumber(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", false
	}
	if raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", false
		}
		return value, true
	}
	return string(raw), true
}

func parsePositiveRat(value string) (*big.Rat, bool) {
	r, ok := new(big.Rat).SetString(strings.TrimSpace(value))
	return r, ok && r.Sign() > 0
}

// roundMicroseconds rounds positive rational seconds to the nearest microsecond.
// An exact half microsecond rounds up, away from zero.
func roundMicroseconds(seconds *big.Rat) (domain.DurationUS, bool) {
	if seconds == nil || seconds.Sign() <= 0 {
		return 0, false
	}
	scaled := new(big.Rat).Mul(seconds, big.NewRat(1_000_000, 1))
	quotient, remainder := new(big.Int).QuoRem(scaled.Num(), scaled.Denom(), new(big.Int))
	if new(big.Int).Lsh(remainder, 1).Cmp(scaled.Denom()) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if !quotient.IsInt64() {
		return 0, false
	}
	return domain.DurationUS(quotient.Int64()), true
}
