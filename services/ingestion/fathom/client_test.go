package fathom

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const realMeetingsPayload = `{
  "limit": 10,
  "next_cursor": null,
  "items": [
    {
      "title": "Acme <> Meridian pricing call",
      "meeting_title": "Acme <> Meridian pricing call",
      "meeting_type": "external",
      "recording_id": 90210,
      "url": "https://fathom.video/calls/90210",
      "share_url": "https://fathom.video/share/abc",
      "created_at": "2026-08-12T15:59:00Z",
      "scheduled_start_time": "2026-08-12T16:00:00Z",
      "recording_start_time": "2026-08-12T16:00:05Z",
      "recording_end_time": "2026-08-12T16:32:00Z",
      "recorded_by": { "email": "rep@acme.test", "name": "Rep" },
      "transcript_language": "en",
      "calendar_invitees": [
        { "email": "rep@acme.test", "name": "Rep" },
        { "email": "buyer@meridian.test", "name": "Buyer" }
      ],
      "transcript": [
        { "speaker": { "display_name": "Rep" }, "text": "Our list price is 1499 per month.", "timestamp": "00:02:10" },
        { "speaker": { "display_name": "Buyer" }, "text": "Can you do 1299?", "timestamp": "00:02:40" }
      ]
    }
  ]
}`

func testClient(baseURL string) *Client {
	c := NewClient("test-key")
	c.baseURL = baseURL
	return c
}

func TestListDecodesRealFathomPayload(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/external/v1/meetings" ||
			r.Header.Get("X-Api-Key") != "test-key" ||
			r.URL.Query().Get("include_transcript") != "true" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(realMeetingsPayload))
	}))
	defer server.Close()

	page, err := testClient(server.URL).List(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Meetings) != 1 {
		t.Fatalf("expected one meeting, got %d", len(page.Meetings))
	}
	m := page.Meetings[0]
	if m.RecordingID != 90210 {
		t.Fatalf("recording_id not decoded, got %d", m.RecordingID)
	}
	if m.StartTime.Format("2006-01-02T15:04:05Z") != "2026-08-12T16:00:05Z" {
		t.Fatalf("recording_start_time not parsed, got %v", m.StartTime)
	}
	if len(m.Invitees) != 2 || m.Invitees[1] != "buyer@meridian.test" {
		t.Fatalf("calendar_invitees not decoded, got %v", m.Invitees)
	}
	if m.RecordedBy != "rep@acme.test" {
		t.Fatalf("recorded_by not decoded, got %q", m.RecordedBy)
	}
	if len(m.Transcript) != 2 || m.Transcript[0].Speaker != "Rep" ||
		!strings.Contains(m.Transcript[0].Text, "1499") {
		t.Fatalf("transcript segments not decoded, got %+v", m.Transcript)
	}
	if page.NextCursor != "" {
		t.Fatalf("a null next_cursor must decode to empty, got %q", page.NextCursor)
	}
}

func TestListForwardsCursor(t *testing.T) {
	t.Parallel()
	var seen string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Query().Get("cursor")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"next_cursor":"more","items":[]}`))
	}))
	defer server.Close()

	page, err := testClient(server.URL).List(context.Background(), "page2token")
	if err != nil {
		t.Fatal(err)
	}
	if seen != "page2token" {
		t.Fatalf("cursor must be forwarded, server saw %q", seen)
	}
	if page.NextCursor != "more" {
		t.Fatalf("next_cursor not read, got %q", page.NextCursor)
	}
}

func TestListSurfacesErrorStatus(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	if _, err := testClient(server.URL).List(context.Background(), ""); err == nil {
		t.Fatal("a non-200 must surface as an error")
	}
}
