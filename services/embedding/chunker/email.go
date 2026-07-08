package chunker

import (
	"regexp"
	"strings"

	"github.com/google/uuid"
)

const (
	emailTokenTarget  = 650
	emailTokenMax     = 800
	emailOverlapChars = 320
)

var quoteHeaderPattern = regexp.MustCompile(`^On .+ wrote:\s*$`)

func ChunkEmail(messages []Message) []Chunk {
	var chunks []Chunk
	for _, message := range messages {
		body := StripQuotedHistory(message.Text)
		if strings.TrimSpace(body) == "" {
			continue
		}
		windowKey := "email:" + message.EventID.String()
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

func StripQuotedHistory(body string) string {
	var kept []string
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if quoteHeaderPattern.MatchString(trimmed) {
			break
		}
		if strings.HasPrefix(trimmed, ">") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func splitEmailBody(body string) []string {
	if EstimateTokens(body) <= emailTokenMax {
		return []string{body}
	}
	paragraphs := strings.Split(body, "\n\n")
	var parts []string
	var current []string
	currentTokens := 0
	for _, paragraph := range paragraphs {
		tokens := EstimateTokens(paragraph)
		if len(current) > 0 && currentTokens+tokens > emailTokenTarget {
			parts = append(parts, strings.Join(current, "\n\n"))
			overlap := overlapTail(strings.Join(current, "\n\n"))
			current = nil
			currentTokens = 0
			if overlap != "" {
				current = append(current, overlap)
				currentTokens = EstimateTokens(overlap)
			}
		}
		current = append(current, paragraph)
		currentTokens += tokens
	}
	if len(current) > 0 {
		parts = append(parts, strings.Join(current, "\n\n"))
	}
	return parts
}

func overlapTail(text string) string {
	runes := []rune(text)
	if len(runes) <= emailOverlapChars {
		return text
	}
	return string(runes[len(runes)-emailOverlapChars:])
}
