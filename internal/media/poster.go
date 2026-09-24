package media

import (
	"context"
	"os"
	"path/filepath"

	"github.com/lifei6671/clipweaver/internal/domain"
)

// PosterGenerator extracts a small JPEG from an already probed video.
type PosterGenerator struct {
	Binary string
	run    func(context.Context, string, ...string) (string, error)
}

func NewPosterGenerator(binary string) PosterGenerator {
	if binary == "" {
		binary = "ffmpeg"
	}
	return PosterGenerator{Binary: binary, run: runFFmpeg}
}

func posterArgs(input, output string, duration domain.DurationUS) []string {
	at := duration / 5
	// Very short clips may have only a frame at timestamp zero.
	if duration <= 250_000 {
		at = 0
	}
	if at > 1_000_000 {
		at = 1_000_000
	}
	return []string{"-hide_banner", "-nostdin", "-v", "error", "-y", "-i", input,
		"-ss", secondsUS(at), "-map", "0:v:0", "-frames:v", "1",
		"-vf", "scale='min(320,iw)':'min(320,ih)':force_original_aspect_ratio=decrease",
		"-an", "-c:v", "mjpeg", "-q:v", "3", "-f", "image2", output}
}

func (p PosterGenerator) Generate(ctx context.Context, input, output string, duration domain.DurationUS) error {
	ctx, cancel := context.WithTimeout(ctx, defaultProbeTimeout)
	defer cancel()
	if p.Binary == "" {
		p.Binary = "ffmpeg"
	}
	if p.run == nil {
		p.run = runFFmpeg
	}
	tmp, err := os.CreateTemp(filepath.Dir(output), ".poster-*.tmp")
	if err != nil {
		return err
	}
	path := tmp.Name()
	if err := tmp.Close(); err != nil {
		os.Remove(path)
		return err
	}
	defer os.Remove(path)
	stderr, err := p.run(ctx, p.Binary, posterArgs(input, path, duration)...)
	if err != nil || ctx.Err() != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return &FFmpegError{Phase: "poster", Stderr: stderr, Cause: err}
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return &FFmpegError{Phase: "poster", Cause: os.ErrInvalid}
	}
	return os.Rename(path, output)
}
