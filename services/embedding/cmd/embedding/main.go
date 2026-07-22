package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/attempttechnologies/company-brain/services/embedding/payload"
	"github.com/attempttechnologies/company-brain/services/embedding/pipeline"
	"github.com/attempttechnologies/company-brain/services/embedding/provider"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(logger); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("embedding exited", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	interval := 30 * time.Second
	if raw := os.Getenv("EMBED_INTERVAL"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return fmt.Errorf("EMBED_INTERVAL must be a positive duration, got %q", raw)
		}
		interval = parsed
	}
	var embedProvider provider.Provider
	switch kind := os.Getenv("EMBEDDING_PROVIDER"); kind {
	case "", "fake":
		embedProvider = provider.NewFake()
	case "voyage":
		apiKey := os.Getenv("VOYAGE_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("VOYAGE_API_KEY is required for the voyage provider")
		}
		voyage := provider.NewVoyage(apiKey)
		if model := os.Getenv("VOYAGE_MODEL"); model != "" {
			voyage.Model = model
		}
		embedProvider = voyage
	default:
		return fmt.Errorf("unsupported EMBEDDING_PROVIDER %q", kind)
	}

	poolCfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return err
	}
	role := os.Getenv("DATABASE_SET_ROLE")
	if role == "" {
		role = "brain_embedder"
	}
	if role != "none" {
		sanitized := pgx.Identifier{role}.Sanitize()
		poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
			_, err := conn.Exec(ctx, "set role "+sanitized)
			return err
		}
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	payloads, err := payload.Build(ctx, payload.Options{
		Root:         os.Getenv("PAYLOAD_ROOT"),
		MasterKeyHex: os.Getenv("MASTER_KEY"),
		S3Bucket:     os.Getenv("S3_BUCKET"),
		S3Endpoint:   os.Getenv("S3_ENDPOINT"),
		S3Region:     os.Getenv("S3_REGION"),
		Pool:         pool,
	})
	if err != nil {
		return err
	}

	p := &pipeline.Pipeline{
		Pool:     pool,
		Provider: embedProvider,
		Payloads: payloads,
	}
	logger.Info("embedding started",
		"interval", interval.String(),
		"model", embedProvider.ModelID())

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		result, err := p.Run(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.Error("pipeline run", "error", err)
		} else if result.EventsScanned > 0 || result.StatementsEmbedded > 0 {
			logger.Info("pipeline run",
				"events_scanned", result.EventsScanned,
				"chunks_written", result.ChunksWritten,
				"statements_embedded", result.StatementsEmbedded,
				"dead_lettered", result.DeadLettered)
		}
		select {
		case <-ctx.Done():
			logger.Info("embedding stopped")
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
