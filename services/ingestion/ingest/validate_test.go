package ingest

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
)

func validEvent() connector.NormalizedEvent {
	return connector.NormalizedEvent{
		TenantID:    uuid.New(),
		ConnectorID: uuid.New(),
		SourceType:  "slack",
		ExternalID:  "1751713200.000100",
		OccurredAt:  time.Date(2026, 7, 3, 9, 15, 0, 0, time.UTC),
		ACL: connector.ACL{
			Scope:       connector.ACLScopeTenant,
			SourceScope: connector.SourceScope{Type: "slack_channel", ID: "C1"},
		},
		Payload: connector.Payload{Body: "hello"},
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		mutate     func(*connector.NormalizedEvent)
		wantReason string
	}{
		{
			name:   "accepts_a_complete_event",
			mutate: func(ev *connector.NormalizedEvent) {},
		},
		{
			name: "accepts_principals_scope_with_principals",
			mutate: func(ev *connector.NormalizedEvent) {
				ev.ACL.Scope = connector.ACLScopePrincipals
				ev.ACL.Principals = []string{"person:x"}
			},
		},
		{
			name:       "rejects_missing_tenant",
			mutate:     func(ev *connector.NormalizedEvent) { ev.TenantID = uuid.Nil },
			wantReason: "tenant_id_missing",
		},
		{
			name:       "rejects_missing_connector",
			mutate:     func(ev *connector.NormalizedEvent) { ev.ConnectorID = uuid.Nil },
			wantReason: "connector_id_missing",
		},
		{
			name:       "rejects_empty_source_type",
			mutate:     func(ev *connector.NormalizedEvent) { ev.SourceType = "" },
			wantReason: "source_type_missing",
		},
		{
			name:       "rejects_empty_external_id",
			mutate:     func(ev *connector.NormalizedEvent) { ev.ExternalID = "" },
			wantReason: "external_id_missing",
		},
		{
			name:       "rejects_zero_occurred_at",
			mutate:     func(ev *connector.NormalizedEvent) { ev.OccurredAt = time.Time{} },
			wantReason: "occurred_at_missing",
		},
		{
			name:       "rejects_unknown_acl_scope",
			mutate:     func(ev *connector.NormalizedEvent) { ev.ACL.Scope = "everyone" },
			wantReason: "acl_scope_invalid",
		},
		{
			name:       "rejects_empty_acl_scope",
			mutate:     func(ev *connector.NormalizedEvent) { ev.ACL.Scope = "" },
			wantReason: "acl_scope_invalid",
		},
		{
			name: "rejects_principals_scope_without_principals",
			mutate: func(ev *connector.NormalizedEvent) {
				ev.ACL.Scope = connector.ACLScopePrincipals
				ev.ACL.Principals = nil
			},
			wantReason: "acl_principals_empty",
		},
		{
			name: "rejects_group_scope_without_source_scope_id",
			mutate: func(ev *connector.NormalizedEvent) {
				ev.ACL.Scope = connector.ACLScopeGroup
				ev.ACL.SourceScope.ID = ""
			},
			wantReason: "acl_group_unresolvable",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ev := validEvent()
			tc.mutate(&ev)

			err := Validate(ev)

			if tc.wantReason == "" {
				if err != nil {
					t.Fatalf("expected valid, got %v", err)
				}
				return
			}
			var verr *ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("expected ValidationError, got %v", err)
			}
			if verr.Reason != tc.wantReason {
				t.Fatalf("expected reason %q, got %q", tc.wantReason, verr.Reason)
			}
		})
	}
}
