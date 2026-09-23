package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lifei6671/clipweaver/internal/domain"
)

// Executor renders an already planned mix. The caller owns stagingDir and outputPath;
// Execute owns and removes only its render-* child and removes outputPath on failure.
type Executor struct {
	Binary    string
	Validator Validator
	run       func(context.Context, string, ...string) (string, error)
}

func NewExecutor(ffmpegBinary, ffprobeBinary string) Executor {
	if ffmpegBinary == "" {
		ffmpegBinary = "ffmpeg"
	}
	return Executor{Binary: ffmpegBinary, Validator: NewValidator(ffprobeBinary), run: runFFmpeg}
}

func (e Executor) Execute(ctx context.Context, plan domain.MixPlan, videos map[string]domain.Asset, narration domain.Asset, stagingDir, outputPath string) (result OutputInfo, err error) {
	if e.Binary == "" {
		e.Binary = "ffmpeg"
	}
	if e.run == nil {
		e.run = runFFmpeg
	}
	if stagingDir == "" || outputPath == "" || narration.Kind != domain.AssetKindAudio || narration.StoredPath == "" || narration.DurationUS != plan.TargetDurationUS {
		return OutputInfo{}, errors.New("invalid render input")
	}
	assets := make([]domain.Asset, 0, len(videos))
	for _, asset := range videos {
		assets = append(assets, asset)
	}
	if err := plan.Validate(assets); err != nil {
		return OutputInfo{}, fmt.Errorf("invalid mix plan: %w", err)
	}
	for _, clip := range plan.Clips {
		if videos[clip.AssetID].StoredPath == "" {
			return OutputInfo{}, errors.New("video path is empty")
		}
	}
	if _, err := os.Stat(outputPath); err == nil {
		return OutputInfo{}, errors.New("render output already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return OutputInfo{}, err
	}
	workDir, err := os.MkdirTemp(stagingDir, "render-")
	if err != nil {
		return OutputInfo{}, err
	}
	defer os.RemoveAll(workDir)
	defer func() {
		if err != nil {
			_ = os.Remove(outputPath)
		}
	}()
	run := func(phase string, args []string) error {
		stderr, runErr := e.run(ctx, e.Binary, args...)
		if runErr != nil || ctx.Err() != nil {
			if ctx.Err() != nil {
				runErr = ctx.Err()
			}
			return &FFmpegError{Phase: phase, Stderr: stderr, Cause: runErr}
		}
		return nil
	}
	manifest := filepath.Join(workDir, "concat.txt")
	file, err := os.Create(manifest)
	if err != nil {
		return OutputInfo{}, err
	}
	for i, clip := range plan.Clips {
		name := fmt.Sprintf("clip-%06d.mp4", i)
		path := filepath.Join(workDir, name)
		if err := run("normalize", clipArgs(videos[clip.AssetID].StoredPath, path, clip)); err != nil {
			file.Close()
			return OutputInfo{}, err
		}
		if _, err := fmt.Fprintf(file, "file '%s'\n", name); err != nil {
			file.Close()
			return OutputInfo{}, err
		}
	}
	if err := file.Close(); err != nil {
		return OutputInfo{}, err
	}
	silent := filepath.Join(workDir, "silent.mp4")
	if err := run("concat", concatArgs(manifest, silent)); err != nil {
		return OutputInfo{}, err
	}
	if err := run("mux", muxArgs(silent, narration.StoredPath, outputPath, plan.TargetDurationUS)); err != nil {
		return OutputInfo{}, err
	}
	return e.Validator.Validate(ctx, outputPath, plan.TargetDurationUS)
}
