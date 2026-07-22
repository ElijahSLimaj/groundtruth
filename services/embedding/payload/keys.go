package payload

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const dataKeySize = 32

type dataKeys interface {
	DataKey(ctx context.Context, tenantID uuid.UUID) ([]byte, error)
}

type masterKey struct {
	aead cipher.AEAD
}

func newMasterKey(masterKeyHex string) (*masterKey, error) {
	raw, err := hex.DecodeString(masterKeyHex)
	if err != nil {
		return nil, fmt.Errorf("master key must be hex: %w", err)
	}
	if len(raw) != dataKeySize {
		return nil, fmt.Errorf("master key must be %d bytes, got %d", dataKeySize, len(raw))
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, fmt.Errorf("master key cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("master key gcm: %w", err)
	}
	return &masterKey{aead: aead}, nil
}

func (m *masterKey) unwrap(wrapped []byte) ([]byte, error) {
	if len(wrapped) < m.aead.NonceSize() {
		return nil, errors.New("wrapped key too short")
	}
	nonce, sealed := wrapped[:m.aead.NonceSize()], wrapped[m.aead.NonceSize():]
	plain, err := m.aead.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, fmt.Errorf("unwrap data key: %w", err)
	}
	return plain, nil
}

type tenantDataKeys struct {
	pool   *pgxpool.Pool
	master *masterKey

	mu    sync.Mutex
	cache map[uuid.UUID][]byte
}

func newTenantDataKeys(pool *pgxpool.Pool, master *masterKey) *tenantDataKeys {
	return &tenantDataKeys{pool: pool, master: master, cache: map[uuid.UUID][]byte{}}
}

func (t *tenantDataKeys) DataKey(ctx context.Context, tenantID uuid.UUID) ([]byte, error) {
	t.mu.Lock()
	if key, ok := t.cache[tenantID]; ok {
		t.mu.Unlock()
		return key, nil
	}
	t.mu.Unlock()

	var wrapped []byte
	err := t.pool.QueryRow(ctx,
		`select wrapped_key from tenant_keys where tenant_id = $1`, tenantID,
	).Scan(&wrapped)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("no data key for tenant %s", tenantID)
		}
		return nil, fmt.Errorf("fetch data key: %w", err)
	}

	key, err := t.master.unwrap(wrapped)
	if err != nil {
		return nil, err
	}

	t.mu.Lock()
	t.cache[tenantID] = key
	t.mu.Unlock()
	return key, nil
}

func open(dataKey, aad, sealed []byte) ([]byte, error) {
	block, err := aes.NewCipher(dataKey)
	if err != nil {
		return nil, fmt.Errorf("payload cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("payload gcm: %w", err)
	}
	if len(sealed) < aead.NonceSize() {
		return nil, errors.New("payload too short to be an envelope")
	}
	nonce, ct := sealed[:aead.NonceSize()], sealed[aead.NonceSize():]
	plain, err := aead.Open(nil, nonce, ct, aad)
	if err != nil {
		return nil, fmt.Errorf("payload open: %w", err)
	}
	return plain, nil
}
