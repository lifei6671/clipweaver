package media

import (
	"context"
	"encoding/json"
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

// TestPosterGeneratorRealFFmpeg uses a lavfi video and skips when media tools are absent.
func TestPosterGeneratorRealFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	stage := t.TempDir()
	video := filepath.Join(stage, "source.mp4")
	create := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-nostdin", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30", "-t", "2",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", video)
	if output, err := create.CombinedOutput(); err != nil {
		t.Fatalf("generate video: %v: %s", err, output)
	}

	poster := filepath.Join(stage, "poster.jpg")
	if err := NewPosterGenerator("ffmpeg").Generate(ctx, video, poster, 2_000_000); err != nil {
		t.Fatalf("generate poster: %v", err)
	}
	info, err := os.Stat(poster)
	if err != nil {
		t.Fatalf("poster stat: %v", err)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		t.Fatalf("poster is not a nonempty regular file: mode=%v size=%d", info.Mode(), info.Size())
	}

	probe := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=codec_name,width,height", "-of", "json", poster)
	output, err := probe.CombinedOutput()
	if err != nil {
		t.Fatalf("probe poster: %v: %s", err, output)
	}
	var result struct {
		Streams []struct {
			CodecName string `json:"codec_name"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode poster probe: %v: %s", err, output)
	}
	if len(result.Streams) != 1 {
		t.Fatalf("poster video streams = %d, want 1", len(result.Streams))
	}
	stream := result.Streams[0]
	if stream.CodecName != "mjpeg" || stream.Width != 320 || stream.Height != 180 {
		t.Fatalf("poster codec and size = %s %dx%d, want mjpeg 320x180", stream.CodecName, stream.Width, stream.Height)
	}
}
