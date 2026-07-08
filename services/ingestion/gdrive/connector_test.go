package gdrive

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/attempttechnologies/company-brain/services/ingestion/connector"
)

type fakeAPI struct {
	startToken string
	changes    map[string]struct {
		files    []File
		nextPage string
		newStart string
	}
	listed      []File
	content     map[string]string
	permissions map[string][]Permission
}

func (f *fakeAPI) StartPageToken(context.Context) (string, error) {
	return f.startToken, nil
}

func (f *fakeAPI) Changes(_ context.Context, pageToken string) ([]File, string, string, error) {
	page := f.changes[pageToken]
	return page.files, page.nextPage, page.newStart, nil
}

func (f *fakeAPI) ListFiles(context.Context, time.Time, time.Time) ([]File, error) {
	return f.listed, nil
}

func (f *fakeAPI) ExportText(_ context.Context, file File) (string, error) {
	return f.content[file.ID], nil
}

func (f *fakeAPI) Permissions(_ context.Context, fileID string) ([]Permission, error) {
	return f.permissions[fileID], nil
}

func (f *fakeAPI) About(context.Context) error { return nil }

type collectSink struct {
	items []connector.RawItem
}

func (s *collectSink) Emit(_ context.Context, item connector.RawItem) error {
	s.items = append(s.items, item)
	return nil
}

func docFile(id, name string, version int64) File {
	return File{
		ID:           id,
		Name:         name,
		MimeType:     "application/vnd.google-apps.document",
		ModifiedTime: time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC),
		Version:      version,
		LastModifier: "ada@acme.test",
	}
}

func testConfig() connector.Config {
	return connector.Config{
		ConnectorID: uuid.New(),
		TenantID:    uuid.New(),
		SourceType:  SourceType,
		Settings:    map[string]any{"token": "drive-token"},
	}
}

func TestDriveConnector(t *testing.T) {
	ctx := context.Background()

	t.Run("bootstraps the cursor without emitting", func(t *testing.T) {
		fake := &fakeAPI{startToken: "tok-1"}
		sink := &collectSink{}
		c := New(func(string) API { return fake })
		next, err := c.Poll(ctx, testConfig(), "", sink)
		if err != nil {
			t.Fatal(err)
		}
		if next != "tok-1" || len(sink.items) != 0 {
			t.Fatalf("cursor = %q items = %d", next, len(sink.items))
		}
	})

	t.Run("emits changed documents and skips noise", func(t *testing.T) {
		fake := &fakeAPI{
			changes: map[string]struct {
				files    []File
				nextPage string
				newStart string
			}{
				"tok-1": {
					files: []File{
						docFile("D1", "Pricing playbook", 7),
						{ID: "IMG", MimeType: "image/png", ModifiedTime: time.Now(), Version: 1},
						{ID: "GONE", MimeType: "application/vnd.google-apps.document", ModifiedTime: time.Now(), Version: 2, Trashed: true},
					},
					newStart: "tok-2",
				},
			},
			content: map[string]string{"D1": "Discounts above 15 percent need approval."},
		}
		sink := &collectSink{}
		c := New(func(string) API { return fake })
		next, err := c.Poll(ctx, testConfig(), "tok-1", sink)
		if err != nil {
			t.Fatal(err)
		}
		if next != "tok-2" {
			t.Fatalf("cursor = %q, want tok-2", next)
		}
		if len(sink.items) != 1 {
			t.Fatalf("items = %d, want 1", len(sink.items))
		}
		if sink.items[0].ExternalID != "D1:7" {
			t.Fatalf("external id = %q", sink.items[0].ExternalID)
		}
	})

	t.Run("maps domain sharing to tenant scope", func(t *testing.T) {
		fake := &fakeAPI{
			permissions: map[string][]Permission{
				"D1": {{Type: "domain"}},
			},
		}
		c := New(func(string) API { return fake })
		body, _ := json.Marshal(Envelope{File: docFile("D1", "Doc", 1)})
		acl, err := c.ResolveACL(ctx, testConfig(), connector.RawItem{Body: body})
		if err != nil {
			t.Fatal(err)
		}
		if acl.Scope != connector.ACLScopeTenant || acl.SourceScope.ID != "D1" {
			t.Fatalf("acl = %+v", acl)
		}
	})

	t.Run("maps explicit users to principals", func(t *testing.T) {
		fake := &fakeAPI{
			permissions: map[string][]Permission{
				"D2": {
					{Type: "user", Email: "ada@acme.test"},
					{Type: "user", Email: "sam@acme.test"},
				},
			},
		}
		c := New(func(string) API { return fake })
		body, _ := json.Marshal(Envelope{File: docFile("D2", "Private plan", 1)})
		acl, err := c.ResolveACL(ctx, testConfig(), connector.RawItem{Body: body})
		if err != nil {
			t.Fatal(err)
		}
		if acl.Scope != connector.ACLScopePrincipals {
			t.Fatalf("scope = %q", acl.Scope)
		}
		if len(acl.Principals) != 2 || acl.Principals[0] != "email:ada@acme.test" {
			t.Fatalf("principals = %v", acl.Principals)
		}
	})

	t.Run("refuses files with no resolvable principals", func(t *testing.T) {
		fake := &fakeAPI{permissions: map[string][]Permission{}}
		c := New(func(string) API { return fake })
		body, _ := json.Marshal(Envelope{File: docFile("D3", "Orphan", 1)})
		if _, err := c.ResolveACL(ctx, testConfig(), connector.RawItem{Body: body}); err == nil {
			t.Fatal("expected an error for unresolvable acl")
		}
	})

	t.Run("normalizes documents with the file as thread", func(t *testing.T) {
		cfg := testConfig()
		body, _ := json.Marshal(Envelope{
			File:    docFile("D1", "Pricing playbook", 7),
			Content: "Discounts above 15 percent need approval.",
		})
		ev, err := Normalizer{}.Normalize(cfg, connector.RawItem{ExternalID: "D1:7", Body: body}, connector.ACL{Scope: connector.ACLScopeTenant})
		if err != nil {
			t.Fatal(err)
		}
		if ev.ThreadKey != "D1" || ev.SourceType != SourceType {
			t.Fatalf("event = %+v", ev)
		}
		if ev.AuthorRef.SourceRef != "email:ada@acme.test" {
			t.Fatalf("author = %q", ev.AuthorRef.SourceRef)
		}
		if ev.Payload.Body == "" || ev.Payload.Structure["title"] != "Pricing playbook" {
			t.Fatalf("payload = %+v", ev.Payload)
		}
	})
}
