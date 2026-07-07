package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/attempttechnologies/company-brain/services/embedding/internal/testdb"
	"github.com/attempttechnologies/company-brain/services/embedding/payload"
	"github.com/attempttechnologies/company-brain/services/embedding/provider"
)

func newPipeline(t *testing.T, embedder, admin *pgxpool.Pool, root string) (*Pipeline, *provider.Fake) {
	t.Helper()
	fake := &provider.Fake{Model: "fake-" + uuid.NewString()[:8], Dims: 1536}
	testdb.CleanupWatermark(t, admin, fake.Model)
	_, err := admin.Exec(context.Background(),
		`insert into embedding_watermark (embedding_model, last_ingested_at) values ($1, now())`,
		fake.Model)
	if err != nil {
		t.Fatal(err)
	}
	return &Pipeline{
		Pool:     embedder,
		Provider: fake,
		Payloads: &payload.FSReader{Root: root},
	}, fake
}

type chunkRow struct {
	WindowKey  string
	ChunkIndex int
	Content    string
	ACLScope   string
	TokenCount int
	Members    int
}

func loadChunks(t *testing.T, admin *pgxpool.Pool, tenantID uuid.UUID, model string) []chunkRow {
	t.Helper()
	rows, err := admin.Query(context.Background(), `
		select window_key, chunk_index, content, acl->>'scope', token_count, array_length(member_event_ids, 1)
		from event_chunks
		where tenant_id = $1 and embedding_model = $2
		order by window_key, chunk_index
	`, tenantID, model)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var chunks []chunkRow
	for rows.Next() {
		var c chunkRow
		if err := rows.Scan(&c.WindowKey, &c.ChunkIndex, &c.Content, &c.ACLScope, &c.TokenCount, &c.Members); err != nil {
			t.Fatal(err)
		}
		chunks = append(chunks, c)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return chunks
}

func TestPipelineChunksThreadsAndWindowsAndAdvancesWatermark(t *testing.T) {
	embedder, admin := testdb.Pools(t)
	root := t.TempDir()
	f := testdb.CreateFixture(t, admin, root)
	p, fake := newPipeline(t, embedder, admin, root)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Hour).Add(5 * time.Minute)

	thread := "C1:" + base.Format("150405")
	testdb.InsertEvent(t, admin, f, testdb.EventSpec{
		ThreadKey: thread, OccurredAt: base, Author: "slack:U1", Body: "should we raise pricing?"})
	testdb.InsertEvent(t, admin, f, testdb.EventSpec{
		ThreadKey: thread, OccurredAt: base.Add(time.Minute), Author: "slack:U2", Body: "yes, 1799 from August"})
	testdb.InsertEvent(t, admin, f, testdb.EventSpec{
		ThreadKey: "C2:standalone", OccurredAt: base, Author: "slack:U3", Body: "deploy finished"})

	result, err := p.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.EventsScanned != 3 || result.ChunksWritten != 2 || result.DeadLettered != 0 {
		t.Fatalf("unexpected result %+v", result)
	}

	chunks := loadChunks(t, admin, f.TenantID, fake.Model)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %+v", chunks)
	}
	threadChunk, windowChunk := chunks[0], chunks[1]
	if !strings.HasPrefix(threadChunk.WindowKey, "C1:") {
		threadChunk, windowChunk = windowChunk, threadChunk
	}
	if threadChunk.Content != "U1: should we raise pricing?\nU2: yes, 1799 from August" {
		t.Fatalf("unexpected thread chunk content %q", threadChunk.Content)
	}
	if threadChunk.Members != 2 || threadChunk.ACLScope != "tenant" || threadChunk.TokenCount < 1 {
		t.Fatalf("unexpected thread chunk metadata %+v", threadChunk)
	}
	if !strings.HasPrefix(windowChunk.WindowKey, "C2@") {
		t.Fatalf("expected channel-hour window key, got %q", windowChunk.WindowKey)
	}

	again, err := p.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if again.EventsScanned != 0 {
		t.Fatalf("watermark must prevent rescanning, got %+v", again)
	}
}

func TestLateReplyRechunksThreadWithoutStaleChunks(t *testing.T) {
	embedder, admin := testdb.Pools(t)
	root := t.TempDir()
	f := testdb.CreateFixture(t, admin, root)
	p, fake := newPipeline(t, embedder, admin, root)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Hour).Add(5 * time.Minute)
	thread := "C1:thread-late"

	testdb.InsertEvent(t, admin, f, testdb.EventSpec{
		ThreadKey: thread, OccurredAt: base, Author: "slack:U1", Body: "kickoff"})
	testdb.InsertEvent(t, admin, f, testdb.EventSpec{
		ThreadKey: thread, OccurredAt: base.Add(time.Minute), Author: "slack:U2", Body: "first reply"})
	if _, err := p.Run(ctx); err != nil {
		t.Fatal(err)
	}

	testdb.InsertEvent(t, admin, f, testdb.EventSpec{
		ThreadKey: thread, OccurredAt: base.Add(2 * time.Minute), Author: "slack:U3", Body: "late reply"})
	result, err := p.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.EventsScanned != 1 {
		t.Fatalf("expected only the late reply to scan, got %+v", result)
	}

	chunks := loadChunks(t, admin, f.TenantID, fake.Model)
	if len(chunks) != 1 {
		t.Fatalf("expected the thread rebuilt into one chunk, got %+v", chunks)
	}
	c := chunks[0]
	if c.Members != 3 || !strings.Contains(c.Content, "late reply") || !strings.Contains(c.Content, "kickoff") {
		t.Fatalf("rechunked thread is wrong: %+v", c)
	}
}

func TestStandaloneMovingIntoThreadRebuildsBothWindows(t *testing.T) {
	embedder, admin := testdb.Pools(t)
	root := t.TempDir()
	f := testdb.CreateFixture(t, admin, root)
	p, fake := newPipeline(t, embedder, admin, root)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Hour).Add(5 * time.Minute)
	thread := "C1:parent-ts"

	testdb.InsertEvent(t, admin, f, testdb.EventSpec{
		ThreadKey: thread, OccurredAt: base, Author: "slack:U1", Body: "a question nobody answered yet"})
	testdb.InsertEvent(t, admin, f, testdb.EventSpec{
		ThreadKey: "C1:other-ts", OccurredAt: base.Add(time.Minute), Author: "slack:U2", Body: "unrelated status note"})
	if _, err := p.Run(ctx); err != nil {
		t.Fatal(err)
	}

	first := loadChunks(t, admin, f.TenantID, fake.Model)
	if len(first) != 1 || first[0].Members != 2 || !strings.HasPrefix(first[0].WindowKey, "C1@") {
		t.Fatalf("expected both standalones in one hour window, got %+v", first)
	}

	testdb.InsertEvent(t, admin, f, testdb.EventSpec{
		ThreadKey: thread, OccurredAt: base.Add(2 * time.Minute), Author: "slack:U3", Body: "answering now"})
	if _, err := p.Run(ctx); err != nil {
		t.Fatal(err)
	}

	chunks := loadChunks(t, admin, f.TenantID, fake.Model)
	if len(chunks) != 2 {
		t.Fatalf("expected a thread chunk plus a rebuilt hour window, got %+v", chunks)
	}
	questionAppearances := 0
	for _, c := range chunks {
		if strings.Contains(c.Content, "a question nobody answered yet") {
			questionAppearances++
			if c.WindowKey != thread {
				t.Fatalf("the threaded message must live in the thread window, found in %q", c.WindowKey)
			}
		}
	}
	if questionAppearances != 1 {
		t.Fatalf("threaded message must appear exactly once across chunks, appeared %d times", questionAppearances)
	}
}

func TestACLBoundarySurvivesTheFullWritePath(t *testing.T) {
	embedder, admin := testdb.Pools(t)
	root := t.TempDir()
	f := testdb.CreateFixture(t, admin, root)
	p, fake := newPipeline(t, embedder, admin, root)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Hour).Add(5 * time.Minute)
	thread := "P1:secret-thread"
	private := `{"scope": "principals", "principals": ["slack:U1", "slack:U2"]}`

	testdb.InsertEvent(t, admin, f, testdb.EventSpec{
		ThreadKey: thread, OccurredAt: base, Author: "slack:U1", Body: "posted while public"})
	testdb.InsertEvent(t, admin, f, testdb.EventSpec{
		ThreadKey: thread, OccurredAt: base.Add(time.Minute), Author: "slack:U2", Body: "posted after going private", ACL: private})

	if _, err := p.Run(ctx); err != nil {
		t.Fatal(err)
	}
	chunks := loadChunks(t, admin, f.TenantID, fake.Model)
	if len(chunks) != 2 {
		t.Fatalf("ACL change inside a thread must split chunks, got %+v", chunks)
	}
	for _, c := range chunks {
		if c.ACLScope == "tenant" && strings.Contains(c.Content, "private") {
			t.Fatal("private content leaked into a tenant-scoped chunk")
		}
	}
}

func TestPoisonedChunkDeadLettersWithoutBlockingOthers(t *testing.T) {
	embedder, admin := testdb.Pools(t)
	root := t.TempDir()
	f := testdb.CreateFixture(t, admin, root)
	p, fake := newPipeline(t, embedder, admin, root)
	fake.Fail = func(content string) error {
		if strings.Contains(content, "poison") {
			return errors.New("provider rejected content")
		}
		return nil
	}
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Hour).Add(5 * time.Minute)

	testdb.InsertEvent(t, admin, f, testdb.EventSpec{
		ThreadKey: "C1:ok", OccurredAt: base, Author: "slack:U1", Body: "healthy message"})
	testdb.InsertEvent(t, admin, f, testdb.EventSpec{
		ThreadKey: "C2:bad", OccurredAt: base, Author: "slack:U2", Body: "poison message"})

	result, err := p.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.ChunksWritten != 1 || result.DeadLettered != 1 {
		t.Fatalf("expected 1 written and 1 dead lettered, got %+v", result)
	}

	var reason string
	if err := admin.QueryRow(ctx,
		`select reason from embedding_dlq where tenant_id = $1`, f.TenantID).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(reason, "provider rejected content") {
		t.Fatalf("unexpected dlq reason %q", reason)
	}
	chunks := loadChunks(t, admin, f.TenantID, fake.Model)
	if len(chunks) != 1 || strings.Contains(chunks[0].Content, "poison") {
		t.Fatalf("healthy chunk must be written and poisoned one dropped, got %+v", chunks)
	}
}
