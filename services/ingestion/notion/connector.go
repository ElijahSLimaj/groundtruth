package notion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
)

const SourceType = "notion"

var ErrSubscribeUnsupported = errors.New("notion has no event push, poll instead")

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

func (c *Connector) Poll(ctx context.Context, cfg connector.Config, cursor connector.Cursor, sink connector.RawItemSink) (connector.Cursor, error) {
	api, err := c.api(cfg)
	if err != nil {
		return cursor, err
	}
	watermark := time.Time{}
	if cursor != "" {
		watermark, err = time.Parse(time.RFC3339Nano, string(cursor))
		if err != nil {
			return cursor, fmt.Errorf("notion cursor %q is not a timestamp: %w", cursor, err)
		}
	}

	pages, err := c.collectPages(ctx, api, func(page Page) bool {
		return page.LastEditedTime.After(watermark)
	})
	if err != nil {
		return cursor, err
	}
	if err := c.emitPages(ctx, api, pages, sink); err != nil {
		return cursor, err
	}

	next := watermark
	for _, page := range pages {
		if page.LastEditedTime.After(next) {
			next = page.LastEditedTime
		}
	}
	if next.IsZero() {
		next = time.Now().UTC()
	}
	return connector.Cursor(next.UTC().Format(time.RFC3339Nano)), nil
}

func (c *Connector) Backfill(ctx context.Context, cfg connector.Config, window connector.BackfillWindow, sink connector.RawItemSink) error {
	api, err := c.api(cfg)
	if err != nil {
		return err
	}
	pages, err := c.collectPages(ctx, api, func(page Page) bool {
		return page.LastEditedTime.After(window.From) && !page.LastEditedTime.After(window.To)
	})
	if err != nil {
		return err
	}
	return c.emitPages(ctx, api, pages, sink)
}

func (c *Connector) collectPages(ctx context.Context, api API, keep func(Page) bool) ([]Page, error) {
	var pages []Page
	cursor := ""
	for {
		batch, next, err := api.SearchPages(ctx, cursor)
		if err != nil {
			return nil, fmt.Errorf("notion search: %w", err)
		}
		for _, page := range batch {
			if keep(page) {
				pages = append(pages, page)
			}
		}
		if next == "" {
			break
		}
		cursor = next
	}
	sort.Slice(pages, func(i, j int) bool {
		if pages[i].LastEditedTime.Equal(pages[j].LastEditedTime) {
			return pages[i].ID < pages[j].ID
		}
		return pages[i].LastEditedTime.Before(pages[j].LastEditedTime)
	})
	return pages, nil
}

func (c *Connector) emitPages(ctx context.Context, api API, pages []Page, sink connector.RawItemSink) error {
	for _, page := range pages {
		content, err := api.PageText(ctx, page.ID)
		if err != nil {
			return fmt.Errorf("notion page text %s: %w", page.ID, err)
		}
		body, err := json.Marshal(Envelope{Page: page, Content: content})
		if err != nil {
			return fmt.Errorf("notion marshal envelope: %w", err)
		}
		item := connector.RawItem{
			ExternalID: page.ID + ":" + strconv.FormatInt(page.LastEditedTime.UnixMilli(), 10),
			Body:       body,
			ReceivedAt: time.Now().UTC(),
		}
		if err := sink.Emit(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func (c *Connector) ResolveACL(_ context.Context, _ connector.Config, item connector.RawItem) (connector.ACL, error) {
	var env Envelope
	if err := json.Unmarshal(item.Body, &env); err != nil {
		return connector.ACL{}, fmt.Errorf("notion resolve acl: decode envelope: %w", err)
	}
	if env.Page.ID == "" {
		return connector.ACL{}, fmt.Errorf("notion resolve acl: envelope has no page")
	}
	return connector.ACL{
		Scope: connector.ACLScopeTenant,
		SourceScope: connector.SourceScope{
			Type:       "notion_page",
			ID:         env.Page.ID,
			Visibility: "workspace",
		},
	}, nil
}

func (c *Connector) HealthCheck(ctx context.Context, cfg connector.Config) connector.Health {
	api, err := c.api(cfg)
	if err != nil {
		return connector.Health{State: connector.HealthDegraded, Message: err.Error()}
	}
	if err := api.Me(ctx); err != nil {
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
