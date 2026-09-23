package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lifei6671/clipweaver/internal/domain"
)

func TestFFmpegArgumentContract(t *testing.T) {
	clip := domain.PlannedClip{AssetID: "v", StartUS: 1_500_000, DurationUS: 3_000_000}
	args := clipArgs("input.mp4", "clip.mp4", clip)
	joined := strings.Join(args, " ")
	for _, required := range []string{
		"trim=start=1.5:end=4.5,setpts=PTS-STARTPTS",
		"scale=1080:1920:force_original_aspect_ratio=decrease",
		"pad=1080:1920:(ow-iw)/2:(oh-ih)/2:black", "fps=30", "setsar=1", "format=yuv420p",
		"-an", "-c:v libx264", "-pix_fmt yuv420p",
	} {
		if !strings.Contains(joined, required) {
			t.Errorf("missing %q: %s", required, joined)
		}
	}
	for _, forbidden := range []string{"-ss", "-to", "-noautorotate", "-c copy"} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("unexpected %q", forbidden)
		}
	}
	if got := secondsUS(1_234_567); got != "1.234567" {
		t.Fatalf("microseconds = %s", got)
	}
	mux := strings.Join(muxArgs("silent.mp4", "narration.m4a", "out.mp4", 4_500_001), " ")
	for _, required := range []string{"-i silent.mp4 -i narration.m4a", "[0:v:0]tpad=stop=-1:stop_mode=clone[v]", "[1:a:0]asetpts=PTS-STARTPTS[a]", "-map [v] -map [a]", "-t 4.500001", "-c:v libx264", "-pix_fmt yuv420p", "-c:a aac", "-movflags +faststart", "-f mp4"} {
		if !strings.Contains(mux, required) {
			t.Errorf("missing %q: %s", required, mux)
		}
	}
	if strings.Contains(mux, "-shortest") || strings.Contains(mux, "-noautorotate") {
		t.Fatal(mux)
	}
}

func testRenderInputs(t *testing.T) (domain.MixPlan, map[string]domain.Asset, domain.Asset, string, string) {
	t.Helper()
	stage := t.TempDir()
	video := domain.Asset{ID: "video-1", Kind: domain.AssetKindVideo, DurationUS: 6_000_000, StoredPath: filepath.Join(stage, "source.mp4")}
	audio := domain.Asset{ID: "audio-1", Kind: domain.AssetKindAudio, DurationUS: 4_500_000, StoredPath: filepath.Join(stage, "narration.m4a")}
	plan := domain.MixPlan{TargetDurationUS: 4_500_000, ClipDurationUS: 3_000_000, Clips: []domain.PlannedClip{{AssetID: video.ID, StartUS: 3_000_000, DurationUS: 3_000_000}, {AssetID: video.ID, StartUS: 0, DurationUS: 1_500_000}}}
	return plan, map[string]domain.Asset{video.ID: video}, audio, stage, filepath.Join(stage, "output.mp4")
}

func TestExecutorOrderAndCleanup(t *testing.T) {
	plan, videos, audio, stage, output := testRenderInputs(t)
	valid, err := os.ReadFile("testdata/render_valid.json")
	if err != nil {
		t.Fatal(err)
	}
	e := NewExecutor("fake-ffmpeg", "fake-ffprobe")
	var phases []string
	var seenStarts []string
	e.run = func(ctx context.Context, binary string, args ...string) (string, error) {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "trim=start=3:end=6") {
			seenStarts = append(seenStarts, "3")
		}
		if strings.Contains(joined, "trim=start=0:end=1.5") {
			seenStarts = append(seenStarts, "0")
		}
		phase := "mux"
		if strings.Contains(joined, " -vf ") {
			phase = "normalize"
		}
		if strings.Contains(joined, " -f concat ") {
			phase = "concat"
			manifest := ""
			for i, arg := range args {
				if arg == "-i" {
					manifest = args[i+1]
					break
				}
			}
			data, err := os.ReadFile(manifest)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != "file 'clip-000000.mp4'\nfile 'clip-000001.mp4'\n" {
				t.Fatalf("concat manifest %q", data)
			}
		}
		phases = append(phases, phase)
		if err := os.WriteFile(args[len(args)-1], []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		return "", nil
	}
	e.Validator.run = func(context.Context, string, ...string) ([]byte, error) { return valid, nil }
	if _, err := e.Execute(context.Background(), plan, videos, audio, stage, output); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(phases, []string{"normalize", "normalize", "concat", "mux"}) || !reflect.DeepEqual(seenStarts, []string{"3", "0"}) {
		t.Fatalf("phases %v starts %v", phases, seenStarts)
	}
	entries, err := os.ReadDir(stage)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "output.mp4" {
		t.Fatalf("stage contents %v", entries)
	}
}

func TestExecutorFailureAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cancel bool
	}{{"ffmpeg failure", false}, {"canceled", true}} {
		t.Run(tc.name, func(t *testing.T) {
			plan, videos, audio, stage, output := testRenderInputs(t)
			e := NewExecutor("fake", "fake")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			e.run = func(context.Context, string, ...string) (string, error) {
				if err := os.WriteFile(output, []byte("partial"), 0600); err != nil {
					t.Fatal(err)
				}
				if tc.cancel {
					cancel()
					return "canceled", context.Canceled
				}
				return "failed", errors.New("exit status 1")
			}
			_, err := e.Execute(ctx, plan, videos, audio, stage, output)
			if !errors.Is(err, ErrFFmpegFailed) {
				t.Fatalf("error = %v", err)
			}
			var fferr *FFmpegError
			if !errors.As(err, &fferr) || fferr.Phase != "normalize" || fferr.Stderr == "" {
				t.Fatalf("diagnostics = %+v", fferr)
			}
			if tc.cancel && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("output remains: %v", err)
			}
			entries, err := os.ReadDir(stage)
			if err != nil || len(entries) != 0 {
				t.Fatalf("stage entries %v, %v", entries, err)
			}
		})
	}
}

func TestExecutorValidationFailureCleansOutput(t *testing.T) {
	plan, videos, audio, stage, output := testRenderInputs(t)
	e := NewExecutor("fake", "fake")
	e.run = func(_ context.Context, _ string, args ...string) (string, error) {
		return "", os.WriteFile(args[len(args)-1], []byte("x"), 0600)
	}
	e.Validator.run = func(context.Context, string, ...string) ([]byte, error) { return []byte("{"), nil }
	_, err := e.Execute(context.Background(), plan, videos, audio, stage, output)
	if !errors.Is(err, ErrRenderValidationFailed) {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output remains: %v", err)
	}
	entries, err := os.ReadDir(stage)
	if err != nil || len(entries) != 0 {
		t.Fatalf("stage entries %v, %v", entries, err)
	}
}

func TestBoundedStderr(t *testing.T) {
	var b boundedStderr
	_, _ = b.Write([]byte(strings.Repeat("a", maxFFmpegStderr)))
	_, _ = b.Write([]byte("tail"))
	if b.Len() != maxFFmpegStderr || !strings.HasSuffix(b.String(), "tail") {
		t.Fatalf("stderr len=%d", b.Len())
	}
}

func TestRunFFmpegCancellation(t *testing.T) {
	t.Setenv("CLIPWEAVER_FFMPEG_HELPER", "sleep")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(50*time.Millisecond, cancel)
	_, err := runFFmpeg(ctx, os.Args[0], "-test.run=^TestFFmpegChild$")
	if err == nil || !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("command=%v context=%v", err, ctx.Err())
	}
}

func TestFFmpegChild(t *testing.T) {
	if os.Getenv("CLIPWEAVER_FFMPEG_HELPER") != "sleep" {
		return
	}
	time.Sleep(10 * time.Second)
}
