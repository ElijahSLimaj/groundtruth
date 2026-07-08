package chunker

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func emailMessage(text string) Message {
	return Message{
		EventID:    uuid.New(),
		OccurredAt: time.Date(2026, 7, 5, 8, 45, 0, 0, time.UTC),
		SourceType: "gmail",
		ThreadKey:  "gmail-thread-1",
		Author:     "sam@acme.test",
		Text:       text,
		ACL:        []byte(`{"scope": "principals", "principals": ["email:sam@acme.test"]}`),
	}
}

func TestEmailBecomesOneChunkPerMessage(t *testing.T) {
	t.Parallel()
	message := emailMessage("Growth is 1499 per month.\n\nHappy to jump on a call.")

	chunks := ChunkEmail([]Message{message})

	if len(chunks) != 1 {
		t.Fatalf("expected one chunk, got %d", len(chunks))
	}
	c := chunks[0]
	if c.WindowKey != "email:"+message.EventID.String() {
		t.Fatalf("unexpected window key %q", c.WindowKey)
	}
	if c.SourceType != "gmail" || len(c.EventIDs) != 1 {
		t.Fatalf("unexpected chunk %+v", c)
	}
	if string(c.ACL) != string(message.ACL) {
		t.Fatal("email chunk must carry its message ACL untouched")
	}
}

func TestQuotedHistoryIsStrippedNotEmbedded(t *testing.T) {
	t.Parallel()
	message := emailMessage(
		"Yes, 1799 works for us.\n\nOn Mon, Jul 6, 2026 at 9:00 AM Sam Sales wrote:\n> the growth plan is 1499\n> two months free on annual",
	)

	chunks := ChunkEmail([]Message{message})

	if len(chunks) != 1 {
		t.Fatalf("expected one chunk, got %d", len(chunks))
	}
	if strings.Contains(chunks[0].Content, "1499") {
		t.Fatal("quoted history must be stripped before embedding")
	}
	if !strings.Contains(chunks[0].Content, "1799") {
		t.Fatal("the reply body must be kept")
	}
}

func TestInterleavedQuoteLinesAreDropped(t *testing.T) {
	t.Parallel()
	message := emailMessage("> old context line\nOur reply about the roadmap.\n> more old context")

	chunks := ChunkEmail([]Message{message})

	if len(chunks) != 1 || strings.Contains(chunks[0].Content, "old context") {
		t.Fatalf("quote lines must be dropped, got %+v", chunks)
	}
}

func TestLongEmailSplitsAtParagraphsWithOverlap(t *testing.T) {
	t.Parallel()
	paragraph := strings.Repeat("pricing detail sentence ", 60)
	message := emailMessage(strings.Join([]string{paragraph, paragraph, paragraph}, "\n\n"))

	chunks := ChunkEmail([]Message{message})

	if len(chunks) < 2 {
		t.Fatalf("expected a long email to split, got %d chunks", len(chunks))
	}
	for i, c := range chunks {
		if c.ChunkIndex != i {
			t.Fatalf("expected sequential indexes, got %+v", c)
		}
		if c.TokenEstimate > 900 {
			t.Fatalf("chunk %d exceeds the token budget: %d", i, c.TokenEstimate)
		}
	}
	tail := chunks[0].Content[len(chunks[0].Content)-50:]
	if !strings.Contains(chunks[1].Content, tail) {
		t.Fatal("consecutive chunks must overlap")
	}
}

func TestQuoteOnlyEmailProducesNoChunks(t *testing.T) {
	t.Parallel()
	message := emailMessage("> just a forward\n> nothing new")

	if chunks := ChunkEmail([]Message{message}); len(chunks) != 0 {
		t.Fatalf("expected no chunks, got %d", len(chunks))
	}
}
