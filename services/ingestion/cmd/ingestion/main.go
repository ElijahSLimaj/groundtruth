package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
	"github.com/attempttechnologies/company-brain/services/ingestion/gmail"
	"github.com/attempttechnologies/company-brain/services/ingestion/internal/config"
	"github.com/attempttechnologies/company-brain/services/ingestion/keys"
	"github.com/attempttechnologies/company-brain/services/ingestion/runtime"
	"github.com/attempttechnologies/company-brain/services/ingestion/slack"
	"github.com/attempttechnologies/company-brain/services/ingestion/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(logger); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("ingestion exited", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	pool, err := newPool(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()

	slackConnector := slack.New(func(token string) slack.API { return slack.NewClient(token) })
	gmailConnector := gmail.New(func(token string) gmail.API { return gmail.NewClient(token) })
	runner := &runtime.Runner{
		Pool: pool,
		Connectors: map[string]connector.Connector{
			slack.SourceType: slackConnector,
			gmail.SourceType: gmailConnector,
		},
		Normalizers: map[string]runtime.Normalizer{
			slack.SourceType: slack.Normalizer{},
			gmail.SourceType: gmail.Normalizer{},
		},
	}
	payloads, err := buildPayloadStore(ctx, cfg, pool, logger)
	if err != nil {
		return err
	}
	processor := &store.Processor{
		Pool:     pool,
		Payloads: payloads,
	}

	logger.Info("ingestion started",
		"poll_interval", cfg.PollInterval.String(),
		"health_interval", cfg.HealthInterval.String(),
		"process_batch", cfg.ProcessBatch)

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		pollLoop(ctx, logger, pool, runner, cfg.PollInterval)
	}()
	go func() {
		defer wg.Done()
		healthLoop(ctx, logger, pool, runner, cfg.HealthInterval)
	}()
	go func() {
		defer wg.Done()
		drainLoop(ctx, logger, processor, cfg.ProcessBatch)
	}()
	wg.Wait()
	logger.Info("ingestion stopped")
	return ctx.Err()
}

func buildPayloadStore(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, logger *slog.Logger) (store.PayloadStore, error) {
	if cfg.MasterKeyHex == "" {
		logger.Warn("payload encryption disabled, set MASTER_KEY to encrypt at rest")
		return &store.FSPayloadStore{Root: cfg.PayloadRoot}, nil
	}
	wrapper, err := keys.NewAESWrapper(cfg.MasterKeyHex)
	if err != nil {
		return nil, err
	}
	keyService := &keys.Service{Pool: pool, Wrapper: wrapper}

	var blobs store.BlobStore
	if cfg.S3Bucket != "" {
		awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.S3Region))
		if err != nil {
			return nil, fmt.Errorf("aws config: %w", err)
		}
		client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
			if cfg.S3Endpoint != "" {
				o.BaseEndpoint = &cfg.S3Endpoint
				o.UsePathStyle = true
			}
		})
		blobs = &store.S3BlobStore{Client: client, Bucket: cfg.S3Bucket}
		logger.Info("payloads encrypted to object storage", "bucket", cfg.S3Bucket)
	} else {
		blobs = &store.FSBlobStore{Root: cfg.PayloadRoot}
		logger.Info("payloads encrypted to local storage", "root", cfg.PayloadRoot)
	}
	return &store.EncryptedPayloadStore{Blobs: blobs, Keys: keyService}, nil
}

func newPool(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	if cfg.DatabaseSetRole != "" && cfg.DatabaseSetRole != "none" {
		role := pgx.Identifier{cfg.DatabaseSetRole}.Sanitize()
		poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
			_, err := conn.Exec(ctx, "set role "+role)
			return err
		}
	}
	return pgxpool.NewWithConfig(ctx, poolCfg)
}

func pollLoop(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool, runner *runtime.Runner, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		for _, id := range listConnectors(ctx, logger, pool, runner) {
			result, err := runner.RunPollCycle(ctx, id)
			if err != nil {
				logger.Error("poll_cycle", "connector", id, "error", err)
				continue
			}
			if !result.Skipped {
				logger.Info("poll_cycle",
					"connector", id,
					"enqueued", result.Enqueued,
					"dead_lettered", result.DeadLettered)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func healthLoop(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool, runner *runtime.Runner, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		for _, id := range listConnectors(ctx, logger, pool, runner) {
			transition, err := runner.RunHealthCheck(ctx, id)
			if err != nil {
				logger.Error("health_check", "connector", id, "error", err)
				continue
			}
			if transition.From != transition.To {
				logger.Warn("connector_status_changed",
					"connector", id, "from", transition.From, "to", transition.To)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func drainLoop(ctx context.Context, logger *slog.Logger, processor *store.Processor, batch int) {
	for {
		result, err := processor.ProcessBatch(ctx, batch)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			logger.Error("process_batch", "error", err)
		}
		if result.Written > 0 || result.Duplicates > 0 || result.DeadLettered > 0 || result.RetryScheduled > 0 {
			logger.Info("process_batch",
				"written", result.Written,
				"duplicates", result.Duplicates,
				"dead_lettered", result.DeadLettered,
				"retry_scheduled", result.RetryScheduled)
		}
		idle := result.Written+result.Duplicates+result.DeadLettered+result.RetryScheduled == 0
		if !idle {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

func listConnectors(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool, runner *runtime.Runner) []uuid.UUID {
	rows, err := pool.Query(ctx,
		`select id, source_type from connectors where status in ('live', 'degraded') order by id`)
	if err != nil {
		if ctx.Err() == nil {
			logger.Error("list_connectors", "error", err)
		}
		return nil
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		var sourceType string
		if err := rows.Scan(&id, &sourceType); err != nil {
			logger.Error("list_connectors", "error", err)
			return nil
		}
		if _, ok := runner.Connectors[sourceType]; ok {
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil && ctx.Err() == nil {
		logger.Error("list_connectors", "error", err)
	}
	return ids
}
