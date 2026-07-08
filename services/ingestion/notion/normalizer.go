package notion

import (
	"encoding/json"
	"fmt"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
)

type Normalizer struct{}

func (Normalizer) Normalize(cfg connector.Config, item connector.RawItem, acl connector.ACL) (connector.NormalizedEvent, error) {
	var env Envelope
	if err := json.Unmarshal(item.Body, &env); err != nil {
		return connector.NormalizedEvent{}, fmt.Errorf("decode envelope: %w", err)
	}
	if env.Page.ID == "" {
		return connector.NormalizedEvent{}, fmt.Errorf("page has no id")
	}
	if env.Page.LastEditedTime.IsZero() {
		return connector.NormalizedEvent{}, fmt.Errorf("page %s has no last edited time", env.Page.ID)
	}

	authorRef := connector.AuthorRef{}
	if env.Page.LastEditedBy != "" {
		authorRef.SourceRef = "notion:" + env.Page.LastEditedBy
	}

	return connector.NormalizedEvent{
		TenantID:    cfg.TenantID,
		ConnectorID: cfg.ConnectorID,
		SourceType:  SourceType,
		ExternalID:  item.ExternalID,
		AuthorRef:   authorRef,
		ThreadKey:   env.Page.ID,
		OccurredAt:  env.Page.LastEditedTime,
		ACL:         acl,
		Payload: connector.Payload{
			Body: env.Content,
			Structure: map[string]any{
				"title": env.Page.Title,
				"url":   env.Page.URL,
			},
		},
	}, nil
}
