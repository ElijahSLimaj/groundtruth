package gdrive

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

const SourceType = "gdrive"

var ErrSubscribeUnsupported = errors.New("drive push channels require the events receiver, poll instead")

var documentMimeTypes = map[string]bool{
	"application/vnd.google-apps.document": true,
	"text/plain":                           true,
	"text/markdown":                        true,
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

func (c *Connector) Poll(ctx context.Context, cfg connector.Config, cursor connector.Cursor, sink connector.RawItemSink) (connector.Cursor, error) {
	api, err := c.api(cfg)
	if err != nil {
		return cursor, err
	}
	if cursor == "" {
		token, err := api.StartPageToken(ctx)
		if err != nil {
			return cursor, fmt.Errorf("drive start page token: %w", err)
		}
		return connector.Cursor(token), nil
	}

	pageToken := string(cursor)
	next := cursor
	for pageToken != "" {
		files, nextPage, newStart, err := api.Changes(ctx, pageToken)
		if err != nil {
			return next, fmt.Errorf("drive changes: %w", err)
		}
		if err := c.emitFiles(ctx, api, files, sink); err != nil {
			return next, err
		}
		pageToken = nextPage
		if newStart != "" {
			next = connector.Cursor(newStart)
		}
	}
	return next, nil
}

func (c *Connector) Backfill(ctx context.Context, cfg connector.Config, window connector.BackfillWindow, sink connector.RawItemSink) error {
	api, err := c.api(cfg)
	if err != nil {
		return err
	}
	files, err := api.ListFiles(ctx, window.From, window.To)
	if err != nil {
		return fmt.Errorf("drive backfill list: %w", err)
	}
	return c.emitFiles(ctx, api, files, sink)
}

func (c *Connector) emitFiles(ctx context.Context, api API, files []File, sink connector.RawItemSink) error {
	for _, file := range files {
		if file.Trashed || !documentMimeTypes[file.MimeType] {
			continue
		}
		content, err := api.ExportText(ctx, file)
		if err != nil {
			return fmt.Errorf("drive export %s: %w", file.ID, err)
		}
		body, err := json.Marshal(Envelope{File: file, Content: content})
		if err != nil {
			return fmt.Errorf("drive marshal envelope: %w", err)
		}
		item := connector.RawItem{
			ExternalID: file.ID + ":" + strconv.FormatInt(file.Version, 10),
			Body:       body,
			ReceivedAt: time.Now().UTC(),
		}
		if err := sink.Emit(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func (c *Connector) ResolveACL(ctx context.Context, cfg connector.Config, item connector.RawItem) (connector.ACL, error) {
	var env Envelope
	if err := json.Unmarshal(item.Body, &env); err != nil {
		return connector.ACL{}, fmt.Errorf("drive resolve acl: decode envelope: %w", err)
	}
	api, err := c.api(cfg)
	if err != nil {
		return connector.ACL{}, err
	}
	permissions, err := api.Permissions(ctx, env.File.ID)
	if err != nil {
		return connector.ACL{}, fmt.Errorf("drive permissions for %s: %w", env.File.ID, err)
	}

	sourceScope := connector.SourceScope{Type: "gdrive_file", ID: env.File.ID}
	principals := map[string]bool{}
	for _, permission := range permissions {
		switch permission.Type {
		case "anyone", "domain":
			sourceScope.Visibility = "shared"
			return connector.ACL{Scope: connector.ACLScopeTenant, SourceScope: sourceScope}, nil
		case "user", "group":
			if permission.Email != "" {
				principals["email:"+permission.Email] = true
			}
		}
	}
	if len(principals) == 0 {
		return connector.ACL{}, fmt.Errorf("drive resolve acl: file %s has no resolvable principals", env.File.ID)
	}
	sourceScope.Visibility = "restricted"
	list := make([]string, 0, len(principals))
	for principal := range principals {
		list = append(list, principal)
	}
	sort.Strings(list)
	return connector.ACL{
		Scope:       connector.ACLScopePrincipals,
		Principals:  list,
		SourceScope: sourceScope,
	}, nil
}

func (c *Connector) HealthCheck(ctx context.Context, cfg connector.Config) connector.Health {
	api, err := c.api(cfg)
	if err != nil {
		return connector.Health{State: connector.HealthDegraded, Message: err.Error()}
	}
	if err := api.About(ctx); err != nil {
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
