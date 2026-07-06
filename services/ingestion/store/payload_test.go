package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestFSPayloadStore(t *testing.T) {
	t.Parallel()

	t.Run("same_content_yields_same_ref_and_one_file", func(t *testing.T) {
		t.Parallel()
		s := &FSPayloadStore{Root: t.TempDir()}
		tenantID := uuid.New()

		ref1, err := s.Put(context.Background(), tenantID, []byte(`{"body":"hello"}`))
		if err != nil {
			t.Fatal(err)
		}
		ref2, err := s.Put(context.Background(), tenantID, []byte(`{"body":"hello"}`))
		if err != nil {
			t.Fatal(err)
		}

		if ref1 != ref2 {
			t.Fatalf("expected identical refs, got %q and %q", ref1, ref2)
		}
		entries, err := filepath.Glob(filepath.Join(s.Root, tenantID.String(), "*"))
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected one stored file, got %d", len(entries))
		}
	})

	t.Run("different_tenants_get_different_refs", func(t *testing.T) {
		t.Parallel()
		s := &FSPayloadStore{Root: t.TempDir()}

		ref1, err := s.Put(context.Background(), uuid.New(), []byte("same"))
		if err != nil {
			t.Fatal(err)
		}
		ref2, err := s.Put(context.Background(), uuid.New(), []byte("same"))
		if err != nil {
			t.Fatal(err)
		}

		if ref1 == ref2 {
			t.Fatalf("expected tenant-scoped refs to differ, both were %q", ref1)
		}
	})

	t.Run("stored_content_round_trips", func(t *testing.T) {
		t.Parallel()
		s := &FSPayloadStore{Root: t.TempDir()}
		tenantID := uuid.New()
		content := []byte(`{"body":"round trip"}`)

		ref, err := s.Put(context.Background(), tenantID, content)
		if err != nil {
			t.Fatal(err)
		}

		matches, err := filepath.Glob(filepath.Join(s.Root, tenantID.String(), "*"))
		if err != nil || len(matches) != 1 {
			t.Fatalf("expected one file for ref %q, got %v (%v)", ref, matches, err)
		}
		stored, err := os.ReadFile(matches[0])
		if err != nil {
			t.Fatal(err)
		}
		if string(stored) != string(content) {
			t.Fatalf("content mismatch: %q", stored)
		}
	})
}
