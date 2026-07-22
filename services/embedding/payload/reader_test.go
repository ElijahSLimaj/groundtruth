package payload

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type stubKeys struct {
	key []byte
	err error
}

func (s stubKeys) DataKey(_ context.Context, _ uuid.UUID) ([]byte, error) {
	return s.key, s.err
}

type stubBlobs struct {
	data map[string][]byte
	err  error
}

func (s stubBlobs) Get(_ context.Context, key string) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	data, ok := s.data[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return data, nil
}

func seal(t *testing.T, dataKey, aad, plain []byte) []byte {
	t.Helper()
	block, err := aes.NewCipher(dataKey)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, aead.NonceSize())
	for i := range nonce {
		nonce[i] = byte(i)
	}
	return aead.Seal(nonce, nonce, plain, aad)
}

func TestEncryptedReader(t *testing.T) {
	dataKey := []byte("0123456789abcdef0123456789abcdef")
	tenant := "11111111-1111-1111-1111-111111111111"
	digest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	ref := "payloads/" + tenant + "/" + digest
	key := tenant + "/" + digest
	plain := []byte(`{"body":"the canon remembers"}`)

	t.Run("decrypts a payload written by ingestion", func(t *testing.T) {
		reader := &encryptedReader{
			blobs: stubBlobs{data: map[string][]byte{key: seal(t, dataKey, []byte(ref), plain)}},
			keys:  stubKeys{key: dataKey},
		}
		got, err := reader.Get(context.Background(), ref)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(plain) {
			t.Fatalf("decrypted payload mismatch: got %q", got)
		}
	})

	t.Run("rejects a ref bound to a different tenant", func(t *testing.T) {
		otherRef := "payloads/22222222-2222-2222-2222-222222222222/" + digest
		reader := &encryptedReader{
			blobs: stubBlobs{data: map[string][]byte{key: seal(t, dataKey, []byte(otherRef), plain)}},
			keys:  stubKeys{key: dataKey},
		}
		if _, err := reader.Get(context.Background(), ref); err == nil {
			t.Fatal("ref with a mismatched aad decrypted cleanly")
		}
	})

	t.Run("rejects malformed refs", func(t *testing.T) {
		reader := &encryptedReader{blobs: stubBlobs{}, keys: stubKeys{key: dataKey}}
		if _, err := reader.Get(context.Background(), "../etc/passwd"); err == nil {
			t.Fatal("malformed ref accepted")
		}
	})

	t.Run("surfaces a missing tenant key", func(t *testing.T) {
		reader := &encryptedReader{
			blobs: stubBlobs{data: map[string][]byte{key: seal(t, dataKey, []byte(ref), plain)}},
			keys:  stubKeys{err: errors.New("no data key for tenant")},
		}
		if _, err := reader.Get(context.Background(), ref); err == nil {
			t.Fatal("missing tenant key did not surface")
		}
	})
}
