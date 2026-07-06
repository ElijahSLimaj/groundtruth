package slack

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
	"github.com/attempttechnologies/company-brain/services/ingestion/internal/testdb"
	"github.com/attempttechnologies/company-brain/services/ingestion/runtime"
	"github.com/attempttechnologies/company-brain/services/ingestion/store"
)

func TestSlackEndToEndWithBackfillOverlap(t *testing.T) {
	worker, admin := testdb.Pools(t)
	f := testdb.CreateFixture(t, admin)
	ctx := context.Background()

	if _, err := admin.Exec(ctx,
		`update connectors set config = '{"token": "xoxb-test"}' where id = $1`, f.ConnectorID); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	ts := func(offset time.Duration) string { return formatTS(now.Add(offset)) }
	fake := &fakeAPI{
		channels: []Channel{
			{ID: "C1", Name: "general"},
			{ID: "P1", Name: "founders", IsPrivate: true},
		},
		history: map[string][]json.RawMessage{
			"C1": {msg(ts(-1*time.Minute), "U1", "newest public"), msg(ts(-3*time.Minute), "U2", "oldest public")},
			"P1": {msg(ts(-2*time.Minute), "U1", "private note")},
		},
		members: map[string][]string{"P1": {"U1", "U3"}},
	}

	slackConnector := New(func(string) API { return fake })
	r := &runtime.Runner{
		Pool:        worker,
		Connectors:  map[string]connector.Connector{SourceType: slackConnector},
		Normalizers: map[string]runtime.Normalizer{SourceType: Normalizer{}},
	}
	p := &store.Processor{Pool: worker, Payloads: &store.FSPayloadStore{Root: t.TempDir()}}

	cycle, err := r.RunPollCycle(ctx, f.ConnectorID)
	if err != nil {
		t.Fatal(err)
	}
	if cycle.Enqueued != 3 {
		t.Fatalf("expected 3 enqueued from poll, got %+v", cycle)
	}
	drained, err := p.ProcessBatch(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if drained.Written != 3 || drained.DeadLettered != 0 {
		t.Fatalf("expected 3 events written, got %+v (dlq %v)", drained, testdb.DLQReasons(t, admin, f.TenantID))
	}

	var publicCount, privateCount int
	err = admin.QueryRow(ctx, `
		select
			count(*) filter (where acl->>'scope' = 'tenant'),
			count(*) filter (where acl->>'scope' = 'principals')
		from events where tenant_id = $1
	`, f.TenantID).Scan(&publicCount, &privateCount)
	if err != nil {
		t.Fatal(err)
	}
	if publicCount != 2 || privateCount != 1 {
		t.Fatalf("expected 2 tenant-scoped and 1 principals-scoped events, got %d and %d", publicCount, privateCount)
	}

	var principals []string
	err = admin.QueryRow(ctx, `
		select array(select jsonb_array_elements_text(acl->'principals'))
		from events where tenant_id = $1 and acl->>'scope' = 'principals'
	`, f.TenantID).Scan(&principals)
	if err != nil {
		t.Fatal(err)
	}
	if len(principals) != 2 || principals[0] != "slack:U1" || principals[1] != "slack:U3" {
		t.Fatalf("private event lost its membership ACL: %v", principals)
	}

	backfill, err := r.RunBackfill(ctx, f.ConnectorID, connector.BackfillWindow{
		From: now.Add(-10 * time.Minute),
		To:   now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if backfill.Enqueued != 3 {
		t.Fatalf("expected backfill to re-emit 3 overlapping items, got %+v", backfill)
	}
	redrained, err := p.ProcessBatch(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if redrained.Written != 0 || redrained.Duplicates != 3 {
		t.Fatalf("backfill overlap must be absorbed as duplicates, got %+v", redrained)
	}
	if n := testdb.CountRows(t, admin, "events", f.TenantID); n != 3 {
		t.Fatalf("expected exactly 3 events after overlap, got %d", n)
	}
}
