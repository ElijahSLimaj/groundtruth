package payload_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/attempttechnologies/company-brain/services/embedding/payload"
)

const interopTenant = "11111111-1111-1111-1111-111111111111"

const interopContent = `{"body":"the growth plan is 1499 per month billed annually"}`

func TestS3InteropReadsPayloadWrittenByIngestion(t *testing.T) {
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

	reader, err := payload.Build(ctx, payload.Options{
		MasterKeyHex: masterKey,
		S3Bucket:     bucket,
		S3Endpoint:   endpoint,
		S3Region:     os.Getenv("QA_S3_REGION"),
		Pool:         pool,
	})
	if err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256([]byte(interopContent))
	ref := fmt.Sprintf("payloads/%s/%s", interopTenant, hex.EncodeToString(sum[:]))

	got, err := reader.Get(ctx, ref)
	if err != nil {
		t.Fatalf("read and decrypt payload written by ingestion: %v", err)
	}
	if string(got) != interopContent {
		t.Fatalf("decrypted payload mismatch:\n got: %s\nwant: %s", got, interopContent)
	}
	t.Logf("decrypted payload written by ingestion: %s", got)
}
