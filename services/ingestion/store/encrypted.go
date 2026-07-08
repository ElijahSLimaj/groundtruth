package store

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"

	"github.com/attempttechnologies/company-brain/services/ingestion/keys"
)

type EncryptedPayloadStore struct {
	Blobs BlobStore
	Keys  *keys.Service
}

func (s *EncryptedPayloadStore) Put(ctx context.Context, tenantID uuid.UUID, content []byte) (string, error) {
	sum := sha256.Sum256(content)
	digest := hex.EncodeToString(sum[:])
	ref := fmt.Sprintf("payloads/%s/%s", tenantID, digest)
	key := fmt.Sprintf("%s/%s", tenantID, digest)

	exists, err := s.Blobs.Exists(ctx, key)
	if err != nil {
		return "", err
	}
	if exists {
		return ref, nil
	}

	dataKey, err := s.Keys.DataKey(ctx, tenantID)
	if err != nil {
		return "", err
	}
	sealed, err := seal(dataKey, []byte(ref), content)
	if err != nil {
		return "", err
	}
	if err := s.Blobs.Put(ctx, key, sealed); err != nil {
		return "", err
	}
	return ref, nil
}

func seal(dataKey, aad, plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(dataKey)
	if err != nil {
		return nil, fmt.Errorf("payload cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("payload gcm: %w", err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("payload nonce: %w", err)
	}
	return aead.Seal(nonce, nonce, plain, aad), nil
}

func Open(dataKey, aad, sealed []byte) ([]byte, error) {
	block, err := aes.NewCipher(dataKey)
	if err != nil {
		return nil, fmt.Errorf("payload cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("payload gcm: %w", err)
	}
	if len(sealed) < aead.NonceSize() {
		return nil, fmt.Errorf("payload too short to be an envelope")
	}
	nonce, ct := sealed[:aead.NonceSize()], sealed[aead.NonceSize():]
	plain, err := aead.Open(nil, nonce, ct, aad)
	if err != nil {
		return nil, fmt.Errorf("payload open: %w", err)
	}
	return plain, nil
}
