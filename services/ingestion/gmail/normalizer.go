package gmail

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
	message := env.Message
	if message.ID == "" {
		return connector.NormalizedEvent{}, fmt.Errorf("message has no id")
	}
	if message.InternalDate.IsZero() {
		return connector.NormalizedEvent{}, fmt.Errorf("message %s has no internal date", message.ID)
	}

	authorRef := connector.AuthorRef{}
	if message.From != "" {
		authorRef.SourceRef = "email:" + message.From
	}

	return connector.NormalizedEvent{
		TenantID:    cfg.TenantID,
		ConnectorID: cfg.ConnectorID,
		SourceType:  SourceType,
		ExternalID:  message.ID,
		AuthorRef:   authorRef,
		ThreadKey:   message.ThreadID,
		OccurredAt:  message.InternalDate,
		ACL:         acl,
		Payload: connector.Payload{
			Body: message.Body,
			Structure: map[string]any{
				"subject": message.Subject,
				"from":    message.From,
				"to":      message.To,
				"cc":      message.Cc,
			},
		},
	}, nil
}
