package hubspot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
)

const SourceType = "hubspot"

var ErrSubscribeUnsupported = errors.New("hubspot change notifications require the events receiver, poll instead")

type Envelope struct {
	Record Record `json:"record"`
}

type Connector struct {
	NewAPI func(token string) API
}

func New(newAPI func(token string) API) *Connector {
	return &Connector{NewAPI: newAPI}
}

func (c *Connector) SourceType() string {
	return SourceType
}

func (c *Connector) Subscribe(context.Context, connector.Config, connector.RawItemSink) error {
	return ErrSubscribeUnsupported
}

func millis(t time.Time) string {
	return strconv.FormatInt(t.UnixMilli(), 10)
}

func (c *Connector) Poll(ctx context.Context, cfg connector.Config, cursor connector.Cursor, sink connector.RawItemSink) (connector.Cursor, error) {
	api, err := c.api(cfg)
	if err != nil {
		return cursor, err
	}
	if cursor == "" {
		return connector.Cursor(millis(time.Now().UTC())), nil
	}
	high, err := c.drain(ctx, api, string(cursor), "", sink)
	if err != nil {
		return cursor, err
	}
	if high == "" {
		return cursor, nil
	}
	return connector.Cursor(high), nil
}

func (c *Connector) Backfill(ctx context.Context, cfg connector.Config, window connector.BackfillWindow, sink connector.RawItemSink) error {
	api, err := c.api(cfg)
	if err != nil {
		return err
	}
	_, err = c.drain(ctx, api, millis(window.From.UTC()), millis(window.To.UTC()), sink)
	return err
}

func (c *Connector) drain(ctx context.Context, api API, gte, lte string, sink connector.RawItemSink) (string, error) {
	after := ""
	high := ""
	for {
		page, err := api.Search(ctx, gte, lte, after)
		if err != nil {
			return high, fmt.Errorf("hubspot search: %w", err)
		}
		for _, record := range page.Records {
			body, err := json.Marshal(Envelope{Record: record})
			if err != nil {
				return high, fmt.Errorf("hubspot marshal: %w", err)
			}
			item := connector.RawItem{
				ExternalID: record.ID,
				Body:       body,
				ReceivedAt: time.Now().UTC(),
			}
			if err := sink.Emit(ctx, item); err != nil {
				return high, err
			}
			next := millis(record.UpdatedAt.Add(time.Millisecond))
			if next > high {
				high = next
			}
		}
		if page.NextAfter == "" {
			return high, nil
		}
		after = page.NextAfter
	}
}

func (c *Connector) ResolveACL(_ context.Context, _ connector.Config, item connector.RawItem) (connector.ACL, error) {
	var env Envelope
	if err := json.Unmarshal(item.Body, &env); err != nil {
		return connector.ACL{}, fmt.Errorf("hubspot resolve acl: decode envelope: %w", err)
	}
	if env.Record.ID == "" {
		return connector.ACL{}, fmt.Errorf("hubspot resolve acl: record has no id")
	}
	return connector.ACL{
		Scope: connector.ACLScopeTenant,
		SourceScope: connector.SourceScope{
			Type: "hubspot_deal",
			ID:   env.Record.ID,
		},
	}, nil
}

func (c *Connector) HealthCheck(ctx context.Context, cfg connector.Config) connector.Health {
	api, err := c.api(cfg)
	if err != nil {
		return connector.Health{State: connector.HealthDegraded, Message: err.Error()}
	}
	if err := api.Ping(ctx); err != nil {
		return connector.Health{State: connector.HealthDegraded, Message: err.Error()}
	}
	return connector.Health{State: connector.HealthLive}
}

func (c *Connector) api(cfg connector.Config) (API, error) {
	token, _ := cfg.Settings["token"].(string)
	if token == "" {
		return nil, fmt.Errorf("connector %s has no token configured", cfg.ConnectorID)
	}
	return c.NewAPI(token), nil
}
