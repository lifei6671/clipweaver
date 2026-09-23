package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lifei6671/clipweaver/internal/domain"
)

func TestListAssetsOnlyCompleteAndRejectsCorruption(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	id := NewAssetID()
	stageID, stage, err := store.NewUploadStaging()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "source.bin"), []byte("audio"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.PromoteUpload(stageID, id); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "assets", "uncommitted"), 0700); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListAssets()
	if err != nil || len(items) != 0 {
		t.Fatalf("uncommitted asset visible: %+v, %v", items, err)
	}
	asset := domain.Asset{ID: id, Kind: domain.AssetKindAudio, Name: "voice", DurationUS: 10, CreatedAt: time.Now().UTC()}
	if err := store.SaveAsset(asset); err != nil {
		t.Fatal(err)
	}
	items, err = store.ListAssets()
	if err != nil || len(items) != 1 || items[0].ID != id {
		t.Fatalf("committed asset missing: %+v, %v", items, err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", id, "meta.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ListAssets(); err == nil {
		t.Fatal("corrupt manifest was ignored")
	}
}
