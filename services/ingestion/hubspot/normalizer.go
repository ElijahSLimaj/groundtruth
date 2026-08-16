package hubspot

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
)

type Normalizer struct{}

func summarize(props map[string]string) string {
	var parts []string
	if name := props["dealname"]; name != "" {
		parts = append(parts, "Deal: "+name)
	}
	if amount := props["amount"]; amount != "" {
		parts = append(parts, "amount "+amount)
	}
	if stage := props["dealstage"]; stage != "" {
		parts = append(parts, "stage "+stage)
	}
	if close := props["closedate"]; close != "" {
		parts = append(parts, "close "+close)
	}
	return strings.Join(parts, ", ")
}

func (Normalizer) Normalize(cfg connector.Config, item connector.RawItem, acl connector.ACL) (connector.NormalizedEvent, error) {
	var env Envelope
	if err := json.Unmarshal(item.Body, &env); err != nil {
		return connector.NormalizedEvent{}, fmt.Errorf("decode envelope: %w", err)
	}
	record := env.Record
	if record.ID == "" {
		return connector.NormalizedEvent{}, fmt.Errorf("record has no id")
	}
	if record.UpdatedAt.IsZero() {
		return connector.NormalizedEvent{}, fmt.Errorf("record %s has no updated time", record.ID)
	}
	body := summarize(record.Properties)
	if body == "" {
		return connector.NormalizedEvent{}, fmt.Errorf("record %s has no summarizable content", record.ID)
	}
	return connector.NormalizedEvent{
		TenantID:    cfg.TenantID,
		ConnectorID: cfg.ConnectorID,
		SourceType:  SourceType,
		ExternalID:  record.ID,
		ThreadKey:   record.ID,
		OccurredAt:  record.UpdatedAt,
		ACL:         acl,
		Payload: connector.Payload{
			Body:      body,
			Structure: map[string]any{"properties": record.Properties},
		},
	}, nil
}
