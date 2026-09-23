package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/lifei6671/clipweaver/internal/domain"
)

var ErrInvalidID = errors.New("INVALID_ID")
var ErrCorruptManifest = errors.New("CORRUPT_MANIFEST")

const sourceName = "source.bin"

type Local struct {
	root string
}

// NewLocal initializes the data directory and discards interrupted staging work.
// The caller owns root; do not call it while uploads or mixes are active.
func NewLocal(root string) (*Local, error) {
	if root == "" {
		return nil, errors.New("data root is empty")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absolute, 0700); err != nil {
		return nil, fmt.Errorf("create data root: %w", err)
	}
	for _, path := range []string{"assets", "mixes", "tmp", filepath.Join("tmp", "uploads"), filepath.Join("tmp", "mixes")} {
		if err := ensureDirectory(filepath.Join(absolute, path)); err != nil {
			return nil, fmt.Errorf("create data directory %s: %w", path, err)
		}
	}
	store := &Local{root: absolute}
	for _, kind := range []string{"uploads", "mixes"} {
		stagingRoot := filepath.Join(absolute, "tmp", kind)
		entries, err := os.ReadDir(stagingRoot)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if err := os.RemoveAll(filepath.Join(stagingRoot, entry.Name())); err != nil {
				return nil, fmt.Errorf("clean %s staging: %w", kind, err)
			}
		}
	}
	return store, nil
}

func ensureDirectory(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("unsafe data directory %s", path)
	}
	return nil
}

func canonicalID(id string) (string, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return "", ErrInvalidID
	}
	return parsed.String(), nil
}

func NewAssetID() string { return uuid.NewString() }

func NewMixID() string { return uuid.NewString() }

func (s *Local) assetDir(id string) (string, string, error) {
	canonical, err := canonicalID(id)
	if err != nil {
		return "", "", err
	}
	dir := filepath.Join(s.root, "assets", canonical)
	if info, err := os.Lstat(dir); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", "", fmt.Errorf("unsafe asset directory %s", canonical)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}
	return canonical, dir, nil
}

// SourcePath returns the server-chosen final filename, independent of asset.Name.
func (s *Local) SourcePath(id string) (string, error) {
	_, dir, err := s.assetDir(id)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, sourceName)
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("unsafe source file for asset %s", id)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return path, nil
}

// NewUploadStaging creates a server-named directory for an upcoming upload.
func (s *Local) NewUploadStaging() (string, string, error) {
	return s.newStaging("uploads")
}

// NewMixStaging creates a server-named directory for an upcoming mix.
func (s *Local) NewMixStaging() (string, string, error) {
	return s.newStaging("mixes")
}

func (s *Local) newStaging(kind string) (string, string, error) {
	id := uuid.NewString()
	path := filepath.Join(s.root, "tmp", kind, id)
	if err := os.Mkdir(path, 0700); err != nil {
		return "", "", err
	}
	return id, path, nil
}

// PromoteUpload atomically moves a completed upload into its final asset directory.
func (s *Local) PromoteUpload(uploadID, assetID string) error {
	uploadID, err := canonicalID(uploadID)
	if err != nil {
		return err
	}
	_, finalDir, err := s.assetDir(assetID)
	if err != nil {
		return err
	}
	stageDir := filepath.Join(s.root, "tmp", "uploads", uploadID)
	if info, err := os.Lstat(stageDir); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("invalid upload staging: %v", err)
	}
	if _, err := os.Lstat(finalDir); err == nil {
		return fmt.Errorf("asset directory already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(stageDir, finalDir)
}

func (s *Local) RemoveUploadStaging(id string) error {
	id, err := canonicalID(id)
	if err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(s.root, "tmp", "uploads", id))
}

func (s *Local) RemoveAsset(id string) error {
	_, dir, err := s.assetDir(id)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

// ListAssets reads committed manifests from disk in stable ID order.
func (s *Local) ListAssets() ([]domain.Asset, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "assets"))
	if err != nil {
		return nil, err
	}
	assets := make([]domain.Asset, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		manifest := filepath.Join(s.root, "assets", id, "meta.json")
		if _, err := os.Lstat(manifest); errors.Is(err, os.ErrNotExist) {
			continue // Promotion not yet committed by SaveAsset.
		} else if err != nil {
			return nil, err
		}
		asset, err := s.ReadAsset(id)
		if err != nil {
			return nil, err
		}
		if info, err := os.Lstat(asset.StoredPath); err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("invalid asset source %s: %v", id, err)
		}
		assets = append(assets, asset)
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].ID < assets[j].ID })
	return assets, nil
}

type assetManifest struct {
	ID         string            `json:"id"`
	Kind       domain.AssetKind  `json:"kind"`
	Name       string            `json:"name"`
	DurationUS domain.DurationUS `json:"durationUS"`
	Width      int               `json:"width"`
	Height     int               `json:"height"`
	CreatedAt  time.Time         `json:"createdAt"`
}

// SaveAsset writes a complete manifest through a synced file in the same directory.
// StoredPath is intentionally excluded from the persisted representation.
func (s *Local) SaveAsset(asset domain.Asset) error {
	id, dir, err := s.assetDir(asset.ID)
	if err != nil {
		return err
	}
	if err := validateAsset(asset); err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if _, _, err := s.assetDir(id); err != nil {
		return err
	}
	manifest := assetManifest{
		ID: id, Kind: asset.Kind, Name: asset.Name,
		DurationUS: asset.DurationUS, Width: asset.Width, Height: asset.Height,
		CreatedAt: asset.CreatedAt,
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".meta-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, "meta.json"))
}

// ReadAsset rebuilds StoredPath from the store root and the validated UUID.
func (s *Local) ReadAsset(id string) (domain.Asset, error) {
	canonical, dir, err := s.assetDir(id)
	if err != nil {
		return domain.Asset{}, err
	}
	path := filepath.Join(dir, "meta.json")
	info, err := os.Lstat(path)
	if err != nil {
		return domain.Asset{}, err
	}
	if !info.Mode().IsRegular() {
		return domain.Asset{}, fmt.Errorf("unsafe manifest for asset %s", canonical)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return domain.Asset{}, err
	}
	var manifest assetManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return domain.Asset{}, err
	}
	manifestID, err := canonicalID(manifest.ID)
	if err != nil || manifestID != canonical {
		return domain.Asset{}, errors.Join(ErrCorruptManifest, ErrInvalidID)
	}
	asset := domain.Asset{
		ID: canonical, Kind: manifest.Kind, Name: manifest.Name,
		DurationUS: manifest.DurationUS, Width: manifest.Width, Height: manifest.Height,
		CreatedAt: manifest.CreatedAt,
	}
	if err := validateAsset(asset); err != nil {
		return domain.Asset{}, err
	}
	asset.StoredPath, err = s.SourcePath(canonical)
	return asset, err
}

func validateAsset(asset domain.Asset) error {
	if asset.Kind != domain.AssetKindVideo && asset.Kind != domain.AssetKindAudio {
		return fmt.Errorf("invalid asset kind %q", asset.Kind)
	}
	if asset.DurationUS <= 0 {
		return errors.New("asset duration must be positive")
	}
	if asset.Kind == domain.AssetKindVideo && (asset.Width <= 0 || asset.Height <= 0) {
		return errors.New("video dimensions must be positive")
	}
	return nil
}

func (s *Local) mixDir(id string) (string, string, error) {
	canonical, err := canonicalID(id)
	if err != nil {
		return "", "", err
	}
	dir := filepath.Join(s.root, "mixes", canonical)
	if info, err := os.Lstat(dir); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", "", fmt.Errorf("unsafe mix directory %s", canonical)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}
	return canonical, dir, nil
}

func (s *Local) SaveMixMeta(meta domain.MixMeta) error {
	id, dir, err := s.mixDir(meta.ID)
	if err != nil {
		return err
	}
	if meta.Status != domain.MixStatusRendering && meta.Status != domain.MixStatusCompleted && meta.Status != domain.MixStatusFailed {
		return errors.New("invalid mix status")
	}
	if err := os.Mkdir(dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if _, _, err := s.mixDir(id); err != nil {
		return err
	}
	meta.ID = id
	return atomicJSON(dir, "meta.json", meta)
}

func (s *Local) SaveMixPlan(id string, plan domain.MixPlan) error {
	_, dir, err := s.mixDir(id)
	if err != nil {
		return err
	}
	return atomicJSON(dir, "plan.json", plan)
}

func atomicJSON(dir, name string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+name+"-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, name))
}

func (s *Local) ReadMixMeta(id string) (domain.MixMeta, error) {
	canonical, dir, err := s.mixDir(id)
	if err != nil {
		return domain.MixMeta{}, err
	}
	var meta domain.MixMeta
	if err := readMixJSON(filepath.Join(dir, "meta.json"), &meta); err != nil {
		return domain.MixMeta{}, err
	}
	if meta.ID != canonical || (meta.Status != domain.MixStatusRendering && meta.Status != domain.MixStatusCompleted && meta.Status != domain.MixStatusFailed) {
		return domain.MixMeta{}, errors.New("invalid mix metadata")
	}
	return meta, nil
}

func (s *Local) ReadMixPlan(id string) (domain.MixPlan, error) {
	_, dir, err := s.mixDir(id)
	if err != nil {
		return domain.MixPlan{}, err
	}
	var plan domain.MixPlan
	if err := readMixJSON(filepath.Join(dir, "plan.json"), &plan); err != nil {
		return domain.MixPlan{}, err
	}
	if plan.TargetDurationUS <= 0 || plan.ClipDurationUS <= 0 || len(plan.Clips) == 0 {
		return domain.MixPlan{}, errors.New("invalid mix plan")
	}
	return plan, nil
}

func readMixJSON(path string, value any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("unsafe mix manifest")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func (s *Local) MixOutputPath(id string) (string, error) {
	_, dir, err := s.mixDir(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "output.mp4"), nil
}

func (s *Local) RemoveMixStaging(id string) error {
	id, err := canonicalID(id)
	if err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(s.root, "tmp", "mixes", id))
}

// OpenCompletedMixFile validates the persisted state and returns an open regular file.
// The caller must close the returned handle.
func (s *Local) OpenCompletedMixFile(id string) (*os.File, error) {
	meta, err := s.ReadMixMeta(id)
	if err != nil {
		return nil, err
	}
	if meta.Status != domain.MixStatusCompleted {
		return nil, os.ErrNotExist
	}
	path, err := s.MixOutputPath(id)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("unsafe mix output")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		file.Close()
		return nil, errors.New("unsafe mix output")
	}
	return file, nil
}
