package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/lifei6671/clipweaver/internal/domain"
	"github.com/lifei6671/clipweaver/internal/media"
	"github.com/lifei6671/clipweaver/internal/service"
	"github.com/lifei6671/clipweaver/internal/storage"
)

type testProber struct{}

func (testProber) ProbeVideo(_ context.Context, path string) (media.Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return media.Result{}, err
	}
	switch string(data) {
	case "bad":
		return media.Result{}, media.ErrMissingStream
	case "dimensions":
		return media.Result{DurationUS: 100, Width: 0, Height: 1080}, nil
	case "probe failure":
		return media.Result{}, fmt.Errorf("%w: %s", media.ErrProbeFailed, path)
	default:
		return media.Result{DurationUS: 8_000_000, Width: 1920, Height: 1080}, nil
	}
}

func (testProber) ProbeAudio(_ context.Context, path string) (media.Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return media.Result{}, err
	}
	if string(data) == "no audio" {
		return media.Result{}, media.ErrMissingStream
	}
	// "mixed container" stands for video/subtitle streams plus a selected audio stream.
	return media.Result{DurationUS: 9_700_000}, nil
}

func testApp(t *testing.T, root string) *fiber.App {
	return testAppWithPoster(t, root, testPoster(func(string) error { return errors.New("poster unavailable") }))
}

type testPoster func(string) error

func (p testPoster) Generate(_ context.Context, _, output string, _ domain.DurationUS) error {
	return p(output)
}

func testAppWithPoster(t *testing.T, root string, poster testPoster) *fiber.App {
	t.Helper()
	store, err := storage.NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler(logger)})
	Register(app, service.NewAssetService(store, testProber{}, poster, logger), logger)
	return app
}

type testFile struct{ name, body string }

func uploadRequest(t *testing.T, path, field string, files ...testFile) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, file := range files {
		part, err := writer.CreateFormFile(field, file.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(part, file.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func call(t *testing.T, app *fiber.App, req *http.Request, status int) map[string]any {
	t.Helper()
	response, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status {
		t.Fatalf("status=%d want=%d body=%s", response.StatusCode, status, data)
	}
	if strings.Contains(string(data), "StoredPath") || strings.Contains(string(data), `\\source.bin`) || strings.Contains(string(data), "/source.bin") {
		t.Fatalf("stored path leaked: %s", data)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON %s: %v", data, err)
	}
	return result
}

func items(t *testing.T, response map[string]any) []any {
	t.Helper()
	value, ok := response["items"].([]any)
	if !ok {
		t.Fatalf("missing items: %#v", response)
	}
	return value
}

func item(t *testing.T, value any) map[string]any {
	t.Helper()
	got, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("invalid item: %#v", value)
	}
	return got
}

func TestVideoBatchAndDiskReload(t *testing.T) {
	root := t.TempDir()
	app := testApp(t, root)
	response := call(t, app, uploadRequest(t, "/api/assets/videos", "files",
		testFile{"a.mp4", "first"}, testFile{"b.mp4", "second"}), 200)
	got := items(t, response)
	if len(got) != 2 {
		t.Fatalf("items=%#v", got)
	}
	ids := map[string]bool{}
	for i, value := range got {
		entry := item(t, value)
		asset := item(t, entry["asset"])
		id, _ := asset["id"].(string)
		if entry["status"] != "ready" || entry["filename"] != []string{"a.mp4", "b.mp4"}[i] || asset["name"] != entry["filename"] || asset["durationUs"] != float64(8_000_000) || asset["kind"] != "video" {
			t.Fatalf("incorrect ready item: %#v", entry)
		}
		if _, ok := asset["posterUrl"]; ok {
			t.Fatalf("failed poster should not have URL: %#v", asset)
		}
		if _, err := uuid.Parse(id); err != nil || ids[id] {
			t.Fatalf("invalid or duplicate id %q", id)
		}
		ids[id] = true
		entries, err := os.ReadDir(filepath.Join(root, "assets", id))
		if err != nil || len(entries) != 2 || entries[0].Name() != "meta.json" || entries[1].Name() != "source.bin" {
			t.Fatalf("asset files=%v err=%v", entries, err)
		}
	}
	// NewLocal reconstructs from disk; ordering is stable by ID.
	app = testApp(t, root)
	listed := items(t, call(t, app, httptest.NewRequest(http.MethodGet, "/api/assets", nil), 200))
	if len(listed) != 2 || item(t, listed[0])["id"].(string) >= item(t, listed[1])["id"].(string) {
		t.Fatalf("reloaded assets=%#v", listed)
	}
}

func TestVideoBatchReusesContentAndKeepsPublicDigestPrivate(t *testing.T) {
	root := t.TempDir()
	posterCalls := 0
	app := testAppWithPoster(t, root, testPoster(func(path string) error {
		posterCalls++
		return os.WriteFile(path, []byte("jpeg"), 0600)
	}))
	batch := items(t, call(t, app, uploadRequest(t, "/api/assets/videos", "files",
		testFile{"one.mp4", "same"}, testFile{"renamed.mp4", "same"}), 200))
	if len(batch) != 2 {
		t.Fatalf("batch = %#v", batch)
	}
	first := item(t, item(t, batch[0])["asset"])
	second := item(t, item(t, batch[1])["asset"])
	id := first["id"].(string)
	if item(t, batch[0])["status"] != "ready" || item(t, batch[1])["status"] != "ready" ||
		second["id"] != id || second["name"] != "one.mp4" || first["posterUrl"] != second["posterUrl"] || posterCalls != 1 {
		t.Fatalf("deduped batch = %#v, poster calls = %d", batch, posterCalls)
	}
	for _, asset := range []map[string]any{first, second} {
		if _, exposed := asset["contentSHA256"]; exposed {
			t.Fatalf("digest exposed: %#v", asset)
		}
	}
	listed := items(t, call(t, app, httptest.NewRequest(http.MethodGet, "/api/assets", nil), 200))
	if len(listed) != 1 || item(t, listed[0])["id"] != id {
		t.Fatalf("listed = %#v", listed)
	}
	if _, exposed := item(t, listed[0])["contentSHA256"]; exposed {
		t.Fatal("digest exposed in list")
	}
	app = testApp(t, root)
	again := items(t, call(t, app, uploadRequest(t, "/api/assets/videos", "files", testFile{"one.mp4", "different"}), 200))
	if item(t, item(t, again[0])["asset"])["id"] == id {
		t.Fatalf("same name, different bytes reused: %#v", again)
	}
	entries, err := os.ReadDir(filepath.Join(root, "assets"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("asset dirs = %v, %v", entries, err)
	}
}

func TestDeleteVideoRemovesDigestGroupAndKeepsOtherAssets(t *testing.T) {
	root := t.TempDir()
	app := testAppWithPoster(t, root, testPoster(func(path string) error {
		return os.WriteFile(path, []byte("jpeg"), 0600)
	}))
	first := item(t, items(t, call(t, app, uploadRequest(t, "/api/assets/videos", "files", testFile{"same.mp4", "original"}), 200))[0])
	firstID := item(t, first["asset"])["id"].(string)
	if _, err := os.Stat(filepath.Join(root, "assets", firstID, "poster.jpg")); err != nil {
		t.Fatalf("poster fixture was not created: %v", err)
	}
	other := item(t, items(t, call(t, app, uploadRequest(t, "/api/assets/videos", "files", testFile{"same.mp4", "different"}), 200))[0])
	otherID := item(t, other["asset"])["id"].(string)
	audio := item(t, call(t, app, uploadRequest(t, "/api/assets/audio", "file", testFile{"voice.m4a", "voice"}), 200)["asset"])
	audioID := audio["id"].(string)
	store, err := storage.NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := store.ReadAsset(firstID)
	if err != nil {
		t.Fatal(err)
	}
	duplicate.ID = storage.NewAssetID()
	duplicate.ContentSHA256 = "" // Historical manifest; deletion must backfill before comparing.
	duplicate.CreatedAt = duplicate.CreatedAt.Add(time.Hour)
	duplicatePath, err := store.SourcePath(duplicate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Dir(duplicatePath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(duplicatePath, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveAsset(duplicate); err != nil {
		t.Fatal(err)
	}
	if got := call(t, app, httptest.NewRequest(http.MethodDelete, "/api/assets/"+firstID, nil), 200); got["id"] != firstID || got["deleted"] != true || len(got) != 2 {
		t.Fatalf("delete response = %#v", got)
	}
	for _, id := range []string{firstID, duplicate.ID} {
		if _, err := os.Stat(filepath.Join(root, "assets", id)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("asset directory %s survived deletion: %v", id, err)
		}
	}
	for _, id := range []string{otherID, audioID} {
		if _, err := os.Stat(filepath.Join(root, "assets", id)); err != nil {
			t.Fatalf("unrelated asset %s was deleted: %v", id, err)
		}
	}
	app = testApp(t, root)
	listed := items(t, call(t, app, httptest.NewRequest(http.MethodGet, "/api/assets", nil), 200))
	if len(listed) != 2 {
		t.Fatalf("assets after reload = %#v", listed)
	}
	for _, entry := range listed {
		id := item(t, entry)["id"]
		if id != otherID && id != audioID {
			t.Fatalf("deleted video revived: %#v", listed)
		}
	}
	for _, tc := range []struct {
		id, code string
		status   int
	}{
		{"not-a-uuid", "INVALID_ID", 400},
		{firstID, "ASSET_NOT_FOUND", 404},
	} {
		result := call(t, app, httptest.NewRequest(http.MethodDelete, "/api/assets/"+tc.id, nil), tc.status)
		if got := item(t, result["error"])["code"]; got != tc.code {
			t.Fatalf("delete %s: code=%v want=%s", tc.id, got, tc.code)
		}
	}
}

func TestDeleteAudioRemovesOnlyTarget(t *testing.T) {
	root := t.TempDir()
	app := testApp(t, root)
	videoResult := items(t, call(t, app, uploadRequest(t, "/api/assets/videos", "files", testFile{"clip.mp4", "video"}), 200))
	videoID := item(t, item(t, videoResult[0])["asset"])["id"].(string)
	first := item(t, call(t, app, uploadRequest(t, "/api/assets/audio", "file", testFile{"first.m4a", "same"}), 200)["asset"])["id"].(string)
	second := item(t, call(t, app, uploadRequest(t, "/api/assets/audio", "file", testFile{"second.m4a", "same"}), 200)["asset"])["id"].(string)
	if first == second {
		t.Fatal("same-content audio uploads must have distinct IDs")
	}
	if got := call(t, app, httptest.NewRequest(http.MethodDelete, "/api/assets/"+first, nil), 200); got["id"] != first || got["deleted"] != true {
		t.Fatalf("delete response = %#v", got)
	}
	store, err := storage.NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadAsset(first); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted audio remains: %v", err)
	}
	for _, id := range []string{second, videoID} {
		if _, err := store.ReadAsset(id); err != nil {
			t.Fatalf("unrelated asset %s missing: %v", id, err)
		}
	}
	if listed := items(t, call(t, app, httptest.NewRequest(http.MethodGet, "/api/assets", nil), 200)); len(listed) != 2 {
		t.Fatalf("assets after audio deletion = %#v", listed)
	}
	for _, tc := range []struct {
		id, code string
		status   int
	}{
		{first, "ASSET_NOT_FOUND", 404},
		{"not-a-uuid", "INVALID_ID", 400},
	} {
		result := call(t, app, httptest.NewRequest(http.MethodDelete, "/api/assets/"+tc.id, nil), tc.status)
		if got := item(t, result["error"])["code"]; got != tc.code {
			t.Fatalf("delete %s: code=%v want=%s", tc.id, got, tc.code)
		}
	}
}

func TestDeleteVideoWithoutPosterRemovesDirectory(t *testing.T) {
	root := t.TempDir()
	app := testApp(t, root)
	response := items(t, call(t, app, uploadRequest(t, "/api/assets/videos", "files", testFile{"plain.mp4", "video"}), 200))
	id := item(t, item(t, response[0])["asset"])["id"].(string)
	if got := call(t, app, httptest.NewRequest(http.MethodDelete, "/api/assets/"+id, nil), 200); got["deleted"] != true {
		t.Fatalf("delete response = %#v", got)
	}
	if _, err := os.Stat(filepath.Join(root, "assets", id)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("asset directory survived deletion: %v", err)
	}
	if listed := items(t, call(t, app, httptest.NewRequest(http.MethodGet, "/api/assets", nil), 200)); len(listed) != 0 {
		t.Fatalf("deleted video remains listed: %#v", listed)
	}
}

func TestPosterAPIAndReload(t *testing.T) {
	root := t.TempDir()
	jpeg := []byte{0xff, 0xd8, 0xff, 0xd9}
	posterCalls := 0
	app := testAppWithPoster(t, root, testPoster(func(path string) error {
		posterCalls++
		return os.WriteFile(path, jpeg, 0600)
	}))
	videoResult := items(t, call(t, app, uploadRequest(t, "/api/assets/videos", "files", testFile{"clip.mp4", "video"}), 200))
	video := item(t, item(t, videoResult[0])["asset"])
	id := video["id"].(string)
	url := "/api/assets/" + id + "/poster"
	if video["posterUrl"] != url || video["durationUs"] != float64(8_000_000) {
		t.Fatalf("video response = %#v", video)
	}
	response, err := app.Test(httptest.NewRequest(http.MethodGet, url, nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || response.Header.Get("Content-Type") != "image/jpeg" || !bytes.Equal(data, jpeg) {
		t.Fatalf("poster response: status=%d type=%q data=%v error=%v", response.StatusCode, response.Header.Get("Content-Type"), data, err)
	}
	audioResult := call(t, app, uploadRequest(t, "/api/assets/audio", "file", testFile{"voice.m4a", "audio"}), 200)
	audio := item(t, audioResult["asset"])
	if _, ok := audio["posterUrl"]; ok || posterCalls != 1 {
		t.Fatalf("audio response has poster: %#v", audio)
	}
	app = testApp(t, root)
	listed := items(t, call(t, app, httptest.NewRequest(http.MethodGet, "/api/assets", nil), 200))
	for _, entry := range listed {
		asset := item(t, entry)
		if asset["kind"] == "video" && asset["posterUrl"] != url {
			t.Fatalf("reloaded video has no poster: %#v", asset)
		}
		if asset["kind"] == "audio" {
			if _, ok := asset["posterUrl"]; ok {
				t.Fatalf("reloaded audio has poster: %#v", asset)
			}
		}
	}
	for _, tc := range []struct {
		id, code string
		status   int
	}{
		{"not-a-uuid", "INVALID_ID", 400},
		{uuid.NewString(), "ASSET_NOT_FOUND", 404},
		{audio["id"].(string), "ASSET_NOT_FOUND", 404},
	} {
		result := call(t, app, httptest.NewRequest(http.MethodGet, "/api/assets/"+tc.id+"/poster", nil), tc.status)
		if got := item(t, result["error"])["code"]; got != tc.code {
			t.Fatalf("poster error for %s = %v", tc.id, got)
		}
	}
	if err := os.Remove(filepath.Join(root, "assets", id, "poster.jpg")); err != nil {
		t.Fatal(err)
	}
	missing := call(t, app, httptest.NewRequest(http.MethodGet, url, nil), 404)
	if item(t, missing["error"])["code"] != "ASSET_NOT_FOUND" {
		t.Fatalf("missing poster = %#v", missing)
	}
	listed = items(t, call(t, app, httptest.NewRequest(http.MethodGet, "/api/assets", nil), 200))
	for _, entry := range listed {
		asset := item(t, entry)
		if asset["kind"] == "video" {
			if _, ok := asset["posterUrl"]; ok {
				t.Fatalf("historical video has poster URL: %#v", asset)
			}
		}
	}
}

func TestVideoPartialFailureAndUnsafeNames(t *testing.T) {
	root := t.TempDir()
	app := testApp(t, root)
	response := call(t, app, uploadRequest(t, "/api/assets/videos", "files",
		testFile{`../../../evil.mp4`, "good"}, testFile{`..\..\evil.mp4`, "bad"}), 200)
	got := items(t, response)
	if len(got) != 2 || item(t, got[0])["status"] != "ready" || item(t, got[1])["status"] != "failed" {
		t.Fatalf("partial result=%#v", got)
	}
	if item(t, item(t, got[1])["error"])["code"] != "INVALID_VIDEO" {
		t.Fatalf("unstable failure=%#v", got[1])
	}
	id := item(t, item(t, got[0])["asset"])["id"].(string)
	assetFiles, err := os.ReadDir(filepath.Join(root, "assets", id))
	if err != nil || len(assetFiles) != 2 || assetFiles[0].Name() != "meta.json" || assetFiles[1].Name() != "source.bin" {
		t.Fatalf("unsafe filename affected stored paths: %v, %v", assetFiles, err)
	}
	for _, dir := range []string{filepath.Join(root, "assets"), filepath.Join(root, "tmp", "uploads")} {
		entries, err := os.ReadDir(dir)
		want := 0
		if filepath.Base(dir) == "assets" {
			want = 1
		}
		if err != nil || len(entries) != want {
			t.Fatalf("%s entries=%v err=%v", dir, entries, err)
		}
	}
	for _, name := range []string{"evil.mp4"} {
		if _, err := os.Stat(filepath.Join(root, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("unexpected path %s: %v", name, err)
		}
	}
}

func TestInvalidMediaAndAudioHistory(t *testing.T) {
	root := t.TempDir()
	app := testApp(t, root)
	for _, body := range []string{"bad", "dimensions"} {
		got := items(t, call(t, app, uploadRequest(t, "/api/assets/videos", "files", testFile{"bad.mp4", body}), 200))
		if item(t, got[0])["status"] != "failed" {
			t.Fatalf("invalid video accepted: %#v", got)
		}
	}
	for _, name := range []string{"first.m4a", "second.m4a"} {
		result := call(t, app, uploadRequest(t, "/api/assets/audio", "file", testFile{name, "mixed container"}), 200)
		asset := item(t, result["asset"])
		if result["status"] != "ready" || asset["durationUs"] != float64(9_700_000) || asset["kind"] != "audio" {
			t.Fatalf("audio response=%#v", result)
		}
	}
	listed := items(t, call(t, app, httptest.NewRequest(http.MethodGet, "/api/assets", nil), 200))
	if len(listed) != 2 || item(t, listed[0])["kind"] != "audio" || item(t, listed[1])["kind"] != "audio" {
		t.Fatalf("audio history=%#v", listed)
	}
	result := call(t, app, uploadRequest(t, "/api/assets/audio", "file", testFile{"bad.m4a", "no audio"}), 400)
	if item(t, result["error"])["code"] != "INVALID_AUDIO" {
		t.Fatalf("invalid audio error=%#v", result)
	}
	for _, dir := range []string{filepath.Join(root, "tmp", "uploads"), filepath.Join(root, "assets")} {
		entries, err := os.ReadDir(dir)
		want := 0
		if filepath.Base(dir) == "assets" {
			want = 2
		}
		if err != nil || len(entries) != want {
			t.Fatalf("%s entries=%v err=%v", dir, entries, err)
		}
	}
}

func TestUploadRequestErrors(t *testing.T) {
	app := testApp(t, t.TempDir())
	for _, tc := range []struct {
		path  string
		field string
		files []testFile
	}{
		{path: "/api/assets/videos", field: "files"},
		{path: "/api/assets/videos", field: "wrong", files: []testFile{{"a.mp4", "good"}}},
		{path: "/api/assets/audio", field: "file"},
		{path: "/api/assets/audio", field: "file", files: []testFile{{"a.m4a", "good"}, {"b.m4a", "good"}}},
	} {
		result := call(t, app, uploadRequest(t, tc.path, tc.field, tc.files...), 400)
		if item(t, result["error"])["code"] != "INVALID_REQUEST" {
			t.Fatalf("request error=%#v", result)
		}
	}
	result := call(t, app, httptest.NewRequest(http.MethodPost, "/api/assets/videos", strings.NewReader("not multipart")), 400)
	if item(t, result["error"])["code"] != "INVALID_REQUEST" {
		t.Fatalf("malformed request=%#v", result)
	}
}

func TestProbeFailureHasStablePublicError(t *testing.T) {
	root := t.TempDir()
	app := testApp(t, root)
	got := items(t, call(t, app, uploadRequest(t, "/api/assets/videos", "files", testFile{"probe.mp4", "probe failure"}), 200))
	failed := item(t, got[0])
	if failed["status"] != "failed" || item(t, failed["error"])["code"] != "FFPROBE_FAILED" {
		t.Fatalf("probe result=%#v", failed)
	}
	encoded, _ := json.Marshal(failed)
	if bytes.Contains(encoded, []byte(root)) || bytes.Contains(encoded, []byte("source.bin")) {
		t.Fatalf("probe path leaked: %s", encoded)
	}
}

func TestCorruptManifestIsInternalError(t *testing.T) {
	root := t.TempDir()
	app := testApp(t, root)
	got := items(t, call(t, app, uploadRequest(t, "/api/assets/videos", "files", testFile{"a.mp4", "good"}), 200))
	id := item(t, item(t, got[0])["asset"])["id"].(string)
	path := filepath.Join(root, "assets", id, "meta.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["id"] = uuid.NewString()
	data, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	result := call(t, app, httptest.NewRequest(http.MethodGet, "/api/assets", nil), 500)
	if item(t, result["error"])["code"] != "INTERNAL_ERROR" {
		t.Fatalf("corrupt manifest error=%#v", result)
	}
}
