package chunker

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func docMessage(text string) Message {
	return Message{
		EventID:    uuid.New(),
		SourceType: "gdrive",
		OccurredAt: time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC),
		Text:       text,
		ACL:        []byte(`{"scope": "tenant"}`),
	}
}

func TestChunkDocument(t *testing.T) {
	t.Run("short documents become one chunk keyed to the event", func(t *testing.T) {
		message := docMessage("Discounts above 15 percent need approval.")
		chunks := ChunkDocument([]Message{message})
		if len(chunks) != 1 {
			t.Fatalf("chunks = %d, want 1", len(chunks))
		}
		if chunks[0].WindowKey != "doc:"+message.EventID.String() {
			t.Fatalf("window key = %q", chunks[0].WindowKey)
		}
		if chunks[0].SourceType != "gdrive" {
			t.Fatalf("source type = %q", chunks[0].SourceType)
		}
	})

	t.Run("long documents split into overlapping parts", func(t *testing.T) {
		paragraph := strings.Repeat("the pricing playbook covers escalation and approval paths ", 40)
		text := strings.Join([]string{paragraph, paragraph, paragraph, paragraph}, "\n\n")
		chunks := ChunkDocument([]Message{docMessage(text)})
		if len(chunks) < 2 {
			t.Fatalf("chunks = %d, want a split", len(chunks))
		}
		for _, chunk := range chunks {
			if chunk.TokenEstimate > emailTokenMax {
				t.Fatalf("chunk of %d tokens exceeds the cap", chunk.TokenEstimate)
			}
		}
		if !strings.Contains(chunks[1].Content, chunks[0].Content[len(chunks[0].Content)-40:]) {
			t.Fatal("second chunk lacks overlap from the first")
		}
	})

	t.Run("empty documents produce nothing", func(t *testing.T) {
		if got := ChunkDocument([]Message{docMessage("   \n  ")}); len(got) != 0 {
			t.Fatalf("chunks = %d, want 0", len(got))
		}
	})
}
