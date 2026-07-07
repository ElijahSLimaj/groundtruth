package chunker

import (
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

var (
	tenantACL  = []byte(`{"scope": "tenant"}`)
	privateACL = []byte(`{"scope": "principals", "principals": ["slack:U1"]}`)
	baseTime   = time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
)

func message(thread string, minute int, author, text string, acl []byte) Message {
	return Message{
		EventID:    uuid.New(),
		OccurredAt: baseTime.Add(time.Duration(minute) * time.Minute),
		ThreadKey:  thread,
		Author:     author,
		Text:       text,
		ACL:        acl,
	}
}

func TestShortThreadBecomesOneChunk(t *testing.T) {
	t.Parallel()
	m1 := message("C1:100", 0, "U1", "should we raise pricing?", tenantACL)
	m2 := message("C1:100", 1, "U2", "yes, to 1799 from August", tenantACL)

	chunks := ChunkChat([]Message{m1, m2})

	if len(chunks) != 1 {
		t.Fatalf("expected one chunk, got %d", len(chunks))
	}
	c := chunks[0]
	if c.Content != "U1: should we raise pricing?\nU2: yes, to 1799 from August" {
		t.Fatalf("unexpected content %q", c.Content)
	}
	if c.AnchorEventID != m2.EventID || len(c.EventIDs) != 2 {
		t.Fatalf("unexpected anchor or members %+v", c)
	}
	if string(c.ACL) != string(tenantACL) || c.WindowKey != "C1:100" || c.ChunkIndex != 0 {
		t.Fatalf("unexpected chunk metadata %+v", c)
	}
}

func TestLongThreadSplitsAtReplyBoundaryWithOverlap(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("pricing discussion detail ", 45)
	msgs := []Message{
		message("C1:100", 0, "U1", long, tenantACL),
		message("C1:100", 1, "U2", long, tenantACL),
		message("C1:100", 2, "U3", long, tenantACL),
		message("C1:100", 3, "U4", long, tenantACL),
	}

	chunks := ChunkChat(msgs)

	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if c.ChunkIndex != i {
			t.Fatalf("expected sequential indexes, got %+v", c)
		}
	}
	if len(chunks[0].EventIDs) != 2 || chunks[0].EventIDs[0] != msgs[0].EventID {
		t.Fatalf("unexpected first chunk members %+v", chunks[0].EventIDs)
	}
	if chunks[1].EventIDs[0] != msgs[1].EventID {
		t.Fatalf("second chunk must start with the one-message overlap, got %+v", chunks[1].EventIDs)
	}
	if chunks[2].EventIDs[0] != msgs[2].EventID || chunks[2].AnchorEventID != msgs[3].EventID {
		t.Fatalf("unexpected final chunk %+v", chunks[2].EventIDs)
	}
}

func TestACLChangeForcesChunkBoundary(t *testing.T) {
	t.Parallel()
	m1 := message("C1:100", 0, "U1", "visible to the channel", tenantACL)
	m2 := message("C1:100", 1, "U2", "posted after the channel went private", privateACL)

	chunks := ChunkChat([]Message{m1, m2})

	if len(chunks) != 2 {
		t.Fatalf("messages with different ACLs must never share a chunk, got %d", len(chunks))
	}
	if string(chunks[0].ACL) != string(tenantACL) || string(chunks[1].ACL) != string(privateACL) {
		t.Fatalf("chunk ACLs must match their members exactly")
	}
	if strings.Contains(chunks[0].Content, "private") {
		t.Fatal("private content leaked into the tenant-scoped chunk")
	}
}

func TestStandaloneMessagesGroupByChannelAndHour(t *testing.T) {
	t.Parallel()
	sameHourA := message("C1:100", 5, "U1", "deploy done", tenantACL)
	sameHourB := message("C1:200", 10, "U2", "metrics look good", tenantACL)
	nextHour := message("C1:300", 65, "U3", "seeing a latency spike", tenantACL)
	otherChannel := message("C2:400", 5, "U4", "new lead from the fair", tenantACL)

	chunks := ChunkChat([]Message{sameHourA, sameHourB, nextHour, otherChannel})

	if len(chunks) != 3 {
		t.Fatalf("expected 3 window chunks, got %d: %+v", len(chunks), chunks)
	}
	byWindow := map[string]Chunk{}
	for _, c := range chunks {
		byWindow[c.WindowKey] = c
	}
	first, ok := byWindow["C1@2026-07-06T10"]
	if !ok || len(first.EventIDs) != 2 {
		t.Fatalf("expected two standalone messages in the C1 hour window, got %+v", byWindow)
	}
	if _, ok := byWindow["C1@2026-07-06T11"]; !ok {
		t.Fatalf("expected a separate window for the next hour, got %+v", byWindow)
	}
	if _, ok := byWindow["C2@2026-07-06T10"]; !ok {
		t.Fatalf("expected a separate window for the other channel, got %+v", byWindow)
	}
}

func TestChunkingIsDeterministicUnderInputOrder(t *testing.T) {
	t.Parallel()
	msgs := []Message{
		message("C1:100", 0, "U1", "first", tenantACL),
		message("C1:100", 1, "U2", "second", tenantACL),
		message("C1:900", 2, "U3", "standalone", tenantACL),
		message("C2:100", 3, "U4", "other channel", tenantACL),
	}

	first := ChunkChat(msgs)
	shuffled := make([]Message, len(msgs))
	copy(shuffled, msgs)
	rand.New(rand.NewSource(42)).Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})
	second := ChunkChat(shuffled)

	if len(first) != len(second) {
		t.Fatalf("chunk counts differ: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Content != second[i].Content || first[i].WindowKey != second[i].WindowKey {
			t.Fatalf("chunk %d differs across input orders", i)
		}
	}
}

func TestEmptyInputProducesNoChunks(t *testing.T) {
	t.Parallel()
	if chunks := ChunkChat(nil); len(chunks) != 0 {
		t.Fatalf("expected no chunks, got %d", len(chunks))
	}
}
