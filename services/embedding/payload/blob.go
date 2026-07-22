package payload

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type blobGetter interface {
	Get(ctx context.Context, key string) ([]byte, error)
}

type fsBlobGetter struct {
	Root string
}

func (g *fsBlobGetter) Get(_ context.Context, key string) ([]byte, error) {
	content, err := os.ReadFile(filepath.Join(g.Root, filepath.FromSlash(key)))
	if err != nil {
		return nil, fmt.Errorf("blob read %s: %w", key, err)
	}
	return content, nil
}

type s3Getter interface {
	GetObject(ctx context.Context, input *s3.GetObjectInput, opts ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

type s3BlobGetter struct {
	Client s3Getter
	Bucket string
}

func (g *s3BlobGetter) Get(ctx context.Context, key string) ([]byte, error) {
	out, err := g.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &g.Bucket,
		Key:    &key,
	})
	if err != nil {
		return nil, fmt.Errorf("blob get %s: %w", key, err)
	}
	defer out.Body.Close()
	content, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("blob read %s: %w", key, err)
	}
	if len(content) == 0 {
		return nil, errors.New("blob get returned empty body")
	}
	return bytes.Clone(content), nil
}
