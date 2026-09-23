package mixer

import (
	"errors"
	"fmt"
	"math/rand"
	"slices"

	"github.com/lifei6671/clipweaver/internal/domain"
)

const ClipDurationUS domain.DurationUS = 3_000_000

// Plan creates a deterministic sequence of non-overlapping source intervals.
func Plan(videos []domain.Asset, targetDurationUS, clipDurationUS domain.DurationUS, seed int64) (domain.MixPlan, error) {
	if len(videos) == 0 {
		return domain.MixPlan{}, errors.New("NO_VIDEO_SELECTED")
	}
	if targetDurationUS <= 0 || clipDurationUS <= 0 {
		return domain.MixPlan{}, errors.New("target and clip durations must be positive")
	}
	ordered := slices.Clone(videos)
	slices.SortStableFunc(ordered, func(a, b domain.Asset) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	var available domain.DurationUS
	for i, video := range ordered {
		if video.ID == "" || video.Kind != domain.AssetKindVideo || video.DurationUS <= 0 {
			return domain.MixPlan{}, fmt.Errorf("invalid video asset %q", video.ID)
		}
		if i > 0 && video.ID == ordered[i-1].ID {
			return domain.MixPlan{}, fmt.Errorf("duplicate video asset %q", video.ID)
		}
		if available < targetDurationUS {
			remaining := targetDurationUS - available
			if video.DurationUS >= remaining {
				available = targetDurationUS
			} else {
				available += video.DurationUS
			}
		}
	}
	if available < targetDurationUS {
		return domain.MixPlan{}, domain.ErrInsufficientVideoDuration
	}

	var candidates []domain.PlannedClip
	for _, video := range ordered {
		for start := domain.DurationUS(0); start < video.DurationUS; {
			duration := clipDurationUS
			if left := video.DurationUS - start; left < duration {
				duration = left
			}
			candidates = append(candidates, domain.PlannedClip{
				AssetID: video.ID, StartUS: start, DurationUS: duration,
			})
			start += duration
		}
	}
	rng := rand.New(rand.NewSource(seed))
	for i := len(candidates) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		candidates[i], candidates[j] = candidates[j], candidates[i]
	}
	plan := domain.MixPlan{Seed: seed, TargetDurationUS: targetDurationUS, ClipDurationUS: clipDurationUS}
	remaining := targetDurationUS
	for _, candidate := range candidates {
		if remaining == 0 {
			break
		}
		if candidate.DurationUS > remaining {
			candidate.DurationUS = remaining
		}
		plan.Clips = append(plan.Clips, candidate)
		remaining -= candidate.DurationUS
	}
	if err := plan.Validate(ordered); err != nil {
		return domain.MixPlan{}, fmt.Errorf("invalid generated plan: %w", err)
	}
	return plan, nil
}
