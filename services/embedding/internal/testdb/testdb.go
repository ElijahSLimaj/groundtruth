package testdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Pools(t *testing.T) (embedder, admin *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping database integration tests")
	}
	ctx := context.Background()

	embedderCfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	embedderCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "set role brain_embedder")
		return err
	}
	embedder, err = pgxpool.NewWithConfig(ctx, embedderCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(embedder.Close)

	admin, err = pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	return embedder, admin
}

type Fixture struct {
	TenantID    uuid.UUID
	ConnectorID uuid.UUID
	PayloadRoot string
}

func CreateFixture(t *testing.T, admin *pgxpool.Pool, payloadRoot string) Fixture {
	t.Helper()
	ctx := context.Background()
	f := Fixture{TenantID: uuid.New(), ConnectorID: uuid.New(), PayloadRoot: payloadRoot}

	_, err := admin.Exec(ctx,
		`insert into tenants (id, name, tier) values ($1, $2, 'growth')`,
		f.TenantID, "embed-test-"+f.TenantID.String())
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
			`delete from embedding_dlq where tenant_id = $1`,
			`delete from event_chunks where tenant_id = $1`,
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

func CleanupWatermark(t *testing.T, admin *pgxpool.Pool, model string) {
	t.Helper()
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(),
			`delete from embedding_watermark where embedding_model = $1`, model); err != nil {
			t.Errorf("watermark cleanup failed: %v", err)
		}
	})
}

type EventSpec struct {
	ThreadKey  string
	OccurredAt time.Time
	IngestedAt time.Time
	Author     string
	Body       string
	ACL        string
}

func InsertEvent(t *testing.T, admin *pgxpool.Pool, f Fixture, spec EventSpec) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	id := uuid.New()

	content, err := json.Marshal(map[string]any{"body": spec.Body})
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	digest := hex.EncodeToString(sum[:])
	dir := filepath.Join(f.PayloadRoot, f.TenantID.String())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, digest), content, 0o600); err != nil {
		t.Fatal(err)
	}

	acl := spec.ACL
	if acl == "" {
		acl = `{"scope": "tenant"}`
	}
	ingested := spec.IngestedAt
	if ingested.IsZero() {
		ingested = time.Now().UTC()
	}
	_, err = admin.Exec(ctx, `
		insert into events (
			id, tenant_id, connector_id, source_type, external_id,
			author_source_ref, thread_key, occurred_at, ingested_at, acl, payload_ref
		)
		values ($1, $2, $3, 'slack', $4, nullif($5, ''), $6, $7, $8, $9, $10)
	`, id, f.TenantID, f.ConnectorID, id.String(), spec.Author, spec.ThreadKey,
		spec.OccurredAt, ingested, acl, "payloads/"+f.TenantID.String()+"/"+digest)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
