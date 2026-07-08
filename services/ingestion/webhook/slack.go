package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
	"github.com/attempttechnologies/company-brain/services/ingestion/runtime"
	"github.com/attempttechnologies/company-brain/services/ingestion/slack"
)

const (
	maxBodyBytes     = 1 << 20
	maxTimestampSkew = 5 * time.Minute
)

type SlackReceiver struct {
	Pool          *pgxpool.Pool
	Runner        *runtime.Runner
	SigningSecret string
	Logger        *slog.Logger
	Now           func() time.Time
}

type outerPayload struct {
	Type      string          `json:"type"`
	Challenge string          `json:"challenge"`
	TeamID    string          `json:"team_id"`
	Event     json.RawMessage `json:"event"`
}

type innerEvent struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype"`
	Channel   string          `json:"channel"`
	TS        string          `json:"ts"`
	DeletedTS string          `json:"deleted_ts"`
	EventTS   string          `json:"event_ts"`
	Message   json.RawMessage `json:"message"`
}

func (h *SlackReceiver) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func (h *SlackReceiver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		http.Error(w, "unreadable body", http.StatusBadRequest)
		return
	}
	if !h.verifySignature(r.Header.Get("X-Slack-Request-Timestamp"), r.Header.Get("X-Slack-Signature"), body) {
		http.Error(w, "signature verification failed", http.StatusUnauthorized)
		return
	}

	var payload outerPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "malformed payload", http.StatusBadRequest)
		return
	}

	switch payload.Type {
	case "url_verification":
		w.Header().Set("content-type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"challenge": payload.Challenge}); err != nil {
			h.Logger.Error("challenge_response", "error", err)
		}
	case "event_callback":
		if err := h.handleEvent(r.Context(), payload); err != nil {
			h.Logger.Error("slack_webhook", "team", payload.TeamID, "error", err)
			http.Error(w, "event handling failed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusOK)
	}
}

func (h *SlackReceiver) verifySignature(timestampRaw, signature string, body []byte) bool {
	if h.SigningSecret == "" || timestampRaw == "" || signature == "" {
		return false
	}
	timestamp, err := strconv.ParseInt(timestampRaw, 10, 64)
	if err != nil {
		return false
	}
	skew := math.Abs(float64(h.now().Unix() - timestamp))
	if skew > maxTimestampSkew.Seconds() {
		return false
	}
	mac := hmac.New(sha256.New, []byte(h.SigningSecret))
	fmt.Fprintf(mac, "v0:%s:", timestampRaw)
	mac.Write(body)
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func (h *SlackReceiver) handleEvent(ctx context.Context, payload outerPayload) error {
	connectorID, found, err := h.connectorForTeam(ctx, payload.TeamID)
	if err != nil {
		return err
	}
	if !found {
		h.Logger.Warn("slack_webhook_unknown_team", "team", payload.TeamID)
		return nil
	}

	var event innerEvent
	if err := json.Unmarshal(payload.Event, &event); err != nil {
		return fmt.Errorf("decode event: %w", err)
	}
	if event.Type != "message" || event.Channel == "" {
		return nil
	}

	switch event.Subtype {
	case "":
		return h.ingest(ctx, connectorID, event.Channel, event.Channel+":"+event.TS, payload.Event)
	case "message_changed":
		var edited struct {
			TS string `json:"ts"`
		}
		if err := json.Unmarshal(event.Message, &edited); err != nil {
			return fmt.Errorf("decode edited message: %w", err)
		}
		externalID := event.Channel + ":" + edited.TS + ":edit:" + event.EventTS
		return h.ingest(ctx, connectorID, event.Channel, externalID, event.Message)
	case "message_deleted":
		return h.tombstone(ctx, connectorID, event.Channel+":"+event.DeletedTS)
	default:
		return nil
	}
}

func (h *SlackReceiver) connectorForTeam(ctx context.Context, teamID string) (uuid.UUID, bool, error) {
	if teamID == "" {
		return uuid.Nil, false, nil
	}
	var id uuid.UUID
	err := h.Pool.QueryRow(ctx, `
		select id from connectors
		where source_type = 'slack' and config ->> 'team_id' = $1
		order by created_at
		limit 1
	`, teamID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("resolve team connector: %w", err)
	}
	return id, true, nil
}

func (h *SlackReceiver) ingest(ctx context.Context, connectorID uuid.UUID, channel, externalID string, message json.RawMessage) error {
	body, err := json.Marshal(slack.Envelope{
		Channel: slack.EnvelopeChannel{ID: channel},
		Message: message,
	})
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	item := connector.RawItem{
		ExternalID: externalID,
		Body:       body,
		ReceivedAt: h.now().UTC(),
	}
	err = h.Runner.IngestItem(ctx, connectorID, item)
	if errors.Is(err, runtime.ErrConnectorNotLive) {
		h.Logger.Warn("slack_webhook_connector_paused", "connector", connectorID)
		return nil
	}
	return err
}

func (h *SlackReceiver) tombstone(ctx context.Context, connectorID uuid.UUID, externalID string) error {
	var count int
	err := h.Pool.QueryRow(ctx,
		`select public.event_tombstone_by_external($1, $2)`,
		connectorID, externalID,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("tombstone %s: %w", externalID, err)
	}
	if count == 0 {
		h.Logger.Warn("slack_webhook_delete_unmatched", "external_id", externalID)
	}
	return nil
}
