package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/lifei6671/clipweaver/internal/domain"
	"github.com/lifei6671/clipweaver/internal/media"
	"github.com/lifei6671/clipweaver/internal/storage"
)

var (
	ErrInvalidVideo = errors.New("INVALID_VIDEO")
	ErrInvalidAudio = errors.New("INVALID_AUDIO")
	ErrCleanup      = errors.New("ASSET_CLEANUP_FAILED")
)

type AssetStore interface {
	NewUploadStaging() (string, string, error)
	PromoteUpload(uploadID, assetID string) error
	RemoveUploadStaging(id string) error
	RemoveAsset(id string) error
	SaveAsset(asset domain.Asset) error
	ListAssets() ([]domain.Asset, error)
	SourcePath(id string) (string, error)
}

type AssetProber interface {
	ProbeVideo(context.Context, string) (media.Result, error)
	ProbeAudio(context.Context, string) (media.Result, error)
}

type AssetService struct {
	store  AssetStore
	prober AssetProber
}

func NewAssetService(store AssetStore, prober AssetProber) *AssetService {
	return &AssetService{store: store, prober: prober}
}

func (s *AssetService) UploadVideo(ctx context.Context, name string, source io.Reader) (domain.Asset, error) {
	return s.upload(ctx, domain.AssetKindVideo, name, source)
}

func (s *AssetService) UploadAudio(ctx context.Context, name string, source io.Reader) (domain.Asset, error) {
	return s.upload(ctx, domain.AssetKindAudio, name, source)
}

func (s *AssetService) ListAssets() ([]domain.Asset, error) {
	return s.store.ListAssets()
}

func (s *AssetService) upload(ctx context.Context, kind domain.AssetKind, name string, source io.Reader) (asset domain.Asset, err error) {
	uploadID, stageDir, err := s.store.NewUploadStaging()
	if err != nil {
		return domain.Asset{}, err
	}
	promoted := false
	defer func() {
		if promoted {
			return
		}
		if cleanupErr := s.store.RemoveUploadStaging(uploadID); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("%w: %v", ErrCleanup, cleanupErr))
		}
	}()
	stagePath := filepath.Join(stageDir, "source.bin")
	file, err := os.OpenFile(stagePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return domain.Asset{}, err
	}
	_, copyErr := io.Copy(file, source)
	closeErr := file.Close()
	if copyErr != nil {
		return domain.Asset{}, copyErr
	}
	if closeErr != nil {
		return domain.Asset{}, closeErr
	}

	var result media.Result
	if kind == domain.AssetKindVideo {
		result, err = s.prober.ProbeVideo(ctx, stagePath)
	} else {
		result, err = s.prober.ProbeAudio(ctx, stagePath)
	}
	if err != nil {
		if errors.Is(err, media.ErrMissingStream) || errors.Is(err, media.ErrInvalidDuration) || errors.Is(err, media.ErrInvalidDimensions) {
			if kind == domain.AssetKindVideo {
				return domain.Asset{}, fmt.Errorf("%w: %w", ErrInvalidVideo, err)
			}
			return domain.Asset{}, fmt.Errorf("%w: %w", ErrInvalidAudio, err)
		}
		return domain.Asset{}, err
	}
	if result.DurationUS <= 0 || (kind == domain.AssetKindVideo && (result.Width <= 0 || result.Height <= 0)) {
		if kind == domain.AssetKindVideo {
			return domain.Asset{}, ErrInvalidVideo
		}
		return domain.Asset{}, ErrInvalidAudio
	}
	asset = domain.Asset{
		ID: storage.NewAssetID(), Kind: kind, Name: name,
		DurationUS: result.DurationUS, CreatedAt: time.Now().UTC(),
	}
	if kind == domain.AssetKindVideo {
		asset.Width, asset.Height = result.Width, result.Height
	}
	if err = s.store.PromoteUpload(uploadID, asset.ID); err != nil {
		return domain.Asset{}, err
	}
	promoted = true
	finalID := asset.ID
	defer func() {
		if err != nil {
			if cleanupErr := s.store.RemoveAsset(finalID); cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("%w: %v", ErrCleanup, cleanupErr))
			}
		}
	}()
	asset.StoredPath, err = s.store.SourcePath(asset.ID)
	if err != nil {
		return domain.Asset{}, err
	}
	if err = s.store.SaveAsset(asset); err != nil {
		return domain.Asset{}, err
	}
	return asset, nil
}
