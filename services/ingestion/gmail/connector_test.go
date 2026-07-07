package gmail

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
)

type fakeAPI struct {
	email       string
	historyID   uint64
	historyMsgs map[uint64][]string
	nextHistory uint64
	queries     []string
	queryMsgs   []string
	messages    map[string]Message
	profileErr  error
}

func (f *fakeAPI) Profile(context.Context) (string, uint64, error) {
	if f.profileErr != nil {
		return "", 0, f.profileErr
	}
	return f.email, f.historyID, nil
}

func (f *fakeAPI) ListHistory(_ context.Context, start uint64) ([]string, uint64, error) {
	return f.historyMsgs[start], f.nextHistory, nil
}

func (f *fakeAPI) ListMessages(_ context.Context, query string) ([]string, error) {
	f.queries = append(f.queries, query)
	return f.queryMsgs, nil
}

func (f *fakeAPI) GetMessage(_ context.Context, id string) (Message, error) {
	message, ok := f.messages[id]
	if !ok {
		return Message{}, errors.New("message not found")
	}
	return message, nil
}

type collectSink struct {
	items []connector.RawItem
}

func (s *collectSink) Emit(_ context.Context, item connector.RawItem) error {
	s.items = append(s.items, item)
	return nil
}

func message(id, subject, body string) Message {
	return Message{
		ID:           id,
		ThreadID:     "thread-" + id,
		InternalDate: time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC),
		From:         "sam@acme.test",
		To:           []string{"ada@acme.test"},
		Cc:           []string{"eve@acme.test"},
		Subject:      subject,
		Body:         body,
	}
}

func testConfig() connector.Config {
	return connector.Config{
		ConnectorID: uuid.New(),
		TenantID:    uuid.New(),
		SourceType:  SourceType,
		Settings:    map[string]any{"token": "gmail-token"},
	}
}

func TestPollInitializesCursorFromProfileWithoutIngesting(t *testing.T) {
	t.Parallel()
	fake := &fakeAPI{email: "brain@acme.test", historyID: 5000}
	c := New(func(string) API { return fake })
	sink := &collectSink{}

	cursor, err := c.Poll(context.Background(), testConfig(), "", sink)
	if err != nil {
		t.Fatal(err)
	}
	if cursor != "5000" {
		t.Fatalf("expected cursor initialized to profile history id, got %q", cursor)
	}
	if len(sink.items) != 0 {
		t.Fatalf("initial poll must not ingest, got %d items", len(sink.items))
	}
}

func TestPollEmitsHistoryMessagesAndAdvances(t *testing.T) {
	t.Parallel()
	fake := &fakeAPI{
		historyMsgs: map[uint64][]string{5000: {"m1", "m2"}},
		nextHistory: 5100,
		messages: map[string]Message{
			"m1": message("m1", "Quote for Meridian", "growth at 1499/mo"),
			"m2": message("m2", "Re: onboarding", "welcome aboard"),
		},
	}
	c := New(func(string) API { return fake })
	sink := &collectSink{}

	cursor, err := c.Poll(context.Background(), testConfig(), "5000", sink)
	if err != nil {
		t.Fatal(err)
	}
	if cursor != "5100" {
		t.Fatalf("expected cursor 5100, got %q", cursor)
	}
	if len(sink.items) != 2 || sink.items[0].ExternalID != "m1" {
		t.Fatalf("unexpected items %+v", sink.items)
	}
}

func TestExclusionFiltersDropAndCountBeforeEmit(t *testing.T) {
	t.Parallel()
	fake := &fakeAPI{
		historyMsgs: map[uint64][]string{1: {"ok", "pay", "hr", "labeled"}},
		nextHistory: 2,
		messages: map[string]Message{
			"ok":  message("ok", "Quote for Meridian", "growth at 1499/mo"),
			"pay": message("pay", "March payroll run", "salaries attached"),
			"hr":  message("hr", "confidential", "the disciplinary hearing is tuesday"),
			"labeled": func() Message {
				m := message("labeled", "board notes", "hello")
				m.LabelIDs = []string{"Label_HR"}
				return m
			}(),
		},
	}
	c := New(func(string) API { return fake })
	cfg := testConfig()
	cfg.Settings["excluded_labels"] = []any{"Label_HR"}
	sink := &collectSink{}

	if _, err := c.Poll(context.Background(), cfg, "1", sink); err != nil {
		t.Fatal(err)
	}
	if len(sink.items) != 1 || sink.items[0].ExternalID != "ok" {
		t.Fatalf("expected only the clean message, got %+v", sink.items)
	}
	if c.ExcludedCount() != 3 {
		t.Fatalf("expected 3 exclusions counted, got %d", c.ExcludedCount())
	}
	for _, item := range sink.items {
		if strings.Contains(string(item.Body), "payroll") {
			t.Fatal("excluded content must never be stored")
		}
	}
}

func TestBackfillQueriesTheWindow(t *testing.T) {
	t.Parallel()
	fake := &fakeAPI{
		queryMsgs: []string{"m9"},
		messages:  map[string]Message{"m9": message("m9", "old quote", "starter at 499")},
	}
	c := New(func(string) API { return fake })
	sink := &collectSink{}
	window := connector.BackfillWindow{
		From: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC),
	}

	if err := c.Backfill(context.Background(), testConfig(), window, sink); err != nil {
		t.Fatal(err)
	}
	if len(fake.queries) != 1 || fake.queries[0] != "after:2026/04/01 before:2026/07/01" {
		t.Fatalf("unexpected query %v", fake.queries)
	}
	if len(sink.items) != 1 {
		t.Fatalf("expected 1 backfilled item, got %d", len(sink.items))
	}
}

func TestResolveACLIsAlwaysPrincipalsFromRecipients(t *testing.T) {
	t.Parallel()
	c := New(func(string) API { return &fakeAPI{} })
	sink := &collectSink{}
	fake := &fakeAPI{
		historyMsgs: map[uint64][]string{1: {"m1"}},
		nextHistory: 2,
		messages:    map[string]Message{"m1": message("m1", "subject", "body")},
	}
	c.NewAPI = func(string) API { return fake }
	if _, err := c.Poll(context.Background(), testConfig(), "1", sink); err != nil {
		t.Fatal(err)
	}

	acl, err := c.ResolveACL(context.Background(), testConfig(), sink.items[0])
	if err != nil {
		t.Fatal(err)
	}
	if acl.Scope != connector.ACLScopePrincipals {
		t.Fatalf("gmail ACL widened to %q, must always be principals", acl.Scope)
	}
	want := []string{"email:ada@acme.test", "email:eve@acme.test", "email:sam@acme.test"}
	if len(acl.Principals) != len(want) {
		t.Fatalf("unexpected principals %v", acl.Principals)
	}
	for i, principal := range want {
		if acl.Principals[i] != principal {
			t.Fatalf("expected %v, got %v", want, acl.Principals)
		}
	}
	if acl.SourceScope.Type != "gmail_thread" || acl.SourceScope.ID != "thread-m1" {
		t.Fatalf("unexpected source scope %+v", acl.SourceScope)
	}
}

func TestResolveACLWithoutRecipientsIsStuck(t *testing.T) {
	t.Parallel()
	c := New(func(string) API { return &fakeAPI{} })
	item := connector.RawItem{
		ExternalID: "m1",
		Body:       []byte(`{"message": {"ID": "m1"}}`),
	}
	if _, err := c.ResolveACL(context.Background(), testConfig(), item); err == nil {
		t.Fatal("a message without recipients must never produce an ACL")
	}
}

func TestHealthCheckReflectsProfileAuth(t *testing.T) {
	t.Parallel()
	healthy := New(func(string) API { return &fakeAPI{email: "x@y.z"} })
	if h := healthy.HealthCheck(context.Background(), testConfig()); h.State != connector.HealthLive {
		t.Fatalf("expected live, got %+v", h)
	}
	broken := New(func(string) API {
		return &fakeAPI{profileErr: errors.New("invalid_grant")}
	})
	if h := broken.HealthCheck(context.Background(), testConfig()); h.State != connector.HealthDegraded {
		t.Fatalf("expected degraded, got %+v", h)
	}
}

func TestNormalizerMapsMessagesAndPreservesACL(t *testing.T) {
	t.Parallel()
	msg := message("m1", "Quote", "growth at 1499/mo")
	body, err := json.Marshal(Envelope{Message: msg})
	if err != nil {
		t.Fatal(err)
	}
	acl := connector.ACL{
		Scope:      connector.ACLScopePrincipals,
		Principals: []string{"email:ada@acme.test"},
		SourceScope: connector.SourceScope{
			Type: "gmail_thread",
			ID:   "thread-m1",
		},
	}
	cfg := testConfig()

	ev, err := Normalizer{}.Normalize(cfg, connector.RawItem{ExternalID: "m1", Body: body}, acl)
	if err != nil {
		t.Fatal(err)
	}
	if ev.ExternalID != "m1" || ev.ThreadKey != "thread-m1" || ev.SourceType != "gmail" {
		t.Fatalf("unexpected event %+v", ev)
	}
	if ev.AuthorRef.SourceRef != "email:sam@acme.test" {
		t.Fatalf("unexpected author %+v", ev.AuthorRef)
	}
	if ev.Payload.Body != "growth at 1499/mo" {
		t.Fatalf("unexpected body %q", ev.Payload.Body)
	}
	if ev.ACL.Scope != connector.ACLScopePrincipals || len(ev.ACL.Principals) != 1 {
		t.Fatal("normalizer must pass the ACL through untouched")
	}
}
