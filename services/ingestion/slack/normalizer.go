package slack

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
	if env.Channel.ID == "" {
		return connector.NormalizedEvent{}, fmt.Errorf("envelope has no channel")
	}
	fields, err := parseMessage(env.Message)
	if err != nil {
		return connector.NormalizedEvent{}, err
	}
	occurredAt, err := parseTS(fields.TS)
	if err != nil {
		return connector.NormalizedEvent{}, err
	}

	threadTS := fields.ThreadTS
	if threadTS == "" {
		threadTS = fields.TS
	}

	var message map[string]any
	if err := json.Unmarshal(env.Message, &message); err != nil {
		return connector.NormalizedEvent{}, fmt.Errorf("decode message structure: %w", err)
	}

	authorRef := connector.AuthorRef{}
	if fields.User != "" {
		authorRef.SourceRef = "slack:" + fields.User
	}

	return connector.NormalizedEvent{
		TenantID:    cfg.TenantID,
		ConnectorID: cfg.ConnectorID,
		SourceType:  SourceType,
		ExternalID:  item.ExternalID,
		AuthorRef:   authorRef,
		ThreadKey:   env.Channel.ID + ":" + threadTS,
		OccurredAt:  occurredAt,
		ACL:         acl,
		Payload: connector.Payload{
			Body: fields.Text,
			Structure: map[string]any{
				"channel": map[string]any{"id": env.Channel.ID, "name": env.Channel.Name},
				"message": message,
			},
		},
	}, nil
}
