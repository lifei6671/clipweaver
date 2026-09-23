package domain

import (
	"errors"
	"fmt"
	"time"
)

var ErrInsufficientVideoDuration = errors.New("INSUFFICIENT_VIDEO_DURATION")

type MixRequest struct {
	VideoIDs []string
	AudioID  string
	Seed     *int64
}

type MixStatus string

const (
	MixStatusRendering MixStatus = "rendering"
	MixStatusCompleted MixStatus = "completed"
	MixStatusFailed    MixStatus = "failed"
)

type MixMeta struct {
	ID         string     `json:"id"`
	Status     MixStatus  `json:"status"`
	Seed       int64      `json:"seed"`
	DurationUS DurationUS `json:"durationUs"`
	VideoIDs   []string   `json:"videoIds"`
	AudioID    string     `json:"audioId"`
	CreatedAt  time.Time  `json:"createdAt"`
	ErrorCode  string     `json:"errorCode,omitempty"`
}

// PlannedClip uses the half-open source interval [StartUS, StartUS+DurationUS).
type PlannedClip struct {
	AssetID    string
	StartUS    DurationUS
	DurationUS DurationUS
}

type MixPlan struct {
	Seed             int64
	TargetDurationUS DurationUS
	ClipDurationUS   DurationUS
	Clips            []PlannedClip
}

// Validate checks the plan against the source assets used to construct it.
func (plan MixPlan) Validate(assets []Asset) error {
	if plan.TargetDurationUS <= 0 || plan.ClipDurationUS <= 0 || len(plan.Clips) == 0 {
		return errors.New("invalid plan duration or empty clips")
	}
	durations := make(map[string]DurationUS, len(assets))
	for _, asset := range assets {
		if asset.ID == "" || asset.Kind != AssetKindVideo || asset.DurationUS <= 0 {
			return fmt.Errorf("invalid video asset %q", asset.ID)
		}
		if _, exists := durations[asset.ID]; exists {
			return fmt.Errorf("duplicate video asset %q", asset.ID)
		}
		durations[asset.ID] = asset.DurationUS
	}
	seen := make(map[string]map[DurationUS]bool)
	var total DurationUS
	for i, clip := range plan.Clips {
		sourceDuration, exists := durations[clip.AssetID]
		if !exists || clip.StartUS < 0 || clip.DurationUS <= 0 || clip.StartUS >= sourceDuration {
			return fmt.Errorf("clip %d has invalid source interval", i)
		}
		if clip.StartUS%plan.ClipDurationUS != 0 || clip.DurationUS > plan.ClipDurationUS || clip.DurationUS > sourceDuration-clip.StartUS {
			return fmt.Errorf("clip %d exceeds its candidate interval", i)
		}
		if seen[clip.AssetID] == nil {
			seen[clip.AssetID] = make(map[DurationUS]bool)
		}
		if seen[clip.AssetID][clip.StartUS] {
			return fmt.Errorf("clip %d repeats a candidate interval", i)
		}
		seen[clip.AssetID][clip.StartUS] = true
		candidateDuration := plan.ClipDurationUS
		if available := sourceDuration - clip.StartUS; available < candidateDuration {
			candidateDuration = available
		}
		if i < len(plan.Clips)-1 && clip.DurationUS != candidateDuration {
			return fmt.Errorf("clip %d truncates before the last clip", i)
		}
		if clip.DurationUS > plan.TargetDurationUS-total {
			return errors.New("clip durations exceed target")
		}
		total += clip.DurationUS
	}
	if total != plan.TargetDurationUS {
		return errors.New("clip durations do not equal target")
	}
	return nil
}
