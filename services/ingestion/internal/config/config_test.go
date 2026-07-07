package config

import (
	"strings"
	"testing"
	"time"
)

func env(pairs map[string]string) func(string) string {
	return func(key string) string { return pairs[key] }
}

func TestLoad(t *testing.T) {
	t.Parallel()

	t.Run("applies_defaults_with_required_values_set", func(t *testing.T) {
		t.Parallel()
		cfg, err := Load(env(map[string]string{
			"DATABASE_URL": "postgresql://x",
			"PAYLOAD_ROOT": "/var/payloads",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.DatabaseSetRole != "brain_worker" ||
			cfg.PollInterval != 30*time.Second ||
			cfg.HealthInterval != 5*time.Minute ||
			cfg.ProcessBatch != 100 {
			t.Fatalf("unexpected defaults %+v", cfg)
		}
	})

	t.Run("fails_fast_on_missing_database_url", func(t *testing.T) {
		t.Parallel()
		_, err := Load(env(map[string]string{"PAYLOAD_ROOT": "/x"}))
		if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
			t.Fatalf("expected DATABASE_URL error, got %v", err)
		}
	})

	t.Run("fails_fast_on_missing_payload_root", func(t *testing.T) {
		t.Parallel()
		_, err := Load(env(map[string]string{"DATABASE_URL": "postgresql://x"}))
		if err == nil || !strings.Contains(err.Error(), "PAYLOAD_ROOT") {
			t.Fatalf("expected PAYLOAD_ROOT error, got %v", err)
		}
	})

	t.Run("parses_overrides", func(t *testing.T) {
		t.Parallel()
		cfg, err := Load(env(map[string]string{
			"DATABASE_URL":      "postgresql://x",
			"PAYLOAD_ROOT":      "/x",
			"DATABASE_SET_ROLE": "none",
			"POLL_INTERVAL":     "10s",
			"HEALTH_INTERVAL":   "1m",
			"PROCESS_BATCH":     "25",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.DatabaseSetRole != "none" || cfg.PollInterval != 10*time.Second ||
			cfg.HealthInterval != time.Minute || cfg.ProcessBatch != 25 {
			t.Fatalf("unexpected overrides %+v", cfg)
		}
	})

	t.Run("rejects_invalid_values", func(t *testing.T) {
		t.Parallel()
		base := map[string]string{"DATABASE_URL": "postgresql://x", "PAYLOAD_ROOT": "/x"}
		for key, bad := range map[string]string{
			"POLL_INTERVAL":   "soon",
			"HEALTH_INTERVAL": "-5m",
			"PROCESS_BATCH":   "0",
		} {
			pairs := map[string]string{key: bad}
			for k, v := range base {
				pairs[k] = v
			}
			if _, err := Load(env(pairs)); err == nil {
				t.Fatalf("expected error for %s=%q", key, bad)
			}
		}
	})
}
