package fathom

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
)

type fakeAPI struct {
	pages   map[string]Page
	listErr error
}

func (f *fakeAPI) List(_ context.Context, cursor string) (Page, error) {
	if f.listErr != nil {
		return Page{}, f.listErr
	}
	return f.pages[cursor], nil
}

type collectSink struct {
	items []connector.RawItem
}

func (s *collectSink) Emit(_ context.Context, item connector.RawItem) error {
	s.items = append(s.items, item)
	return nil
}

func meeting(id int64, start time.Time, invitees []string, text string) Meeting {
	return Meeting{
		RecordingID: id,
		Title:       "Sync",
		StartTime:   start.UTC(),
		Invitees:    invitees,
		RecordedBy:  "host@acme.test",
		Transcript:  []Segment{{Speaker: "Ada", Text: text, Timestamp: "00:01:00"}},
	}
}

func testConfig() connector.Config {
	return connector.Config{
		ConnectorID: uuid.New(),
		TenantID:    uuid.New(),
		SourceType:  SourceType,
		Settings:    map[string]any{"token": "fathom-key"},
	}
}

func day(d int) time.Time {
	return time.Date(2026, 8, d, 12, 0, 0, 0, time.UTC)
}

func TestPollBootstrapsCursorWithoutIngesting(t *testing.T) {
	t.Parallel()
	c := New(func(string) API { return &fakeAPI{} })
	sink := &collectSink{}
	cursor, err := c.Poll(context.Background(), testConfig(), "", sink)
	if err != nil {
		t.Fatal(err)
	}
	if cursor == "" || len(sink.items) != 0 {
		t.Fatalf("bootstrap must set a cursor and ingest nothing, got cursor=%q items=%d", cursor, len(sink.items))
	}
}

func TestPollEmitsNewerMeetingsAndStopsAtSeen(t *testing.T) {
	t.Parallel()
	fake := &fakeAPI{
		pages: map[string]Page{
			"": {
				Meetings:   []Meeting{meeting(5, day(10), []string{"a@acme.test"}, "new"), meeting(4, day(5), []string{"a@acme.test"}, "also new")},
				NextCursor: "p2",
			},
			"p2": {
				Meetings: []Meeting{meeting(3, day(20-19), []string{"a@acme.test"}, "old")},
			},
		},
	}
	c := New(func(string) API { return fake })
	sink := &collectSink{}
	cursor, err := c.Poll(context.Background(), testConfig(), connector.Cursor(day(1).Format(time.RFC3339)), sink)
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.items) != 2 {
		t.Fatalf("expected the two meetings newer than the watermark, got %d", len(sink.items))
	}
	if string(cursor) != day(10).Format(time.RFC3339) {
		t.Fatalf("cursor must advance to the newest meeting, got %q", cursor)
	}
}

func TestBackfillEmitsWithinWindow(t *testing.T) {
	t.Parallel()
	fake := &fakeAPI{
		pages: map[string]Page{
			"": {Meetings: []Meeting{
				meeting(9, day(15), []string{"a@acme.test"}, "in"),
				meeting(8, day(2), []string{"a@acme.test"}, "below"),
			}},
		},
	}
	c := New(func(string) API { return fake })
	sink := &collectSink{}
	window := connector.BackfillWindow{From: day(10), To: day(20)}
	if err := c.Backfill(context.Background(), testConfig(), window, sink); err != nil {
		t.Fatal(err)
	}
	if len(sink.items) != 1 || sink.items[0].ExternalID != "9" {
		t.Fatalf("backfill must emit only meetings inside the window, got %+v", sink.items)
	}
}

func TestResolveACLIsParticipantScoped(t *testing.T) {
	t.Parallel()
	fake := &fakeAPI{pages: map[string]Page{"": {Meetings: []Meeting{meeting(7, day(10), []string{"guest@partner.test"}, "hi")}}}}
	c := New(func(string) API { return fake })
	sink := &collectSink{}
	if _, err := c.Poll(context.Background(), testConfig(), connector.Cursor(day(1).Format(time.RFC3339)), sink); err != nil {
		t.Fatal(err)
	}
	acl, err := c.ResolveACL(context.Background(), testConfig(), sink.items[0])
	if err != nil {
		t.Fatal(err)
	}
	if acl.Scope != connector.ACLScopePrincipals {
		t.Fatalf("a transcript is scoped to its participants, got %q", acl.Scope)
	}
	want := []string{"email:guest@partner.test", "email:host@acme.test"}
	if len(acl.Principals) != len(want) || acl.Principals[0] != want[0] || acl.Principals[1] != want[1] {
		t.Fatalf("expected %v, got %v", want, acl.Principals)
	}
}

func TestResolveACLRejectsMeetingWithoutParticipants(t *testing.T) {
	t.Parallel()
	c := New(func(string) API { return &fakeAPI{} })
	item := connector.RawItem{Body: []byte(`{"meeting":{"RecordingID":1}}`)}
	if _, err := c.ResolveACL(context.Background(), testConfig(), item); err == nil {
		t.Fatal("a meeting with no participants must not produce an ACL")
	}
}

func TestHealthCheckReflectsList(t *testing.T) {
	t.Parallel()
	healthy := New(func(string) API { return &fakeAPI{} })
	if h := healthy.HealthCheck(context.Background(), testConfig()); h.State != connector.HealthLive {
		t.Fatalf("expected live, got %+v", h)
	}
	broken := New(func(string) API { return &fakeAPI{listErr: errors.New("401")} })
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
