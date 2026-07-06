package connector

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Connector interface {
	SourceType() string
	Subscribe(ctx context.Context, cfg Config, sink RawItemSink) error
	Poll(ctx context.Context, cfg Config, cursor Cursor, sink RawItemSink) (Cursor, error)
	Backfill(ctx context.Context, cfg Config, window BackfillWindow, sink RawItemSink) error
	ResolveACL(ctx context.Context, cfg Config, item RawItem) (ACL, error)
	HealthCheck(ctx context.Context, cfg Config) Health
}

type Config struct {
	ConnectorID uuid.UUID
	TenantID    uuid.UUID
	SourceType  string
	Settings    map[string]any
}

type RawItem struct {
	ExternalID string
	Body       []byte
	ReceivedAt time.Time
}

type RawItemSink interface {
	Emit(ctx context.Context, item RawItem) error
}

type Cursor string

type BackfillWindow struct {
	From time.Time
	To   time.Time
}

type ACLScope string

const (
	ACLScopePrincipals ACLScope = "principals"
	ACLScopeGroup      ACLScope = "group"
	ACLScopeTenant     ACLScope = "tenant"
)

type ACL struct {
	Scope       ACLScope    `json:"scope"`
	Principals  []string    `json:"principals,omitempty"`
	SourceScope SourceScope `json:"source_scope"`
}

type SourceScope struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Visibility string `json:"visibility,omitempty"`
}

type HealthState string

const (
	HealthLive     HealthState = "live"
	HealthDegraded HealthState = "degraded"
)

type Health struct {
	State   HealthState
	Message string
}

type AuthorRef struct {
	PersonID  *uuid.UUID
	SourceRef string
}

type Payload struct {
	Body      string
	Structure map[string]any
}

type NormalizedEvent struct {
	TenantID    uuid.UUID
	ConnectorID uuid.UUID
	SourceType  string
	ExternalID  string
	AuthorRef   AuthorRef
	ThreadKey   string
	OccurredAt  time.Time
	ACL         ACL
	Payload     Payload
}
