package notion

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
)

type fakeAPI struct {
	pages   [][]Page
	cursors []string
	text    map[string]string
}

func (f *fakeAPI) SearchPages(_ context.Context, startCursor string) ([]Page, string, error) {
	index := 0
	for i, cursor := range f.cursors {
		if cursor == startCursor {
			index = i
			break
		}
	}
	next := ""
	if index+1 < len(f.pages) {
		next = f.cursors[index+1]
	}
	return f.pages[index], next, nil
}

func (f *fakeAPI) PageText(_ context.Context, pageID string) (string, error) {
	return f.text[pageID], nil
}

func (f *fakeAPI) Me(context.Context) error { return nil }

type collectSink struct {
	items []connector.RawItem
}

func (s *collectSink) Emit(_ context.Context, item connector.RawItem) error {
	s.items = append(s.items, item)
	return nil
}

func page(id string, edited time.Time) Page {
	return Page{
		ID:             id,
		Title:          "Runbook " + id,
		URL:            "https://notion.so/" + id,
		LastEditedTime: edited,
		LastEditedBy:   "user-1",
	}
}

func testConfig() connector.Config {
	return connector.Config{
		ConnectorID: uuid.New(),
		TenantID:    uuid.New(),
		SourceType:  SourceType,
		Settings:    map[string]any{"token": "notion-token"},
	}
}

func TestNotionConnector(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	t.Run("emits pages newer than the watermark across pagination", func(t *testing.T) {
		fake := &fakeAPI{
			pages: [][]Page{
				{page("P3", base.Add(3*time.Hour)), page("P2", base.Add(2*time.Hour))},
				{page("P1", base.Add(1*time.Hour)), page("P0", base.Add(-1*time.Hour))},
			},
			cursors: []string{"", "c2"},
			text: map[string]string{
				"P1": "deploys happen tuesdays",
				"P2": "refunds within 30 days",
				"P3": "oncall rotates weekly",
			},
		}
		sink := &collectSink{}
		c := New(func(string) API { return fake })
		next, err := c.Poll(ctx, testConfig(), connector.Cursor(base.Format(time.RFC3339Nano)), sink)
		if err != nil {
			t.Fatal(err)
		}
		if len(sink.items) != 3 {
			t.Fatalf("items = %d, want 3", len(sink.items))
		}
		var first Envelope
		if err := json.Unmarshal(sink.items[0].Body, &first); err != nil {
			t.Fatal(err)
		}
		if first.Page.ID != "P1" {
			t.Fatalf("first emitted = %s, want oldest P1", first.Page.ID)
		}
		wantCursor := base.Add(3 * time.Hour).Format(time.RFC3339Nano)
		if string(next) != wantCursor {
			t.Fatalf("cursor = %q, want %q", next, wantCursor)
		}
	})

	t.Run("second poll with the advanced cursor emits nothing new", func(t *testing.T) {
		fake := &fakeAPI{
			pages:   [][]Page{{page("P3", base.Add(3*time.Hour))}},
			cursors: []string{""},
			text:    map[string]string{"P3": "oncall rotates weekly"},
		}
		sink := &collectSink{}
		c := New(func(string) API { return fake })
		cursor := connector.Cursor(base.Add(3 * time.Hour).Format(time.RFC3339Nano))
		next, err := c.Poll(ctx, testConfig(), cursor, sink)
		if err != nil {
			t.Fatal(err)
		}
		if len(sink.items) != 0 {
			t.Fatalf("items = %d, want 0", len(sink.items))
		}
		if next != cursor {
			t.Fatalf("cursor moved to %q", next)
		}
	})

	t.Run("workspace pages resolve to tenant scope", func(t *testing.T) {
		c := New(func(string) API { return &fakeAPI{} })
		body, _ := json.Marshal(Envelope{Page: page("P9", base)})
		acl, err := c.ResolveACL(ctx, testConfig(), connector.RawItem{Body: body})
		if err != nil {
			t.Fatal(err)
		}
		if acl.Scope != connector.ACLScopeTenant || acl.SourceScope.Type != "notion_page" {
			t.Fatalf("acl = %+v", acl)
		}
	})

	t.Run("normalizes pages with the page as thread", func(t *testing.T) {
		cfg := testConfig()
		body, _ := json.Marshal(Envelope{Page: page("P1", base), Content: "deploys happen tuesdays"})
		ev, err := Normalizer{}.Normalize(cfg, connector.RawItem{ExternalID: "P1:123", Body: body}, connector.ACL{Scope: connector.ACLScopeTenant})
		if err != nil {
			t.Fatal(err)
		}
		if ev.ThreadKey != "P1" || ev.SourceType != SourceType {
			t.Fatalf("event = %+v", ev)
		}
		if ev.AuthorRef.SourceRef != "notion:user-1" {
			t.Fatalf("author = %q", ev.AuthorRef.SourceRef)
		}
		if ev.Payload.Structure["url"] != "https://notion.so/P1" {
			t.Fatalf("structure = %+v", ev.Payload.Structure)
		}
	})
}
