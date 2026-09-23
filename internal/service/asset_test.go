package service

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lifei6671/clipweaver/internal/domain"
	"github.com/lifei6671/clipweaver/internal/media"
	"github.com/lifei6671/clipweaver/internal/storage"
)

type fakeProber struct {
	video func(string) (media.Result, error)
	audio func(string) (media.Result, error)
}

func (p fakeProber) ProbeVideo(_ context.Context, path string) (media.Result, error) {
	return p.video(path)
}

func (p fakeProber) ProbeAudio(_ context.Context, path string) (media.Result, error) {
	return p.audio(path)
}

type faultStore struct {
	*storage.Local
	promoteErr error
	saveErr    error
}

func (s faultStore) PromoteUpload(uploadID, assetID string) error {
	if s.promoteErr != nil {
		return s.promoteErr
	}
	return s.Local.PromoteUpload(uploadID, assetID)
}

func (s faultStore) SaveAsset(asset domain.Asset) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	return s.Local.SaveAsset(asset)
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestUploadFailureCleansStagingAndFinal(t *testing.T) {
	for _, tc := range []struct {
		name       string
		probe      func(string) (media.Result, error)
		promoteErr error
		saveErr    error
		input      io.Reader
	}{
		{name: "copy", probe: validVideo, input: brokenReader{}},
		{name: "probe", probe: func(string) (media.Result, error) { return media.Result{}, media.ErrMissingStream }, input: strings.NewReader("bad")},
		{name: "promote", probe: validVideo, promoteErr: errors.New("promotion failed"), input: strings.NewReader("video")},
		{name: "manifest", probe: validVideo, saveErr: errors.New("manifest failed"), input: strings.NewReader("video")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			local, err := storage.NewLocal(root)
			if err != nil {
				t.Fatal(err)
			}
			store := faultStore{Local: local, promoteErr: tc.promoteErr, saveErr: tc.saveErr}
			svc := NewAssetService(store, fakeProber{video: tc.probe})
			if _, err := svc.UploadVideo(context.Background(), "video.mp4", tc.input); err == nil {
				t.Fatal("upload unexpectedly succeeded")
			}
			for _, dir := range []string{filepath.Join(root, "tmp", "uploads"), filepath.Join(root, "assets")} {
				entries, err := os.ReadDir(dir)
				if err != nil || len(entries) != 0 {
					t.Fatalf("%s: entries=%v err=%v", dir, entries, err)
				}
			}
		})
	}
}

func validVideo(path string) (media.Result, error) {
	if _, err := os.ReadFile(path); err != nil {
		return media.Result{}, err
	}
	return media.Result{DurationUS: 8_000_000, Width: 1920, Height: 1080}, nil
}
