package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/lifei6671/clipweaver/internal/domain"
)

// TestExecutorRealFFmpeg uses only FFmpeg lavfi inputs and is skipped when host tools are absent.
func TestExecutorRealFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	stage := t.TempDir()
	generate := func(name string, args ...string) string {
		t.Helper()
		path := filepath.Join(stage, name)
		cmd := exec.CommandContext(ctx, "ffmpeg", append([]string{"-hide_banner", "-nostdin", "-v", "error", "-y"}, append(args, path)...)...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("generate %s: %v: %s", name, err, output)
		}
		return path
	}
	landscape := generate("landscape.mp4", "-f", "lavfi", "-i", "testsrc=size=640x360:rate=30", "-f", "lavfi", "-i", "sine=frequency=330:sample_rate=48000", "-t", "6", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac")
	portrait := generate("portrait.mp4", "-f", "lavfi", "-i", "testsrc=size=360x640:rate=30", "-t", "6", "-c:v", "libx264", "-pix_fmt", "yuv420p")
	narration := generate("narration.m4a", "-f", "lavfi", "-i", "sine=frequency=880:sample_rate=48000", "-t", "4.5", "-c:a", "aac")
	plan := domain.MixPlan{TargetDurationUS: 4_500_000, ClipDurationUS: 3_000_000, Clips: []domain.PlannedClip{{AssetID: "landscape", StartUS: 0, DurationUS: 3_000_000}, {AssetID: "portrait", StartUS: 3_000_000, DurationUS: 1_500_000}}}
	videos := map[string]domain.Asset{
		"landscape": {ID: "landscape", Kind: domain.AssetKindVideo, DurationUS: 6_000_000, StoredPath: landscape},
		"portrait":  {ID: "portrait", Kind: domain.AssetKindVideo, DurationUS: 6_000_000, StoredPath: portrait},
	}
	audio := domain.Asset{ID: "narration", Kind: domain.AssetKindAudio, DurationUS: 4_500_000, StoredPath: narration}
	output := filepath.Join(stage, "render.mp4")
	e := NewExecutor("ffmpeg", "ffprobe")
	result, err := e.Execute(ctx, plan, videos, audio, stage, output)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(output); err != nil || info.Size() == 0 {
		t.Fatalf("output stat %v, %v", info, err)
	}
	t.Logf("output=%s video_us=%d audio_us=%d format_us=%d; one AAC stream enforced by Validator", output, result.VideoDurationUS, result.AudioDurationUS, result.FormatDurationUS)
}
