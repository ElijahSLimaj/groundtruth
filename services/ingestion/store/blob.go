package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type BlobStore interface {
	Put(ctx context.Context, key string, data []byte) error
	Exists(ctx context.Context, key string) (bool, error)
}

type FSBlobStore struct {
	Root string
}

func (s *FSBlobStore) path(key string) string {
	return filepath.Join(s.Root, filepath.FromSlash(key))
}

func (s *FSBlobStore) Exists(_ context.Context, key string) (bool, error) {
	if _, err := os.Stat(s.path(key)); err == nil {
		return true, nil
	} else if errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else {
		return false, fmt.Errorf("blob stat: %w", err)
	}
}

func (s *FSBlobStore) Put(_ context.Context, key string, data []byte) error {
	path := s.path(key)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("blob mkdir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "put-*")
	if err != nil {
		return fmt.Errorf("blob temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("blob write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("blob close: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("blob rename: %w", err)
	}
	return nil
}

type S3API interface {
	PutObject(ctx context.Context, input *s3.PutObjectInput, opts ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	HeadObject(ctx context.Context, input *s3.HeadObjectInput, opts ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
}

type S3BlobStore struct {
	Client S3API
	Bucket string
}

func (s *S3BlobStore) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &s.Bucket,
		Key:    &key,
	})
	if err == nil {
		return true, nil
	}
	if strings.Contains(err.Error(), "NotFound") ||
		strings.Contains(err.Error(), "StatusCode: 404") {
		return false, nil
	}
	return false, fmt.Errorf("blob head: %w", err)
}

func (s *S3BlobStore) Put(ctx context.Context, key string, data []byte) error {
	_, err := s.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &s.Bucket,
		Key:    &key,
		Body:   bytes.NewReader(data),
	})
	if err != nil {
		return fmt.Errorf("blob put: %w", err)
	}
	return nil
}
