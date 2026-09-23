package media

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lifei6671/clipweaver/internal/domain"
)

func TestRenderValidatorFixtures(t *testing.T) {
	valid, err := os.ReadFile("testdata/render_valid.json")
	if err != nil {
		t.Fatal(err)
	}
	base := string(valid)
	var duplicate renderDocument
	if err := json.Unmarshal(valid, &duplicate); err != nil {
		t.Fatal(err)
	}
	missing := duplicate
	missing.Streams = missing.Streams[:1]
	missingJSON, err := json.Marshal(missing)
	if err != nil {
		t.Fatal(err)
	}
	duplicate.Streams = append(duplicate.Streams, duplicate.Streams[1])
	duplicateJSON, err := json.Marshal(duplicate)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		json  string
		valid bool
	}{
		{"valid", base, true},
		{"mov format", strings.Replace(base, "mov,mp4,m4a,3gp,3g2,mj2", "mov", 1), true},
		{"wrong codec", strings.Replace(base, `"h264"`, `"hevc"`, 1), false},
		{"wrong pix fmt", strings.Replace(base, `"yuv420p"`, `"yuv444p"`, 1), false},
		{"wrong size", strings.Replace(base, `"width":1080`, `"width":720`, 1), false},
		{"wrong fps", strings.Replace(base, `"avg_frame_rate":"30/1"`, `"avg_frame_rate":"24/1"`, 1), false},
		{"missing audio", string(missingJSON), false},
		{"multiple audio", string(duplicateJSON), false},
		{"wrong audio codec", strings.Replace(base, `"codec_name":"aac"`, `"codec_name":"mp3"`, 1), false},
		{"video outside tolerance", strings.Replace(base, `"duration_ts":135`, `"duration_ts":139`, 1), false},
		{"video audio drift", strings.Replace(strings.Replace(base, `"duration_ts":135`, `"duration_ts":138`, 1), `"duration_ts":216000`, `"duration_ts":215040`, 1), false},
		{"format outside tolerance", strings.Replace(base, `"format": {"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"4.5"}`, `"format": {"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"4.601"}`, 1), false},
		{"bad format", strings.Replace(base, "mov,mp4,m4a,3gp,3g2,mj2", "matroska,webm", 1), false},
		{"malformed JSON", `{`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseRenderJSON([]byte(tc.json), 4_500_000)
			if tc.valid {
				if err != nil || got.VideoDurationUS != 4_500_000 || got.AudioDurationUS != 4_500_000 || got.FormatDurationUS != 4_500_000 {
					t.Fatalf("got %+v, %v", got, err)
				}
			} else if !errors.Is(err, ErrRenderValidationFailed) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestValidatorProbeFailureAndFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "render.mp4")
	v := NewValidator("ffprobe")
	if _, err := v.Validate(context.Background(), path, 4_500_000); !errors.Is(err, ErrRenderValidationFailed) {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	v.run = func(context.Context, string, ...string) ([]byte, error) { return nil, errors.New("probe exited") }
	if _, err := v.Validate(context.Background(), path, 4_500_000); !errors.Is(err, ErrRenderValidationFailed) {
		t.Fatal(err)
	}
}

func TestRenderDurationRationalAndBoundaries(t *testing.T) {
	valid, err := os.ReadFile("testdata/render_valid.json")
	if err != nil {
		t.Fatal(err)
	}
	json := strings.Replace(string(valid), `"duration_ts":135`, `"duration_ts":138`, 1)
	json = strings.Replace(json, `"duration_ts":216000`, `"duration_ts":218400`, 1) // 4.55s: video/audio drift 50ms.
	json = strings.Replace(json, `"duration":"4.5"}`, `"duration":"4.55"}`, 1)      // format duration.
	if _, err := parseRenderJSON([]byte(json), 4_500_000); err != nil {
		t.Fatal(err)
	}
	if !withinUS(4_600_000, 4_500_000, renderToleranceUS) || withinUS(4_600_001, 4_500_000, renderToleranceUS) {
		t.Fatal("100ms boundary")
	}
	if us, ok := streamDuration([]byte("216001"), "1/48000", nil); !ok || us != domain.DurationUS(4_500_021) {
		t.Fatalf("rational duration %d %v", us, ok)
	}
}
