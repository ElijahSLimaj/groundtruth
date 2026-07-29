package outlook

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
)

type deltaStep struct {
	ids  []string
	next string
}

type fakeAPI struct {
	email      string
	steps      map[string]deltaStep
	queries    []string
	queryMsgs  []string
	messages   map[string]Message
	profileErr error
}

func stepKey(folderID, deltaLink string) string {
	return folderID + "\x00" + deltaLink
}

func (f *fakeAPI) Profile(context.Context) (string, error) {
	if f.profileErr != nil {
		return "", f.profileErr
	}
	return f.email, nil
}

func (f *fakeAPI) FolderDelta(_ context.Context, folderID, deltaLink string) ([]string, string, error) {
	step := f.steps[stepKey(folderID, deltaLink)]
	return step.ids, step.next, nil
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
		ID:               id,
		ThreadID:         "conversation-" + id,
		ReceivedDateTime: time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC),
		From:             "sam@acme.test",
		To:               []string{"ada@acme.test"},
		Cc:               []string{"eve@acme.test"},
		Subject:          subject,
		Body:             body,
	}
}

func testConfig() connector.Config {
	return connector.Config{
		ConnectorID: uuid.New(),
		TenantID:    uuid.New(),
		SourceType:  SourceType,
		Settings:    map[string]any{"token": "outlook-token"},
	}
}

func TestPollBootstrapsInboxDeltaAndEmitsInitialSync(t *testing.T) {
	t.Parallel()
	fake := &fakeAPI{
		steps: map[string]deltaStep{
			stepKey("inbox", ""): {ids: []string{"m1", "m2"}, next: "https://graph/delta?token=1"},
		},
		messages: map[string]Message{
			"m1": message("m1", "Quote for Meridian", "growth at 1499/mo"),
			"m2": message("m2", "Re: onboarding", "welcome aboard"),
		},
	}
	c := New(func(string) API { return fake })
	sink := &collectSink{}

	cursor, err := c.Poll(context.Background(), testConfig(), "", sink)
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.items) != 2 || sink.items[0].ExternalID != "m1" {
		t.Fatalf("initial folder delta must full-sync the folder, got %+v", sink.items)
	}
	if string(cursor) != `{"inbox":"https://graph/delta?token=1"}` {
		t.Fatalf("cursor must carry the per-folder delta link, got %q", cursor)
	}
}

func TestPollAdvancesFromStoredDeltaLink(t *testing.T) {
	t.Parallel()
	fake := &fakeAPI{
		steps: map[string]deltaStep{
			stepKey("inbox", "https://graph/delta?token=1"): {ids: []string{"m3"}, next: "https://graph/delta?token=2"},
		},
		messages: map[string]Message{"m3": message("m3", "follow up", "as discussed")},
	}
	c := New(func(string) API { return fake })
	sink := &collectSink{}

	cursor, err := c.Poll(context.Background(), testConfig(), `{"inbox":"https://graph/delta?token=1"}`, sink)
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.items) != 1 || sink.items[0].ExternalID != "m3" {
		t.Fatalf("unexpected items %+v", sink.items)
	}
	if string(cursor) != `{"inbox":"https://graph/delta?token=2"}` {
		t.Fatalf("cursor must advance to the new delta link, got %q", cursor)
	}
}

func TestPollSyncsEachConfiguredFolderIndependently(t *testing.T) {
	t.Parallel()
	fake := &fakeAPI{
		steps: map[string]deltaStep{
			stepKey("inbox", ""):     {ids: []string{"i1"}, next: "https://graph/inbox?d=1"},
			stepKey("sentitems", ""): {ids: []string{"s1"}, next: "https://graph/sent?d=1"},
		},
		messages: map[string]Message{
			"i1": message("i1", "received", "body"),
			"s1": message("s1", "sent", "body"),
		},
	}
	c := New(func(string) API { return fake })
	cfg := testConfig()
	cfg.Settings["folders"] = []any{"inbox", "sentitems"}
	sink := &collectSink{}

	cursor, err := c.Poll(context.Background(), cfg, "", sink)
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.items) != 2 {
		t.Fatalf("expected both folders synced, got %d items", len(sink.items))
	}
	if !strings.Contains(string(cursor), `"inbox":"https://graph/inbox?d=1"`) ||
		!strings.Contains(string(cursor), `"sentitems":"https://graph/sent?d=1"`) {
		t.Fatalf("cursor must track each folder delta link, got %q", cursor)
	}
}

func TestExclusionFiltersDropAndCountBeforeEmit(t *testing.T) {
	t.Parallel()
	fake := &fakeAPI{
		steps: map[string]deltaStep{
			stepKey("inbox", ""): {ids: []string{"ok", "pay", "hr", "categorized"}, next: "https://graph/delta?d=2"},
		},
		messages: map[string]Message{
			"ok":  message("ok", "Quote for Meridian", "growth at 1499/mo"),
			"pay": message("pay", "March payroll run", "salaries attached"),
			"hr":  message("hr", "confidential", "the disciplinary hearing is tuesday"),
			"categorized": func() Message {
				m := message("categorized", "board notes", "hello")
				m.Categories = []string{"HR"}
				return m
			}(),
		},
	}
	c := New(func(string) API { return fake })
	cfg := testConfig()
	cfg.Settings["excluded_categories"] = []any{"HR"}
	sink := &collectSink{}

	if _, err := c.Poll(context.Background(), cfg, "", sink); err != nil {
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
	wantQuery := "receivedDateTime ge 2026-04-01T00:00:00Z and receivedDateTime le 2026-07-01T00:00:00Z"
	if len(fake.queries) != 1 || fake.queries[0] != wantQuery {
		t.Fatalf("unexpected query %v", fake.queries)
	}
	if len(sink.items) != 1 {
		t.Fatalf("expected 1 backfilled item, got %d", len(sink.items))
	}
}

func pollOne(t *testing.T, id, subject, body string) connector.RawItem {
	t.Helper()
	fake := &fakeAPI{
		steps:    map[string]deltaStep{stepKey("inbox", ""): {ids: []string{id}, next: "https://graph/delta?d=2"}},
		messages: map[string]Message{id: message(id, subject, body)},
	}
	c := New(func(string) API { return fake })
	sink := &collectSink{}
	if _, err := c.Poll(context.Background(), testConfig(), "", sink); err != nil {
		t.Fatal(err)
	}
	if len(sink.items) != 1 {
		t.Fatalf("expected one polled item, got %d", len(sink.items))
	}
	return sink.items[0]
}

func TestResolveACLIsAlwaysPrincipalsFromRecipients(t *testing.T) {
	t.Parallel()
	c := New(func(string) API { return &fakeAPI{} })
	item := pollOne(t, "m1", "subject", "body")

	acl, err := c.ResolveACL(context.Background(), testConfig(), item)
	if err != nil {
		t.Fatal(err)
	}
	if acl.Scope != connector.ACLScopePrincipals {
		t.Fatalf("outlook ACL widened to %q, must always be principals", acl.Scope)
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
	if acl.SourceScope.Type != "outlook_thread" || acl.SourceScope.ID != "conversation-m1" {
		t.Fatalf("unexpected source scope %+v", acl.SourceScope)
	}
}

func TestResolveACLNeverWidensForPrivateMessage(t *testing.T) {
	t.Parallel()
	c := New(func(string) API { return &fakeAPI{} })
	item := pollOne(t, "private-1", "internal strategy", "confidential to the three of us")

	acl, err := c.ResolveACL(context.Background(), testConfig(), item)
	if err != nil {
		t.Fatal(err)
	}
	if acl.Scope != connector.ACLScopePrincipals {
		t.Fatalf("private message ACL widened to %q", acl.Scope)
	}
	if acl.SourceScope.Visibility == "shared" {
		t.Fatal("private message must never be marked shared")
	}
	want := []string{"email:ada@acme.test", "email:eve@acme.test", "email:sam@acme.test"}
	if len(acl.Principals) != len(want) {
		t.Fatalf("private message principals leaked or dropped: %v", acl.Principals)
	}
	for i, principal := range want {
		if acl.Principals[i] != principal {
			t.Fatalf("expected %v, got %v", want, acl.Principals)
		}
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

func TestSubscribeIsUnsupported(t *testing.T) {
	t.Parallel()
	c := New(func(string) API { return &fakeAPI{} })
	if err := c.Subscribe(context.Background(), testConfig(), &collectSink{}); !errors.Is(err, ErrSubscribeUnsupported) {
		t.Fatalf("expected ErrSubscribeUnsupported, got %v", err)
	}
}
