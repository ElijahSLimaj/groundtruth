package outlook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"sync/atomic"
	"time"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
)

const SourceType = "outlook"

var ErrSubscribeUnsupported = errors.New("outlook change notifications require the events receiver, poll instead")

var defaultExclusionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(payroll|salary|salaries|compensation review|comp band)\b`),
	regexp.MustCompile(`(?i)\b(hr case|human resources|disciplinary|termination letter)\b`),
	regexp.MustCompile(`(?i)\b(attorney|privileged|legal hold|litigation)\b`),
	regexp.MustCompile(`(?i)\b(medical|diagnosis|health insurance claim)\b`),
}

type Envelope struct {
	Message Message `json:"message"`
}

type Connector struct {
	NewAPI func(token string) API

	excluded atomic.Int64
}

func New(newAPI func(token string) API) *Connector {
	return &Connector{NewAPI: newAPI}
}

func (c *Connector) SourceType() string {
	return SourceType
}

func (c *Connector) ExcludedCount() int64 {
	return c.excluded.Load()
}

func (c *Connector) Subscribe(context.Context, connector.Config, connector.RawItemSink) error {
	return ErrSubscribeUnsupported
}

func (c *Connector) Poll(ctx context.Context, cfg connector.Config, cursor connector.Cursor, sink connector.RawItemSink) (connector.Cursor, error) {
	api, err := c.api(cfg)
	if err != nil {
		return cursor, err
	}

	state, err := decodeDeltaState(cursor)
	if err != nil {
		return cursor, err
	}
	next := map[string]string{}
	for _, folder := range foldersFor(cfg) {
		messageIDs, newDeltaLink, err := api.FolderDelta(ctx, folder, state[folder])
		if err != nil {
			return cursor, fmt.Errorf("outlook delta folder %s: %w", folder, err)
		}
		if err := c.emitMessages(ctx, api, cfg, messageIDs, sink); err != nil {
			return cursor, err
		}
		next[folder] = newDeltaLink
	}
	return encodeDeltaState(next)
}

func foldersFor(cfg connector.Config) []string {
	raw, ok := cfg.Settings["folders"].([]any)
	if !ok {
		return []string{"inbox"}
	}
	folders := make([]string, 0, len(raw))
	for _, entry := range raw {
		if folder, ok := entry.(string); ok && folder != "" {
			folders = append(folders, folder)
		}
	}
	if len(folders) == 0 {
		return []string{"inbox"}
	}
	return folders
}

func decodeDeltaState(cursor connector.Cursor) (map[string]string, error) {
	if cursor == "" {
		return map[string]string{}, nil
	}
	var state map[string]string
	if err := json.Unmarshal([]byte(cursor), &state); err != nil {
		return nil, fmt.Errorf("outlook cursor %q is not a delta state: %w", cursor, err)
	}
	return state, nil
}

func encodeDeltaState(state map[string]string) (connector.Cursor, error) {
	encoded, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("outlook encode delta state: %w", err)
	}
	return connector.Cursor(encoded), nil
}

func (c *Connector) Backfill(ctx context.Context, cfg connector.Config, window connector.BackfillWindow, sink connector.RawItemSink) error {
	api, err := c.api(cfg)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(
		"receivedDateTime ge %s and receivedDateTime le %s",
		window.From.UTC().Format(time.RFC3339),
		window.To.UTC().Add(24*time.Hour).Format(time.RFC3339),
	)
	messageIDs, err := api.ListMessages(ctx, query)
	if err != nil {
		return fmt.Errorf("outlook backfill list: %w", err)
	}
	return c.emitMessages(ctx, api, cfg, messageIDs, sink)
}

func (c *Connector) emitMessages(ctx context.Context, api API, cfg connector.Config, messageIDs []string, sink connector.RawItemSink) error {
	excludedCategories := excludedCategorySet(cfg)
	for _, id := range messageIDs {
		message, err := api.GetMessage(ctx, id)
		if err != nil {
			return fmt.Errorf("outlook get message %s: %w", id, err)
		}
		if c.isExcluded(message, excludedCategories) {
			c.excluded.Add(1)
			continue
		}
		body, err := json.Marshal(Envelope{Message: message})
		if err != nil {
			return fmt.Errorf("outlook marshal envelope: %w", err)
		}
		item := connector.RawItem{
			ExternalID: message.ID,
			Body:       body,
			ReceivedAt: time.Now().UTC(),
		}
		if err := sink.Emit(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func (c *Connector) isExcluded(message Message, excludedCategories map[string]bool) bool {
	for _, category := range message.Categories {
		if excludedCategories[category] {
			return true
		}
	}
	haystacks := []string{message.Subject, message.Body}
	for _, haystack := range haystacks {
		for _, pattern := range defaultExclusionPatterns {
			if pattern.MatchString(haystack) {
				return true
			}
		}
	}
	return false
}

func excludedCategorySet(cfg connector.Config) map[string]bool {
	set := map[string]bool{}
	raw, ok := cfg.Settings["excluded_categories"].([]any)
	if !ok {
		return set
	}
	for _, entry := range raw {
		if category, ok := entry.(string); ok {
			set[category] = true
		}
	}
	return set
}

func (c *Connector) ResolveACL(_ context.Context, _ connector.Config, item connector.RawItem) (connector.ACL, error) {
	var env Envelope
	if err := json.Unmarshal(item.Body, &env); err != nil {
		return connector.ACL{}, fmt.Errorf("outlook resolve acl: decode envelope: %w", err)
	}
	principals := map[string]bool{}
	if env.Message.From != "" {
		principals["email:"+env.Message.From] = true
	}
	for _, address := range env.Message.To {
		principals["email:"+address] = true
	}
	for _, address := range env.Message.Cc {
		principals["email:"+address] = true
	}
	if len(principals) == 0 {
		return connector.ACL{}, fmt.Errorf("outlook resolve acl: message %s has no resolvable recipients", env.Message.ID)
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
			Type: "outlook_thread",
			ID:   env.Message.ThreadID,
		},
	}, nil
}

func (c *Connector) HealthCheck(ctx context.Context, cfg connector.Config) connector.Health {
	api, err := c.api(cfg)
	if err != nil {
		return connector.Health{State: connector.HealthDegraded, Message: err.Error()}
	}
	if _, err := api.Profile(ctx); err != nil {
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
