package payload

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Reader interface {
	Get(ctx context.Context, ref string) ([]byte, error)
}

type FSReader struct {
	Root string
}

var refPattern = regexp.MustCompile(`^payloads/([0-9a-f-]{36})/[0-9a-f]{64}$`)

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

type encryptedReader struct {
	blobs blobGetter
	keys  dataKeys
}

func (r *encryptedReader) Get(ctx context.Context, ref string) ([]byte, error) {
	match := refPattern.FindStringSubmatch(ref)
	if match == nil {
		return nil, fmt.Errorf("malformed payload ref %q", ref)
	}
	tenantID, err := uuid.Parse(match[1])
	if err != nil {
		return nil, fmt.Errorf("payload ref tenant %q: %w", match[1], err)
	}
	dataKey, err := r.keys.DataKey(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	key := strings.TrimPrefix(ref, "payloads/")
	sealed, err := r.blobs.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	plain, err := open(dataKey, []byte(ref), sealed)
	if err != nil {
		return nil, fmt.Errorf("decrypt payload %s: %w", ref, err)
	}
	return plain, nil
}

type Options struct {
	Root         string
	MasterKeyHex string
	S3Bucket     string
	S3Endpoint   string
	S3Region     string
	Pool         *pgxpool.Pool
}

func Build(ctx context.Context, opts Options) (Reader, error) {
	if opts.MasterKeyHex == "" {
		if opts.S3Bucket != "" {
			return nil, fmt.Errorf("MASTER_KEY is required to read encrypted payloads from object storage")
		}
		if opts.Root == "" {
			return nil, fmt.Errorf("PAYLOAD_ROOT or S3_BUCKET is required")
		}
		return &FSReader{Root: opts.Root}, nil
	}

	master, err := newMasterKey(opts.MasterKeyHex)
	if err != nil {
		return nil, err
	}
	if opts.Pool == nil {
		return nil, fmt.Errorf("a database pool is required to resolve tenant data keys")
	}
	keys := newTenantDataKeys(opts.Pool, master)

	var blobs blobGetter
	if opts.S3Bucket != "" {
		awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(opts.S3Region))
		if err != nil {
			return nil, fmt.Errorf("aws config: %w", err)
		}
		client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
			if opts.S3Endpoint != "" {
				o.BaseEndpoint = &opts.S3Endpoint
				o.UsePathStyle = true
			}
		})
		blobs = &s3BlobGetter{Client: client, Bucket: opts.S3Bucket}
	} else {
		if opts.Root == "" {
			return nil, fmt.Errorf("PAYLOAD_ROOT or S3_BUCKET is required")
		}
		blobs = &fsBlobGetter{Root: opts.Root}
	}
	return &encryptedReader{blobs: blobs, keys: keys}, nil
}
