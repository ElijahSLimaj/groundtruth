package store_test

import (
	"context"
	"os"
	"testing"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/attempttechnologies/company-brain/services/ingestion/keys"
	"github.com/attempttechnologies/company-brain/services/ingestion/store"
)

const InteropTenant = "11111111-1111-1111-1111-111111111111"

const InteropContent = `{"body":"the growth plan is 1499 per month billed annually"}`

func TestS3InteropWritesEncryptedPayload(t *testing.T) {
	bucket := os.Getenv("QA_S3_BUCKET")
	endpoint := os.Getenv("QA_S3_ENDPOINT")
	masterKey := os.Getenv("QA_MASTER_KEY")
	databaseURL := os.Getenv("QA_DATABASE_URL")
	if bucket == "" || endpoint == "" || masterKey == "" || databaseURL == "" {
		t.Skip("set QA_S3_BUCKET, QA_S3_ENDPOINT, QA_MASTER_KEY, QA_DATABASE_URL to run the storage interop test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	wrapper, err := keys.NewAESWrapper(masterKey)
	if err != nil {
		t.Fatal(err)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(os.Getenv("QA_S3_REGION")))
	if err != nil {
		t.Fatal(err)
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = &endpoint
		o.UsePathStyle = true
	})

	payloads := &store.EncryptedPayloadStore{
		Blobs: &store.S3BlobStore{Client: client, Bucket: bucket},
		Keys:  &keys.Service{Pool: pool, Wrapper: wrapper},
	}

	ref, err := payloads.Put(ctx, uuid.MustParse(InteropTenant), []byte(InteropContent))
	if err != nil {
		t.Fatalf("put encrypted payload to object storage: %v", err)
	}
	t.Logf("wrote encrypted payload ref=%s", ref)
}
