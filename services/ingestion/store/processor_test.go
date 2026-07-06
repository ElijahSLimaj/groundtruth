package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
	"github.com/attempttechnologies/company-brain/services/ingestion/internal/testdb"
)

func eventFor(f testdb.Fixture) connector.NormalizedEvent {
	return connector.NormalizedEvent{
		TenantID:    f.TenantID,
		ConnectorID: f.ConnectorID,
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

func TestProcessWritesEventAndAcks(t *testing.T) {
	worker, admin := testdb.Pools(t)
	f := testdb.CreateFixture(t, admin)
	p := newProcessor(t, worker)
	ctx := context.Background()

	ev := eventFor(f)
	if err := Enqueue(ctx, worker, ev); err != nil {
		t.Fatal(err)
	}

	outcome, found, err := p.ProcessOne(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !found || outcome != OutcomeWritten {
		t.Fatalf("expected written, got found=%v outcome=%q dlq=%v",
			found, outcome, testdb.DLQReasons(t, admin, f.TenantID))
	}

	var externalID, payloadRef string
	err = admin.QueryRow(ctx,
		`select external_id, payload_ref from events where tenant_id = $1`, f.TenantID,
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
	if n := testdb.CountRows(t, admin, "ingestion_queue", f.TenantID); n != 0 {
		t.Fatalf("expected empty queue, found %d rows", n)
	}
}

func TestDuplicateDeliveryIsCountedNoOp(t *testing.T) {
	worker, admin := testdb.Pools(t)
	f := testdb.CreateFixture(t, admin)
	p := newProcessor(t, worker)
	ctx := context.Background()

	ev := eventFor(f)
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
	if n := testdb.CountRows(t, admin, "events", f.TenantID); n != 1 {
		t.Fatalf("expected exactly one event, got %d", n)
	}
	if n := testdb.CountRows(t, admin, "ingestion_queue", f.TenantID); n != 0 {
		t.Fatalf("expected empty queue, found %d rows", n)
	}
}

func TestInvalidACLGoesToDeadLetter(t *testing.T) {
	worker, admin := testdb.Pools(t)
	f := testdb.CreateFixture(t, admin)
	p := newProcessor(t, worker)
	ctx := context.Background()

	ev := eventFor(f)
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

	reasons := testdb.DLQReasons(t, admin, f.TenantID)
	if len(reasons) != 1 || reasons[0] != "invalid event: acl_principals_empty" {
		t.Fatalf("unexpected reasons %v", reasons)
	}
	if n := testdb.CountRows(t, admin, "events", f.TenantID); n != 0 {
		t.Fatalf("invalid event must never reach events, found %d rows", n)
	}
}

func TestMalformedQueuePayloadGoesToDeadLetter(t *testing.T) {
	worker, admin := testdb.Pools(t)
	f := testdb.CreateFixture(t, admin)
	p := newProcessor(t, worker)
	ctx := context.Background()

	_, err := admin.Exec(ctx,
		`insert into ingestion_queue (tenant_id, event) values ($1, '"garbage"')`, f.TenantID)
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
	if n := testdb.CountRows(t, admin, "ingestion_dlq", f.TenantID); n != 1 {
		t.Fatalf("expected one dead letter, got %d", n)
	}
}

func TestTenantMismatchGoesToDeadLetter(t *testing.T) {
	worker, admin := testdb.Pools(t)
	fA := testdb.CreateFixture(t, admin)
	fB := testdb.CreateFixture(t, admin)
	p := newProcessor(t, worker)
	ctx := context.Background()

	ev := eventFor(fA)
	if err := Enqueue(ctx, worker, ev); err != nil {
		t.Fatal(err)
	}
	_, err := admin.Exec(ctx,
		`update ingestion_queue set tenant_id = $1 where tenant_id = $2`, fB.TenantID, fA.TenantID)
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

	reasons := testdb.DLQReasons(t, admin, fB.TenantID)
	if len(reasons) != 1 || reasons[0] != "tenant_mismatch" {
		t.Fatalf("unexpected reasons %v", reasons)
	}
	if n := testdb.CountRows(t, admin, "events", fA.TenantID); n != 0 {
		t.Fatalf("mismatched event must not be written, found %d rows", n)
	}
}

func TestUnknownConnectorGoesToDeadLetter(t *testing.T) {
	worker, admin := testdb.Pools(t)
	f := testdb.CreateFixture(t, admin)
	p := newProcessor(t, worker)
	ctx := context.Background()

	ev := eventFor(f)
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
	if n := testdb.CountRows(t, admin, "ingestion_dlq", f.TenantID); n != 1 {
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
	worker, admin := testdb.Pools(t)
	f := testdb.CreateFixture(t, admin)
	ctx := context.Background()
	p := &Processor{
		Pool:        worker,
		Payloads:    failingPayloadStore{err: errors.New("storage unavailable")},
		MaxAttempts: 3,
		Backoff:     func(int) time.Duration { return 0 },
	}

	if err := Enqueue(ctx, worker, eventFor(f)); err != nil {
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
			`select attempts, last_error from ingestion_queue where tenant_id = $1`, f.TenantID,
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
	if n := testdb.CountRows(t, admin, "ingestion_queue", f.TenantID); n != 0 {
		t.Fatalf("expected empty queue, found %d rows", n)
	}
}

func TestConcurrentWorkersDrainQueueExactlyOnce(t *testing.T) {
	worker, admin := testdb.Pools(t)
	f := testdb.CreateFixture(t, admin)
	ctx := context.Background()
	const total = 10

	for range total {
		if err := Enqueue(ctx, worker, eventFor(f)); err != nil {
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
	if n := testdb.CountRows(t, admin, "events", f.TenantID); n != total {
		t.Fatalf("expected %d events, got %d", total, n)
	}
	if n := testdb.CountRows(t, admin, "ingestion_queue", f.TenantID); n != 0 {
		t.Fatalf("expected empty queue, found %d rows", n)
	}
}
