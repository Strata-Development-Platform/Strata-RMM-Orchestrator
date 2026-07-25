package encrypt

import (
	"testing"
)

func TestGenerateEncryptionKey(t *testing.T) {
	key, err := GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("GenerateEncryptionKey: %v", err)
	}
	if len(key) != 64 {
		t.Errorf("key length: got %d, want 64", len(key))
	}
}

func TestNewKeyStore(t *testing.T) {
	ks := NewKeyStore(nil)
	if ks == nil {
		t.Fatal("expected key store")
	}
}

func TestEncryptorEncryptDecrypt(t *testing.T) {
	key := &TenantKey{
		KeyMaterial: []byte("01234567890123456789012345678901"),
		Encryption:  AES256GCM,
	}

	e := NewEncryptor(key)
	plaintext := []byte("sensitive session data")

	ciphertext, err := e.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if ciphertext == "" {
		t.Fatal("expected non-empty ciphertext")
	}

	decrypted, err := e.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Errorf("decrypted mismatch: got %q, want %q", string(decrypted), string(plaintext))
	}
}

func TestEncryptorShortKey(t *testing.T) {
	key := &TenantKey{
		KeyMaterial: []byte("short-key"),
		Encryption:  AES256GCM,
	}

	e := NewEncryptor(key)
	ciphertext, err := e.Encrypt([]byte("test"))
	if err != nil {
		t.Fatalf("Encrypt short key: %v", err)
	}

	decrypted, err := e.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt short key: %v", err)
	}
	if string(decrypted) != "test" {
		t.Errorf("decrypted: got %q, want test", string(decrypted))
	}
}

func TestEncryptorInvalidCiphertext(t *testing.T) {
	key := &TenantKey{
		KeyMaterial: []byte("test-key-32-bytes-for-aes-256-gcm!"),
		Encryption:  AES256GCM,
	}

	e := NewEncryptor(key)
	_, err := e.Decrypt("too-short")
	if err == nil {
		t.Error("expected error for invalid ciphertext")
	}

	_, err = e.Decrypt("aW52YWxpZC1iYXNlNjQ=")
	if err == nil {
		t.Error("expected error for invalid base64 content")
	}
}

func TestEncryptorDifferentKeys(t *testing.T) {
	key1 := &TenantKey{
		KeyMaterial: []byte("key-material-one-32-bytes-long-aes"),
		Encryption:  AES256GCM,
	}
	key2 := &TenantKey{
		KeyMaterial: []byte("key-material-two-different-bytes-ok"),
		Encryption:  AES256GCM,
	}

	e1 := NewEncryptor(key1)
	e2 := NewEncryptor(key2)

	ciphertext, err := e1.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	_, err = e2.Decrypt(ciphertext)
	if err == nil {
		t.Error("expected error decrypting with different key")
	}
}

func TestDeriveKey(t *testing.T) {
	material := []byte("some-material")
	key := deriveKey(material)
	if len(key) != 32 {
		t.Errorf("derived key length: got %d, want 32", len(key))
	}

	// Same material should produce same key
	key2 := deriveKey(material)
	if string(key) != string(key2) {
		t.Error("deriveKey not deterministic")
	}

	// Different material should produce different key
	key3 := deriveKey([]byte("other-material"))
	if string(key) == string(key3) {
		t.Error("deriveKey should produce different output for different input")
	}
}
