package mixer

import (
	"errors"
	"reflect"
	"testing"

	"github.com/lifei6671/clipweaver/internal/domain"
)

func video(id string, duration domain.DurationUS) domain.Asset {
	return domain.Asset{ID: id, Kind: domain.AssetKindVideo, DurationUS: duration}
}

func mustPlan(t *testing.T, videos []domain.Asset, target domain.DurationUS, seed int64) domain.MixPlan {
	t.Helper()
	plan, err := Plan(videos, target, ClipDurationUS, seed)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestPlan_NoOverlapPerVideo(t *testing.T) {
	videos := []domain.Asset{video("b", 10_000_000), video("a", 8_000_000)}
	plan := mustPlan(t, videos, 17_000_000, 7)
	if err := plan.Validate(videos); err != nil {
		t.Fatal(err)
	}
	for i, a := range plan.Clips {
		for _, b := range plan.Clips[i+1:] {
			if a.AssetID != b.AssetID {
				continue
			}
			if !(a.StartUS+a.DurationUS <= b.StartUS || b.StartUS+b.DurationUS <= a.StartUS) {
				t.Fatalf("overlapping clips: %+v and %+v", a, b)
			}
		}
	}
}

func TestPlan_SameAssetCanProduceMultipleClips(t *testing.T) {
	plan := mustPlan(t, []domain.Asset{video("one", 9_000_000)}, 7_000_000, 3)
	if len(plan.Clips) < 2 {
		t.Fatalf("expected multiple clips from the same asset, got %+v", plan.Clips)
	}
	for _, clip := range plan.Clips {
		if clip.AssetID != "one" {
			t.Fatalf("unexpected asset: %+v", clip)
		}
	}
}

func TestPlan_TruncatesLastClip(t *testing.T) {
	plan := mustPlan(t, []domain.Asset{video("one", 9_000_000)}, 7_000_000, 3)
	if len(plan.Clips) != 3 || plan.Clips[2].DurationUS != 1_000_000 {
		t.Fatalf("expected 3 clips with a 1-second final clip, got %+v", plan.Clips)
	}
}

func TestPlan_TotalDurationEqualsTarget(t *testing.T) {
	for _, target := range []domain.DurationUS{1, 3_000_000, 7_000_000, 10_000_000} {
		plan := mustPlan(t, []domain.Asset{video("one", 10_000_000)}, target, 4)
		var total domain.DurationUS
		for _, clip := range plan.Clips {
			if clip.DurationUS <= 0 {
				t.Fatalf("zero-length clip: %+v", clip)
			}
			total += clip.DurationUS
		}
		if total != target {
			t.Fatalf("target %d, total %d", target, total)
		}
	}
}

func TestPlan_InsufficientDuration(t *testing.T) {
	plan, err := Plan([]domain.Asset{video("one", 3_000_000), video("two", 2_000_000)}, 5_000_001, ClipDurationUS, 9)
	if err == nil || !errors.Is(err, domain.ErrInsufficientVideoDuration) || err.Error() != "INSUFFICIENT_VIDEO_DURATION" {
		t.Fatalf("expected stable insufficient-duration error, got %v", err)
	}
	if len(plan.Clips) != 0 {
		t.Fatalf("insufficient duration returned clips: %+v", plan.Clips)
	}
}

func TestPlan_SameSeedIsDeterministic(t *testing.T) {
	videos := []domain.Asset{video("b", 9_000_000), video("a", 10_000_000)}
	a := mustPlan(t, videos, 12_000_000, 42)
	b := mustPlan(t, videos, 12_000_000, 42)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("same inputs and seed differed: %+v vs %+v", a, b)
	}
	reversed := []domain.Asset{videos[1], videos[0]}
	c := mustPlan(t, reversed, 12_000_000, 42)
	if !reflect.DeepEqual(a, c) {
		t.Fatalf("input asset order changed plan: %+v vs %+v", a, c)
	}
}

func TestPlan_DifferentSeedCanChangeOrder(t *testing.T) {
	videos := []domain.Asset{video("one", 12_000_000)} // four candidates
	a := mustPlan(t, videos, 12_000_000, 1)
	b := mustPlan(t, videos, 12_000_000, 2)
	if reflect.DeepEqual(a.Clips, b.Clips) {
		t.Fatalf("prevalidated seeds 1 and 2 unexpectedly produced the same order: %+v", a.Clips)
	}
}

func TestPlan_TailCandidateAndMicrosecond(t *testing.T) {
	plan := mustPlan(t, []domain.Asset{video("one", 3_000_001)}, 3_000_001, 5)
	if len(plan.Clips) != 2 {
		t.Fatalf("expected 3-second and 1-microsecond candidates: %+v", plan.Clips)
	}
	var oneMicrosecond bool
	for _, clip := range plan.Clips {
		if clip.DurationUS == 1 {
			oneMicrosecond = true
		}
	}
	if !oneMicrosecond {
		t.Fatalf("missing positive tail candidate: %+v", plan.Clips)
	}
}

func TestPlan_InvalidInputs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		videos []domain.Asset
		target domain.DurationUS
		clip   domain.DurationUS
	}{
		{"no videos", nil, 1, ClipDurationUS},
		{"zero target", []domain.Asset{video("one", 1)}, 0, ClipDurationUS},
		{"negative target", []domain.Asset{video("one", 1)}, -1, ClipDurationUS},
		{"zero clip", []domain.Asset{video("one", 1)}, 1, 0},
		{"negative clip", []domain.Asset{video("one", 1)}, 1, -1},
		{"duplicate asset ID", []domain.Asset{video("one", 1), video("one", 1)}, 1, ClipDurationUS},
		{"invalid asset duration", []domain.Asset{video("one", 0)}, 1, ClipDurationUS},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Plan(tc.videos, tc.target, tc.clip, 1); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestMixPlan_ValidateRejectsForgedPlans(t *testing.T) {
	assets := []domain.Asset{video("one", 6_000_000)}
	base := domain.MixPlan{
		TargetDurationUS: 4_000_000,
		ClipDurationUS:   ClipDurationUS,
		Clips: []domain.PlannedClip{
			{AssetID: "one", StartUS: 0, DurationUS: 3_000_000},
			{AssetID: "one", StartUS: 3_000_000, DurationUS: 1_000_000},
		},
	}
	if err := base.Validate(assets); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func(*domain.MixPlan)
	}{
		{"overlap and duplicate candidate", func(p *domain.MixPlan) { p.Clips[1].StartUS = 0 }},
		{"unaligned overlapping interval", func(p *domain.MixPlan) { p.Clips[1].StartUS = 2_000_000 }},
		{"out of bounds", func(p *domain.MixPlan) { p.Clips[1].StartUS = 6_000_000 }},
		{"wrong total", func(p *domain.MixPlan) { p.TargetDurationUS = 5_000_000 }},
		{"zero clip", func(p *domain.MixPlan) { p.Clips[1].DurationUS = 0 }},
		{"early truncation", func(p *domain.MixPlan) { p.Clips[0].DurationUS = 2_000_000 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := base
			plan.Clips = append([]domain.PlannedClip(nil), base.Clips...)
			tc.edit(&plan)
			if err := plan.Validate(assets); err == nil {
				t.Fatal("forged plan passed validation")
			}
		})
	}
}
