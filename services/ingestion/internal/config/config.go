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
	MasterKeyHex    string
	S3Bucket        string
	S3Endpoint      string
	S3Region        string
	WebhookAddr     string
	SlackSecret     string
	PollInterval    time.Duration
	HealthInterval  time.Duration
	ProcessBatch    int
}

func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		DatabaseURL:     getenv("DATABASE_URL"),
		DatabaseSetRole: valueOr(getenv, "DATABASE_SET_ROLE", "brain_worker"),
		PayloadRoot:     getenv("PAYLOAD_ROOT"),
		MasterKeyHex:    getenv("MASTER_KEY"),
		S3Bucket:        getenv("S3_BUCKET"),
		S3Endpoint:      getenv("S3_ENDPOINT"),
		S3Region:        getenv("S3_REGION"),
		WebhookAddr:     getenv("WEBHOOK_ADDR"),
		SlackSecret:     getenv("SLACK_SIGNING_SECRET"),
		PollInterval:    30 * time.Second,
		HealthInterval:  5 * time.Minute,
		ProcessBatch:    100,
	}
	if cfg.DatabaseURL == "" {
		return cfg, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.PayloadRoot == "" && cfg.S3Bucket == "" {
		return cfg, fmt.Errorf("PAYLOAD_ROOT or S3_BUCKET is required")
	}
	if cfg.S3Bucket != "" && cfg.MasterKeyHex == "" {
		return cfg, fmt.Errorf("MASTER_KEY is required when payloads go to object storage")
	}
	if cfg.WebhookAddr != "" && cfg.SlackSecret == "" {
		return cfg, fmt.Errorf("SLACK_SIGNING_SECRET is required when the webhook receiver is enabled")
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
