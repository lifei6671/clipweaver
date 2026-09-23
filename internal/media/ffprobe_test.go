package media

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/lifei6671/clipweaver/internal/domain"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseProbeFixtures(t *testing.T) {
	cases := []struct {
		name    string
		kind    domain.AssetKind
		index   int
		us      domain.DurationUS
		width   int
		height  int
		wantErr error
	}{
		{"video_default.json", domain.AssetKindVideo, 3, 3_003_000, 1920, 1080, nil},
		{"video_lowest.json", domain.AssetKindVideo, 2, 2_000_001, 200, 100, nil},
		{"audio_mixed.json", domain.AssetKindAudio, 4, 4_409_990, 0, 0, nil},
		{"stream_fallback.json", domain.AssetKindAudio, 0, 1_234_568, 0, 0, nil},
		{"format_fallback.json", domain.AssetKindAudio, 0, 2_000_001, 0, 0, nil},
		{"missing_video.json", domain.AssetKindVideo, 0, 0, 0, 0, ErrMissingStream},
		{"missing_audio.json", domain.AssetKindAudio, 0, 0, 0, 0, ErrMissingStream},
		{"invalid_duration.json", domain.AssetKindAudio, 0, 0, 0, 0, ErrInvalidDuration},
		{"invalid_json.json", domain.AssetKindVideo, 0, 0, 0, 0, ErrInvalidJSON},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseProbeJSON(fixture(t, tc.name), tc.kind)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr == nil && (got.StreamIndex != tc.index || got.DurationUS != tc.us || got.Width != tc.width || got.Height != tc.height) {
				t.Fatalf("result = %+v", got)
			}
		})
	}
}

func TestStreamSelectionIgnoresJSONOrder(t *testing.T) {
	var doc probeDocument
	if err := json.Unmarshal(fixture(t, "video_default.json"), &doc); err != nil {
		t.Fatal(err)
	}
	slices.Reverse(doc.Streams)
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	got, err := parseProbeJSON(data, domain.AssetKindVideo)
	if err != nil || got.StreamIndex != 3 {
		t.Fatalf("reversed streams: %+v, %v", got, err)
	}
	data = []byte(`{"streams":[{"index":9,"codec_type":"audio","duration":"1","disposition":{"default":2}},{"index":1,"codec_type":"audio","duration":"2","disposition":{"default":0}}]}`)
	got, err = parseProbeJSON(data, domain.AssetKindAudio)
	if err != nil || got.StreamIndex != 1 {
		t.Fatalf("only default=1 has priority: %+v, %v", got, err)
	}
}

func TestRoundMicroseconds(t *testing.T) {
	for _, tc := range []struct {
		seconds string
		want    domain.DurationUS
	}{
		{"0.0000004", 0},
		{"0.0000005", 1},
		{"0.0000006", 1},
		{"1.2345675", 1_234_568},
	} {
		r, ok := new(big.Rat).SetString(tc.seconds)
		if !ok {
			t.Fatal(tc.seconds)
		}
		got, valid := roundMicroseconds(r)
		if !valid || got != tc.want {
			t.Fatalf("%s: got %d, valid %v; want %d", tc.seconds, got, valid, tc.want)
		}
	}
	tooLarge, _ := new(big.Rat).SetString("9223372036854.775808")
	if _, ok := roundMicroseconds(tooLarge); ok {
		t.Fatal("overflow must be rejected")
	}
}

func TestProbeCommandAndErrors(t *testing.T) {
	p := NewProber("custom-ffprobe", time.Second)
	p.run = func(ctx context.Context, binary string, args ...string) ([]byte, error) {
		if binary != "custom-ffprobe" || !slices.Equal(args, []string{"-v", "error", "-show_streams", "-show_format", "-of", "json", "-i", "file;$(echo unsafe).mp4"}) {
			t.Fatalf("unexpected command: %q %q", binary, args)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("probe context has no deadline")
		}
		return fixture(t, "video_default.json"), nil
	}
	if got, err := p.ProbeVideo(context.Background(), "file;$(echo unsafe).mp4"); err != nil || got.StreamIndex != 3 {
		t.Fatalf("probe result: %+v, %v", got, err)
	}
	p.run = func(context.Context, string, ...string) ([]byte, error) { return nil, errors.New("exit 1") }
	if _, err := p.ProbeAudio(context.Background(), "bad"); !errors.Is(err, ErrProbeFailed) {
		t.Fatalf("process error = %v", err)
	}
	p.run = func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	p.Timeout = time.Millisecond
	if _, err := p.ProbeAudio(context.Background(), "slow"); !errors.Is(err, ErrProbeTimeout) {
		t.Fatalf("timeout error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.ProbeAudio(ctx, "canceled"); !errors.Is(err, ErrProbeCanceled) {
		t.Fatalf("cancellation error = %v", err)
	}
}

func TestInvalidDimensionsAndSubMicrosecondDuration(t *testing.T) {
	if _, err := parseProbeJSON([]byte(`{"streams":[{"index":0,"codec_type":"video","width":0,"height":1,"duration":"1"}]}`), domain.AssetKindVideo); !errors.Is(err, ErrInvalidDimensions) {
		t.Fatalf("dimensions error = %v", err)
	}
	if _, err := parseProbeJSON([]byte(`{"streams":[{"index":0,"codec_type":"audio","duration":"0.0000004"}]}`), domain.AssetKindAudio); !errors.Is(err, ErrInvalidDuration) {
		t.Fatalf("sub-microsecond error = %v", err)
	}
	for _, first := range []string{`"0.0000004"`, `"9223372036854.775808"`} {
		input := `{"streams":[{"index":0,"codec_type":"audio","duration":` + first + `}],"format":{"duration":"0.0000005"}}`
		got, err := parseProbeJSON([]byte(input), domain.AssetKindAudio)
		if err != nil || got.DurationUS != 1 {
			t.Fatalf("fallback after unrepresentable duration %s: %+v, %v", first, got, err)
		}
	}
}
