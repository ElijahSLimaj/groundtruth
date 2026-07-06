package store

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
)

func testPools(t *testing.T) (worker, admin *pgxpool.Pool) {
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

type fixture struct {
	tenantID    uuid.UUID
	connectorID uuid.UUID
}

func createFixture(t *testing.T, admin *pgxpool.Pool) fixture {
	t.Helper()
	ctx := context.Background()
	f := fixture{tenantID: uuid.New(), connectorID: uuid.New()}

	_, err := admin.Exec(ctx,
		`insert into tenants (id, name, tier) values ($1, $2, 'growth')`,
		f.tenantID, "test-"+f.tenantID.String())
	if err != nil {
		t.Fatal(err)
	}
	_, err = admin.Exec(ctx,
		`insert into connectors (id, tenant_id, source_type, status, config) values ($1, $2, 'slack', 'live', '{}')`,
		f.connectorID, f.tenantID)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		ctx := context.Background()
		for _, stmt := range []string{
			`delete from ingestion_dlq where tenant_id = $1`,
			`delete from ingestion_queue where tenant_id = $1`,
			`delete from events where tenant_id = $1`,
			`delete from connectors where tenant_id = $1`,
			`delete from tenants where id = $1`,
		} {
			if _, err := admin.Exec(ctx, stmt, f.tenantID); err != nil {
				t.Errorf("cleanup failed: %v", err)
			}
		}
	})
	return f
}

func (f fixture) event() connector.NormalizedEvent {
	return connector.NormalizedEvent{
		TenantID:    f.tenantID,
		ConnectorID: f.connectorID,
		SourceType:  "slack",
		ExternalID:  uuid.NewString(),
		ThreadKey:   "thread-1",
		OccurredAt:  time.Now().UTC().Truncate(time.Microsecond),
		ACL: connector.ACL{
			Scope:       connector.ACLScopeTenant,
			SourceScope: connector.SourceScope{Type: "slack_channel", ID: "C1", Visibility: "public"},
		},
		Payload: connector.Payload{Body: "hello from test"},
	}
}

func newProcessor(t *testing.T, worker *pgxpool.Pool) *Processor {
	t.Helper()
	return &Processor{
		Pool:     worker,
		Payloads: &FSPayloadStore{Root: t.TempDir()},
	}
}

func dlqReasons(t *testing.T, admin *pgxpool.Pool, tenantID uuid.UUID) []string {
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

func countRows(t *testing.T, admin *pgxpool.Pool, table string, tenantID uuid.UUID) int {
	t.Helper()
	var n int
	err := admin.QueryRow(context.Background(),
		`select count(*) from `+table+` where tenant_id = $1`, tenantID).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestProcessWritesEventAndAcks(t *testing.T) {
	worker, admin := testPools(t)
	f := createFixture(t, admin)
	p := newProcessor(t, worker)
	ctx := context.Background()

	ev := f.event()
	if err := Enqueue(ctx, worker, ev); err != nil {
		t.Fatal(err)
	}

	outcome, found, err := p.ProcessOne(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !found || outcome != OutcomeWritten {
		t.Fatalf("expected written, got found=%v outcome=%q dlq=%v", found, outcome, dlqReasons(t, admin, f.tenantID))
	}

	var externalID, payloadRef string
	err = admin.QueryRow(ctx,
		`select external_id, payload_ref from events where tenant_id = $1`, f.tenantID,
	).Scan(&externalID, &payloadRef)
	if err != nil {
		t.Fatal(err)
	}
	if externalID != ev.ExternalID {
		t.Fatalf("external_id mismatch: %q", externalID)
	}
	if payloadRef == "" {
		t.Fatal("expected a payload ref")
	}
	if n := countRows(t, admin, "ingestion_queue", f.tenantID); n != 0 {
		t.Fatalf("expected empty queue, found %d rows", n)
	}
}

func TestDuplicateDeliveryIsCountedNoOp(t *testing.T) {
	worker, admin := testPools(t)
	f := createFixture(t, admin)
	p := newProcessor(t, worker)
	ctx := context.Background()

	ev := f.event()
	for range 2 {
		if err := Enqueue(ctx, worker, ev); err != nil {
			t.Fatal(err)
		}
	}

	result, err := p.ProcessBatch(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if result.Written != 1 || result.Duplicates != 1 {
		t.Fatalf("expected 1 written and 1 duplicate, got %+v", result)
	}
	if n := countRows(t, admin, "events", f.tenantID); n != 1 {
		t.Fatalf("expected exactly one event, got %d", n)
	}
	if n := countRows(t, admin, "ingestion_queue", f.tenantID); n != 0 {
		t.Fatalf("expected empty queue, found %d rows", n)
	}
}

func TestInvalidACLGoesToDeadLetter(t *testing.T) {
	worker, admin := testPools(t)
	f := createFixture(t, admin)
	p := newProcessor(t, worker)
	ctx := context.Background()

	ev := f.event()
	ev.ACL = connector.ACL{Scope: connector.ACLScopePrincipals}
	if err := Enqueue(ctx, worker, ev); err != nil {
		t.Fatal(err)
	}

	outcome, _, err := p.ProcessOne(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeDeadLettered {
		t.Fatalf("expected dead letter, got %q", outcome)
	}

	var reason string
	err = admin.QueryRow(ctx,
		`select reason from ingestion_dlq where tenant_id = $1`, f.tenantID).Scan(&reason)
	if err != nil {
		t.Fatal(err)
	}
	if reason != "invalid event: acl_principals_empty" {
		t.Fatalf("unexpected reason %q", reason)
	}
	if n := countRows(t, admin, "events", f.tenantID); n != 0 {
		t.Fatalf("invalid event must never reach events, found %d rows", n)
	}
}

func TestMalformedQueuePayloadGoesToDeadLetter(t *testing.T) {
	worker, admin := testPools(t)
	f := createFixture(t, admin)
	p := newProcessor(t, worker)
	ctx := context.Background()

	_, err := admin.Exec(ctx,
		`insert into ingestion_queue (tenant_id, event) values ($1, '"garbage"')`, f.tenantID)
	if err != nil {
		t.Fatal(err)
	}

	outcome, _, err := p.ProcessOne(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeDeadLettered {
		t.Fatalf("expected dead letter, got %q", outcome)
	}
	if n := countRows(t, admin, "ingestion_dlq", f.tenantID); n != 1 {
		t.Fatalf("expected one dead letter, got %d", n)
	}
}

func TestTenantMismatchGoesToDeadLetter(t *testing.T) {
	worker, admin := testPools(t)
	fA := createFixture(t, admin)
	fB := createFixture(t, admin)
	p := newProcessor(t, worker)
	ctx := context.Background()

	ev := fA.event()
	if err := Enqueue(ctx, worker, ev); err != nil {
		t.Fatal(err)
	}
	_, err := admin.Exec(ctx,
		`update ingestion_queue set tenant_id = $1 where tenant_id = $2`, fB.tenantID, fA.tenantID)
	if err != nil {
		t.Fatal(err)
	}

	outcome, _, err := p.ProcessOne(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeDeadLettered {
		t.Fatalf("expected dead letter, got %q", outcome)
	}

	var reason string
	err = admin.QueryRow(ctx,
		`select reason from ingestion_dlq where tenant_id = $1`, fB.tenantID).Scan(&reason)
	if err != nil {
		t.Fatal(err)
	}
	if reason != "tenant_mismatch" {
		t.Fatalf("unexpected reason %q", reason)
	}
	if n := countRows(t, admin, "events", fA.tenantID); n != 0 {
		t.Fatalf("mismatched event must not be written, found %d rows", n)
	}
}

func TestUnknownConnectorGoesToDeadLetter(t *testing.T) {
	worker, admin := testPools(t)
	f := createFixture(t, admin)
	p := newProcessor(t, worker)
	ctx := context.Background()

	ev := f.event()
	ev.ConnectorID = uuid.New()
	if err := Enqueue(ctx, worker, ev); err != nil {
		t.Fatal(err)
	}

	outcome, _, err := p.ProcessOne(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeDeadLettered {
		t.Fatalf("expected dead letter, got %q", outcome)
	}
	if n := countRows(t, admin, "ingestion_dlq", f.tenantID); n != 1 {
		t.Fatalf("expected one dead letter, got %d", n)
	}
}

type failingPayloadStore struct {
	err error
}

func (s failingPayloadStore) Put(context.Context, uuid.UUID, []byte) (string, error) {
	return "", s.err
}

func TestTransientFailureRetriesThenDeadLetters(t *testing.T) {
	worker, admin := testPools(t)
	f := createFixture(t, admin)
	ctx := context.Background()
	p := &Processor{
		Pool:        worker,
		Payloads:    failingPayloadStore{err: errors.New("storage unavailable")},
		MaxAttempts: 3,
		Backoff:     func(int) time.Duration { return 0 },
	}

	if err := Enqueue(ctx, worker, f.event()); err != nil {
		t.Fatal(err)
	}

	for attempt := 1; attempt <= 2; attempt++ {
		outcome, _, err := p.ProcessOne(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if outcome != OutcomeRetryScheduled {
			t.Fatalf("attempt %d: expected retry, got %q", attempt, outcome)
		}
		var attempts int
		var lastError string
		err = admin.QueryRow(ctx,
			`select attempts, last_error from ingestion_queue where tenant_id = $1`, f.tenantID,
		).Scan(&attempts, &lastError)
		if err != nil {
			t.Fatal(err)
		}
		if attempts != attempt {
			t.Fatalf("expected attempts=%d, got %d", attempt, attempts)
		}
		if lastError == "" {
			t.Fatal("expected last_error to be recorded")
		}
	}

	outcome, _, err := p.ProcessOne(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != OutcomeDeadLettered {
		t.Fatalf("expected dead letter after max attempts, got %q", outcome)
	}
	if n := countRows(t, admin, "ingestion_queue", f.tenantID); n != 0 {
		t.Fatalf("expected empty queue, found %d rows", n)
	}
}

func TestConcurrentWorkersDrainQueueExactlyOnce(t *testing.T) {
	worker, admin := testPools(t)
	f := createFixture(t, admin)
	ctx := context.Background()
	const total = 10

	for range total {
		if err := Enqueue(ctx, worker, f.event()); err != nil {
			t.Fatal(err)
		}
	}

	var mu sync.Mutex
	var combined BatchResult
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := newProcessor(t, worker)
			result, err := p.ProcessBatch(ctx, total)
			if err != nil {
				errs <- err
				return
			}
			mu.Lock()
			combined.Written += result.Written
			combined.Duplicates += result.Duplicates
			mu.Unlock()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	if combined.Written != total || combined.Duplicates != 0 {
		t.Fatalf("expected %d written with no duplicates, got %+v", total, combined)
	}
	if n := countRows(t, admin, "events", f.tenantID); n != total {
		t.Fatalf("expected %d events, got %d", total, n)
	}
	if n := countRows(t, admin, "ingestion_queue", f.tenantID); n != 0 {
		t.Fatalf("expected empty queue, found %d rows", n)
	}
}
