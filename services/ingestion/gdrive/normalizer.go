package gdrive

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
	if env.File.ID == "" {
		return connector.NormalizedEvent{}, fmt.Errorf("file has no id")
	}
	if env.File.ModifiedTime.IsZero() {
		return connector.NormalizedEvent{}, fmt.Errorf("file %s has no modified time", env.File.ID)
	}

	authorRef := connector.AuthorRef{}
	if env.File.LastModifier != "" {
		authorRef.SourceRef = "email:" + env.File.LastModifier
	}

	return connector.NormalizedEvent{
		TenantID:    cfg.TenantID,
		ConnectorID: cfg.ConnectorID,
		SourceType:  SourceType,
		ExternalID:  item.ExternalID,
		AuthorRef:   authorRef,
		ThreadKey:   env.File.ID,
		OccurredAt:  env.File.ModifiedTime,
		ACL:         acl,
		Payload: connector.Payload{
			Body: env.Content,
			Structure: map[string]any{
				"title":     env.File.Name,
				"mime_type": env.File.MimeType,
				"version":   env.File.Version,
			},
		},
	}, nil
}
