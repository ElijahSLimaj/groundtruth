package chunker

import (
	"strings"

	"github.com/google/uuid"
)

func ChunkDocument(messages []Message) []Chunk {
	var chunks []Chunk
	for _, message := range messages {
		body := strings.TrimSpace(message.Text)
		if body == "" {
			continue
		}
		windowKey := "doc:" + message.EventID.String()
		parts := splitEmailBody(body)
		for index, part := range parts {
			chunks = append(chunks, Chunk{
				AnchorEventID:    message.EventID,
				AnchorOccurredAt: message.OccurredAt,
				SourceType:       message.SourceType,
				WindowKey:        windowKey,
				ChunkIndex:       index,
				Content:          part,
				TokenEstimate:    EstimateTokens(part),
				ACL:              message.ACL,
				EventIDs:         []uuid.UUID{message.EventID},
			})
		}
	}
	return chunks
}
