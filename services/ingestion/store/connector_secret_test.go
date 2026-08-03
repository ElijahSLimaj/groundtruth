package store

import (
	"encoding/hex"
	"testing"

	"github.com/google/uuid"
)

func TestOpenConnectorSecretGoldenVectorFromTS(t *testing.T) {
	t.Parallel()
	dataKey, err := hex.DecodeString(
		"2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a",
	)
	if err != nil {
		t.Fatal(err)
	}
	connectorID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	sealedB64 := "7skI2pAQfKmKSkV5jE+Ys+VzypKwtHuadg0c92MfS6mKWLD2DF7QuxiChVildAmrKckm3WvGJM7Zb8lSTq0zBrK/fL6Go/hpwIBU0/2vyy1BBcxJmqSdS6MD5W1ujFXpMA=="

	plain, err := OpenConnectorSecret(dataKey, connectorID, sealedB64)
	if err != nil {
		t.Fatalf("Go must open the TS-sealed connector secret: %v", err)
	}
	want := `{"access_token":"ya29.demo-access","refresh_token":"1//demo-refresh"}`
	if string(plain) != want {
		t.Fatalf("cross-language envelope mismatch:\n got  %s\n want %s", plain, want)
	}
}

func TestOpenConnectorSecretRejectsWrongConnectorAAD(t *testing.T) {
	t.Parallel()
	dataKey, _ := hex.DecodeString(
		"2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a",
	)
	wrong := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	sealedB64 := "7skI2pAQfKmKSkV5jE+Ys+VzypKwtHuadg0c92MfS6mKWLD2DF7QuxiChVildAmrKckm3WvGJM7Zb8lSTq0zBrK/fL6Go/hpwIBU0/2vyy1BBcxJmqSdS6MD5W1ujFXpMA=="

	if _, err := OpenConnectorSecret(dataKey, wrong, sealedB64); err == nil {
		t.Fatal("a secret sealed for one connector must not open under another connector's AAD")
	}
}
