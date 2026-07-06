package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
)

type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func Enqueue(ctx context.Context, db Execer, ev connector.NormalizedEvent) error {
	body, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("enqueue marshal: %w", err)
	}
	_, err = db.Exec(ctx,
		`insert into ingestion_queue (tenant_id, event) values ($1, $2)`,
		ev.TenantID, body,
	)
	if err != nil {
		return fmt.Errorf("enqueue insert: %w", err)
	}
	return nil
}

type queueItem struct {
	ID         int64
	TenantID   string
	Event      []byte
	Attempts   int
	EnqueuedAt time.Time
}
