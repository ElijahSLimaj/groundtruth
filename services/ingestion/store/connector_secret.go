package store

import (
	"encoding/base64"
	"fmt"

	"github.com/google/uuid"
)

func ConnectorSecretAAD(connectorID uuid.UUID) []byte {
	return []byte("connector:" + connectorID.String())
}

func SealConnectorSecret(dataKey []byte, connectorID uuid.UUID, plain []byte) (string, error) {
	sealed, err := seal(dataKey, ConnectorSecretAAD(connectorID), plain)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func OpenConnectorSecret(dataKey []byte, connectorID uuid.UUID, sealedB64 string) ([]byte, error) {
	sealed, err := base64.StdEncoding.DecodeString(sealedB64)
	if err != nil {
		return nil, fmt.Errorf("connector secret base64: %w", err)
	}
	return Open(dataKey, ConnectorSecretAAD(connectorID), sealed)
}
