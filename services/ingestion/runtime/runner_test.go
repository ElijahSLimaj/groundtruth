package runtime

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
	"github.com/attempttechnologies/company-brain/services/ingestion/internal/testdb"
	"github.com/attempttechnologies/company-brain/services/ingestion/keys"
	"github.com/attempttechnologies/company-brain/services/ingestion/store"
)

func testKeyService(t *testing.T, worker *pgxpool.Pool) *keys.Service {
	t.Helper()
	master := make([]byte, 32)
	if _, err := cryptorand.Read(master); err != nil {
		t.Fatal(err)
	}
	wrapper, err := keys.NewAESWrapper(hex.EncodeToString(master))
	if err != nil {
		t.Fatal(err)
	}
	return &keys.Service{Pool: worker, Wrapper: wrapper}
}

func TestInjectSecretDecryptsEncryptedConnectorToken(t *testing.T) {
	worker, admin := testdb.Pools(t)
	f := testdb.CreateFixture(t, admin)
	ctx := context.Background()

	keyService := testKeyService(t, worker)
	dataKey, err := keyService.DataKey(ctx, f.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := store.SealConnectorSecret(dataKey, f.ConnectorID,
		[]byte(`{"access_token":"tok-123","refresh_token":"ref-456"}`))
	if err != nil {
		t.Fatal(err)
	}

	r := &Runner{Pool: worker, Keys: keyService}
	row := connectorRow{
		TenantID:   f.TenantID,
		SourceType: "slack",
		Settings:   map[string]any{"secret": sealed},
	}
	if err := r.injectSecret(ctx, f.ConnectorID, &row); err != nil {
		t.Fatal(err)
	}
	if row.Settings["token"] != "tok-123" {
		t.Fatalf("expected decrypted access token injected, got %v", row.Settings["token"])
	}
	if row.Settings["refresh_token"] != "ref-456" {
		t.Fatalf("expected refresh token injected, got %v", row.Settings["refresh_token"])
	}
}

func TestInjectSecretLeavesPlaintextTokenUntouched(t *testing.T) {
	worker, _ := testdb.Pools(t)
	ctx := context.Background()
	r := &Runner{Pool: worker, Keys: testKeyService(t, worker)}
	row := connectorRow{Settings: map[string]any{"token": "legacy-plain"}}
	if err := r.injectSecret(ctx, uuid.New(), &row); err != nil {
		t.Fatal(err)
	}
	if row.Settings["token"] != "legacy-plain" {
		t.Fatalf("legacy plaintext token must survive, got %v", row.Settings["token"])
	}
}

func TestInjectSecretFailsWhenKeyServiceMissing(t *testing.T) {
	worker, _ := testdb.Pools(t)
	r := &Runner{Pool: worker}
	row := connectorRow{Settings: map[string]any{"secret": "not-decryptable"}}
	if err := r.injectSecret(context.Background(), uuid.New(), &row); err == nil {
		t.Fatal("an encrypted secret with no key service must error, never silently skip")
	}
}

type fakeConnector struct {
	pages      map[connector.Cursor][]connector.RawItem
	next       map[connector.Cursor]connector.Cursor
	backfill   []connector.RawItem
	aclErrOn   string
	health     connector.Health
	polledWith []connector.Cursor
}

func (f *fakeConnector) SourceType() string {
	return "slack"
}

func (f *fakeConnector) Subscribe(context.Context, connector.Config, connector.RawItemSink) error {
	return nil
}

func (f *fakeConnector) Poll(ctx context.Context, _ connector.Config, cursor connector.Cursor, sink connector.RawItemSink) (connector.Cursor, error) {
	f.polledWith = append(f.polledWith, cursor)
	for _, item := range f.pages[cursor] {
		if err := sink.Emit(ctx, item); err != nil {
			return cursor, err
		}
	}
	if next, ok := f.next[cursor]; ok {
		return next, nil
	}
	return cursor, nil
}

func (f *fakeConnector) Backfill(ctx context.Context, _ connector.Config, _ connector.BackfillWindow, sink connector.RawItemSink) error {
	for _, item := range f.backfill {
		if err := sink.Emit(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeConnector) ResolveACL(_ context.Context, _ connector.Config, item connector.RawItem) (connector.ACL, error) {
	if f.aclErrOn == item.ExternalID {
		return connector.ACL{}, errors.New("membership lookup failed")
	}
	return connector.ACL{
		Scope:       connector.ACLScopeTenant,
		SourceScope: connector.SourceScope{Type: "slack_channel", ID: "C1", Visibility: "public"},
	}, nil
}

func (f *fakeConnector) HealthCheck(context.Context, connector.Config) connector.Health {
	return f.health
}

type testNormalizer struct {
	failOn string
}

func (n testNormalizer) Normalize(cfg connector.Config, item connector.RawItem, acl connector.ACL) (connector.NormalizedEvent, error) {
	if n.failOn != "" && item.ExternalID == n.failOn {
		return connector.NormalizedEvent{}, errors.New("unexpected payload shape")
	}
	return connector.NormalizedEvent{
		TenantID:    cfg.TenantID,
		ConnectorID: cfg.ConnectorID,
		SourceType:  cfg.SourceType,
		ExternalID:  item.ExternalID,
		OccurredAt:  item.ReceivedAt,
		ACL:         acl,
		Payload:     connector.Payload{Body: string(item.Body)},
	}, nil
}

func rawItem(externalID string) connector.RawItem {
	return connector.RawItem{
		ExternalID: externalID,
		Body:       []byte(`{"text": "` + externalID + `"}`),
		ReceivedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
}

func newRunner(worker *pgxpool.Pool, fake *fakeConnector, norm Normalizer) *Runner {
	return &Runner{
		Pool:        worker,
		Connectors:  map[string]connector.Connector{"slack": fake},
		Normalizers: map[string]Normalizer{"slack": norm},
	}
}

func savedCursor(t *testing.T, admin *pgxpool.Pool, f testdb.Fixture) (string, bool) {
	t.Helper()
	var cursor string
	err := admin.QueryRow(context.Background(),
		`select poll_cursor from connector_state where connector_id = $1`, f.ConnectorID,
	).Scan(&cursor)
	if err != nil {
		return "", false
	}
	return cursor, true
}

func TestPollCycleEnqueuesWritesAndAdvancesCursor(t *testing.T) {
	worker, admin := testdb.Pools(t)
	f := testdb.CreateFixture(t, admin)
	ctx := context.Background()
	fake := &fakeConnector{
		pages: map[connector.Cursor][]connector.RawItem{
			"": {rawItem("a-1"), rawItem("a-2")},
		},
		next: map[connector.Cursor]connector.Cursor{"": "c-2"},
	}
	r := newRunner(worker, fake, testNormalizer{})

	result, err := r.RunPollCycle(ctx, f.ConnectorID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Enqueued != 2 || result.Cursor != "c-2" {
		t.Fatalf("expected 2 enqueued and cursor c-2, got %+v", result)
	}
	if cursor, ok := savedCursor(t, admin, f); !ok || cursor != "c-2" {
		t.Fatalf("expected persisted cursor c-2, got %q found=%v", cursor, ok)
	}

	p := &store.Processor{Pool: worker, Payloads: &store.FSPayloadStore{Root: t.TempDir()}}
	drained, err := p.ProcessBatch(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if drained.Written != 2 {
		t.Fatalf("expected 2 events written, got %+v", drained)
	}
	if n := testdb.CountRows(t, admin, "events", f.TenantID); n != 2 {
		t.Fatalf("expected 2 events, got %d", n)
	}

	result, err = r.RunPollCycle(ctx, f.ConnectorID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Enqueued != 0 {
		t.Fatalf("expected nothing new on second cycle, got %+v", result)
	}
	if len(fake.polledWith) != 2 || fake.polledWith[0] != "" || fake.polledWith[1] != "c-2" {
		t.Fatalf("expected polls with cursors [\"\", c-2], got %v", fake.polledWith)
	}
}

func TestACLFailureAbortsCycleWithoutAdvancingCursor(t *testing.T) {
	worker, admin := testdb.Pools(t)
	f := testdb.CreateFixture(t, admin)
	ctx := context.Background()
	fake := &fakeConnector{
		pages: map[connector.Cursor][]connector.RawItem{
			"": {rawItem("a-1"), rawItem("a-2")},
		},
		next:     map[connector.Cursor]connector.Cursor{"": "c-2"},
		aclErrOn: "a-2",
	}
	r := newRunner(worker, fake, testNormalizer{})

	result, err := r.RunPollCycle(ctx, f.ConnectorID)
	if err == nil {
		t.Fatal("expected the cycle to fail on ACL resolution")
	}
	if !strings.Contains(err.Error(), "resolve acl for a-2") {
		t.Fatalf("unexpected error %v", err)
	}
	if result.Enqueued != 1 {
		t.Fatalf("expected the item before the failure to be enqueued, got %+v", result)
	}
	if _, ok := savedCursor(t, admin, f); ok {
		t.Fatal("cursor must not advance past an item stuck on ACL resolution")
	}
	if n := testdb.CountRows(t, admin, "ingestion_dlq", f.TenantID); n != 0 {
		t.Fatalf("ACL failures are stuck, never dead lettered, found %d dlq rows", n)
	}
}

func TestNormalizeFailureDeadLettersAndContinues(t *testing.T) {
	worker, admin := testdb.Pools(t)
	f := testdb.CreateFixture(t, admin)
	ctx := context.Background()
	fake := &fakeConnector{
		pages: map[connector.Cursor][]connector.RawItem{
			"": {rawItem("a-1"), rawItem("a-2")},
		},
		next: map[connector.Cursor]connector.Cursor{"": "c-2"},
	}
	r := newRunner(worker, fake, testNormalizer{failOn: "a-1"})

	result, err := r.RunPollCycle(ctx, f.ConnectorID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Enqueued != 1 || result.DeadLettered != 1 {
		t.Fatalf("expected 1 enqueued and 1 dead lettered, got %+v", result)
	}
	reasons := testdb.DLQReasons(t, admin, f.TenantID)
	if len(reasons) != 1 || !strings.HasPrefix(reasons[0], "normalize: ") {
		t.Fatalf("unexpected dlq reasons %v", reasons)
	}
	if cursor, ok := savedCursor(t, admin, f); !ok || cursor != "c-2" {
		t.Fatalf("dead lettered item must not block the cursor, got %q found=%v", cursor, ok)
	}
}

func TestDegradedConnectorSkipsPoll(t *testing.T) {
	worker, admin := testdb.Pools(t)
	f := testdb.CreateFixture(t, admin)
	ctx := context.Background()
	if _, err := admin.Exec(ctx,
		`update connectors set status = 'degraded' where id = $1`, f.ConnectorID); err != nil {
		t.Fatal(err)
	}
	fake := &fakeConnector{}
	r := newRunner(worker, fake, testNormalizer{})

	result, err := r.RunPollCycle(ctx, f.ConnectorID)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Skipped {
		t.Fatalf("expected degraded connector to be skipped, got %+v", result)
	}
	if len(fake.polledWith) != 0 {
		t.Fatal("degraded connector must not be polled")
	}
}

func TestHealthCheckFlipsStatusBothWays(t *testing.T) {
	worker, admin := testdb.Pools(t)
	f := testdb.CreateFixture(t, admin)
	ctx := context.Background()
	fake := &fakeConnector{health: connector.Health{State: connector.HealthDegraded, Message: "token expired"}}
	r := newRunner(worker, fake, testNormalizer{})

	transition, err := r.RunHealthCheck(ctx, f.ConnectorID)
	if err != nil {
		t.Fatal(err)
	}
	if transition.From != StatusLive || transition.To != StatusDegraded {
		t.Fatalf("expected live to degraded, got %+v", transition)
	}
	var status string
	if err := admin.QueryRow(ctx,
		`select status from connectors where id = $1`, f.ConnectorID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != StatusDegraded {
		t.Fatalf("expected stored status degraded, got %q", status)
	}

	fake.health = connector.Health{State: connector.HealthLive}
	transition, err = r.RunHealthCheck(ctx, f.ConnectorID)
	if err != nil {
		t.Fatal(err)
	}
	if transition.From != StatusDegraded || transition.To != StatusLive {
		t.Fatalf("expected degraded to live, got %+v", transition)
	}

	transition, err = r.RunHealthCheck(ctx, f.ConnectorID)
	if err != nil {
		t.Fatal(err)
	}
	if transition.From != StatusLive || transition.To != StatusLive {
		t.Fatalf("expected steady live state, got %+v", transition)
	}
}

func TestBackfillFlowsThroughTheSameSink(t *testing.T) {
	worker, admin := testdb.Pools(t)
	f := testdb.CreateFixture(t, admin)
	ctx := context.Background()
	fake := &fakeConnector{
		backfill: []connector.RawItem{rawItem("old-1"), rawItem("old-2"), rawItem("old-3")},
	}
	r := newRunner(worker, fake, testNormalizer{})

	result, err := r.RunBackfill(ctx, f.ConnectorID, connector.BackfillWindow{
		From: time.Now().Add(-24 * time.Hour),
		To:   time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Enqueued != 3 {
		t.Fatalf("expected 3 enqueued from backfill, got %+v", result)
	}
	if _, ok := savedCursor(t, admin, f); ok {
		t.Fatal("backfill must not touch the poll cursor")
	}
	if n := testdb.CountRows(t, admin, "ingestion_queue", f.TenantID); n != 3 {
		t.Fatalf("expected 3 queue rows, got %d", n)
	}
}
