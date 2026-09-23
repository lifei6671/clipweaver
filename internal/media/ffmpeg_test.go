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
	normalize := narrationNormalizeArgs("source.mp3", "narration.wav")
	if want := []string{"-hide_banner", "-nostdin", "-v", "error", "-y", "-i", "source.mp3", "-map", "0:a:0", "-vn", "-af", "asetpts=N/SR/TB", "-c:a", "pcm_s16le", "-f", "wav", "narration.wav"}; !reflect.DeepEqual(normalize, want) {
		t.Fatalf("narration normalize args = %v", normalize)
	}
	finalize := videoFinalizeArgs("silent.mp4", "final-video.mp4", 4_500_001)
	if want := []string{"-hide_banner", "-nostdin", "-v", "error", "-y", "-i", "silent.mp4", "-map", "0:v:0", "-vf", "tpad=stop=-1:stop_mode=clone", "-t", "4.500001", "-an", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-r", "30", "-f", "mp4", "final-video.mp4"}; !reflect.DeepEqual(finalize, want) {
		t.Fatalf("video finalize args = %v", finalize)
	}
	muxCommand := muxArgs("final-video.mp4", "narration.wav", "out.mp4", 4_500_001)
	if want := []string{"-hide_banner", "-nostdin", "-v", "error", "-y", "-i", "final-video.mp4", "-i", "narration.wav", "-map", "0:v:0", "-map", "1:a:0", "-t", "4.500001", "-c:v", "copy", "-c:a", "aac", "-movflags", "+faststart", "-f", "mp4", "out.mp4"}; !reflect.DeepEqual(muxCommand, want) {
		t.Fatalf("mux args = %v", muxCommand)
	}
	mux := strings.Join(muxCommand, " ")
	for _, forbidden := range []string{"-filter_complex", "tpad", "asetpts", "apad", "aloop", "-stream_loop", "-shortest", "source.mp3", "silent.mp4"} {
		if strings.Contains(mux, forbidden) {
			t.Fatalf("mux contains %q: %s", forbidden, mux)
		}
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
	var silentVideo, normalizedNarration, finalVideo string
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
			silentVideo = args[len(args)-1]
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
		if strings.Contains(joined, " -f wav ") {
			phase = "narration-normalize"
			normalizedNarration = args[len(args)-1]
			if !strings.Contains(joined, "-i "+audio.StoredPath) || filepath.Base(normalizedNarration) != "narration.wav" || filepath.Dir(normalizedNarration) != filepath.Dir(silentVideo) {
				t.Fatalf("narration normalization inputs %v", args)
			}
		}
		if strings.Contains(joined, "tpad=stop=-1:stop_mode=clone") {
			phase = "video-finalize"
			finalVideo = args[len(args)-1]
			if !strings.Contains(joined, "-i "+silentVideo) || filepath.Base(finalVideo) != "final-video.mp4" || filepath.Dir(finalVideo) != filepath.Dir(silentVideo) || strings.Contains(joined, audio.StoredPath) {
				t.Fatalf("video finalize inputs %v", args)
			}
		}
		if phase == "mux" {
			if !strings.Contains(joined, "-i "+finalVideo+" -i "+normalizedNarration) || strings.Contains(joined, audio.StoredPath) || strings.Contains(joined, videos["video-1"].StoredPath) {
				t.Fatalf("mux inputs %v", args)
			}
			for _, path := range []string{silentVideo, normalizedNarration, finalVideo} {
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("diagnostic input %s: %v", path, err)
				}
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
	if !reflect.DeepEqual(phases, []string{"normalize", "normalize", "concat", "narration-normalize", "video-finalize", "mux"}) || !reflect.DeepEqual(seenStarts, []string{"3", "0"}) {
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

func TestExecutorNarrationNormalizationFailureCleansOutput(t *testing.T) {
	plan, videos, audio, stage, output := testRenderInputs(t)
	e := NewExecutor("fake", "fake")
	var phases []string
	e.run = func(_ context.Context, _ string, args ...string) (string, error) {
		if strings.Contains(strings.Join(args, " "), " -f wav ") {
			phases = append(phases, "narration-normalize")
			if err := os.WriteFile(args[len(args)-1], []byte("partial"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(output, []byte("partial"), 0600); err != nil {
				t.Fatal(err)
			}
			return "decode failed", errors.New("exit status 1")
		}
		phases = append(phases, "earlier phase")
		return "", os.WriteFile(args[len(args)-1], []byte("x"), 0600)
	}
	_, err := e.Execute(context.Background(), plan, videos, audio, stage, output)
	var fferr *FFmpegError
	if !errors.Is(err, ErrFFmpegFailed) || !errors.As(err, &fferr) || fferr.Phase != "narration-normalize" || fferr.Stderr != "decode failed" {
		t.Fatalf("diagnostics = %v, %+v", err, fferr)
	}
	if !reflect.DeepEqual(phases, []string{"earlier phase", "earlier phase", "earlier phase", "narration-normalize"}) {
		t.Fatalf("phases = %v", phases)
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("output remains: %v", err)
	}
	entries, err := os.ReadDir(stage)
	if err != nil || len(entries) != 0 {
		t.Fatalf("stage entries %v, %v", entries, err)
	}
}

func TestExecutorFinalStagesFailureCleansOutput(t *testing.T) {
	for _, failPhase := range []string{"video-finalize", "mux"} {
		t.Run(failPhase, func(t *testing.T) {
			plan, videos, audio, stage, output := testRenderInputs(t)
			e := NewExecutor("fake", "fake")
			var phases []string
			e.run = func(_ context.Context, _ string, args ...string) (string, error) {
				joined := strings.Join(args, " ")
				phase := "mux"
				switch {
				case strings.Contains(joined, " -f concat "):
					phase = "concat"
				case strings.Contains(joined, " -f wav "):
					phase = "narration-normalize"
				case strings.Contains(joined, "tpad=stop=-1:stop_mode=clone"):
					phase = "video-finalize"
				case strings.Contains(joined, " -vf "):
					phase = "normalize"
				}
				phases = append(phases, phase)
				if err := os.WriteFile(args[len(args)-1], []byte("partial"), 0600); err != nil {
					t.Fatal(err)
				}
				if phase == failPhase {
					if err := os.WriteFile(output, []byte("partial"), 0600); err != nil {
						t.Fatal(err)
					}
					return "encode failed", errors.New("exit status 1")
				}
				return "", nil
			}
			_, err := e.Execute(context.Background(), plan, videos, audio, stage, output)
			var fferr *FFmpegError
			if !errors.Is(err, ErrFFmpegFailed) || !errors.As(err, &fferr) || fferr.Phase != failPhase || fferr.Stderr != "encode failed" {
				t.Fatalf("diagnostics = %v, %+v", err, fferr)
			}
			wantPhases := []string{"normalize", "normalize", "concat", "narration-normalize", "video-finalize"}
			if failPhase == "mux" {
				wantPhases = append(wantPhases, "mux")
			}
			if !reflect.DeepEqual(phases, wantPhases) {
				t.Fatalf("phases = %v", phases)
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
