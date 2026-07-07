package payload

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Reader interface {
	Get(ctx context.Context, ref string) ([]byte, error)
}

type FSReader struct {
	Root string
}

var refPattern = regexp.MustCompile(`^payloads/[0-9a-f-]{36}/[0-9a-f]{64}$`)

func (r *FSReader) Get(_ context.Context, ref string) ([]byte, error) {
	if !refPattern.MatchString(ref) {
		return nil, fmt.Errorf("malformed payload ref %q", ref)
	}
	rel := strings.TrimPrefix(ref, "payloads/")
	content, err := os.ReadFile(filepath.Join(r.Root, filepath.FromSlash(rel)))
	if err != nil {
		return nil, fmt.Errorf("read payload %s: %w", ref, err)
	}
	return content, nil
}
