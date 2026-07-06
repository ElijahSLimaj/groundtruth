package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
	"github.com/attempttechnologies/company-brain/services/ingestion/ingest"
)

type Outcome string

const (
	OutcomeWritten        Outcome = "written"
	OutcomeDuplicate      Outcome = "duplicate"
	OutcomeDeadLettered   Outcome = "dead_lettered"
	OutcomeRetryScheduled Outcome = "retry_scheduled"
)

type BatchResult struct {
	Written        int
	Duplicates     int
	DeadLettered   int
	RetryScheduled int
}

type Processor struct {
	Pool        *pgxpool.Pool
	Payloads    PayloadStore
	MaxAttempts int
	Backoff     func(attempts int) time.Duration
}

func (p *Processor) maxAttempts() int {
	if p.MaxAttempts > 0 {
		return p.MaxAttempts
	}
	return 5
}

func (p *Processor) backoff(attempts int) time.Duration {
	if p.Backoff != nil {
		return p.Backoff(attempts)
	}
	delay := 30 * time.Second << (attempts - 1)
	if delay > time.Hour {
		return time.Hour
	}
	return delay
}

func (p *Processor) ProcessBatch(ctx context.Context, limit int) (BatchResult, error) {
	var result BatchResult
	for range limit {
		outcome, found, err := p.ProcessOne(ctx)
		if err != nil {
			return result, err
		}
		if !found {
			return result, nil
		}
		switch outcome {
		case OutcomeWritten:
			result.Written++
		case OutcomeDuplicate:
			result.Duplicates++
		case OutcomeDeadLettered:
			result.DeadLettered++
		case OutcomeRetryScheduled:
			result.RetryScheduled++
		}
	}
	return result, nil
}

func (p *Processor) ProcessOne(ctx context.Context) (Outcome, bool, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return "", false, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	item, found, err := dequeueOne(ctx, tx)
	if err != nil {
		return "", false, err
	}
	if !found {
		return "", false, nil
	}

	outcome, err := p.handle(ctx, tx, item)
	if err != nil {
		return "", true, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", true, fmt.Errorf("commit: %w", err)
	}
	return outcome, true, nil
}

func dequeueOne(ctx context.Context, tx pgx.Tx) (queueItem, bool, error) {
	var item queueItem
	err := tx.QueryRow(ctx, `
		select id, tenant_id, event, attempts, enqueued_at
		from ingestion_queue
		where next_attempt_at <= now()
		order by id
		limit 1
		for update skip locked
	`).Scan(&item.ID, &item.TenantID, &item.Event, &item.Attempts, &item.EnqueuedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, false, nil
	}
	if err != nil {
		return item, false, fmt.Errorf("dequeue: %w", err)
	}
	return item, true, nil
}

func (p *Processor) handle(ctx context.Context, tx pgx.Tx, item queueItem) (Outcome, error) {
	var ev connector.NormalizedEvent
	if err := json.Unmarshal(item.Event, &ev); err != nil {
		return deadLetter(ctx, tx, item, "malformed_event_json: "+err.Error())
	}
	if err := ingest.Validate(ev); err != nil {
		return deadLetter(ctx, tx, item, err.Error())
	}
	if ev.TenantID.String() != item.TenantID {
		return deadLetter(ctx, tx, item, "tenant_mismatch")
	}

	if _, err := tx.Exec(ctx,
		`select set_config('app.tenant_id', $1, true)`, item.TenantID,
	); err != nil {
		return "", fmt.Errorf("set tenant: %w", err)
	}

	content, err := json.Marshal(ev.Payload)
	if err != nil {
		return deadLetter(ctx, tx, item, "payload_marshal: "+err.Error())
	}
	payloadRef, err := p.Payloads.Put(ctx, ev.TenantID, content)
	if err != nil {
		return p.retryOrDeadLetter(ctx, tx, item, fmt.Errorf("payload store: %w", err))
	}

	inserted, err := insertEvent(ctx, tx, ev, payloadRef)
	if err != nil {
		if isPermanent(err) {
			return deadLetter(ctx, tx, item, err.Error())
		}
		return p.retryOrDeadLetter(ctx, tx, item, err)
	}

	if err := ack(ctx, tx, item.ID); err != nil {
		return "", err
	}
	if inserted {
		return OutcomeWritten, nil
	}
	return OutcomeDuplicate, nil
}

func insertEvent(ctx context.Context, tx pgx.Tx, ev connector.NormalizedEvent, payloadRef string) (bool, error) {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("savepoint: %w", err)
	}
	acl, err := json.Marshal(ev.ACL)
	if err != nil {
		return false, fmt.Errorf("acl marshal: %w", err)
	}
	_, err = sp.Exec(ctx, `
		insert into events (
			tenant_id, connector_id, source_type, external_id,
			author_id, thread_key, occurred_at, acl, payload_ref
		)
		values ($1, $2, $3, $4, $5, nullif($6, ''), $7, $8, $9)
	`,
		ev.TenantID, ev.ConnectorID, ev.SourceType, ev.ExternalID,
		ev.AuthorRef.PersonID, ev.ThreadKey, ev.OccurredAt, acl, payloadRef,
	)
	if err != nil {
		sp.Rollback(ctx)
		if isUniqueViolation(err) {
			return false, nil
		}
		return false, err
	}
	if err := sp.Commit(ctx); err != nil {
		return false, fmt.Errorf("savepoint commit: %w", err)
	}
	return true, nil
}

func deadLetter(ctx context.Context, tx pgx.Tx, item queueItem, reason string) (Outcome, error) {
	if _, err := tx.Exec(ctx, `
		insert into ingestion_dlq (tenant_id, event, reason, attempts, enqueued_at)
		values ($1, $2, $3, $4, $5)
	`, item.TenantID, item.Event, reason, item.Attempts, item.EnqueuedAt); err != nil {
		return "", fmt.Errorf("dead letter insert: %w", err)
	}
	if err := ack(ctx, tx, item.ID); err != nil {
		return "", err
	}
	return OutcomeDeadLettered, nil
}

func (p *Processor) retryOrDeadLetter(ctx context.Context, tx pgx.Tx, item queueItem, cause error) (Outcome, error) {
	attempts := item.Attempts + 1
	if attempts >= p.maxAttempts() {
		return deadLetter(ctx, tx, item, fmt.Sprintf("max_attempts_exhausted (%d): %s", attempts, cause))
	}
	if _, err := tx.Exec(ctx, `
		update ingestion_queue
		set attempts = $2,
		    last_error = $3,
		    next_attempt_at = now() + make_interval(secs => $4)
		where id = $1
	`, item.ID, attempts, cause.Error(), p.backoff(attempts).Seconds()); err != nil {
		return "", fmt.Errorf("schedule retry: %w", err)
	}
	return OutcomeRetryScheduled, nil
}

func ack(ctx context.Context, tx pgx.Tx, id int64) error {
	if _, err := tx.Exec(ctx, `delete from ingestion_queue where id = $1`, id); err != nil {
		return fmt.Errorf("ack: %w", err)
	}
	return nil
}
