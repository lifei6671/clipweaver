package storage

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lifei6671/clipweaver/internal/domain"
)

func TestLocalManifestRestartAndCleanup(t *testing.T) {
	root := filepath.Join(t.TempDir(), "data")
	store, err := NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	id := NewAssetID()
	if _, err := uuid.Parse(id); err != nil {
		t.Fatal(err)
	}
	name := "../../../evil\\..\\clip.mp4"
	asset := domain.Asset{
		ID: id, Kind: domain.AssetKindVideo, Name: name,
		StoredPath: filepath.Join(root, "..", "outside"),
		DurationUS: 1_234_567, Width: 1920, Height: 1080,
		CreatedAt: time.Date(2026, 9, 23, 1, 2, 3, 0, time.UTC),
	}
	if err := store.SaveAsset(asset); err != nil {
		t.Fatal(err)
	}
	source, err := store.SourcePath(id)
	if err != nil || source != filepath.Join(root, "assets", id, "source.bin") {
		t.Fatalf("source path = %q, %v", source, err)
	}
	manifestPath := filepath.Join(root, "assets", id, "meta.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "StoredPath") || strings.Contains(string(data), "outside") || strings.Contains(string(data), root) {
		t.Fatalf("manifest contains a host path: %s", data)
	}
	asset.Name = "updated.mp4"
	if err := store.SaveAsset(asset); err != nil {
		t.Fatalf("atomic manifest replacement: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(manifestPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "meta.json" {
		t.Fatalf("temporary manifest remains: %v", entries)
	}
	for _, makeStage := range []func() (string, string, error){store.NewUploadStaging, store.NewMixStaging} {
		stageID, stagePath, err := makeStage()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := uuid.Parse(stageID); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(stagePath, "orphan"), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(source, []byte("video source"), 0600); err != nil {
		t.Fatal(err)
	}
	store, err = NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.ReadAsset(id)
	if err != nil || got.Name != "updated.mp4" || got.DurationUS != asset.DurationUS || got.StoredPath != source {
		t.Fatalf("asset after restart = %+v, %v", got, err)
	}
	for _, kind := range []string{"uploads", "mixes"} {
		path := filepath.Join(root, "tmp", kind)
		entries, err := os.ReadDir(path)
		if err != nil || len(entries) != 0 {
			t.Fatalf("staging root %s = %v, %v", path, entries, err)
		}
	}
}

func TestReadAssetBackfillsHistoricalVideoAndRejectsCorruptDigest(t *testing.T) {
	store, err := NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 9, 23, 1, 2, 3, 0, time.UTC)
	ids := []string{NewAssetID(), NewAssetID()}
	content := []byte("same historical source")
	for _, id := range ids {
		asset := domain.Asset{ID: id, Kind: domain.AssetKindVideo, Name: "original.mp4",
			DurationUS: 5_000_000, Width: 1920, Height: 1080, CreatedAt: created}
		if err := store.SaveAsset(asset); err != nil {
			t.Fatal(err)
		}
		path, err := store.SourcePath(id)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	wantDigest := fmt.Sprintf("%x", sha256.Sum256(content))
	for _, id := range ids {
		asset, err := store.ReadAsset(id)
		if err != nil || asset.ContentSHA256 != wantDigest || asset.Name != "original.mp4" ||
			asset.DurationUS != 5_000_000 || asset.Width != 1920 || asset.Height != 1080 || !asset.CreatedAt.Equal(created) {
			t.Fatalf("historical asset = %+v, %v", asset, err)
		}
		manifest, err := os.ReadFile(filepath.Join(store.root, "assets", id, "meta.json"))
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(manifest, &fields); err != nil {
			t.Fatal(err)
		}
		if fields["contentSHA256"] != wantDigest || fields["name"] != "original.mp4" {
			t.Fatalf("backfilled manifest = %#v", fields)
		}
	}
	path := filepath.Join(store.root, "assets", ids[0], "meta.json")
	manifest, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(manifest, &fields); err != nil {
		t.Fatal(err)
	}
	fields["contentSHA256"] = strings.ToUpper(wantDigest)
	manifest, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, manifest, 0600); err != nil {
		t.Fatal(err)
	}
	if asset, err := store.ReadAsset(ids[0]); err != nil || asset.ContentSHA256 != wantDigest {
		t.Fatalf("uppercase digest = %+v, %v", asset, err)
	}
	fields["contentSHA256"] = "bad"
	manifest, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, manifest, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadAsset(ids[0]); !errors.Is(err, ErrCorruptManifest) {
		t.Fatalf("corrupt digest = %v", err)
	}
}

func TestLocalRejectsInvalidIDsAndManifestPathTampering(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"../../../evil", `..\..\evil`, filepath.Join(root, "absolute"), "not-a-uuid", NewAssetID() + "/file"} {
		t.Run(id, func(t *testing.T) {
			if _, err := store.SourcePath(id); !errors.Is(err, ErrInvalidID) {
				t.Fatalf("SourcePath(%q) = %v", id, err)
			}
			if _, err := store.PosterPath(id); !errors.Is(err, ErrInvalidID) {
				t.Fatalf("PosterPath(%q) = %v", id, err)
			}
			if _, err := store.ReadAsset(id); !errors.Is(err, ErrInvalidID) {
				t.Fatalf("ReadAsset(%q) = %v", id, err)
			}
			if err := store.SaveAsset(domain.Asset{ID: id}); !errors.Is(err, ErrInvalidID) {
				t.Fatalf("SaveAsset(%q) = %v", id, err)
			}
		})
	}
	id := NewAssetID()
	asset := domain.Asset{ID: id, Kind: domain.AssetKindAudio, Name: "voice", DurationUS: 100}
	if err := store.SaveAsset(asset); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "assets", id, "meta.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["StoredPath"] = filepath.Join(root, "..", "outside")
	data, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := store.ReadAsset(id)
	if err != nil || got.StoredPath != filepath.Join(root, "assets", id, "source.bin") {
		t.Fatalf("tampered path result = %+v, %v", got, err)
	}
	manifest["id"] = NewAssetID()
	data, _ = json.Marshal(manifest)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadAsset(id); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("mismatched manifest ID = %v", err)
	}
}

func TestLocalRejectsSymlinkEscapes(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	id := NewAssetID()
	outside := t.TempDir()
	link := filepath.Join(root, "assets", id)
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if _, err := store.SourcePath(id); err == nil {
		t.Fatal("asset symlink was accepted")
	}
	if err := store.SaveAsset(domain.Asset{ID: id, Kind: domain.AssetKindAudio, DurationUS: 1}); err == nil {
		t.Fatal("asset symlink was accepted for manifest write")
	}
}
