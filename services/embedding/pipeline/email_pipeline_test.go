package pipeline

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/attempttechnologies/company-brain/services/embedding/internal/testdb"
)

func TestPipelineChunksGmailEventsPerMessage(t *testing.T) {
	embedder, admin := testdb.Pools(t)
	root := t.TempDir()
	f := testdb.CreateFixture(t, admin, root)
	p, fake := newPipeline(t, embedder, admin, root)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Hour).Add(5 * time.Minute)

	spec := testdb.EventSpec{
		ThreadKey:  "gmail-thread-77",
		OccurredAt: base,
		Author:     "email:sam@acme.test",
		Body:       "Quoting growth at 1799 for the enterprise deal.\n\nOn Mon Sam wrote:\n> old quote was 1499",
		ACL:        `{"scope": "principals", "principals": ["email:sam@acme.test", "email:ada@acme.test"]}`,
	}
	eventID := testdb.InsertEventWithSource(t, admin, f, spec, "gmail")

	result, err := p.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.ChunksWritten != 1 {
		t.Fatalf("expected one email chunk, got %+v", result)
	}

	chunks := loadChunks(t, admin, f.TenantID, fake.Model)
	if len(chunks) != 1 {
		t.Fatalf("expected one chunk, got %+v", chunks)
	}
	c := chunks[0]
	if c.WindowKey != "email:"+eventID.String() {
		t.Fatalf("unexpected window key %q", c.WindowKey)
	}
	if strings.Contains(c.Content, "1499") {
		t.Fatal("quoted history leaked into the embedded chunk")
	}
	if c.ACLScope != "principals" {
		t.Fatalf("email chunk must keep the principals ACL, got %q", c.ACLScope)
	}
}
