package pipeline

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/attempttechnologies/company-brain/services/embedding/internal/testdb"
)

func TestPipelineEmbedsActiveCanonStatementsOnce(t *testing.T) {
	embedder, admin := testdb.Pools(t)
	root := t.TempDir()
	f := testdb.CreateFixture(t, admin, root)
	p, fake := newPipeline(t, embedder, admin, root)
	ctx := context.Background()

	personID := uuid.New()
	entryID := uuid.New()
	versionID := uuid.New()
	if _, err := admin.Exec(ctx,
		`insert into people (id, tenant_id, email, display_name, role) values ($1, $2, $3, 'Owner', 'owner')`,
		personID, f.TenantID, personID.String()+"@test"); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `
		insert into canon_entries (id, tenant_id, domain, tier, owner_id, status, visibility, verify_interval)
		values ($1, $2, 'pricing', 'operational', $3, 'active', '{"scope": "tenant"}', interval '60 days')`,
		entryID, f.TenantID, personID); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `
		insert into canon_versions (id, tenant_id, entry_id, version_number, statement, created_by, status)
		values ($1, $2, $3, 1, 'Growth plan is 1499 per month', $4, 'approved')`,
		versionID, f.TenantID, entryID, personID); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx,
		`update canon_entries set current_version_id = $2 where id = $1`, entryID, versionID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		for _, stmt := range []string{
			`delete from canon_statement_embeddings where version_id = $1`,
			`update canon_entries set current_version_id = null where current_version_id = $1`,
			`delete from canon_versions where id = $1`,
		} {
			if _, err := admin.Exec(ctx, stmt, versionID); err != nil {
				t.Errorf("cleanup failed: %v", err)
			}
		}
		if _, err := admin.Exec(ctx, `delete from canon_entries where id = $1`, entryID); err != nil {
			t.Errorf("cleanup failed: %v", err)
		}
	})

	first, err := p.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if first.StatementsEmbedded < 1 {
		t.Fatalf("expected at least the fixture statement embedded, got %+v", first)
	}

	var count int
	if err := admin.QueryRow(ctx,
		`select count(*) from canon_statement_embeddings where version_id = $1 and embedding_model = $2`,
		versionID, fake.Model).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one statement embedding, got %d", count)
	}

	second, err := p.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second.StatementsEmbedded != 0 {
		t.Fatalf("statements must embed once per model, got %+v", second)
	}
}
