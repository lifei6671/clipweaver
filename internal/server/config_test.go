package server

import (
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	t.Setenv("MAX_UPLOAD_MB", "16")
	t.Setenv("MIX_TIMEOUT", "2m")
	t.Setenv("MAX_CONCURRENT_MIXES", "2")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxUploadMB != 16 || cfg.MixTimeout != 2*time.Minute || cfg.MaxConcurrentMixes != 2 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadConfigRejectsInvalidLimit(t *testing.T) {
	t.Setenv("MAX_UPLOAD_MB", "0")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected invalid upload limit error")
	}
}
