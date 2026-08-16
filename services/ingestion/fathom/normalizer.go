package fathom

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
)

type Normalizer struct{}

func transcriptText(meeting Meeting) string {
	var b strings.Builder
	if meeting.Title != "" {
		b.WriteString(meeting.Title)
		b.WriteString("\n")
	}
	for _, segment := range meeting.Transcript {
		if segment.Text == "" {
			continue
		}
		if segment.Speaker != "" {
			b.WriteString(segment.Speaker)
			b.WriteString(": ")
		}
		b.WriteString(segment.Text)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func (Normalizer) Normalize(cfg connector.Config, item connector.RawItem, acl connector.ACL) (connector.NormalizedEvent, error) {
	var env Envelope
	if err := json.Unmarshal(item.Body, &env); err != nil {
		return connector.NormalizedEvent{}, fmt.Errorf("decode envelope: %w", err)
	}
	meeting := env.Meeting
	if meeting.RecordingID == 0 {
		return connector.NormalizedEvent{}, fmt.Errorf("meeting has no recording id")
	}
	if meeting.StartTime.IsZero() {
		return connector.NormalizedEvent{}, fmt.Errorf("meeting %d has no start time", meeting.RecordingID)
	}
	body := transcriptText(meeting)
	if body == "" {
		return connector.NormalizedEvent{}, fmt.Errorf("meeting %d has no transcript content", meeting.RecordingID)
	}
	key := recordingKey(meeting.RecordingID)
	authorRef := connector.AuthorRef{}
	if meeting.RecordedBy != "" {
		authorRef.SourceRef = "email:" + meeting.RecordedBy
	}
	return connector.NormalizedEvent{
		TenantID:    cfg.TenantID,
		ConnectorID: cfg.ConnectorID,
		SourceType:  SourceType,
		ExternalID:  key,
		AuthorRef:   authorRef,
		ThreadKey:   key,
		OccurredAt:  meeting.StartTime,
		ACL:         acl,
		Payload: connector.Payload{
			Body: body,
			Structure: map[string]any{
				"title":        meeting.Title,
				"meeting_type": meeting.MeetingType,
				"share_url":    meeting.ShareURL,
			},
		},
	}, nil
}
