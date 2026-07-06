package slack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
)

const SourceType = "slack"

var ErrSubscribeUnsupported = errors.New("slack webhook subscription requires the events receiver, poll instead")

type Connector struct {
	NewAPI   func(token string) API
	CacheTTL time.Duration

	mu    sync.Mutex
	cache map[string]cachedChannel
}

type cachedChannel struct {
	channel   Channel
	members   []string
	fetchedAt time.Time
}

func New(newAPI func(token string) API) *Connector {
	return &Connector{NewAPI: newAPI, CacheTTL: time.Minute}
}

func (c *Connector) SourceType() string {
	return SourceType
}

func (c *Connector) Subscribe(context.Context, connector.Config, connector.RawItemSink) error {
	return ErrSubscribeUnsupported
}

type pollCursor struct {
	Channels map[string]string `json:"channels"`
}

func decodeCursor(cursor connector.Cursor) (pollCursor, error) {
	decoded := pollCursor{Channels: map[string]string{}}
	if cursor == "" {
		return decoded, nil
	}
	if err := json.Unmarshal([]byte(cursor), &decoded); err != nil {
		return decoded, fmt.Errorf("decode cursor: %w", err)
	}
	if decoded.Channels == nil {
		decoded.Channels = map[string]string{}
	}
	return decoded, nil
}

func encodeCursor(cursor pollCursor) (connector.Cursor, error) {
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("encode cursor: %w", err)
	}
	return connector.Cursor(encoded), nil
}

func (c *Connector) Poll(ctx context.Context, cfg connector.Config, cursor connector.Cursor, sink connector.RawItemSink) (connector.Cursor, error) {
	api, err := c.api(cfg)
	if err != nil {
		return cursor, err
	}
	current, err := decodeCursor(cursor)
	if err != nil {
		return cursor, err
	}

	channels, err := api.ListChannels(ctx)
	if err != nil {
		return cursor, fmt.Errorf("list channels: %w", err)
	}

	next := pollCursor{Channels: map[string]string{}}
	for id, ts := range current.Channels {
		next.Channels[id] = ts
	}

	for _, ch := range eligibleChannels(channels) {
		latestSeen, err := c.emitHistory(ctx, api, sink, ch, current.Channels[ch.ID], "")
		if err != nil {
			return cursor, err
		}
		if latestSeen != "" {
			next.Channels[ch.ID] = latestSeen
		}
	}
	return encodeCursor(next)
}

func (c *Connector) Backfill(ctx context.Context, cfg connector.Config, window connector.BackfillWindow, sink connector.RawItemSink) error {
	api, err := c.api(cfg)
	if err != nil {
		return err
	}
	channels, err := api.ListChannels(ctx)
	if err != nil {
		return fmt.Errorf("list channels: %w", err)
	}
	for _, ch := range eligibleChannels(channels) {
		if _, err := c.emitHistory(ctx, api, sink, ch, formatTS(window.From), formatTS(window.To)); err != nil {
			return err
		}
	}
	return nil
}

func eligibleChannels(channels []Channel) []Channel {
	eligible := make([]Channel, 0, len(channels))
	for _, ch := range channels {
		if ch.IsIM || ch.IsMpIM || ch.IsArchived {
			continue
		}
		eligible = append(eligible, ch)
	}
	sort.Slice(eligible, func(i, j int) bool { return eligible[i].ID < eligible[j].ID })
	return eligible
}

func (c *Connector) emitHistory(ctx context.Context, api API, sink connector.RawItemSink, ch Channel, oldest, latest string) (string, error) {
	messages, err := api.History(ctx, ch.ID, oldest, latest)
	if err != nil {
		return "", fmt.Errorf("history for %s: %w", ch.ID, err)
	}

	latestSeen := ""
	for i := len(messages) - 1; i >= 0; i-- {
		fields, err := parseMessage(messages[i])
		if err != nil {
			return "", fmt.Errorf("channel %s: %w", ch.ID, err)
		}
		body, err := json.Marshal(Envelope{
			Channel: EnvelopeChannel{ID: ch.ID, Name: ch.Name},
			Message: messages[i],
		})
		if err != nil {
			return "", fmt.Errorf("marshal envelope: %w", err)
		}
		item := connector.RawItem{
			ExternalID: ch.ID + ":" + fields.TS,
			Body:       body,
			ReceivedAt: time.Now().UTC(),
		}
		if err := sink.Emit(ctx, item); err != nil {
			return "", err
		}
		if latestSeen == "" || tsAfter(fields.TS, latestSeen) {
			latestSeen = fields.TS
		}
	}
	return latestSeen, nil
}

func (c *Connector) ResolveACL(ctx context.Context, cfg connector.Config, item connector.RawItem) (connector.ACL, error) {
	var env Envelope
	if err := json.Unmarshal(item.Body, &env); err != nil {
		return connector.ACL{}, fmt.Errorf("resolve acl: decode envelope: %w", err)
	}
	if env.Channel.ID == "" {
		return connector.ACL{}, fmt.Errorf("resolve acl: envelope has no channel")
	}
	api, err := c.api(cfg)
	if err != nil {
		return connector.ACL{}, err
	}

	cached, err := c.channelACL(ctx, api, cfg, env.Channel.ID)
	if err != nil {
		return connector.ACL{}, err
	}
	if cached.channel.IsIM || cached.channel.IsMpIM {
		return connector.ACL{}, fmt.Errorf("resolve acl: %s is a direct message, excluded from ingestion", env.Channel.ID)
	}

	sourceScope := connector.SourceScope{Type: "slack_channel", ID: cached.channel.ID}
	if !cached.channel.IsPrivate {
		sourceScope.Visibility = "public"
		return connector.ACL{Scope: connector.ACLScopeTenant, SourceScope: sourceScope}, nil
	}

	sourceScope.Visibility = "private"
	principals := make([]string, 0, len(cached.members))
	for _, member := range cached.members {
		principals = append(principals, "slack:"+member)
	}
	if len(principals) == 0 {
		return connector.ACL{}, fmt.Errorf("resolve acl: private channel %s has no resolvable members", cached.channel.ID)
	}
	return connector.ACL{
		Scope:       connector.ACLScopePrincipals,
		Principals:  principals,
		SourceScope: sourceScope,
	}, nil
}

func (c *Connector) channelACL(ctx context.Context, api API, cfg connector.Config, channelID string) (cachedChannel, error) {
	key := cfg.ConnectorID.String() + ":" + channelID

	c.mu.Lock()
	cached, ok := c.cache[key]
	c.mu.Unlock()
	if ok && time.Since(cached.fetchedAt) < c.ttl() {
		return cached, nil
	}

	info, err := api.ChannelInfo(ctx, channelID)
	if err != nil {
		return cachedChannel{}, fmt.Errorf("channel info for %s: %w", channelID, err)
	}
	entry := cachedChannel{channel: info, fetchedAt: time.Now()}
	if info.IsPrivate && !info.IsIM && !info.IsMpIM {
		members, err := api.Members(ctx, channelID)
		if err != nil {
			return cachedChannel{}, fmt.Errorf("members for %s: %w", channelID, err)
		}
		sort.Strings(members)
		entry.members = members
	}

	c.mu.Lock()
	if c.cache == nil {
		c.cache = map[string]cachedChannel{}
	}
	c.cache[key] = entry
	c.mu.Unlock()
	return entry, nil
}

func (c *Connector) ttl() time.Duration {
	if c.CacheTTL > 0 {
		return c.CacheTTL
	}
	return time.Minute
}

func (c *Connector) HealthCheck(ctx context.Context, cfg connector.Config) connector.Health {
	api, err := c.api(cfg)
	if err != nil {
		return connector.Health{State: connector.HealthDegraded, Message: err.Error()}
	}
	if err := api.AuthTest(ctx); err != nil {
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
