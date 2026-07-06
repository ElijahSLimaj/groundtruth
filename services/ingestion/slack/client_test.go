package slack

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return &Client{
		token:      "xoxb-test",
		baseURL:    server.URL,
		httpClient: server.Client(),
		limiter:    rate.NewLimiter(rate.Inf, 1),
	}
}

func TestClientSendsAuthAndFormParams(t *testing.T) {
	t.Parallel()
	var gotAuth, gotChannel string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		gotChannel = r.FormValue("channel")
		w.Write([]byte(`{"ok": true, "channel": {"id": "C1", "name": "general"}}`))
	})

	ch, err := c.ChannelInfo(context.Background(), "C1")
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer xoxb-test" {
		t.Fatalf("unexpected auth header %q", gotAuth)
	}
	if gotChannel != "C1" || ch.ID != "C1" || ch.Name != "general" {
		t.Fatalf("unexpected request or result: channel=%q result=%+v", gotChannel, ch)
	}
}

func TestClientPaginatesChannelList(t *testing.T) {
	t.Parallel()
	var cursors []string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		cursor := r.FormValue("cursor")
		cursors = append(cursors, cursor)
		if cursor == "" {
			w.Write([]byte(`{"ok": true, "channels": [{"id": "C1"}], "response_metadata": {"next_cursor": "page2"}}`))
			return
		}
		w.Write([]byte(`{"ok": true, "channels": [{"id": "C2"}], "response_metadata": {"next_cursor": ""}}`))
	})

	channels, err := c.ListChannels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != 2 || channels[0].ID != "C1" || channels[1].ID != "C2" {
		t.Fatalf("unexpected channels %+v", channels)
	}
	if len(cursors) != 2 || cursors[1] != "page2" {
		t.Fatalf("unexpected cursor sequence %v", cursors)
	}
}

func TestClientPaginatesHistoryUntilHasMoreFalse(t *testing.T) {
	t.Parallel()
	calls := 0
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Write([]byte(`{"ok": true, "messages": [{"ts": "3.0"}, {"ts": "2.0"}], "has_more": true, "response_metadata": {"next_cursor": "next"}}`))
			return
		}
		w.Write([]byte(`{"ok": true, "messages": [{"ts": "1.0"}], "has_more": false}`))
	})

	messages, err := c.History(context.Background(), "C1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages across pages, got %d", len(messages))
	}
}

func TestClientMapsSlackErrors(t *testing.T) {
	t.Parallel()
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok": false, "error": "invalid_auth"}`))
	})

	err := c.AuthTest(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "invalid_auth" {
		t.Fatalf("expected APIError invalid_auth, got %v", err)
	}
}

func TestClientMapsRateLimiting(t *testing.T) {
	t.Parallel()
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
	})

	_, err := c.ListChannels(context.Background())
	var rateErr *RateLimitedError
	if !errors.As(err, &rateErr) || rateErr.RetryAfter != 7*time.Second {
		t.Fatalf("expected RateLimitedError with 7s, got %v", err)
	}
}
