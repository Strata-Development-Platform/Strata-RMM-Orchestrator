package modules

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type publisherTrustConfigEntry struct {
	Publisher string `json:"publisher"`
	KeyID     string `json:"key_id"`
	PublicKey string `json:"public_key"`
}

// ParsePublisherTrustStoreJSON parses a strict JSON array of trusted Ed25519
// publisher keys. Public keys are standard base64. Duplicate publisher/key IDs,
// unknown fields, malformed keys, and empty trust stores are rejected.
func ParsePublisherTrustStoreJSON(data []byte) (StaticPublisherTrustStore, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("publisher trust configuration is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var entries []publisherTrustConfigEntry
	if err := decoder.Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode publisher trust configuration: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("publisher trust configuration must contain exactly one JSON value")
		}
		return nil, fmt.Errorf("decode trailing publisher trust configuration: %w", err)
	}
	if len(entries) == 0 {
		return nil, errors.New("publisher trust configuration contains no keys")
	}
	store := make(StaticPublisherTrustStore, len(entries))
	for i, entry := range entries {
		publisher := strings.TrimSpace(entry.Publisher)
		keyID := strings.TrimSpace(entry.KeyID)
		encoded := strings.TrimSpace(entry.PublicKey)
		if publisher == "" || keyID == "" || encoded == "" {
			return nil, fmt.Errorf("publisher trust entry %d requires publisher, key_id, and public_key", i)
		}
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("publisher trust entry %d has invalid Ed25519 public_key", i)
		}
		mapKey := publisher + "\x00" + keyID
		if _, exists := store[mapKey]; exists {
			return nil, fmt.Errorf("duplicate publisher trust key %q/%q", publisher, keyID)
		}
		store[mapKey] = PublisherKey{
			Publisher: publisher,
			KeyID:     keyID,
			PublicKey: append(ed25519.PublicKey(nil), raw...),
		}
	}
	return store, nil
}
