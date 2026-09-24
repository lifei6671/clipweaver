package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lifei6671/clipweaver/internal/domain"
	"github.com/lifei6671/clipweaver/internal/media"
	"github.com/lifei6671/clipweaver/internal/storage"
)

type executorFunc func(context.Context, domain.MixPlan, map[string]domain.Asset, domain.Asset, string, string) (media.OutputInfo, error)

func (f executorFunc) Execute(ctx context.Context, plan domain.MixPlan, videos map[string]domain.Asset, audio domain.Asset, stage, output string) (media.OutputInfo, error) {
	return f(ctx, plan, videos, audio, stage, output)
}

func mixAssets(t *testing.T, videoDuration, audioDuration domain.DurationUS) (*storage.Local, domain.MixRequest) {
	t.Helper()
	store, err := storage.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	video := domain.Asset{ID: storage.NewAssetID(), Kind: domain.AssetKindVideo, DurationUS: videoDuration, Width: 1920, Height: 1080}
	audio := domain.Asset{ID: storage.NewAssetID(), Kind: domain.AssetKindAudio, DurationUS: audioDuration}
	for _, asset := range []domain.Asset{video, audio} {
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
	return store, domain.MixRequest{VideoIDs: []string{video.ID}, AudioID: audio.ID}
}

func goodExecutor(t *testing.T, inspect func(domain.MixPlan, string, string)) executorFunc {
	t.Helper()
	return func(_ context.Context, plan domain.MixPlan, _ map[string]domain.Asset, _ domain.Asset, stage, output string) (media.OutputInfo, error) {
		if inspect != nil {
			inspect(plan, stage, output)
		}
		if err := os.WriteFile(output, []byte("0123456789"), 0600); err != nil {
			return media.OutputInfo{}, err
		}
		return media.OutputInfo{VideoDurationUS: plan.TargetDurationUS, AudioDurationUS: plan.TargetDurationUS, FormatDurationUS: plan.TargetDurationUS}, nil
	}
}

type countedStore struct {
	*storage.Local
	reads atomic.Int32
}

func (s *countedStore) ReadAsset(id string) (domain.Asset, error) {
	s.reads.Add(1)
	return s.Local.ReadAsset(id)
}

func TestMixValidationAndDeterminism(t *testing.T) {
	store, req := mixAssets(t, 12_000_000, 9_700_000)
	counted := &countedStore{Local: store}
	seed := int64(9007199254740993)
	req.Seed = &seed
	var plans []domain.MixPlan
	executor := goodExecutor(t, func(plan domain.MixPlan, stage, output string) {
		meta, err := store.ReadMixMeta(filepath.Base(stage))
		if err != nil || meta.Status != domain.MixStatusRendering || meta.Seed != seed {
			t.Fatalf("meta before render=%+v err=%v", meta, err)
		}
		persisted, err := store.ReadMixPlan(meta.ID)
		if err != nil || !reflect.DeepEqual(persisted, plan) {
			t.Fatalf("plan before render=%+v err=%v", persisted, err)
		}
		plans = append(plans, plan)
	})
	svc := NewMixService(counted, executor, time.Second, 1, nil)
	defer svc.Close()
	for _, invalid := range []domain.MixRequest{{}, {VideoIDs: req.VideoIDs}, {VideoIDs: []string{"../bad"}, AudioID: req.AudioID}, {VideoIDs: req.VideoIDs, AudioID: "../bad"}} {
		_, err := svc.Create(invalid)
		if err == nil {
			t.Fatalf("accepted %+v", invalid)
		}
	}
	if counted.reads.Load() != 0 {
		t.Fatal("invalid requests read storage")
	}
	for i := 0; i < 2; i++ {
		meta, err := svc.Create(req)
		if err != nil || meta.Status != domain.MixStatusCompleted || meta.Seed != seed {
			t.Fatalf("mix=%+v err=%v", meta, err)
		}
		stored, err := store.ReadMixMeta(meta.ID)
		if err != nil || stored.Status != domain.MixStatusCompleted {
			t.Fatalf("stored=%+v err=%v", stored, err)
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(mustOutput(t, store, meta.ID)))), "tmp", "mixes", meta.ID)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("staging remains: %v", err)
		}
	}
	if len(plans) != 2 || !reflect.DeepEqual(plans[0], plans[1]) {
		t.Fatalf("plans differ: %+v", plans)
	}
}

func mustOutput(t *testing.T, store *storage.Local, id string) string {
	t.Helper()
	path, err := store.MixOutputPath(id)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMixMissingAssetsAndDuration(t *testing.T) {
	store, req := mixAssets(t, 5_000_000, 9_700_000)
	svc := NewMixService(store, goodExecutor(t, nil), time.Second, 1, nil)
	defer svc.Close()
	_, err := svc.Create(req)
	var shortage *InsufficientVideoError
	if !errors.As(err, &shortage) || shortage.MissingDurationUS != 4_700_000 {
		t.Fatalf("shortage=%v", err)
	}
	req.VideoIDs[0] = storage.NewAssetID()
	if _, err := svc.Create(req); !errors.Is(err, ErrAssetNotFound) {
		t.Fatalf("missing=%v", err)
	}
	req.VideoIDs[0], req.AudioID = req.AudioID, req.VideoIDs[0]
	if _, err := svc.Create(req); !errors.Is(err, ErrInvalidVideo) {
		t.Fatalf("video kind=%v", err)
	}
}

func TestMixCollapsesHistoricalDuplicateVideoIDs(t *testing.T) {
	for _, tc := range []struct {
		name          string
		audioDuration domain.DurationUS
		shortage      domain.DurationUS
	}{
		{name: "insufficient after dedupe", audioDuration: 8_000_000, shortage: 3_000_000},
		{name: "one source reaches planner", audioDuration: 4_000_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, req := mixAssets(t, 5_000_000, tc.audioDuration)
			original, err := store.ReadAsset(req.VideoIDs[0])
			if err != nil {
				t.Fatal(err)
			}
			duplicate := original
			duplicate.ID = storage.NewAssetID()
			duplicate.ContentSHA256 = "" // A historical manifest with no digest.
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
			req.VideoIDs = []string{duplicate.ID, original.ID}
			called := false
			executor := executorFunc(func(ctx context.Context, plan domain.MixPlan, videos map[string]domain.Asset, audio domain.Asset, stage, output string) (media.OutputInfo, error) {
				called = true
				if len(videos) != 1 || videos[duplicate.ID].ID != duplicate.ID || len(plan.Clips) == 0 {
					t.Fatalf("executor received duplicate videos: map=%+v plan=%+v", videos, plan)
				}
				for _, clip := range plan.Clips {
					if clip.AssetID != duplicate.ID {
						t.Fatalf("duplicate content in plan: %+v", plan)
					}
				}
				return goodExecutor(t, nil)(ctx, plan, videos, audio, stage, output)
			})
			svc := NewMixService(store, executor, time.Second, 1, nil)
			defer svc.Close()
			meta, err := svc.Create(req)
			if tc.shortage != 0 {
				var shortage *InsufficientVideoError
				if !errors.As(err, &shortage) || shortage.MissingDurationUS != tc.shortage || called {
					t.Fatalf("shortage = %v; executor called = %v", err, called)
				}
				return
			}
			if err != nil || !called || !reflect.DeepEqual(meta.VideoIDs, []string{duplicate.ID}) {
				t.Fatalf("deduped mix = %+v, %v; executor called = %v", meta, err, called)
			}
			stored, err := store.ReadMixMeta(meta.ID)
			if err != nil || !reflect.DeepEqual(stored.VideoIDs, []string{duplicate.ID}) {
				t.Fatalf("persisted meta = %+v, %v", stored, err)
			}
			plan, err := store.ReadMixPlan(meta.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, clip := range plan.Clips {
				if clip.AssetID != duplicate.ID {
					t.Fatalf("persisted plan uses second duplicate: %+v", plan)
				}
			}
		})
	}
}

func TestMixBusyTimeoutAndShutdown(t *testing.T) {
	store, req := mixAssets(t, 12_000_000, 9_700_000)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var calls atomic.Int32
	executor := executorFunc(func(ctx context.Context, plan domain.MixPlan, videos map[string]domain.Asset, audio domain.Asset, stage, output string) (media.OutputInfo, error) {
		calls.Add(1)
		started <- struct{}{}
		select {
		case <-ctx.Done():
			return media.OutputInfo{}, &media.FFmpegError{Cause: ctx.Err(), Stderr: "secret"}
		case <-release:
			return goodExecutor(t, nil)(ctx, plan, videos, audio, stage, output)
		}
	})
	svc := NewMixService(store, executor, time.Second, 1, nil)
	defer svc.Close()
	first := make(chan error, 1)
	go func() { _, err := svc.Create(req); first <- err }()
	<-started
	if _, err := svc.Create(req); !errors.Is(err, ErrMixBusy) || calls.Load() != 1 {
		t.Fatalf("busy=%v calls=%d", err, calls.Load())
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(req); err != nil {
		t.Fatalf("token leaked: %v", err)
	}
	<-started

	blocked := executorFunc(func(ctx context.Context, _ domain.MixPlan, _ map[string]domain.Asset, _ domain.Asset, _, _ string) (media.OutputInfo, error) {
		started <- struct{}{}
		<-ctx.Done()
		return media.OutputInfo{}, &media.FFmpegError{Cause: ctx.Err(), Stderr: "secret"}
	})
	timed := NewMixService(store, blocked, 20*time.Millisecond, 1, nil)
	_, err := timed.Create(req)
	if !errors.Is(err, ErrMixTimeout) {
		t.Fatalf("timeout=%v", err)
	}
	<-started
	timed.Close()
	closing := NewMixService(store, blocked, time.Second, 1, nil)
	finished := make(chan error, 1)
	go func() { _, err := closing.Create(req); finished <- err }()
	<-started
	closing.Close()
	if err := <-finished; !errors.Is(err, ErrServiceClosed) {
		t.Fatalf("shutdown=%v", err)
	}
}

func TestMixFailedStateAndCleanup(t *testing.T) {
	store, req := mixAssets(t, 12_000_000, 9_700_000)
	for _, failure := range []error{&media.FFmpegError{Stderr: "secret", Cause: errors.New("broken")}, media.ErrRenderValidationFailed} {
		var id string
		executor := executorFunc(func(_ context.Context, _ domain.MixPlan, _ map[string]domain.Asset, _ domain.Asset, stage, output string) (media.OutputInfo, error) {
			id = filepath.Base(stage)
			if err := os.WriteFile(output, []byte("partial"), 0600); err != nil {
				t.Fatal(err)
			}
			return media.OutputInfo{}, failure
		})
		svc := NewMixService(store, executor, time.Second, 1, nil)
		_, err := svc.Create(req)
		if !errors.Is(err, failure) {
			t.Fatalf("failure=%v", err)
		}
		if _, retryErr := svc.Create(req); !errors.Is(retryErr, failure) {
			t.Fatalf("failed request held token: %v", retryErr)
		}
		svc.Close()
		meta, err := store.ReadMixMeta(id)
		if err != nil || meta.Status != domain.MixStatusFailed || meta.ErrorCode != ErrorCode(failure) {
			t.Fatalf("failed meta=%+v err=%v", meta, err)
		}
		if _, err := store.ReadMixPlan(id); err != nil {
			t.Fatalf("lost plan: %v", err)
		}
		if _, err := os.Stat(mustOutput(t, store, id)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("partial output remains: %v", err)
		}
	}
}
