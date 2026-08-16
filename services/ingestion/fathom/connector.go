package fathom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
)

const SourceType = "fathom"

var ErrSubscribeUnsupported = errors.New("fathom delivers new meetings by webhook, poll for backfill")

type Envelope struct {
	Meeting Meeting `json:"meeting"`
}

type Connector struct {
	NewAPI func(apiKey string) API
}

func New(newAPI func(apiKey string) API) *Connector {
	return &Connector{NewAPI: newAPI}
}

func (c *Connector) SourceType() string {
	return SourceType
}

func (c *Connector) Subscribe(context.Context, connector.Config, connector.RawItemSink) error {
	return ErrSubscribeUnsupported
}

func (c *Connector) Poll(ctx context.Context, cfg connector.Config, cursor connector.Cursor, sink connector.RawItemSink) (connector.Cursor, error) {
	api, err := c.api(cfg)
	if err != nil {
		return cursor, err
	}
	if cursor == "" {
		return connector.Cursor(time.Now().UTC().Format(time.RFC3339)), nil
	}
	watermark, err := time.Parse(time.RFC3339, string(cursor))
	if err != nil {
		return cursor, fmt.Errorf("fathom cursor %q is not a timestamp: %w", cursor, err)
	}

	high := watermark
	pageCursor := ""
	for {
		page, err := api.List(ctx, pageCursor)
		if err != nil {
			return connector.Cursor(high.Format(time.RFC3339)), fmt.Errorf("fathom list: %w", err)
		}
		reachedSeen := false
		for _, meeting := range page.Meetings {
			if !meeting.StartTime.After(watermark) {
				reachedSeen = true
				continue
			}
			if err := c.emit(ctx, meeting, sink); err != nil {
				return connector.Cursor(high.Format(time.RFC3339)), err
			}
			if meeting.StartTime.After(high) {
				high = meeting.StartTime
			}
		}
		if reachedSeen || page.NextCursor == "" {
			break
		}
		pageCursor = page.NextCursor
	}
	return connector.Cursor(high.Format(time.RFC3339)), nil
}

func (c *Connector) Backfill(ctx context.Context, cfg connector.Config, window connector.BackfillWindow, sink connector.RawItemSink) error {
	api, err := c.api(cfg)
	if err != nil {
		return err
	}
	pageCursor := ""
	for {
		page, err := api.List(ctx, pageCursor)
		if err != nil {
			return fmt.Errorf("fathom backfill: %w", err)
		}
		belowWindow := false
		for _, meeting := range page.Meetings {
			if meeting.StartTime.Before(window.From) {
				belowWindow = true
				continue
			}
			if meeting.StartTime.After(window.To) {
				continue
			}
			if err := c.emit(ctx, meeting, sink); err != nil {
				return err
			}
		}
		if belowWindow || page.NextCursor == "" {
			return nil
		}
		pageCursor = page.NextCursor
	}
}

func (c *Connector) emit(ctx context.Context, meeting Meeting, sink connector.RawItemSink) error {
	body, err := json.Marshal(Envelope{Meeting: meeting})
	if err != nil {
		return fmt.Errorf("fathom marshal: %w", err)
	}
	return sink.Emit(ctx, connector.RawItem{
		ExternalID: recordingKey(meeting.RecordingID),
		Body:       body,
		ReceivedAt: time.Now().UTC(),
	})
}

func (c *Connector) ResolveACL(_ context.Context, _ connector.Config, item connector.RawItem) (connector.ACL, error) {
	var env Envelope
	if err := json.Unmarshal(item.Body, &env); err != nil {
		return connector.ACL{}, fmt.Errorf("fathom resolve acl: decode envelope: %w", err)
	}
	principals := map[string]bool{}
	for _, invitee := range env.Meeting.Invitees {
		principals["email:"+invitee] = true
	}
	if env.Meeting.RecordedBy != "" {
		principals["email:"+env.Meeting.RecordedBy] = true
	}
	if len(principals) == 0 {
		return connector.ACL{}, fmt.Errorf("fathom resolve acl: meeting %d has no participants", env.Meeting.RecordingID)
	}
	list := make([]string, 0, len(principals))
	for principal := range principals {
		list = append(list, principal)
	}
	sort.Strings(list)
	return connector.ACL{
		Scope:      connector.ACLScopePrincipals,
		Principals: list,
		SourceScope: connector.SourceScope{
			Type: "fathom_meeting",
			ID:   recordingKey(env.Meeting.RecordingID),
		},
	}, nil
}

func (c *Connector) HealthCheck(ctx context.Context, cfg connector.Config) connector.Health {
	api, err := c.api(cfg)
	if err != nil {
		return connector.Health{State: connector.HealthDegraded, Message: err.Error()}
	}
	if _, err := api.List(ctx, ""); err != nil {
		return connector.Health{State: connector.HealthDegraded, Message: err.Error()}
	}
	return connector.Health{State: connector.HealthLive}
}

func (c *Connector) api(cfg connector.Config) (API, error) {
	token, _ := cfg.Settings["token"].(string)
	if token == "" {
		return nil, fmt.Errorf("connector %s has no api key configured", cfg.ConnectorID)
	}
	return c.NewAPI(token), nil
}
