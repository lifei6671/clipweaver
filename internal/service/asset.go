package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
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
	ReadAsset(id string) (domain.Asset, error)
	ListAssets() ([]domain.Asset, error)
	SourcePath(id string) (string, error)
	PosterPath(id string) (string, error)
	OpenPoster(id string) (*os.File, error)
}

type AssetProber interface {
	ProbeVideo(context.Context, string) (media.Result, error)
	ProbeAudio(context.Context, string) (media.Result, error)
}

type AssetPoster interface {
	Generate(context.Context, string, string, domain.DurationUS) error
}

type AssetService struct {
	videoMu  sync.Mutex
	posterMu sync.Mutex
	store    AssetStore
	prober   AssetProber
	poster   AssetPoster
	logger   *slog.Logger
}

func NewAssetService(store AssetStore, prober AssetProber, poster AssetPoster, logger *slog.Logger) *AssetService {
	return &AssetService{store: store, prober: prober, poster: poster, logger: logger}
}

func (s *AssetService) UploadVideo(ctx context.Context, name string, source io.Reader) (domain.Asset, error) {
	return s.upload(ctx, domain.AssetKindVideo, name, source)
}

func (s *AssetService) UploadAudio(ctx context.Context, name string, source io.Reader) (domain.Asset, error) {
	return s.upload(ctx, domain.AssetKindAudio, name, source)
}

func (s *AssetService) ListAssets() ([]domain.Asset, error) {
	s.videoMu.Lock()
	defer s.videoMu.Unlock()
	assets, err := s.store.ListAssets()
	if err != nil {
		return nil, err
	}
	canonical := make(map[string]domain.Asset)
	for _, asset := range assets {
		if asset.Kind == domain.AssetKindVideo {
			if asset.ContentSHA256 == "" {
				return nil, fmt.Errorf("video content digest missing for asset %s", asset.ID)
			}
			if prior, ok := canonical[asset.ContentSHA256]; !ok || earlierAsset(asset, prior) {
				canonical[asset.ContentSHA256] = asset
			}
		}
	}
	visible := make([]domain.Asset, 0, len(assets))
	for _, asset := range assets {
		if asset.Kind == domain.AssetKindVideo && canonical[asset.ContentSHA256].ID != asset.ID {
			s.logger.Info("hiding duplicate video asset", "assetId", asset.ID, "canonicalId", canonical[asset.ContentSHA256].ID)
			continue
		}
		visible = append(visible, asset)
	}
	for i := range visible {
		if visible[i].Kind == domain.AssetKindVideo && !visible[i].HasPoster {
			s.backfillPoster(context.Background(), &visible[i])
		}
	}
	return visible, nil
}

func (s *AssetService) DeleteAsset(id string) (string, error) {
	asset, err := s.store.ReadAsset(id)
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrAssetNotFound
	}
	if err != nil {
		return "", err
	}
	if asset.Kind == domain.AssetKindAudio {
		if err := s.store.RemoveAsset(asset.ID); err != nil {
			return "", err
		}
		return asset.ID, nil
	}
	s.videoMu.Lock()
	defer s.videoMu.Unlock()
	asset, err = s.store.ReadAsset(id)
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrAssetNotFound
	}
	if err != nil {
		return "", err
	}
	if asset.Kind != domain.AssetKindVideo {
		return "", ErrInvalidMixRequest
	}
	assets, err := s.store.ListAssets()
	if err != nil {
		return "", err
	}
	for _, candidate := range assets {
		if candidate.Kind != domain.AssetKindVideo || candidate.ContentSHA256 != asset.ContentSHA256 {
			continue
		}
		if err := s.store.RemoveAsset(candidate.ID); err != nil {
			s.logger.Error("delete video asset group failed", "assetId", asset.ID, "failedId", candidate.ID, "error", err)
			return "", fmt.Errorf("delete video asset %s: %w", candidate.ID, err)
		}
	}
	return asset.ID, nil
}

func (s *AssetService) backfillPoster(ctx context.Context, asset *domain.Asset) {
	s.posterMu.Lock()
	defer s.posterMu.Unlock()
	if file, err := s.store.OpenPoster(asset.ID); err == nil {
		file.Close()
		asset.HasPoster = true
		return
	}
	path, err := s.store.PosterPath(asset.ID)
	if err != nil {
		s.logger.Warn("video poster path unavailable", "assetId", asset.ID, "error", err)
		return
	}
	if err := s.poster.Generate(ctx, asset.StoredPath, path, asset.DurationUS); err != nil {
		s.logger.Warn("video poster generation failed", "assetId", asset.ID, "error", err)
		return
	}
	asset.HasPoster = true
}

func earlierAsset(a, b domain.Asset) bool {
	return a.CreatedAt.Before(b.CreatedAt) || (a.CreatedAt.Equal(b.CreatedAt) && a.ID < b.ID)
}

func (s *AssetService) OpenPoster(id string) (*os.File, error) {
	return s.store.OpenPoster(id)
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
	var hash hash.Hash
	var target io.Writer = file
	if kind == domain.AssetKindVideo {
		hash = sha256.New()
		target = io.MultiWriter(file, hash)
	}
	_, copyErr := io.Copy(target, source)
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
		asset.ContentSHA256 = hex.EncodeToString(hash.Sum(nil))
		s.videoMu.Lock()
		defer s.videoMu.Unlock()
		existing, listErr := s.store.ListAssets()
		if listErr != nil {
			return domain.Asset{}, fmt.Errorf("compare video content: %w", listErr)
		}
		var canonical domain.Asset
		for _, candidate := range existing {
			if candidate.Kind == domain.AssetKindVideo && candidate.ContentSHA256 == "" {
				return domain.Asset{}, fmt.Errorf("video content digest missing for asset %s", candidate.ID)
			}
			if candidate.Kind == domain.AssetKindVideo && candidate.ContentSHA256 == asset.ContentSHA256 &&
				(canonical.ID == "" || earlierAsset(candidate, canonical)) {
				canonical = candidate
			}
		}
		if canonical.ID != "" {
			if !canonical.HasPoster {
				s.backfillPoster(ctx, &canonical)
			}
			return canonical, nil
		}
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
	if kind == domain.AssetKindVideo {
		posterPath, pathErr := s.store.PosterPath(asset.ID)
		if pathErr != nil {
			s.logger.Warn("video poster path unavailable", "assetId", asset.ID, "error", pathErr)
		} else if posterErr := s.poster.Generate(ctx, asset.StoredPath, posterPath, asset.DurationUS); posterErr != nil {
			var ffmpegErr *media.FFmpegError
			if errors.As(posterErr, &ffmpegErr) {
				s.logger.Warn("video poster generation failed", "assetId", asset.ID,
					"cause", ffmpegErr.Cause, "stderr", ffmpegErr.Stderr)
			} else {
				s.logger.Warn("video poster generation failed", "assetId", asset.ID, "error", posterErr)
			}
		} else {
			asset.HasPoster = true
		}
	}
	if err = s.store.SaveAsset(asset); err != nil {
		return domain.Asset{}, err
	}
	return asset, nil
}
