package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lifei6671/clipweaver/internal/domain"
	"github.com/lifei6671/clipweaver/internal/media"
	"github.com/lifei6671/clipweaver/internal/mixer"
	"github.com/lifei6671/clipweaver/internal/storage"
)

var (
	ErrNoVideoSelected   = errors.New("NO_VIDEO_SELECTED")
	ErrAudioRequired     = errors.New("AUDIO_REQUIRED")
	ErrInvalidMixRequest = errors.New("INVALID_REQUEST")
	ErrAssetNotFound     = errors.New("ASSET_NOT_FOUND")
	ErrMixNotFound       = errors.New("MIX_NOT_FOUND")
	ErrMixBusy           = errors.New("MIX_BUSY")
	ErrMixTimeout        = errors.New("MIX_TIMEOUT")
	ErrServiceClosed     = errors.New("MIX_SERVICE_CLOSED")
)

type InsufficientVideoError struct{ MissingDurationUS domain.DurationUS }

func (e *InsufficientVideoError) Error() string { return domain.ErrInsufficientVideoDuration.Error() }
func (e *InsufficientVideoError) Unwrap() error { return domain.ErrInsufficientVideoDuration }

type MixStore interface {
	ReadAsset(id string) (domain.Asset, error)
	NewMixStaging() (string, string, error)
	SaveMixMeta(domain.MixMeta) error
	SaveMixPlan(id string, plan domain.MixPlan) error
	MixOutputPath(id string) (string, error)
	RemoveMixStaging(id string) error
}

type MixExecutor interface {
	Execute(context.Context, domain.MixPlan, map[string]domain.Asset, domain.Asset, string, string) (media.OutputInfo, error)
}

type MixService struct {
	store     MixStore
	executor  MixExecutor
	seed      func() (int64, error)
	timeout   time.Duration
	semaphore chan struct{}
	root      context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
}

func NewMixService(store MixStore, executor MixExecutor, timeout time.Duration, maxConcurrent int, seedGenerator func() (int64, error)) *MixService {
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	if seedGenerator == nil {
		seedGenerator = randomSeed
	}
	root, cancel := context.WithCancel(context.Background())
	return &MixService{store: store, executor: executor, seed: seedGenerator, timeout: timeout, semaphore: make(chan struct{}, maxConcurrent), root: root, cancel: cancel}
}

func randomSeed() (int64, error) {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return 0, err
	}
	return int64(binary.LittleEndian.Uint64(bytes[:])), nil
}

func (s *MixService) Close() { s.closeOnce.Do(s.cancel) }

func validUUID(id string) bool { _, err := uuid.Parse(id); return err == nil }

func (s *MixService) Create(request domain.MixRequest) (meta domain.MixMeta, err error) {
	if len(request.VideoIDs) == 0 {
		return meta, ErrNoVideoSelected
	}
	if request.AudioID == "" {
		return meta, ErrAudioRequired
	}
	seen := make(map[string]bool, len(request.VideoIDs))
	for _, id := range request.VideoIDs {
		parsed, parseErr := uuid.Parse(id)
		if parseErr != nil {
			return meta, storage.ErrInvalidID
		}
		if seen[parsed.String()] {
			return meta, ErrInvalidMixRequest
		}
		seen[parsed.String()] = true
	}
	if !validUUID(request.AudioID) {
		return meta, storage.ErrInvalidID
	}
	if s.root.Err() != nil {
		return meta, ErrServiceClosed
	}
	select {
	case s.semaphore <- struct{}{}:
		defer func() { <-s.semaphore }()
	default:
		return meta, ErrMixBusy
	}
	ctx, cancel := context.WithTimeout(s.root, s.timeout)
	defer cancel()
	videos := make([]domain.Asset, 0, len(request.VideoIDs))
	videoMap := make(map[string]domain.Asset, len(request.VideoIDs))
	seenContent := make(map[string]bool, len(request.VideoIDs))
	videoIDs := make([]string, 0, len(request.VideoIDs))
	var available domain.DurationUS
	for _, id := range request.VideoIDs {
		asset, readErr := s.store.ReadAsset(id)
		if readErr != nil {
			return meta, assetReadError(readErr)
		}
		if asset.Kind != domain.AssetKindVideo {
			return meta, ErrInvalidVideo
		}
		digest, decodeErr := hex.DecodeString(asset.ContentSHA256)
		if decodeErr != nil || len(digest) != sha256.Size {
			return meta, fmt.Errorf("invalid video content digest for asset %s", asset.ID)
		}
		contentSHA256 := strings.ToLower(asset.ContentSHA256)
		if seenContent[contentSHA256] {
			continue
		}
		seenContent[contentSHA256] = true
		videos = append(videos, asset)
		videoIDs = append(videoIDs, asset.ID)
		videoMap[asset.ID] = asset
		if available <= domain.DurationUS(^uint64(0)>>1)-asset.DurationUS {
			available += asset.DurationUS
		} else {
			available = domain.DurationUS(^uint64(0) >> 1)
		}
	}
	audio, readErr := s.store.ReadAsset(request.AudioID)
	if readErr != nil {
		return meta, assetReadError(readErr)
	}
	if audio.Kind != domain.AssetKindAudio {
		return meta, ErrInvalidAudio
	}
	seed := int64(0)
	if request.Seed != nil {
		seed = *request.Seed
	} else if seed, err = s.seed(); err != nil {
		return meta, err
	}
	plan, planErr := mixer.Plan(videos, audio.DurationUS, mixer.ClipDurationUS, seed)
	if planErr != nil {
		if errors.Is(planErr, domain.ErrInsufficientVideoDuration) {
			return meta, &InsufficientVideoError{MissingDurationUS: audio.DurationUS - available}
		}
		return meta, planErr
	}
	if err := renderContextError(ctx); err != nil {
		return meta, err
	}
	id, stageDir, stageErr := s.store.NewMixStaging()
	if stageErr != nil {
		return meta, stageErr
	}
	meta = domain.MixMeta{ID: id, Status: domain.MixStatusRendering, Seed: seed, DurationUS: audio.DurationUS, VideoIDs: videoIDs, AudioID: request.AudioID, CreatedAt: time.Now().UTC()}
	defer func() {
		if err != nil {
			meta.Status = domain.MixStatusFailed
			meta.ErrorCode = ErrorCode(err)
			if saveErr := s.store.SaveMixMeta(meta); saveErr != nil {
				err = errors.Join(err, fmt.Errorf("save failed mix state: %w", saveErr))
			}
			if outputPath, pathErr := s.store.MixOutputPath(id); pathErr == nil {
				if removeErr := os.Remove(outputPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
					err = errors.Join(err, removeErr)
				}
			}
		}
		if cleanupErr := s.store.RemoveMixStaging(id); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("%w: %v", ErrCleanup, cleanupErr))
		}
	}()
	if err = s.store.SaveMixMeta(meta); err != nil {
		return meta, err
	}
	if err = s.store.SaveMixPlan(id, plan); err != nil {
		return meta, err
	}
	outputPath, pathErr := s.store.MixOutputPath(id)
	if pathErr != nil {
		return meta, pathErr
	}
	outputInfo, executeErr := s.executor.Execute(ctx, plan, videoMap, audio, stageDir, outputPath)
	err = executeErr
	if contextErr := renderContextError(ctx); contextErr != nil {
		err = contextErr
	} else if errors.Is(err, context.DeadlineExceeded) {
		err = ErrMixTimeout
	}
	if err != nil {
		return meta, err
	}
	if outputInfo.VideoDurationUS <= 0 || outputInfo.AudioDurationUS <= 0 || outputInfo.FormatDurationUS <= 0 {
		err = media.ErrRenderValidationFailed
		return meta, err
	}
	info, statErr := os.Lstat(outputPath)
	if statErr != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		err = media.ErrRenderValidationFailed
		return meta, err
	}
	if err = s.store.RemoveMixStaging(id); err != nil {
		return meta, err
	}
	if err = renderContextError(ctx); err != nil {
		return meta, err
	}
	meta.Status = domain.MixStatusCompleted
	if err = s.store.SaveMixMeta(meta); err != nil {
		return meta, err
	}
	if err = renderContextError(ctx); err != nil {
		return meta, err
	}
	return meta, nil
}

func assetReadError(err error) error {
	if errors.Is(err, storage.ErrVideoDigest) || errors.Is(err, storage.ErrCorruptManifest) {
		return err
	}
	if errors.Is(err, os.ErrNotExist) {
		return ErrAssetNotFound
	}
	return err
}

func renderContextError(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrMixTimeout
	}
	if ctx.Err() != nil {
		return ErrServiceClosed
	}
	return nil
}

func ErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrMixTimeout):
		return "MIX_TIMEOUT"
	case errors.Is(err, ErrServiceClosed):
		return "INTERNAL_ERROR"
	case errors.Is(err, media.ErrFFmpegFailed):
		return "FFMPEG_FAILED"
	case errors.Is(err, media.ErrProbeFailed), errors.Is(err, media.ErrProbeTimeout), errors.Is(err, media.ErrProbeCanceled), errors.Is(err, media.ErrInvalidJSON):
		return "FFPROBE_FAILED"
	case errors.Is(err, media.ErrRenderValidationFailed):
		return "RENDER_VALIDATION_FAILED"
	default:
		return "INTERNAL_ERROR"
	}
}
