package keys_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/attempttechnologies/company-brain/services/ingestion/internal/testdb"
	"github.com/attempttechnologies/company-brain/services/ingestion/keys"
)

func masterHex(t *testing.T) string {
	t.Helper()
	master := make([]byte, 32)
	if _, err := rand.Read(master); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(master)
}

func TestAESWrapper(t *testing.T) {
	wrapper, err := keys.NewAESWrapper(masterHex(t))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("round trips a data key", func(t *testing.T) {
		plain := []byte("0123456789abcdef0123456789abcdef")
		wrapped, err := wrapper.Wrap(plain)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(wrapped, plain) {
			t.Fatal("wrapped key leaks the plaintext key")
		}
		got, err := wrapper.Unwrap(wrapped)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, plain) {
			t.Fatal("unwrap did not return the original key")
		}
	})

	t.Run("rejects tampering", func(t *testing.T) {
		wrapped, err := wrapper.Wrap([]byte("0123456789abcdef0123456789abcdef"))
		if err != nil {
			t.Fatal(err)
		}
		wrapped[len(wrapped)-1] ^= 0xff
		if _, err := wrapper.Unwrap(wrapped); err == nil {
			t.Fatal("tampered wrapped key unwrapped cleanly")
		}
	})

	t.Run("rejects another master key", func(t *testing.T) {
		other, err := keys.NewAESWrapper(masterHex(t))
		if err != nil {
			t.Fatal(err)
		}
		wrapped, err := wrapper.Wrap([]byte("0123456789abcdef0123456789abcdef"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := other.Unwrap(wrapped); err == nil {
			t.Fatal("foreign master key unwrapped the data key")
		}
	})

	t.Run("rejects short master keys", func(t *testing.T) {
		if _, err := keys.NewAESWrapper("deadbeef"); err == nil {
			t.Fatal("short master key accepted")
		}
	})
}

func TestServicePersistsPerTenantKeys(t *testing.T) {
	worker, admin := testdb.Pools(t)
	fixtureA := testdb.CreateFixture(t, admin)
	fixtureB := testdb.CreateFixture(t, admin)
	ctx := context.Background()

	wrapper, err := keys.NewAESWrapper(masterHex(t))
	if err != nil {
		t.Fatal(err)
	}
	service := &keys.Service{Pool: worker, Wrapper: wrapper}

	keyA, err := service.DataKey(ctx, fixtureA.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	keyB, err := service.DataKey(ctx, fixtureB.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(keyA, keyB) {
		t.Fatal("tenants share a data key")
	}

	fresh := &keys.Service{Pool: worker, Wrapper: wrapper}
	again, err := fresh.DataKey(ctx, fixtureA.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(keyA, again) {
		t.Fatal("persisted data key did not survive a service restart")
	}
}

func TestRewrapRotatesTheMasterKey(t *testing.T) {
	worker, admin := testdb.Pools(t)
	fixture := testdb.CreateFixture(t, admin)
	ctx := context.Background()

	oldWrapper, err := keys.NewAESWrapper(masterHex(t))
	if err != nil {
		t.Fatal(err)
	}
	newWrapper, err := keys.NewAESWrapper(masterHex(t))
	if err != nil {
		t.Fatal(err)
	}

	service := &keys.Service{Pool: worker, Wrapper: oldWrapper}
	original, err := service.DataKey(ctx, fixture.TenantID)
	if err != nil {
		t.Fatal(err)
	}

	rotated, err := keys.Rewrap(ctx, worker, oldWrapper, newWrapper)
	if err != nil {
		t.Fatal(err)
	}
	if rotated < 1 {
		t.Fatalf("expected at least one key rewrapped, got %d", rotated)
	}

	after := &keys.Service{Pool: worker, Wrapper: newWrapper}
	got, err := after.DataKey(ctx, fixture.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, got) {
		t.Fatal("rewrap changed the data key instead of only the wrapping")
	}

	stale := &keys.Service{Pool: worker, Wrapper: oldWrapper}
	if _, err := stale.DataKey(ctx, fixture.TenantID); err == nil {
		t.Fatal("old master key still unwraps after rotation")
	}
}
