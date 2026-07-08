package keys

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const DataKeySize = 32

type Wrapper interface {
	Wrap(plain []byte) ([]byte, error)
	Unwrap(wrapped []byte) ([]byte, error)
}

type AESWrapper struct {
	aead cipher.AEAD
}

func NewAESWrapper(masterKeyHex string) (*AESWrapper, error) {
	master, err := hex.DecodeString(masterKeyHex)
	if err != nil {
		return nil, fmt.Errorf("master key must be hex: %w", err)
	}
	if len(master) != DataKeySize {
		return nil, fmt.Errorf("master key must be %d bytes, got %d", DataKeySize, len(master))
	}
	block, err := aes.NewCipher(master)
	if err != nil {
		return nil, fmt.Errorf("master key cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("master key gcm: %w", err)
	}
	return &AESWrapper{aead: aead}, nil
}

func (w *AESWrapper) Wrap(plain []byte) ([]byte, error) {
	nonce := make([]byte, w.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("wrap nonce: %w", err)
	}
	return w.aead.Seal(nonce, nonce, plain, nil), nil
}

func (w *AESWrapper) Unwrap(wrapped []byte) ([]byte, error) {
	if len(wrapped) < w.aead.NonceSize() {
		return nil, errors.New("wrapped key too short")
	}
	nonce, sealed := wrapped[:w.aead.NonceSize()], wrapped[w.aead.NonceSize():]
	plain, err := w.aead.Open(nil, nonce, sealed, nil)
	if err != nil {
		return nil, fmt.Errorf("unwrap data key: %w", err)
	}
	return plain, nil
}

type Service struct {
	Pool    *pgxpool.Pool
	Wrapper Wrapper

	mu    sync.Mutex
	cache map[uuid.UUID][]byte
}

func (s *Service) DataKey(ctx context.Context, tenantID uuid.UUID) ([]byte, error) {
	s.mu.Lock()
	if key, ok := s.cache[tenantID]; ok {
		s.mu.Unlock()
		return key, nil
	}
	s.mu.Unlock()

	key, err := s.loadOrCreate(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	if s.cache == nil {
		s.cache = map[uuid.UUID][]byte{}
	}
	s.cache[tenantID] = key
	s.mu.Unlock()
	return key, nil
}

func (s *Service) loadOrCreate(ctx context.Context, tenantID uuid.UUID) ([]byte, error) {
	wrapped, err := s.fetchWrapped(ctx, tenantID)
	if err == nil {
		return s.Wrapper.Unwrap(wrapped)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	fresh := make([]byte, DataKeySize)
	if _, err := rand.Read(fresh); err != nil {
		return nil, fmt.Errorf("generate data key: %w", err)
	}
	sealed, err := s.Wrapper.Wrap(fresh)
	if err != nil {
		return nil, err
	}
	if _, err := s.Pool.Exec(ctx, `
		insert into tenant_keys (tenant_id, wrapped_key)
		values ($1, $2)
		on conflict (tenant_id) do nothing
	`, tenantID, sealed); err != nil {
		return nil, fmt.Errorf("persist data key: %w", err)
	}

	wrapped, err = s.fetchWrapped(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return s.Wrapper.Unwrap(wrapped)
}

func (s *Service) fetchWrapped(ctx context.Context, tenantID uuid.UUID) ([]byte, error) {
	var wrapped []byte
	err := s.Pool.QueryRow(ctx,
		`select wrapped_key from tenant_keys where tenant_id = $1`, tenantID,
	).Scan(&wrapped)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, fmt.Errorf("fetch data key: %w", err)
	}
	return wrapped, nil
}

func Rewrap(ctx context.Context, pool *pgxpool.Pool, old, next Wrapper) (int, error) {
	rows, err := pool.Query(ctx, `select tenant_id, wrapped_key, key_version from tenant_keys`)
	if err != nil {
		return 0, fmt.Errorf("list tenant keys: %w", err)
	}
	type entry struct {
		tenant  uuid.UUID
		wrapped []byte
		version int
	}
	var entries []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.tenant, &e.wrapped, &e.version); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan tenant key: %w", err)
		}
		entries = append(entries, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	count := 0
	for _, e := range entries {
		plain, err := old.Unwrap(e.wrapped)
		if err != nil {
			return count, fmt.Errorf("rewrap tenant %s: %w", e.tenant, err)
		}
		sealed, err := next.Wrap(plain)
		if err != nil {
			return count, err
		}
		tag, err := pool.Exec(ctx, `
			update tenant_keys
			set wrapped_key = $2, key_version = key_version + 1, rewrapped_at = now()
			where tenant_id = $1 and key_version = $3
		`, e.tenant, sealed, e.version)
		if err != nil {
			return count, fmt.Errorf("store rewrapped key for %s: %w", e.tenant, err)
		}
		if tag.RowsAffected() == 1 {
			count++
		}
	}
	return count, nil
}
