package testdb

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Pools(t *testing.T) (worker, admin *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping database integration tests")
	}
	ctx := context.Background()

	workerCfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	workerCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "set role brain_worker")
		return err
	}
	worker, err = pgxpool.NewWithConfig(ctx, workerCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(worker.Close)

	admin, err = pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	return worker, admin
}

type Fixture struct {
	TenantID    uuid.UUID
	ConnectorID uuid.UUID
}

func CreateFixture(t *testing.T, admin *pgxpool.Pool) Fixture {
	t.Helper()
	ctx := context.Background()
	f := Fixture{TenantID: uuid.New(), ConnectorID: uuid.New()}

	_, err := admin.Exec(ctx,
		`insert into tenants (id, name, tier) values ($1, $2, 'growth')`,
		f.TenantID, "test-"+f.TenantID.String())
	if err != nil {
		t.Fatal(err)
	}
	_, err = admin.Exec(ctx,
		`insert into connectors (id, tenant_id, source_type, status, config) values ($1, $2, 'slack', 'live', '{}')`,
		f.ConnectorID, f.TenantID)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		for _, stmt := range []string{
			`delete from ingestion_dlq where tenant_id = $1`,
			`delete from ingestion_queue where tenant_id = $1`,
			`delete from connector_state where tenant_id = $1`,
			`delete from events where tenant_id = $1`,
			`delete from connectors where tenant_id = $1`,
			`delete from tenants where id = $1`,
		} {
			if _, err := admin.Exec(ctx, stmt, f.TenantID); err != nil {
				t.Errorf("cleanup failed: %v", err)
			}
		}
	})
	return f
}

func CountRows(t *testing.T, admin *pgxpool.Pool, table string, tenantID uuid.UUID) int {
	t.Helper()
	var n int
	err := admin.QueryRow(context.Background(),
		`select count(*) from `+table+` where tenant_id = $1`, tenantID).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func DLQReasons(t *testing.T, admin *pgxpool.Pool, tenantID uuid.UUID) []string {
	t.Helper()
	rows, err := admin.Query(context.Background(),
		`select reason from ingestion_dlq where tenant_id = $1`, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var reasons []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			t.Fatal(err)
		}
		reasons = append(reasons, r)
	}
	return reasons
}
