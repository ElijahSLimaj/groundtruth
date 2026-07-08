package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/attempttechnologies/company-brain/services/embedding/internal/testdb"
)

func TestPipelineChunksDocumentEvents(t *testing.T) {
	embedder, admin := testdb.Pools(t)
	root := t.TempDir()
	f := testdb.CreateFixture(t, admin, root)
	p, fake := newPipeline(t, embedder, admin, root)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Hour).Add(5 * time.Minute)

	driveSpec := testdb.EventSpec{
		ThreadKey:  "file-D1",
		OccurredAt: base,
		Author:     "email:ada@acme.test",
		Body:       "Discounts above 15 percent always need founder approval.",
		ACL:        `{"scope": "principals", "principals": ["email:ada@acme.test"]}`,
	}
	driveID := testdb.InsertEventWithSource(t, admin, f, driveSpec, "gdrive")

	notionSpec := testdb.EventSpec{
		ThreadKey:  "page-P1",
		OccurredAt: base.Add(time.Minute),
		Author:     "notion:user-1",
		Body:       "Deploys happen on tuesdays after the standup.",
		ACL:        `{"scope": "tenant"}`,
	}
	notionID := testdb.InsertEventWithSource(t, admin, f, notionSpec, "notion")

	result, err := p.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.ChunksWritten != 2 {
		t.Fatalf("expected two document chunks, got %+v", result)
	}

	chunks := loadChunks(t, admin, f.TenantID, fake.Model)
	if len(chunks) != 2 {
		t.Fatalf("expected two chunks, got %+v", chunks)
	}
	scopesByWindow := map[string]string{}
	for _, c := range chunks {
		scopesByWindow[c.WindowKey] = c.ACLScope
	}
	driveWindow := "doc:" + driveID.String()
	notionWindow := "doc:" + notionID.String()
	if _, ok := scopesByWindow[driveWindow]; !ok {
		t.Fatalf("no chunk for the drive document, got %v", scopesByWindow)
	}
	if _, ok := scopesByWindow[notionWindow]; !ok {
		t.Fatalf("no chunk for the notion page, got %v", scopesByWindow)
	}
	if scopesByWindow[driveWindow] != "principals" {
		t.Fatalf("gdrive chunk lost its principals ACL, got %q", scopesByWindow[driveWindow])
	}

	again, err := p.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if again.ChunksWritten != 0 {
		t.Fatalf("rerun rewrote chunks: %+v", again)
	}
}
