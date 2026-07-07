package config

import (
	"fmt"
	"strconv"
	"time"
)

type Config struct {
	DatabaseURL     string
	DatabaseSetRole string
	PayloadRoot     string
	PollInterval    time.Duration
	HealthInterval  time.Duration
	ProcessBatch    int
}

func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		DatabaseURL:     getenv("DATABASE_URL"),
		DatabaseSetRole: valueOr(getenv, "DATABASE_SET_ROLE", "brain_worker"),
		PayloadRoot:     getenv("PAYLOAD_ROOT"),
		PollInterval:    30 * time.Second,
		HealthInterval:  5 * time.Minute,
		ProcessBatch:    100,
	}
	if cfg.DatabaseURL == "" {
		return cfg, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.PayloadRoot == "" {
		return cfg, fmt.Errorf("PAYLOAD_ROOT is required")
	}

	var err error
	if cfg.PollInterval, err = durationOr(getenv, "POLL_INTERVAL", cfg.PollInterval); err != nil {
		return cfg, err
	}
	if cfg.HealthInterval, err = durationOr(getenv, "HEALTH_INTERVAL", cfg.HealthInterval); err != nil {
		return cfg, err
	}
	if raw := getenv("PROCESS_BATCH"); raw != "" {
		cfg.ProcessBatch, err = strconv.Atoi(raw)
		if err != nil || cfg.ProcessBatch < 1 {
			return cfg, fmt.Errorf("PROCESS_BATCH must be a positive integer, got %q", raw)
		}
	}
	return cfg, nil
}

func valueOr(getenv func(string) string, key, fallback string) string {
	if v := getenv(key); v != "" {
		return v
	}
	return fallback
}

func durationOr(getenv func(string) string, key string, fallback time.Duration) (time.Duration, error) {
	raw := getenv(key)
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration, got %q", key, raw)
	}
	return d, nil
}
