package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/lifei6671/clipweaver/internal/domain"
)

func TestPosterArgs(t *testing.T) {
	for _, tc := range []struct {
		duration domain.DurationUS
		at       string
	}{
		{8_000_000, "1"},
		{2_000_000, "0.4"},
		{500_000, "0.1"},
		{250_000, "0"},
		{1_000, "0"},
		{1, "0"},
	} {
		args := posterArgs("input.mp4", "poster.jpg", tc.duration)
		want := []string{"-hide_banner", "-nostdin", "-v", "error", "-y", "-i", "input.mp4",
			"-ss", tc.at, "-map", "0:v:0", "-frames:v", "1",
			"-vf", "scale='min(320,iw)':'min(320,ih)':force_original_aspect_ratio=decrease",
			"-an", "-c:v", "mjpeg", "-q:v", "3", "-f", "image2", "poster.jpg"}
		if !reflect.DeepEqual(args, want) {
			t.Errorf("duration %d: args = %v", tc.duration, args)
		}
		if strings.Contains(strings.Join(args, " "), "-noautorotate") {
			t.Fatal("poster disabled default autorotate")
		}
	}
}

func TestPosterGeneratorPublishesOnlySuccessfulOutput(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "poster.jpg")
	generator := NewPosterGenerator("fake-ffmpeg")
	generator.run = func(_ context.Context, binary string, args ...string) (string, error) {
		if binary != "fake-ffmpeg" || filepath.Dir(args[len(args)-1]) != dir {
			t.Fatalf("unexpected command: %s %v", binary, args)
		}
		return "decode failed", errors.New("exit status 1")
	}
	if err := generator.Generate(context.Background(), "source.bin", output, 1_000_000); !errors.Is(err, ErrFFmpegFailed) {
		t.Fatalf("failed generator = %v", err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("partial poster remains: %v, %v", entries, err)
	}
	generator.run = func(_ context.Context, _ string, args ...string) (string, error) {
		return "", os.WriteFile(args[len(args)-1], []byte{0xff, 0xd8, 0xff, 0xd9}, 0600)
	}
	if err := generator.Generate(context.Background(), "source.bin", output, 1_000_000); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(output); err != nil || !reflect.DeepEqual(data, []byte{0xff, 0xd8, 0xff, 0xd9}) {
		t.Fatalf("poster data = %v, %v", data, err)
	}
}
