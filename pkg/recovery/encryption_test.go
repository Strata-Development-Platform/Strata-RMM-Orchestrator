package recovery

import (
	"bytes"
	"crypto/rand"
	"strings"
	"testing"
)

// TestEncryptDecryptRoundTrip verifies basic encryption/decryption works.
func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate key: %v", err)
	}

	plaintext := []byte("hello, world - this is secret data for testing")
	associatedData := []byte("backup-set-id=abc-123\ncomponent-id=db\nenvironment=production\n")

	enc, err := Encrypt(key, plaintext, associatedData)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	if enc.Version != EnvelopeVersion {
		t.Errorf("version = %d, want %d", enc.Version, EnvelopeVersion)
	}
	if enc.OriginalSize != len(plaintext) {
		t.Errorf("original_size = %d, want %d", enc.OriginalSize, len(plaintext))
	}
	if enc.EncryptedSize == 0 {
		t.Error("encrypted_size must be non-zero")
	}

	decrypted, err := Decrypt(key, enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

// TestUniqueNonce ensures every encryption call produces a unique nonce.
func TestUniqueNonce(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	plaintext := []byte("same plaintext")
	associatedData := []byte("test")

	nonces := make(map[string]bool)
	for i := 0; i < 100; i++ {
		enc, err := Encrypt(key, plaintext, associatedData)
		if err != nil {
			t.Fatalf("encrypt iteration %d: %v", i, err)
		}
		nonceStr := string(enc.Nonce)
		if nonces[nonceStr] {
			t.Errorf("duplicate nonce on iteration %d", i)
		}
		nonces[nonceStr] = true
	}
}

// TestCiphertextModification proves tampering is detected.
func TestCiphertextModification(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	plaintext := []byte("secret data")
	associatedData := []byte("auth-data")

	enc, err := Encrypt(key, plaintext, associatedData)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Flip a byte in the ciphertext
	enc.Ciphertext[0] ^= 0x01

	_, err = Decrypt(key, enc)
	if err == nil {
		t.Fatal("expected decryption failure on ciphertext modification, got nil")
	}
	if !strings.Contains(err.Error(), "decrypt") {
		t.Errorf("error should mention 'decrypt', got: %v", err)
	}
}

// TestNonceModification proves nonce tampering is detected.
func TestNonceModification(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	plaintext := []byte("secret data")
	associatedData := []byte("auth-data")

	enc, err := Encrypt(key, plaintext, associatedData)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Flip a byte in the nonce
	enc.Nonce[0] ^= 0x01

	_, err = Decrypt(key, enc)
	if err == nil {
		t.Fatal("expected decryption failure on nonce modification, got nil")
	}
}

// TestMetadataModification proves metadata tampering is detected.
func TestMetadataModification(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	plaintext := []byte("secret data")
	associatedData := []byte("backup-set-id=abc-123\ncomponent-id=db\n")

	enc, err := Encrypt(key, plaintext, associatedData)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Modify the stored AD hash (simulating metadata tampering during transport)
	enc.ADHash[0] ^= 0x01

	_, err = Decrypt(key, enc)
	if err == nil {
		t.Fatal("expected decryption failure on metadata tampering, got nil")
	}
	if !strings.Contains(err.Error(), "decrypt") {
		t.Errorf("error should mention 'decrypt', got: %v", err)
	}
}

// TestMetadataModificationViaReMarshal proves that modifying AssociatedData
// and re-serializing the envelope is detected.
func TestMetadataModificationViaReMarshal(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	plaintext := []byte("secret data")
	associatedData := []byte("backup-set-id=abc-123\ncomponent-id=db\n")

	enc, err := Encrypt(key, plaintext, associatedData)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Marshal, modify associated data, re-marshal, then decrypt
	_, err = enc.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Modify the associated data in the struct
	enc.AssociatedData = []byte("backup-set-id=TAMPERED\ncomponent-id=db\n")

	// Re-marshal (this updates the AD hash in the serialized form)
	data2, err := enc.Marshal()
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}

	// Unmarshal with tampered metadata
	decoded, err := Unmarshal(data2)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// This should fail because the ciphertext was sealed with the original AD hash
	_, err = Decrypt(key, decoded)
	if err == nil {
		t.Fatal("expected decryption failure on metadata re-marshaling, got nil")
	}
}

// TestWrongKey proves a different key cannot decrypt.
func TestWrongKey(t *testing.T) {
	key1 := make([]byte, 32)
	rand.Read(key1)
	key2 := make([]byte, 32)
	rand.Read(key2)

	plaintext := []byte("secret data")
	associatedData := []byte("auth-data")

	enc, err := Encrypt(key1, plaintext, associatedData)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	_, err = Decrypt(key2, enc)
	if err == nil {
		t.Fatal("expected decryption failure with wrong key, got nil")
	}
}

// TestTruncation proves truncated ciphertext is rejected.
func TestTruncation(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	plaintext := []byte("secret data that is long enough")
	associatedData := []byte("auth-data")

	enc, err := Encrypt(key, plaintext, associatedData)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Truncate the ciphertext
	enc.Ciphertext = enc.Ciphertext[:len(enc.Ciphertext)/2]

	_, err = Decrypt(key, enc)
	if err == nil {
		t.Fatal("expected decryption failure on truncation, got nil")
	}
}

// TestEmptyArtifact proves empty plaintext is handled correctly.
func TestEmptyArtifact(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	plaintext := []byte{}
	associatedData := []byte("auth-data")

	enc, err := Encrypt(key, plaintext, associatedData)
	if err != nil {
		t.Fatalf("encrypt empty: %v", err)
	}

	decrypted, err := Decrypt(key, enc)
	if err != nil {
		t.Fatalf("decrypt empty: %v", err)
	}

	if len(decrypted) != 0 {
		t.Errorf("decrypted = %d bytes, want 0", len(decrypted))
	}
}

// TestUnsupportedEnvelopeVersion proves old versions are rejected.
func TestUnsupportedEnvelopeVersion(t *testing.T) {
	enc := &Envelope{
		Version:        99,
		Nonce:          make([]byte, 12),
		AssociatedData: []byte("data"),
		Ciphertext:     []byte("ct"),
	}

	_, err := Decrypt(make([]byte, 32), enc)
	if err == nil {
		t.Fatal("expected error for unsupported version, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported envelope version") {
		t.Errorf("error should mention version, got: %v", err)
	}
}

// TestKeySizeValidation proves wrong key sizes are rejected.
func TestKeySizeValidation(t *testing.T) {
	plaintext := []byte("secret")
	associatedData := []byte("auth")

	_, err := Encrypt([]byte("short"), plaintext, associatedData)
	if err == nil {
		t.Fatal("expected error for short key, got nil")
	}
	if !strings.Contains(err.Error(), "32 bytes") {
		t.Errorf("error should mention 32 bytes, got: %v", err)
	}
}

// TestMarshalUnmarshalRoundTrip verifies envelope serialization.
func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	plaintext := []byte("serializable data")
	associatedData := []byte("auth-metadata")

	enc, err := Encrypt(key, plaintext, associatedData)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	data, err := enc.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	unmarshaled, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if unmarshaled.Version != enc.Version {
		t.Errorf("version = %d, want %d", unmarshaled.Version, enc.Version)
	}
	if len(unmarshaled.Nonce) != len(enc.Nonce) {
		t.Errorf("nonce length = %d, want %d", len(unmarshaled.Nonce), len(enc.Nonce))
	}
	if !bytes.Equal(unmarshaled.Ciphertext, enc.Ciphertext) {
		t.Error("ciphertext mismatch after unmarshal")
	}

	// Decrypt with the unmarshaled envelope
	decrypted, err := Decrypt(key, unmarshaled)
	if err != nil {
		t.Fatalf("decrypt unmarshaled: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

// TestEncodeDecodeBase64 verifies base64 encoding roundtrip.
func TestEncodeDecodeBase64(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	plaintext := []byte("b64 test")
	associatedData := []byte("auth")

	enc, err := Encrypt(key, plaintext, associatedData)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	encoded, err := EncodeEnvelope(enc)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := DecodeEnvelope(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	decrypted, err := Decrypt(key, decoded)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

// TestDeterministicNonce proves test nonces are unique and reproducible.
func TestDeterministicNonce(t *testing.T) {
	ResetTestNonce()

	key := make([]byte, 32)
	rand.Read(key)

	plaintext := []byte("test")
	associatedData := []byte("auth")

	nonce1 := TestNonce()
	nonce2 := TestNonce()

	if bytes.Equal(nonce1, nonce2) {
		t.Error("test nonces should be unique")
	}

	enc1, err := EncryptWithNonce(key, plaintext, associatedData, nonce1)
	if err != nil {
		t.Fatalf("encrypt with nonce1: %v", err)
	}

	enc2, err := EncryptWithNonce(key, plaintext, associatedData, nonce2)
	if err != nil {
		t.Fatalf("encrypt with nonce2: %v", err)
	}

	// Different nonces should produce different ciphertext
	if bytes.Equal(enc1.Ciphertext, enc2.Ciphertext) {
		t.Error("different nonces should produce different ciphertext")
	}
}

// TestMetadataAuth verifies metadata authentication builder.
func TestMetadataAuth(t *testing.T) {
	m := NewMetadataAuth()
	m.AppendString("backup-set-id", "abc-123")
	m.AppendString("component-id", "db-backup")
	m.AppendString("environment", "production")

	data := m.Bytes()
	if len(data) == 0 {
		t.Error("metadata auth bytes should not be empty")
	}
	if !bytes.Contains(data, []byte("backup-set-id=abc-123")) {
		t.Error("metadata should contain backup-set-id")
	}
}

// TestHexEncodeDecodeKey verifies hex key conversion.
func TestHexEncodeDecodeKey(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	encoded := HexEncodeKey(key)
	decoded, err := HexDecodeKey(encoded)
	if err != nil {
		t.Fatalf("hex decode: %v", err)
	}
	if !bytes.Equal(decoded, key) {
		t.Error("hex decode should match original key")
	}
}

// TestCiphertextModificationWithMetadata proves component substitution is detected.
func TestCiphertextModificationWithMetadata(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)

	plaintext := []byte("database backup data")
	associatedData := []byte("backup-set-id=abc\ncomponent-id=db\n")

	enc, err := Encrypt(key, plaintext, associatedData)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Tamper with the AD hash (simulating component ID substitution)
	enc.ADHash[5] ^= 0xFF

	_, err = Decrypt(key, enc)
	if err == nil {
		t.Fatal("expected failure on component substitution via AD hash tampering, got nil")
	}
}
