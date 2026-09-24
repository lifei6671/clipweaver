package domain

import "time"

// DurationUS represents an integer number of microseconds.
type DurationUS int64

type AssetKind string

const (
	AssetKindVideo AssetKind = "video"
	AssetKindAudio AssetKind = "audio"
)

type Asset struct {
	ID            string
	Kind          AssetKind
	Name          string
	StoredPath    string
	DurationUS    DurationUS
	Width         int
	Height        int
	CreatedAt     time.Time
	ContentSHA256 string // Internal video source digest; absent in historical manifests.
	HasPoster     bool   // Derived from poster.jpg; never persisted in meta.json.
}
