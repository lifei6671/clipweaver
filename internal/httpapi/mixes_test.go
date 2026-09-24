package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/lifei6671/clipweaver/internal/domain"
	"github.com/lifei6671/clipweaver/internal/media"
	"github.com/lifei6671/clipweaver/internal/service"
	"github.com/lifei6671/clipweaver/internal/storage"
)

type mixExecutorFunc func(context.Context, domain.MixPlan, map[string]domain.Asset, domain.Asset, string, string) (media.OutputInfo, error)

func (f mixExecutorFunc) Execute(ctx context.Context, plan domain.MixPlan, videos map[string]domain.Asset, audio domain.Asset, stage, output string) (media.OutputInfo, error) {
	return f(ctx, plan, videos, audio, stage, output)
}

func mixHTTPFixture(t *testing.T, root string, executor mixExecutorFunc, seed func() (int64, error)) (*fiber.App, *storage.Local, *service.MixService, string, string) {
	t.Helper()
	store, err := storage.NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	videoID, audioID := storage.NewAssetID(), storage.NewAssetID()
	for _, asset := range []domain.Asset{{ID: videoID, Kind: domain.AssetKindVideo, DurationUS: 12_000_000, Width: 1080, Height: 1920}, {ID: audioID, Kind: domain.AssetKindAudio, DurationUS: 9_700_000}} {
		if err := store.SaveAsset(asset); err != nil {
			t.Fatal(err)
		}
		path, err := store.SourcePath(asset.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("source"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if executor == nil {
		executor = func(_ context.Context, _ domain.MixPlan, _ map[string]domain.Asset, _ domain.Asset, _, output string) (media.OutputInfo, error) {
			return media.OutputInfo{VideoDurationUS: 9_700_000, AudioDurationUS: 9_700_000, FormatDurationUS: 9_700_000}, os.WriteFile(output, []byte("0123456789"), 0600)
		}
	}
	svc := service.NewMixService(store, executor, time.Second, 1, seed)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler(logger)})
	RegisterMixes(app, svc, store, logger)
	t.Cleanup(svc.Close)
	return app, store, svc, videoID, audioID
}

func mixPost(t *testing.T, app *fiber.App, body string, status int) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/mixes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return call(t, app, req, status)
}

func mixError(t *testing.T, got map[string]any, code string) {
	t.Helper()
	if item(t, got["error"])["code"] != code {
		t.Fatalf("error=%#v want=%s", got, code)
	}
}

func TestMixHTTPDedupesHistoricalVideoContent(t *testing.T) {
	app, store, _, videoID, audioID := mixHTTPFixture(t, t.TempDir(), nil, nil)
	video, err := store.ReadAsset(videoID)
	if err != nil {
		t.Fatal(err)
	}
	video.DurationUS = 5_000_000
	if err := store.SaveAsset(video); err != nil {
		t.Fatal(err)
	}
	duplicate := video
	duplicate.ID = storage.NewAssetID()
	duplicate.ContentSHA256 = ""
	if err := store.SaveAsset(duplicate); err != nil {
		t.Fatal(err)
	}
	path, err := store.SourcePath(duplicate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("source"), 0600); err != nil {
		t.Fatal(err)
	}
	body := `{"videoIds":["` + duplicate.ID + `","` + videoID + `"],"audioId":"` + audioID + `","seed":"42"}`
	result := mixPost(t, app, body, 422)
	mixError(t, result, "INSUFFICIENT_VIDEO_DURATION")
	if details := item(t, result["error"])["details"].(map[string]any); details["missingDurationUs"] != float64(4_700_000) {
		t.Fatalf("wrong available duration: %#v", result)
	}
	manifestPath := filepath.Join(filepath.Dir(path), "meta.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["contentSHA256"] = "broken"
	data, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	mixError(t, mixPost(t, app, body, 500), "INTERNAL_ERROR")
	manifest["contentSHA256"] = ""
	data, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	mixError(t, mixPost(t, app, body, 500), "INTERNAL_ERROR")
}

func TestMixRequestErrorsAndSeed(t *testing.T) {
	app, store, _, videoID, audioID := mixHTTPFixture(t, t.TempDir(), nil, func() (int64, error) { return math.MinInt64, nil })
	for _, tc := range []struct {
		body   string
		status int
		code   string
	}{
		{`{"videoIds":[],"audioId":"` + audioID + `"}`, 400, "NO_VIDEO_SELECTED"},
		{`{"videoIds":["` + videoID + `"]}`, 400, "AUDIO_REQUIRED"},
		{`{"videoIds":["../bad"],"audioId":"` + audioID + `"}`, 400, "INVALID_ID"},
		{`{"videoIds":["` + storage.NewAssetID() + `"],"audioId":"` + audioID + `"}`, 404, "ASSET_NOT_FOUND"},
		{`{"videoIds":["` + audioID + `"],"audioId":"` + audioID + `"}`, 400, "INVALID_VIDEO"},
		{`{"videoIds":["` + videoID + `"],"audioId":"` + videoID + `"}`, 400, "INVALID_AUDIO"},
		{`{"videoIds":["` + videoID + `"],"audioId":"` + audioID + `","seed":"abc"}`, 400, "INVALID_SEED"},
		{`{"videoIds":["` + videoID + `"],"audioId":"` + audioID + `","seed":""}`, 400, "INVALID_SEED"},
		{`{"videoIds":["` + videoID + `"],"audioId":"` + audioID + `","seed":"9223372036854775808"}`, 400, "INVALID_SEED"},
		{`{"videoIds":["` + videoID + `"],"audioId":"` + audioID + `","seed":9007199254740993}`, 400, "INVALID_SEED"},
		{`{"videoIds":["` + videoID + `","` + videoID + `"],"audioId":"` + audioID + `"}`, 400, "INVALID_REQUEST"},
		{`null`, 400, "INVALID_REQUEST"},
		{`{"videoIds":`, 400, "INVALID_REQUEST"},
	} {
		mixError(t, mixPost(t, app, tc.body, tc.status), tc.code)
	}
	for _, seed := range []string{"9007199254740993", strconv.FormatInt(math.MaxInt64, 10)} {
		got := mixPost(t, app, `{"videoIds":["`+videoID+`"],"audioId":"`+audioID+`","seed":"`+seed+`"}`, 200)
		if got["seed"] != seed {
			t.Fatalf("seed=%#v want=%s", got, seed)
		}
		meta, err := store.ReadMixMeta(got["id"].(string))
		if err != nil || strconv.FormatInt(meta.Seed, 10) != seed {
			t.Fatalf("persisted seed=%+v err=%v", meta, err)
		}
	}
	got := mixPost(t, app, `{"videoIds":["`+videoID+`"],"audioId":"`+audioID+`"}`, 200)
	if got["seed"] != strconv.FormatInt(math.MinInt64, 10) || got["status"] != "completed" || got["durationUs"] != float64(9_700_000) {
		t.Fatalf("response=%#v", got)
	}
	id := got["id"].(string)
	if got["previewUrl"] != "/api/mixes/"+id+"/file" || got["downloadUrl"] != "/api/mixes/"+id+"/download" {
		t.Fatalf("URLs=%#v", got)
	}
}

func TestMixFileRangeDownloadAndReload(t *testing.T) {
	root := t.TempDir()
	app, _, _, videoID, audioID := mixHTTPFixture(t, root, nil, nil)
	got := mixPost(t, app, `{"videoIds":["`+videoID+`"],"audioId":"`+audioID+`"}`, 200)
	id := got["id"].(string)
	for _, tc := range []struct {
		rangeHeader        string
		status             int
		body, contentRange string
	}{
		{"", 200, "0123456789", ""},
		{"bytes=0-3", 206, "0123", "bytes 0-3/10"},
		{"bytes=4-", 206, "456789", "bytes 4-9/10"},
		{"bytes=-3", 206, "789", "bytes 7-9/10"},
		{"bytes=20-", 416, "Requested Range Not Satisfiable", "bytes */10"},
		{"bytes=0-1,3-4", 416, "Requested Range Not Satisfiable", "bytes */10"},
		{"bytes=+1-2", 416, "Requested Range Not Satisfiable", "bytes */10"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/mixes/"+id+"/file", nil)
		if tc.rangeHeader != "" {
			req.Header.Set("Range", tc.rangeHeader)
		}
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != tc.status || string(body) != tc.body || resp.Header.Get("Content-Range") != tc.contentRange || resp.Header.Get("Content-Type") != "video/mp4" || resp.Header.Get("Accept-Ranges") != "bytes" {
			t.Fatalf("range=%q status=%d body=%q headers=%v err=%v", tc.rangeHeader, resp.StatusCode, body, resp.Header, err)
		}
		if tc.status != 416 && resp.Header.Get("Content-Length") != strconv.Itoa(len(tc.body)) {
			t.Fatalf("range=%q content-length=%q", tc.rangeHeader, resp.Header.Get("Content-Length"))
		}
	}
	for _, path := range []string{"/file", "/download"} {
		req := httptest.NewRequest(http.MethodGet, "/api/mixes/"+id+path, nil)
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || string(body) != "0123456789" {
			t.Fatalf("%s status=%d body=%q", path, resp.StatusCode, body)
		}
		if path == "/download" && !strings.Contains(resp.Header.Get("Content-Disposition"), "attachment;") {
			t.Fatalf("disposition=%q", resp.Header.Get("Content-Disposition"))
		}
	}
	store, err := storage.NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	reloaded := fiber.New(fiber.Config{ErrorHandler: ErrorHandler(logger)})
	RegisterMixes(reloaded, nil, store, logger)
	for _, path := range []string{"/file", "/download"} {
		resp, err := reloaded.Test(httptest.NewRequest(http.MethodGet, "/api/mixes/"+id+path, nil), -1)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || string(body) != "0123456789" {
			t.Fatalf("reloaded %s status=%d body=%q", path, resp.StatusCode, body)
		}
	}
	for _, invalid := range []string{"bad", ".."} {
		mixError(t, call(t, reloaded, httptest.NewRequest(http.MethodGet, "/api/mixes/"+invalid+"/file", nil), 400), "INVALID_ID")
	}
	mixError(t, call(t, reloaded, httptest.NewRequest(http.MethodGet, "/api/mixes/"+storage.NewMixID()+"/file", nil), 404), "MIX_NOT_FOUND")
	if err := os.Remove(filepath.Join(root, "mixes", id, "output.mp4")); err != nil {
		t.Fatal(err)
	}
	mixError(t, call(t, reloaded, httptest.NewRequest(http.MethodGet, "/api/mixes/"+id+"/file", nil), 404), "MIX_NOT_FOUND")
}

func TestCompletedMixOutputSurvivesVideoDeletion(t *testing.T) {
	root := t.TempDir()
	app, store, _, videoID, audioID := mixHTTPFixture(t, root, nil, nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	Register(app, service.NewAssetService(store, testProber{}, testPoster(func(string) error { return nil }), logger), logger)
	result := mixPost(t, app, `{"videoIds":["`+videoID+`"],"audioId":"`+audioID+`"}`, 200)
	mixID := result["id"].(string)
	call(t, app, httptest.NewRequest(http.MethodDelete, "/api/assets/"+videoID, nil), 200)
	for _, suffix := range []string{"file", "download"} {
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/mixes/"+mixID+"/"+suffix, nil), -1)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != 200 || string(body) != "0123456789" {
			t.Fatalf("%s after asset deletion: status=%d body=%q err=%v", suffix, resp.StatusCode, body, err)
		}
	}
}

func TestMixRenderErrorsDoNotLeak(t *testing.T) {
	for _, failure := range []struct {
		err  error
		code string
	}{{&media.FFmpegError{Stderr: "secret-path", Cause: errors.New("broken")}, "FFMPEG_FAILED"}, {media.ErrProbeFailed, "FFPROBE_FAILED"}, {media.ErrRenderValidationFailed, "RENDER_VALIDATION_FAILED"}} {
		root := t.TempDir()
		var id string
		executor := func(_ context.Context, _ domain.MixPlan, _ map[string]domain.Asset, _ domain.Asset, stage, output string) (media.OutputInfo, error) {
			id = filepath.Base(stage)
			if err := os.WriteFile(output, []byte("partial"), 0600); err != nil {
				t.Fatal(err)
			}
			return media.OutputInfo{}, failure.err
		}
		app, store, _, videoID, audioID := mixHTTPFixture(t, root, executor, nil)
		got := mixPost(t, app, `{"videoIds":["`+videoID+`"],"audioId":"`+audioID+`"}`, 500)
		mixError(t, got, failure.code)
		data, _ := json.Marshal(got)
		if strings.Contains(string(data), "secret-path") || strings.Contains(string(data), root) {
			t.Fatalf("leaked: %s", data)
		}
		meta, err := store.ReadMixMeta(id)
		if err != nil || meta.ErrorCode != failure.code || meta.Status != domain.MixStatusFailed {
			t.Fatalf("meta=%+v err=%v", meta, err)
		}
		mixError(t, call(t, app, httptest.NewRequest(http.MethodGet, "/api/mixes/"+id+"/file", nil), 404), "MIX_NOT_FOUND")
	}
}

func TestMixInsufficientBusyAndTimeoutHTTP(t *testing.T) {
	root := t.TempDir()
	app, store, _, videoID, audioID := mixHTTPFixture(t, root, nil, nil)
	video, err := store.ReadAsset(videoID)
	if err != nil {
		t.Fatal(err)
	}
	video.DurationUS = 5_000_000
	if err := store.SaveAsset(video); err != nil {
		t.Fatal(err)
	}
	body := `{"videoIds":["` + videoID + `"],"audioId":"` + audioID + `"}`
	got := mixPost(t, app, body, 422)
	mixError(t, got, "INSUFFICIENT_VIDEO_DURATION")
	if item(t, got["error"])["details"].(map[string]any)["missingDurationUs"] != float64(4_700_000) {
		t.Fatalf("shortage=%#v", got)
	}
	video.DurationUS = 12_000_000
	if err := store.SaveAsset(video); err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var calls int
	executor := func(ctx context.Context, _ domain.MixPlan, _ map[string]domain.Asset, _ domain.Asset, _, output string) (media.OutputInfo, error) {
		calls++
		started <- struct{}{}
		select {
		case <-release:
			return media.OutputInfo{VideoDurationUS: 9_700_000, AudioDurationUS: 9_700_000, FormatDurationUS: 9_700_000}, os.WriteFile(output, []byte("output"), 0600)
		case <-ctx.Done():
			return media.OutputInfo{}, ctx.Err()
		}
	}
	svc := service.NewMixService(store, mixExecutorFunc(executor), time.Second, 1, nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	busyApp := fiber.New(fiber.Config{ErrorHandler: ErrorHandler(logger)})
	RegisterMixes(busyApp, svc, store, logger)
	first := make(chan map[string]any, 1)
	go func() { first <- mixPost(t, busyApp, body, 200) }()
	<-started
	mixError(t, mixPost(t, busyApp, body, 429), "MIX_BUSY")
	if calls != 1 {
		t.Fatalf("busy entered executor %d times", calls)
	}
	close(release)
	if result := <-first; result["status"] != "completed" {
		t.Fatalf("first=%#v", result)
	}
	svc.Close()

	observed := make(chan struct{}, 1)
	timeoutExecutor := func(ctx context.Context, _ domain.MixPlan, _ map[string]domain.Asset, _ domain.Asset, _, _ string) (media.OutputInfo, error) {
		<-ctx.Done()
		observed <- struct{}{}
		return media.OutputInfo{}, &media.FFmpegError{Stderr: "secret", Cause: ctx.Err()}
	}
	timed := service.NewMixService(store, mixExecutorFunc(timeoutExecutor), 20*time.Millisecond, 1, nil)
	defer timed.Close()
	timeoutApp := fiber.New(fiber.Config{ErrorHandler: ErrorHandler(logger)})
	RegisterMixes(timeoutApp, timed, store, logger)
	mixError(t, mixPost(t, timeoutApp, body, 504), "MIX_TIMEOUT")
	select {
	case <-observed:
	default:
		t.Fatal("executor did not observe cancellation")
	}
}
