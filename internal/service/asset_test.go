package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lifei6671/clipweaver/internal/domain"
	"github.com/lifei6671/clipweaver/internal/media"
	"github.com/lifei6671/clipweaver/internal/storage"
)

type fakeProber struct {
	video func(string) (media.Result, error)
	audio func(string) (media.Result, error)
}

func (p fakeProber) ProbeVideo(_ context.Context, path string) (media.Result, error) {
	return p.video(path)
}

func (p fakeProber) ProbeAudio(_ context.Context, path string) (media.Result, error) {
	return p.audio(path)
}

type faultStore struct {
	*storage.Local
	promoteErr error
	saveErr    error
}

func (s faultStore) PromoteUpload(uploadID, assetID string) error {
	if s.promoteErr != nil {
		return s.promoteErr
	}
	return s.Local.PromoteUpload(uploadID, assetID)
}

func (s faultStore) SaveAsset(asset domain.Asset) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	return s.Local.SaveAsset(asset)
}

type brokenReader struct{}

type posterFunc func(context.Context, string, string, domain.DurationUS) error

func (f posterFunc) Generate(ctx context.Context, input, output string, duration domain.DurationUS) error {
	return f(ctx, input, output, duration)
}

func (brokenReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestUploadFailureCleansStagingAndFinal(t *testing.T) {
	for _, tc := range []struct {
		name       string
		probe      func(string) (media.Result, error)
		promoteErr error
		saveErr    error
		input      io.Reader
	}{
		{name: "copy", probe: validVideo, input: brokenReader{}},
		{name: "probe", probe: func(string) (media.Result, error) { return media.Result{}, media.ErrMissingStream }, input: strings.NewReader("bad")},
		{name: "promote", probe: validVideo, promoteErr: errors.New("promotion failed"), input: strings.NewReader("video")},
		{name: "manifest", probe: validVideo, saveErr: errors.New("manifest failed"), input: strings.NewReader("video")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			local, err := storage.NewLocal(root)
			if err != nil {
				t.Fatal(err)
			}
			store := faultStore{Local: local, promoteErr: tc.promoteErr, saveErr: tc.saveErr}
			svc := NewAssetService(store, fakeProber{video: tc.probe}, posterFunc(func(context.Context, string, string, domain.DurationUS) error {
				return errors.New("poster unavailable")
			}), slog.New(slog.NewTextHandler(io.Discard, nil)))
			if _, err := svc.UploadVideo(context.Background(), "video.mp4", tc.input); err == nil {
				t.Fatal("upload unexpectedly succeeded")
			}
			for _, dir := range []string{filepath.Join(root, "tmp", "uploads"), filepath.Join(root, "assets")} {
				entries, err := os.ReadDir(dir)
				if err != nil || len(entries) != 0 {
					t.Fatalf("%s: entries=%v err=%v", dir, entries, err)
				}
			}
		})
	}
}

func TestPosterFailureDoesNotFailVideoUpload(t *testing.T) {
	root := t.TempDir()
	store, err := storage.NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	poster := posterFunc(func(_ context.Context, input, output string, duration domain.DurationUS) error {
		called = true
		if duration != 8_000_000 || filepath.Base(input) != "source.bin" || filepath.Base(output) != "poster.jpg" {
			t.Fatalf("poster inputs: %s %s %d", input, output, duration)
		}
		return errors.New("ffmpeg failed")
	})
	svc := NewAssetService(store, fakeProber{video: validVideo}, poster, slog.New(slog.NewTextHandler(io.Discard, nil)))
	asset, err := svc.UploadVideo(context.Background(), "clip.mp4", strings.NewReader("video"))
	if err != nil || !called || asset.HasPoster {
		t.Fatalf("upload = %+v, %v; called=%v", asset, err, called)
	}
	listed, err := svc.ListAssets()
	if err != nil || len(listed) != 1 || listed[0].ID != asset.ID || listed[0].HasPoster {
		t.Fatalf("listed = %+v, %v", listed, err)
	}
}

func validVideo(path string) (media.Result, error) {
	if _, err := os.ReadFile(path); err != nil {
		return media.Result{}, err
	}
	return media.Result{DurationUS: 8_000_000, Width: 1920, Height: 1080}, nil
}

func TestVideoContentDedupeAndAudioUnchanged(t *testing.T) {
	root := t.TempDir()
	store, err := storage.NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	var posterCalls int
	poster := posterFunc(func(_ context.Context, _, path string, _ domain.DurationUS) error {
		posterCalls++
		return os.WriteFile(path, []byte("jpeg"), 0600)
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewAssetService(store, fakeProber{video: validVideo, audio: func(string) (media.Result, error) {
		return media.Result{DurationUS: 8_000_000}, nil
	}}, poster, logger)
	first, err := svc.UploadVideo(context.Background(), "one.mp4", strings.NewReader("same bytes"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one.mp4", "renamed.mp4"} {
		got, err := svc.UploadVideo(context.Background(), name, strings.NewReader("same bytes"))
		if err != nil || got.ID != first.ID || got.Name != first.Name || !got.HasPoster {
			t.Fatalf("duplicate = %+v, %v; first = %+v", got, err, first)
		}
	}
	other, err := svc.UploadVideo(context.Background(), "one.mp4", strings.NewReader("different bytes"))
	if err != nil || other.ID == first.ID {
		t.Fatalf("different content = %+v, %v", other, err)
	}
	if posterCalls != 2 {
		t.Fatalf("poster calls = %d", posterCalls)
	}
	for i := 0; i < 2; i++ {
		if _, err := svc.UploadAudio(context.Background(), "voice.m4a", strings.NewReader("same bytes")); err != nil {
			t.Fatal(err)
		}
	}
	listed, err := svc.ListAssets()
	if err != nil || len(listed) != 4 {
		t.Fatalf("assets = %+v, %v", listed, err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "assets"))
	if err != nil || len(entries) != 4 {
		t.Fatalf("asset dirs = %v, %v", entries, err)
	}
	staged, err := os.ReadDir(filepath.Join(root, "tmp", "uploads"))
	if err != nil || len(staged) != 0 {
		t.Fatalf("staging = %v, %v", staged, err)
	}
	restarted, err := storage.NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	svc = NewAssetService(restarted, fakeProber{video: validVideo}, poster, logger)
	again, err := svc.UploadVideo(context.Background(), "after-restart.mp4", strings.NewReader("same bytes"))
	if err != nil || again.ID != first.ID || posterCalls != 2 {
		t.Fatalf("after restart = %+v, %v; poster calls = %d", again, err, posterCalls)
	}
}

func TestHistoricalVideoDigestsAndStableCanonical(t *testing.T) {
	root := t.TempDir()
	store, err := storage.NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ids := []string{storage.NewAssetID(), storage.NewAssetID(), storage.NewAssetID()}
	canonicalID := ids[1]
	if ids[2] < canonicalID {
		canonicalID = ids[2]
	}
	for i, id := range ids {
		source, err := store.SourcePath(id)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Dir(source), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(source, []byte("historical video"), 0600); err != nil {
			t.Fatal(err)
		}
		createdAt := created
		if i == 0 {
			createdAt = created.Add(time.Hour)
		}
		asset := domain.Asset{ID: id, Kind: domain.AssetKindVideo, Name: "old.mp4", DurationUS: 8_000_000,
			Width: 1920, Height: 1080, CreatedAt: createdAt}
		if err := store.SaveAsset(asset); err != nil {
			t.Fatal(err)
		}
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	posterCalls := 0
	svc := NewAssetService(store, fakeProber{video: validVideo}, posterFunc(func(_ context.Context, input, output string, _ domain.DurationUS) error {
		posterCalls++
		if filepath.Base(filepath.Dir(output)) != canonicalID || filepath.Base(input) != "source.bin" {
			t.Fatalf("poster generated for non-canonical asset: %s -> %s", input, output)
		}
		return os.WriteFile(output, []byte("jpeg"), 0600)
	}), logger)
	listed, err := svc.ListAssets()
	if err != nil || len(listed) != 1 || listed[0].ID != canonicalID || !listed[0].HasPoster || posterCalls != 1 {
		t.Fatalf("canonical = %+v, %v", listed, err)
	}
	listed, err = svc.ListAssets()
	if err != nil || len(listed) != 1 || !listed[0].HasPoster || posterCalls != 1 {
		t.Fatalf("second list = %+v, %v; poster calls = %d", listed, err, posterCalls)
	}
	for _, id := range ids {
		data, err := os.ReadFile(filepath.Join(root, "assets", id, "meta.json"))
		if err != nil {
			t.Fatal(err)
		}
		var meta map[string]any
		if err := json.Unmarshal(data, &meta); err != nil {
			t.Fatal(err)
		}
		if meta["contentSHA256"] != listed[0].ContentSHA256 || meta["name"] != "old.mp4" || meta["durationUS"] != float64(8_000_000) {
			t.Fatalf("backfilled manifest %s: %#v", id, meta)
		}
	}
	got, err := svc.UploadVideo(context.Background(), "new.mp4", strings.NewReader("historical video"))
	if err != nil || got.ID != canonicalID || !got.HasPoster || posterCalls != 1 {
		t.Fatalf("historical reuse = %+v, %v", got, err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "assets"))
	if err != nil || len(entries) != 3 {
		t.Fatalf("historical dirs = %v, %v", entries, err)
	}
}

func TestHistoricalPosterBackfillFailureAndAudio(t *testing.T) {
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	video, err := store.SourcePath(storage.NewAssetID())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Dir(video), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(video, []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	asset := domain.Asset{ID: filepath.Base(filepath.Dir(video)), Kind: domain.AssetKindVideo, Name: "old.mp4",
		DurationUS: 8_000_000, Width: 1920, Height: 1080, CreatedAt: time.Now().UTC()}
	if err := store.SaveAsset(asset); err != nil {
		t.Fatal(err)
	}
	audio := domain.Asset{ID: storage.NewAssetID(), Kind: domain.AssetKindAudio, Name: "voice.m4a",
		DurationUS: 8_000_000, CreatedAt: time.Now().UTC()}
	audioPath, err := store.SourcePath(audio.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Dir(audioPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(audioPath, []byte("audio"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveAsset(audio); err != nil {
		t.Fatal(err)
	}
	posterCalls := 0
	svc := NewAssetService(store, fakeProber{}, posterFunc(func(context.Context, string, string, domain.DurationUS) error {
		posterCalls++
		return errors.New("ffmpeg failed")
	}), slog.New(slog.NewTextHandler(io.Discard, nil)))
	listed, err := svc.ListAssets()
	if err != nil || len(listed) != 2 || posterCalls != 1 {
		t.Fatalf("list = %+v, %v; poster calls = %d", listed, err, posterCalls)
	}
	for _, item := range listed {
		if item.HasPoster || (item.Kind == domain.AssetKindVideo && item.ID != asset.ID) {
			t.Fatalf("unexpected asset after poster failure: %+v", item)
		}
	}
}

func TestDuplicateUploadBackfillsHistoricalPoster(t *testing.T) {
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id := storage.NewAssetID()
	path, err := store.SourcePath(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveAsset(domain.Asset{ID: id, Kind: domain.AssetKindVideo, Name: "old.mp4",
		DurationUS: 8_000_000, Width: 1920, Height: 1080, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	posterCalls := 0
	svc := NewAssetService(store, fakeProber{video: validVideo}, posterFunc(func(_ context.Context, _, output string, _ domain.DurationUS) error {
		posterCalls++
		return os.WriteFile(output, []byte("jpeg"), 0600)
	}), slog.New(slog.NewTextHandler(io.Discard, nil)))
	got, err := svc.UploadVideo(context.Background(), "again.mp4", strings.NewReader("video"))
	if err != nil || got.ID != id || !got.HasPoster || posterCalls != 1 {
		t.Fatalf("duplicate = %+v, %v; poster calls = %d", got, err, posterCalls)
	}
}

func TestConcurrentSameVideoHasOneCanonicalAsset(t *testing.T) {
	root := t.TempDir()
	store, err := storage.NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	var posterCalls atomic.Int32
	svc := NewAssetService(store, fakeProber{video: validVideo}, posterFunc(func(_ context.Context, _, output string, _ domain.DurationUS) error {
		posterCalls.Add(1)
		return os.WriteFile(output, []byte("jpeg"), 0600)
	}), slog.New(slog.NewTextHandler(io.Discard, nil)))
	start := make(chan struct{})
	results := make([]domain.Asset, 2)
	errorsByUpload := make([]error, 2)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errorsByUpload[i] = svc.UploadVideo(context.Background(), "video.mp4", strings.NewReader("same bytes"))
		}(i)
	}
	close(start)
	wg.Wait()
	if errorsByUpload[0] != nil || errorsByUpload[1] != nil || results[0].ID != results[1].ID || posterCalls.Load() != 1 {
		t.Fatalf("concurrent results = %+v, errors = %v, poster calls = %d", results, errorsByUpload, posterCalls.Load())
	}
	entries, err := os.ReadDir(filepath.Join(root, "assets"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("asset dirs = %v, %v", entries, err)
	}
}

func TestVideoComparisonFailureDoesNotCreateAnotherAsset(t *testing.T) {
	root := t.TempDir()
	store, err := storage.NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	broken := domain.Asset{ID: storage.NewAssetID(), Kind: domain.AssetKindVideo, Name: "old.mp4",
		DurationUS: 8_000_000, Width: 1920, Height: 1080, CreatedAt: time.Now().UTC()}
	if err := store.SaveAsset(broken); err != nil {
		t.Fatal(err)
	}
	svc := NewAssetService(store, fakeProber{video: validVideo}, posterFunc(func(context.Context, string, string, domain.DurationUS) error {
		t.Fatal("poster should not run when comparison fails")
		return nil
	}), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := svc.ListAssets(); err == nil {
		t.Fatal("missing historical source was ignored")
	}
	if _, err := svc.UploadVideo(context.Background(), "new.mp4", strings.NewReader("new video")); err == nil {
		t.Fatal("upload created a video without comparing historical source")
	}
	entries, err := os.ReadDir(filepath.Join(root, "assets"))
	if err != nil || len(entries) != 1 || entries[0].Name() != broken.ID {
		t.Fatalf("asset dirs = %v, %v", entries, err)
	}
	staged, err := os.ReadDir(filepath.Join(root, "tmp", "uploads"))
	if err != nil || len(staged) != 0 {
		t.Fatalf("staging = %v, %v", staged, err)
	}
}
