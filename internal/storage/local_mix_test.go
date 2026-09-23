package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lifei6671/clipweaver/internal/domain"
)

func TestMixStorageReloadAndCorruptMetadata(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(root, "mixes")); err != nil || !info.IsDir() {
		t.Fatalf("mix root: %v %v", info, err)
	}
	id, _, err := store.NewMixStaging()
	if err != nil {
		t.Fatal(err)
	}
	meta := domain.MixMeta{ID: id, Status: domain.MixStatusCompleted, Seed: 9007199254740993, DurationUS: 9_700_000, VideoIDs: []string{NewAssetID()}, AudioID: NewAssetID(), CreatedAt: time.Now().UTC()}
	plan := domain.MixPlan{Seed: meta.Seed, TargetDurationUS: meta.DurationUS, ClipDurationUS: 3_000_000, Clips: []domain.PlannedClip{
		{AssetID: meta.VideoIDs[0], StartUS: 0, DurationUS: 3_000_000},
		{AssetID: meta.VideoIDs[0], StartUS: 3_000_000, DurationUS: 3_000_000},
		{AssetID: meta.VideoIDs[0], StartUS: 6_000_000, DurationUS: 3_000_000},
		{AssetID: meta.VideoIDs[0], StartUS: 9_000_000, DurationUS: 700_000},
	}}
	if err := store.SaveMixMeta(meta); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMixPlan(id, plan); err != nil {
		t.Fatal(err)
	}
	output, err := store.MixOutputPath(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, []byte("mp4"), 0600); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reloaded.ReadMixMeta(id)
	if err != nil || got.Seed != meta.Seed || got.Status != domain.MixStatusCompleted {
		t.Fatalf("reloaded meta=%+v err=%v", got, err)
	}
	if gotPlan, err := reloaded.ReadMixPlan(id); err != nil || gotPlan.Seed != plan.Seed || len(gotPlan.Clips) != 4 {
		t.Fatalf("reloaded plan=%+v err=%v", gotPlan, err)
	}
	file, err := reloaded.OpenCompletedMixFile(id)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if _, err := reloaded.ReadMixMeta("../escape"); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("invalid id=%v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "mixes", id, "meta.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := reloaded.ReadMixMeta(id); err == nil || errors.Is(err, ErrInvalidID) {
		t.Fatalf("corruption=%v", err)
	}
}

func TestMixOutputRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	id := NewMixID()
	if err := store.SaveMixMeta(domain.MixMeta{ID: id, Status: domain.MixStatusCompleted}); err != nil {
		t.Fatal(err)
	}
	output, err := store.MixOutputPath(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "outside.mp4"), output); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := store.OpenCompletedMixFile(id); err == nil {
		t.Fatal("symlink output accepted")
	}
}
