package store_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/attempttechnologies/company-brain/services/ingestion/internal/testdb"
	"github.com/attempttechnologies/company-brain/services/ingestion/keys"
	"github.com/attempttechnologies/company-brain/services/ingestion/store"
)

func newEncryptedStore(t *testing.T) (*store.EncryptedPayloadStore, *keys.Service, testdb.Fixture, string) {
	t.Helper()
	worker, admin := testdb.Pools(t)
	fixture := testdb.CreateFixture(t, admin)

	master := make([]byte, 32)
	if _, err := rand.Read(master); err != nil {
		t.Fatal(err)
	}
	wrapper, err := keys.NewAESWrapper(hex.EncodeToString(master))
	if err != nil {
		t.Fatal(err)
	}
	service := &keys.Service{Pool: worker, Wrapper: wrapper}
	root := t.TempDir()
	return &store.EncryptedPayloadStore{
		Blobs: &store.FSBlobStore{Root: root},
		Keys:  service,
	}, service, fixture, root
}

func TestEncryptedPayloadStore(t *testing.T) {
	ctx := context.Background()
	content := []byte("the growth plan moved to 1799 effective august")

	t.Run("refs stay content addressed by plaintext", func(t *testing.T) {
		s, _, fixture, _ := newEncryptedStore(t)
		ref, err := s.Put(ctx, fixture.TenantID, content)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(content)
		want := fmt.Sprintf("payloads/%s/%s", fixture.TenantID, hex.EncodeToString(sum[:]))
		if ref != want {
			t.Fatalf("ref = %q, want %q", ref, want)
		}
		again, err := s.Put(ctx, fixture.TenantID, content)
		if err != nil {
			t.Fatal(err)
		}
		if again != ref {
			t.Fatal("identical content produced different refs")
		}
	})

	t.Run("nothing readable lands on disk", func(t *testing.T) {
		s, service, fixture, root := newEncryptedStore(t)
		ref, err := s.Put(ctx, fixture.TenantID, content)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(content)
		path := filepath.Join(root, fixture.TenantID.String(), hex.EncodeToString(sum[:]))
		sealed, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(sealed, content) {
			t.Fatal("payload stored in plaintext")
		}

		dataKey, err := service.DataKey(ctx, fixture.TenantID)
		if err != nil {
			t.Fatal(err)
		}
		plain, err := store.Open(dataKey, []byte(ref), sealed)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(plain, content) {
			t.Fatal("decrypted payload does not match the original")
		}

		if _, err := store.Open(dataKey, []byte("payloads/other/ref"), sealed); err == nil {
			t.Fatal("envelope opened under a foreign ref")
		}
	})

	t.Run("tenants never share an envelope key", func(t *testing.T) {
		s, service, fixtureA, root := newEncryptedStore(t)
		_, admin := testdb.Pools(t)
		fixtureB := testdb.CreateFixture(t, admin)

		refA, err := s.Put(ctx, fixtureA.TenantID, content)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Put(ctx, fixtureB.TenantID, content); err != nil {
			t.Fatal(err)
		}

		keyB, err := service.DataKey(ctx, fixtureB.TenantID)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(content)
		sealedA, err := os.ReadFile(filepath.Join(root, fixtureA.TenantID.String(), hex.EncodeToString(sum[:])))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Open(keyB, []byte(refA), sealedA); err == nil {
			t.Fatal("tenant B key opened tenant A payload")
		}
	})
}
