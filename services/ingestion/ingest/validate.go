package ingest

import (
	"github.com/google/uuid"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
)

type ValidationError struct {
	Reason string
}

func (e *ValidationError) Error() string {
	return "invalid event: " + e.Reason
}

func invalid(reason string) *ValidationError {
	return &ValidationError{Reason: reason}
}

func Validate(ev connector.NormalizedEvent) error {
	if ev.TenantID == uuid.Nil {
		return invalid("tenant_id_missing")
	}
	if ev.ConnectorID == uuid.Nil {
		return invalid("connector_id_missing")
	}
	if ev.SourceType == "" {
		return invalid("source_type_missing")
	}
	if ev.ExternalID == "" {
		return invalid("external_id_missing")
	}
	if ev.OccurredAt.IsZero() {
		return invalid("occurred_at_missing")
	}
	return validateACL(ev.ACL)
}

func validateACL(acl connector.ACL) error {
	switch acl.Scope {
	case connector.ACLScopePrincipals:
		if len(acl.Principals) == 0 {
			return invalid("acl_principals_empty")
		}
	case connector.ACLScopeGroup:
		if acl.SourceScope.ID == "" {
			return invalid("acl_group_unresolvable")
		}
	case connector.ACLScopeTenant:
	default:
		return invalid("acl_scope_invalid")
	}
	return nil
}
