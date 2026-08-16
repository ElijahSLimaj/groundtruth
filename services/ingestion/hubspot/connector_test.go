package hubspot

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
)

type fakeAPI struct {
	pages    map[string]Page
	pingErr  error
	searched []string
}

func (f *fakeAPI) Search(_ context.Context, gte, lte, after string) (Page, error) {
	f.searched = append(f.searched, gte+"|"+lte+"|"+after)
	return f.pages[after], nil
}

func (f *fakeAPI) Ping(context.Context) error {
	return f.pingErr
}

type collectSink struct {
	items []connector.RawItem
}

func (s *collectSink) Emit(_ context.Context, item connector.RawItem) error {
	s.items = append(s.items, item)
	return nil
}

func deal(id, name string, msAfterEpoch int64) Record {
	return Record{
		ID:         id,
		Properties: map[string]string{"dealname": name, "amount": "1500", "dealstage": "presentationscheduled"},
		UpdatedAt:  time.UnixMilli(msAfterEpoch).UTC(),
	}
}

func testConfig() connector.Config {
	return connector.Config{
		ConnectorID: uuid.New(),
		TenantID:    uuid.New(),
		SourceType:  SourceType,
		Settings:    map[string]any{"token": "hs-token"},
	}
}

func TestPollBootstrapsCursorWithoutIngesting(t *testing.T) {
	t.Parallel()
	c := New(func(string) API { return &fakeAPI{} })
	sink := &collectSink{}
	cursor, err := c.Poll(context.Background(), testConfig(), "", sink)
	if err != nil {
		t.Fatal(err)
	}
	if cursor == "" {
		t.Fatal("bootstrap must set a cursor")
	}
	if len(sink.items) != 0 {
		t.Fatalf("bootstrap must not ingest, got %d", len(sink.items))
	}
}

func TestPollDrainsPagesAndAdvancesCursor(t *testing.T) {
	t.Parallel()
	fake := &fakeAPI{
		pages: map[string]Page{
			"":   {Records: []Record{deal("1", "Meridian", 2000), deal("2", "Acme", 3000)}, NextAfter: "p2"},
			"p2": {Records: []Record{deal("3", "Zenith", 5000)}, NextAfter: ""},
		},
	}
	c := New(func(string) API { return fake })
	sink := &collectSink{}
	cursor, err := c.Poll(context.Background(), testConfig(), "1000", sink)
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.items) != 3 {
		t.Fatalf("expected 3 deals emitted, got %d", len(sink.items))
	}
	if string(cursor) != "5001" {
		t.Fatalf("cursor must advance past the newest modified time, got %q", cursor)
	}
	if fake.searched[0] != "1000||" {
		t.Fatalf("first search must use the stored cursor as GTE, got %q", fake.searched[0])
	}
}

func TestBackfillSearchesTheWindow(t *testing.T) {
	t.Parallel()
	fake := &fakeAPI{
		pages: map[string]Page{"": {Records: []Record{deal("9", "Old", 500)}}},
	}
	c := New(func(string) API { return fake })
	sink := &collectSink{}
	window := connector.BackfillWindow{
		From: time.UnixMilli(100).UTC(),
		To:   time.UnixMilli(900).UTC(),
	}
	if err := c.Backfill(context.Background(), testConfig(), window, sink); err != nil {
		t.Fatal(err)
	}
	if len(sink.items) != 1 {
		t.Fatalf("expected 1 backfilled deal, got %d", len(sink.items))
	}
	if fake.searched[0] != "100|900|" {
		t.Fatalf("backfill must bound the window with GTE and LTE, got %q", fake.searched[0])
	}
}

func TestResolveACLIsTenantScoped(t *testing.T) {
	t.Parallel()
	fake := &fakeAPI{pages: map[string]Page{"": {Records: []Record{deal("42", "Deal", 2000)}}}}
	c := New(func(string) API { return fake })
	sink := &collectSink{}
	if _, err := c.Poll(context.Background(), testConfig(), "1000", sink); err != nil {
		t.Fatal(err)
	}
	acl, err := c.ResolveACL(context.Background(), testConfig(), sink.items[0])
	if err != nil {
		t.Fatal(err)
	}
	if acl.Scope != connector.ACLScopeTenant {
		t.Fatalf("CRM records are tenant scoped by convention, got %q", acl.Scope)
	}
	if acl.SourceScope.Type != "hubspot_deal" || acl.SourceScope.ID != "42" {
		t.Fatalf("unexpected source scope %+v", acl.SourceScope)
	}
}

func TestResolveACLRejectsRecordWithoutID(t *testing.T) {
	t.Parallel()
	c := New(func(string) API { return &fakeAPI{} })
	item := connector.RawItem{Body: []byte(`{"record":{"Properties":{}}}`)}
	if _, err := c.ResolveACL(context.Background(), testConfig(), item); err == nil {
		t.Fatal("a record without an id must not produce an ACL")
	}
}

func TestHealthCheckReflectsPing(t *testing.T) {
	t.Parallel()
	healthy := New(func(string) API { return &fakeAPI{} })
	if h := healthy.HealthCheck(context.Background(), testConfig()); h.State != connector.HealthLive {
		t.Fatalf("expected live, got %+v", h)
	}
	broken := New(func(string) API { return &fakeAPI{pingErr: errors.New("401")} })
	if h := broken.HealthCheck(context.Background(), testConfig()); h.State != connector.HealthDegraded {
		t.Fatalf("expected degraded, got %+v", h)
	}
}

func TestSubscribeIsUnsupported(t *testing.T) {
	t.Parallel()
	c := New(func(string) API { return &fakeAPI{} })
	if err := c.Subscribe(context.Background(), testConfig(), &collectSink{}); !errors.Is(err, ErrSubscribeUnsupported) {
		t.Fatalf("expected ErrSubscribeUnsupported, got %v", err)
	}
}
