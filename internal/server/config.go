package server

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr         string
	DataDir            string
	WebDistDir         string
	LogLevel           slog.Level
	MaxUploadMB        int
	MixTimeout         time.Duration
	MaxConcurrentMixes int
}

func LoadConfig() (Config, error) {
	cfg := Config{
		ListenAddr: ":8080",
		DataDir:    "data",
		WebDistDir: "web/dist",
		LogLevel:   slog.LevelInfo,
	}
	if value := os.Getenv("APP_ADDR"); value != "" {
		cfg.ListenAddr = value
	}
	if value := os.Getenv("DATA_DIR"); value != "" {
		cfg.DataDir = value
	}
	if value := os.Getenv("WEB_DIST_DIR"); value != "" {
		cfg.WebDistDir = value
	}
	if value := os.Getenv("LOG_LEVEL"); value != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(strings.ToUpper(value))); err != nil {
			return Config{}, fmt.Errorf("LOG_LEVEL: %w", err)
		}
	}
	var err error
	if cfg.MaxUploadMB, err = positiveInt("MAX_UPLOAD_MB", 512); err != nil {
		return Config{}, err
	}
	if cfg.MaxConcurrentMixes, err = positiveInt("MAX_CONCURRENT_MIXES", 1); err != nil {
		return Config{}, err
	}
	cfg.MixTimeout = 10 * time.Minute
	if value := os.Getenv("MIX_TIMEOUT"); value != "" {
		cfg.MixTimeout, err = time.ParseDuration(value)
		if err != nil || cfg.MixTimeout <= 0 {
			return Config{}, fmt.Errorf("MIX_TIMEOUT must be a positive duration")
		}
	}
	return cfg, nil
}

func positiveInt(name string, fallback int) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	number, err := strconv.Atoi(value)
	if err != nil || number <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return number, nil
}
