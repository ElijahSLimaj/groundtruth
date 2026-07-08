package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
	"github.com/attempttechnologies/company-brain/services/ingestion/internal/testdb"
	"github.com/attempttechnologies/company-brain/services/ingestion/runtime"
	"github.com/attempttechnologies/company-brain/services/ingestion/slack"
)

const signingSecret = "test-signing-secret"

type fakeSlackAPI struct{}

func (fakeSlackAPI) AuthTest(context.Context) error { return nil }
func (fakeSlackAPI) ListChannels(context.Context) ([]slack.Channel, error) {
	return nil, nil
}
func (fakeSlackAPI) History(context.Context, string, string, string) ([]json.RawMessage, error) {
	return nil, nil
}
func (fakeSlackAPI) ChannelInfo(_ context.Context, id string) (slack.Channel, error) {
	return slack.Channel{ID: id, Name: "general"}, nil
}
func (fakeSlackAPI) Members(context.Context, string) ([]string, error) {
	return nil, nil
}

func newReceiver(t *testing.T) (*SlackReceiver, testdb.Fixture, string) {
	t.Helper()
	worker, admin := testdb.Pools(t)
	fixture := testdb.CreateFixture(t, admin)
	teamID := "T" + strings.ToUpper(fixture.TenantID.String()[:8])
	if _, err := admin.Exec(context.Background(),
		`update connectors set config = jsonb_build_object('token', 'xoxb-test', 'team_id', $2::text) where id = $1`,
		fixture.ConnectorID, teamID); err != nil {
		t.Fatal(err)
	}
	runner := &runtime.Runner{
		Pool: worker,
		Connectors: map[string]connector.Connector{
			slack.SourceType: slack.New(func(string) slack.API { return fakeSlackAPI{} }),
		},
		Normalizers: map[string]runtime.Normalizer{slack.SourceType: slack.Normalizer{}},
	}
	receiver := &SlackReceiver{
		Pool:          worker,
		Runner:        runner,
		SigningSecret: signingSecret,
		Logger:        slog.New(slog.DiscardHandler),
	}
	return receiver, fixture, teamID
}

func sign(t *testing.T, body string, at time.Time) (timestamp, signature string) {
	t.Helper()
	timestamp = strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(signingSecret))
	fmt.Fprintf(mac, "v0:%s:%s", timestamp, body)
	return timestamp, "v0=" + hex.EncodeToString(mac.Sum(nil))
}

func post(t *testing.T, receiver *SlackReceiver, body string, at time.Time) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest("POST", "/webhooks/slack", strings.NewReader(body))
	timestamp, signature := sign(t, body, at)
	request.Header.Set("X-Slack-Request-Timestamp", timestamp)
	request.Header.Set("X-Slack-Signature", signature)
	recorder := httptest.NewRecorder()
	receiver.ServeHTTP(recorder, request)
	return recorder
}

func slackTS(at time.Time) string {
	return fmt.Sprintf("%d.%06d", at.Unix(), at.Nanosecond()/1000)
}

func itemLanded(t *testing.T, admin *pgxpool.Pool, tenantID uuid.UUID, pattern string) bool {
	t.Helper()
	var landed bool
	err := admin.QueryRow(context.Background(), `
		select exists (
			select 1 from ingestion_queue
			where tenant_id = $1 and event ->> 'external_id' like $2
		) or exists (
			select 1 from events
			where tenant_id = $1 and external_id like $2
		) or exists (
			select 1 from ingestion_dlq
			where tenant_id = $1 and event ->> 'ExternalID' like $2
		)
	`, tenantID, pattern).Scan(&landed)
	if err != nil {
		t.Fatal(err)
	}
	return landed
}

func TestSlackReceiver(t *testing.T) {
	receiver, fixture, teamID := newReceiver(t)
	_, admin := testdb.Pools(t)
	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("answers the url verification challenge", func(t *testing.T) {
		recorder := post(t, receiver, `{"type": "url_verification", "challenge": "chal-123"}`, now)
		if recorder.Code != 200 {
			t.Fatalf("status = %d", recorder.Code)
		}
		if !strings.Contains(recorder.Body.String(), "chal-123") {
			t.Fatalf("challenge missing from %q", recorder.Body.String())
		}
	})

	t.Run("rejects bad signatures", func(t *testing.T) {
		body := `{"type": "url_verification", "challenge": "x"}`
		request := httptest.NewRequest("POST", "/webhooks/slack", strings.NewReader(body))
		timestamp, _ := sign(t, body, now)
		request.Header.Set("X-Slack-Request-Timestamp", timestamp)
		request.Header.Set("X-Slack-Signature", "v0=deadbeef")
		recorder := httptest.NewRecorder()
		receiver.ServeHTTP(recorder, request)
		if recorder.Code != 401 {
			t.Fatalf("status = %d, want 401", recorder.Code)
		}
	})

	t.Run("rejects stale timestamps", func(t *testing.T) {
		recorder := post(t, receiver, `{"type": "url_verification", "challenge": "x"}`, now.Add(-10*time.Minute))
		if recorder.Code != 401 {
			t.Fatalf("status = %d, want 401", recorder.Code)
		}
	})

	t.Run("enqueues message events for the team connector", func(t *testing.T) {
		ts := slackTS(now.Add(-1 * time.Minute))
		body := fmt.Sprintf(
			`{"type": "event_callback", "team_id": %q, "event": {"type": "message", "channel": "C1", "ts": %q, "user": "U1", "text": "the growth plan moved to 1799"}}`,
			teamID, ts)
		recorder := post(t, receiver, body, now)
		if recorder.Code != 200 {
			t.Fatalf("status = %d body %s", recorder.Code, recorder.Body.String())
		}
		if !itemLanded(t, admin, fixture.TenantID, "C1:"+ts) {
			t.Fatal("message event never landed in the pipeline")
		}
	})

	t.Run("enqueues edits under a distinct external id", func(t *testing.T) {
		ts := slackTS(now.Add(-1 * time.Minute))
		body := fmt.Sprintf(
			`{"type": "event_callback", "team_id": %q, "event": {"type": "message", "subtype": "message_changed", "channel": "C1", "event_ts": "9999.0001", "message": {"type": "message", "ts": %q, "user": "U1", "text": "corrected to 1899"}}}`,
			teamID, ts)
		recorder := post(t, receiver, body, now)
		if recorder.Code != 200 {
			t.Fatalf("status = %d body %s", recorder.Code, recorder.Body.String())
		}
		if !itemLanded(t, admin, fixture.TenantID, "%:edit:%") {
			t.Fatal("edit event never landed with an edit marker")
		}
	})

	t.Run("tombstones deleted messages", func(t *testing.T) {
		deletedTS := slackTS(now.Add(-2 * time.Minute))
		eventID := uuid.New()
		if _, err := admin.Exec(ctx, `
			insert into events (id, tenant_id, connector_id, source_type, external_id, occurred_at, acl, payload_ref)
			values ($1, $2, $3, 'slack', $4, now(), '{"scope": "tenant"}', 'payloads/x')`,
			eventID, fixture.TenantID, fixture.ConnectorID, "C1:"+deletedTS); err != nil {
			t.Fatal(err)
		}
		body := fmt.Sprintf(
			`{"type": "event_callback", "team_id": %q, "event": {"type": "message", "subtype": "message_deleted", "channel": "C1", "deleted_ts": %q}}`,
			teamID, deletedTS)
		recorder := post(t, receiver, body, now)
		if recorder.Code != 200 {
			t.Fatalf("status = %d body %s", recorder.Code, recorder.Body.String())
		}
		var tombstoned bool
		if err := admin.QueryRow(ctx,
			`select tombstoned from events where id = $1`, eventID).Scan(&tombstoned); err != nil {
			t.Fatal(err)
		}
		if !tombstoned {
			t.Fatal("deleted message was not tombstoned")
		}
	})

	t.Run("ignores unknown teams without enqueueing", func(t *testing.T) {
		body := `{"type": "event_callback", "team_id": "TUNKNOWN", "event": {"type": "message", "channel": "C1", "ts": "1.000001", "user": "U1", "text": "hi"}}`
		recorder := post(t, receiver, body, now)
		if recorder.Code != 200 {
			t.Fatalf("status = %d", recorder.Code)
		}
		if itemLanded(t, admin, fixture.TenantID, "C1:1.000001") {
			t.Fatal("unknown team event was ingested")
		}
	})
}
