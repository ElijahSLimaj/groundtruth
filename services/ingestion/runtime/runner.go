package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
	"github.com/attempttechnologies/company-brain/services/ingestion/store"
)

const (
	StatusLive     = "live"
	StatusDegraded = "degraded"
)

var ErrConnectorNotLive = errors.New("connector is not live")

type Normalizer interface {
	Normalize(cfg connector.Config, item connector.RawItem, acl connector.ACL) (connector.NormalizedEvent, error)
}

type Runner struct {
	Pool        *pgxpool.Pool
	Connectors  map[string]connector.Connector
	Normalizers map[string]Normalizer
}

type CycleResult struct {
	Skipped      bool
	Enqueued     int
	DeadLettered int
	Cursor       connector.Cursor
}

type HealthTransition struct {
	From string
	To   string
}

type connectorRow struct {
	TenantID   uuid.UUID
	SourceType string
	Status     string
	Settings   map[string]any
}

func (r *Runner) RunPollCycle(ctx context.Context, connectorID uuid.UUID) (CycleResult, error) {
	row, conn, norm, err := r.resolve(ctx, connectorID)
	if err != nil {
		return CycleResult{}, err
	}
	if row.Status != StatusLive {
		return CycleResult{Skipped: true}, nil
	}
	cfg := configFor(connectorID, row)

	cursor, err := r.loadCursor(ctx, connectorID)
	if err != nil {
		return CycleResult{}, err
	}

	sink := &enqueueSink{pool: r.Pool, conn: conn, norm: norm, cfg: cfg}
	next, err := conn.Poll(ctx, cfg, cursor, sink)
	result := CycleResult{Enqueued: sink.enqueued, DeadLettered: sink.deadLettered, Cursor: cursor}
	if err != nil {
		return result, fmt.Errorf("poll %s: %w", cfg.SourceType, err)
	}

	if err := r.saveCursor(ctx, connectorID, row.TenantID, next); err != nil {
		return result, err
	}
	result.Cursor = next
	return result, nil
}

func (r *Runner) IngestItem(ctx context.Context, connectorID uuid.UUID, item connector.RawItem) error {
	row, conn, norm, err := r.resolve(ctx, connectorID)
	if err != nil {
		return err
	}
	if row.Status != StatusLive {
		return ErrConnectorNotLive
	}
	cfg := configFor(connectorID, row)
	sink := &enqueueSink{pool: r.Pool, conn: conn, norm: norm, cfg: cfg}
	return sink.Emit(ctx, item)
}

func (r *Runner) RunBackfill(ctx context.Context, connectorID uuid.UUID, window connector.BackfillWindow) (CycleResult, error) {
	row, conn, norm, err := r.resolve(ctx, connectorID)
	if err != nil {
		return CycleResult{}, err
	}
	if row.Status != StatusLive {
		return CycleResult{Skipped: true}, nil
	}
	cfg := configFor(connectorID, row)

	sink := &enqueueSink{pool: r.Pool, conn: conn, norm: norm, cfg: cfg}
	if err := conn.Backfill(ctx, cfg, window, sink); err != nil {
		return CycleResult{Enqueued: sink.enqueued, DeadLettered: sink.deadLettered},
			fmt.Errorf("backfill %s: %w", cfg.SourceType, err)
	}
	return CycleResult{Enqueued: sink.enqueued, DeadLettered: sink.deadLettered}, nil
}

func (r *Runner) RunHealthCheck(ctx context.Context, connectorID uuid.UUID) (HealthTransition, error) {
	row, conn, _, err := r.resolve(ctx, connectorID)
	if err != nil {
		return HealthTransition{}, err
	}
	cfg := configFor(connectorID, row)

	health := conn.HealthCheck(ctx, cfg)
	transition := HealthTransition{From: row.Status, To: row.Status}
	switch {
	case health.State == connector.HealthDegraded && row.Status == StatusLive:
		transition.To = StatusDegraded
	case health.State == connector.HealthLive && row.Status == StatusDegraded:
		transition.To = StatusLive
	}
	if transition.To != transition.From {
		if _, err := r.Pool.Exec(ctx,
			`update connectors set status = $2 where id = $1`, connectorID, transition.To,
		); err != nil {
			return transition, fmt.Errorf("update connector status: %w", err)
		}
	}
	return transition, nil
}

func (r *Runner) resolve(ctx context.Context, connectorID uuid.UUID) (connectorRow, connector.Connector, Normalizer, error) {
	row, err := r.loadConnector(ctx, connectorID)
	if err != nil {
		return connectorRow{}, nil, nil, err
	}
	conn, ok := r.Connectors[row.SourceType]
	if !ok {
		return connectorRow{}, nil, nil, fmt.Errorf("no connector registered for source type %q", row.SourceType)
	}
	norm, ok := r.Normalizers[row.SourceType]
	if !ok {
		return connectorRow{}, nil, nil, fmt.Errorf("no normalizer registered for source type %q", row.SourceType)
	}
	return row, conn, norm, nil
}

func (r *Runner) loadConnector(ctx context.Context, connectorID uuid.UUID) (connectorRow, error) {
	var row connectorRow
	err := r.Pool.QueryRow(ctx,
		`select tenant_id, source_type, status, config from connectors where id = $1`,
		connectorID,
	).Scan(&row.TenantID, &row.SourceType, &row.Status, &row.Settings)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, fmt.Errorf("connector %s not found", connectorID)
	}
	if err != nil {
		return row, fmt.Errorf("load connector: %w", err)
	}
	return row, nil
}

func (r *Runner) loadCursor(ctx context.Context, connectorID uuid.UUID) (connector.Cursor, error) {
	var cursor string
	err := r.Pool.QueryRow(ctx,
		`select poll_cursor from connector_state where connector_id = $1`, connectorID,
	).Scan(&cursor)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load cursor: %w", err)
	}
	return connector.Cursor(cursor), nil
}

func (r *Runner) saveCursor(ctx context.Context, connectorID, tenantID uuid.UUID, cursor connector.Cursor) error {
	_, err := r.Pool.Exec(ctx, `
		insert into connector_state (connector_id, tenant_id, poll_cursor)
		values ($1, $2, $3)
		on conflict (connector_id) do update
		set poll_cursor = excluded.poll_cursor, updated_at = now()
	`, connectorID, tenantID, string(cursor))
	if err != nil {
		return fmt.Errorf("save cursor: %w", err)
	}
	return nil
}

func configFor(connectorID uuid.UUID, row connectorRow) connector.Config {
	return connector.Config{
		ConnectorID: connectorID,
		TenantID:    row.TenantID,
		SourceType:  row.SourceType,
		Settings:    row.Settings,
	}
}

type enqueueSink struct {
	pool         *pgxpool.Pool
	conn         connector.Connector
	norm         Normalizer
	cfg          connector.Config
	enqueued     int
	deadLettered int
}

func (s *enqueueSink) Emit(ctx context.Context, item connector.RawItem) error {
	acl, err := s.conn.ResolveACL(ctx, s.cfg, item)
	if err != nil {
		return fmt.Errorf("resolve acl for %s: %w", item.ExternalID, err)
	}
	ev, err := s.norm.Normalize(s.cfg, item, acl)
	if err != nil {
		if dlqErr := s.deadLetter(ctx, item, "normalize: "+err.Error()); dlqErr != nil {
			return dlqErr
		}
		s.deadLettered++
		return nil
	}
	if err := store.Enqueue(ctx, s.pool, ev); err != nil {
		return err
	}
	s.enqueued++
	return nil
}

func (s *enqueueSink) deadLetter(ctx context.Context, item connector.RawItem, reason string) error {
	body, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("dead letter marshal: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		insert into ingestion_dlq (tenant_id, event, reason, attempts, enqueued_at)
		values ($1, $2, $3, 0, now())
	`, s.cfg.TenantID, body, reason)
	if err != nil {
		return fmt.Errorf("dead letter insert: %w", err)
	}
	return nil
}
