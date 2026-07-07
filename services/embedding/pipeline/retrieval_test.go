package pipeline

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/attempttechnologies/company-brain/services/embedding/internal/testdb"
)

func TestPipelineOutputIsRetrievableThroughStreamSearch(t *testing.T) {
	embedder, admin := testdb.Pools(t)
	app := testdb.AppPool(t)
	root := t.TempDir()
	f := testdb.CreateFixture(t, admin, root)
	p, fake := newPipeline(t, embedder, admin, root)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Hour).Add(5 * time.Minute)
	thread := "C1:pricing-thread"

	testdb.InsertEvent(t, admin, f, testdb.EventSpec{
		ThreadKey: thread, OccurredAt: base, Author: "slack:U1",
		Body: "customer asked about the growth plan pricing"})
	testdb.InsertEvent(t, admin, f, testdb.EventSpec{
		ThreadKey: thread, OccurredAt: base.Add(time.Minute), Author: "slack:U2",
		Body: "growth is 1499 per month, two months free on annual"})
	testdb.InsertEvent(t, admin, f, testdb.EventSpec{
		ThreadKey: "C2:offsite", OccurredAt: base, Author: "slack:U3",
		Body: "offsite is confirmed for the second week of September",
		ACL:  `{"scope": "principals", "principals": ["slack:U3", "slack:U4"]}`})

	if _, err := p.Run(ctx); err != nil {
		t.Fatal(err)
	}

	queryVectors, err := fake.Embed(ctx, []string{"what does the growth plan cost?"})
	if err != nil {
		t.Fatal(err)
	}
	queryVector := vectorLiteral(queryVectors[0])

	search := func(t *testing.T, principals []string) []string {
		t.Helper()
		tx, err := app.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx,
			`select set_config('app.tenant_id', $1, true)`, f.TenantID.String()); err != nil {
			t.Fatal(err)
		}
		rows, err := tx.Query(ctx, `
			select chunk_content from public.stream_search($1::extensions.vector, $2, $3, $4)
		`, queryVector, "growth plan pricing", fake.Model, principals)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var contents []string
		for rows.Next() {
			var c string
			if err := rows.Scan(&c); err != nil {
				t.Fatal(err)
			}
			contents = append(contents, c)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return contents
	}

	broad := search(t, []string{})
	if len(broad) == 0 || !strings.Contains(broad[0], "1499 per month") {
		t.Fatalf("expected the pricing thread chunk first, got %v", broad)
	}
	for _, content := range broad {
		if strings.Contains(content, "offsite") {
			t.Fatal("principals-scoped chunk leaked to a caller without the principal")
		}
	}

	scoped := search(t, []string{"slack:U4"})
	found := false
	for _, content := range scoped {
		if strings.Contains(content, "offsite") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the offsite chunk for a matching principal, got %v", scoped)
	}
}
