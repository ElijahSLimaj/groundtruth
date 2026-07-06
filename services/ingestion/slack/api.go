package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Channel struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	IsPrivate  bool   `json:"is_private"`
	IsIM       bool   `json:"is_im"`
	IsMpIM     bool   `json:"is_mpim"`
	IsArchived bool   `json:"is_archived"`
}

type API interface {
	AuthTest(ctx context.Context) error
	ListChannels(ctx context.Context) ([]Channel, error)
	History(ctx context.Context, channelID, oldest, latest string) ([]json.RawMessage, error)
	ChannelInfo(ctx context.Context, channelID string) (Channel, error)
	Members(ctx context.Context, channelID string) ([]string, error)
}

type Envelope struct {
	Channel EnvelopeChannel `json:"channel"`
	Message json.RawMessage `json:"message"`
}

type EnvelopeChannel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type messageFields struct {
	TS       string `json:"ts"`
	ThreadTS string `json:"thread_ts"`
	User     string `json:"user"`
	Text     string `json:"text"`
}

func parseMessage(raw json.RawMessage) (messageFields, error) {
	var fields messageFields
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fields, fmt.Errorf("parse message: %w", err)
	}
	if fields.TS == "" {
		return fields, fmt.Errorf("message has no ts")
	}
	return fields, nil
}

func parseTS(ts string) (time.Time, error) {
	secPart, fracPart, _ := strings.Cut(ts, ".")
	sec, err := strconv.ParseInt(secPart, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse ts %q: %w", ts, err)
	}
	var micros int64
	if fracPart != "" {
		padded := (fracPart + "000000")[:6]
		micros, err = strconv.ParseInt(padded, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse ts fraction %q: %w", ts, err)
		}
	}
	return time.Unix(sec, micros*1000).UTC(), nil
}

func formatTS(t time.Time) string {
	return fmt.Sprintf("%d.%06d", t.Unix(), t.Nanosecond()/1000)
}

func tsAfter(a, b string) bool {
	ta, errA := parseTS(a)
	tb, errB := parseTS(b)
	if errA != nil || errB != nil {
		return a > b
	}
	return ta.After(tb)
}
