package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

type PayloadStore interface {
	Put(ctx context.Context, tenantID uuid.UUID, content []byte) (string, error)
}

type FSPayloadStore struct {
	Root string
}

func (s *FSPayloadStore) Put(_ context.Context, tenantID uuid.UUID, content []byte) (string, error) {
	sum := sha256.Sum256(content)
	digest := hex.EncodeToString(sum[:])
	ref := fmt.Sprintf("payloads/%s/%s", tenantID, digest)
	dir := filepath.Join(s.Root, tenantID.String())
	path := filepath.Join(dir, digest)

	if _, err := os.Stat(path); err == nil {
		return ref, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("payload store mkdir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "put-*")
	if err != nil {
		return "", fmt.Errorf("payload store temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return "", fmt.Errorf("payload store write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("payload store close: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", fmt.Errorf("payload store rename: %w", err)
	}
	return ref, nil
}
