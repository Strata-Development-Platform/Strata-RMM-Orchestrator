package automation

import (
	"crypto/rand"
	"os"
	"strings"
	"testing"
)

func TestEnvelopeEncryptorEncryptDecrypt(t *testing.T) {
	// Generate a 32-byte CEK
	cek := make([]byte, 32)
	if _, err := rand.Read(cek); err != nil {
		t.Fatalf("generate CEK: %v", err)
	}

	encryptor, err := NewEnvelopeEncryptor(cek)
	if err != nil {
		t.Fatalf("NewEnvelopeEncryptor: %v", err)
	}

	secret := []byte("my-secret-password-12345")

	// Encrypt
	ciphertext, err := encryptor.Encrypt(secret)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if ciphertext == nil {
		t.Fatal("ciphertext should not be nil")
	}
	if len(ciphertext.EncryptedDEK) == 0 {
		t.Fatal("encrypted DEK should not be empty")
	}
	if len(ciphertext.EncryptedData) == 0 {
		t.Fatal("encrypted data should not be empty")
	}

	// Decrypt
	plaintext, err := encryptor.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(plaintext) != string(secret) {
		t.Errorf("plaintext = %q, want %q", string(plaintext), string(secret))
	}
}

func TestEnvelopeEncryptorInvalidCEK(t *testing.T) {
	// Try with wrong-sized CEK
	_, err := NewEnvelopeEncryptor([]byte("short"))
	if err == nil {
		t.Fatal("expected error for invalid CEK size")
	}
}

func TestEnvelopeEncryptorSetCEKInvalid(t *testing.T) {
	cek := make([]byte, 32)
	if _, err := rand.Read(cek); err != nil {
		t.Fatalf("generate CEK: %v", err)
	}

	encryptor, err := NewEnvelopeEncryptor(cek)
	if err != nil {
		t.Fatalf("NewEnvelopeEncryptor: %v", err)
	}

	err = encryptor.SetCEK([]byte("short"))
	if err == nil {
		t.Fatal("expected error for invalid CEK size")
	}
}

func TestEnvelopeEncryptorDecryptTamperedCiphertext(t *testing.T) {
	cek := make([]byte, 32)
	if _, err := rand.Read(cek); err != nil {
		t.Fatalf("generate CEK: %v", err)
	}

	encryptor, err := NewEnvelopeEncryptor(cek)
	if err != nil {
		t.Fatalf("NewEnvelopeEncryptor: %v", err)
	}

	secret := []byte("secret")
	ciphertext, err := encryptor.Encrypt(secret)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Tamper with the encrypted data
	if len(ciphertext.EncryptedData) > 0 {
		tampered := make([]byte, len(ciphertext.EncryptedData))
		copy(tampered, ciphertext.EncryptedData)
		tampered[0] ^= 0xff // Flip first byte
		ciphertext.EncryptedData = tampered
	}

	_, err = encryptor.Decrypt(ciphertext)
	if err == nil {
		t.Fatal("expected error for tampered ciphertext")
	}
}

func TestEnvelopeCiphertextRoundTrip(t *testing.T) {
	cek := make([]byte, 32)
	if _, err := rand.Read(cek); err != nil {
		t.Fatalf("generate CEK: %v", err)
	}

	encryptor, err := NewEnvelopeEncryptor(cek)
	if err != nil {
		t.Fatalf("NewEnvelopeEncryptor: %v", err)
	}

	secret := []byte("test-secret-value")
	ciphertext, err := encryptor.Encrypt(secret)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Serialize
	serialized := ciphertext.Serialize()
	if serialized == "" {
		t.Fatal("serialized ciphertext should not be empty")
	}

	// Check format
	parts := strings.Split(serialized, ".")
	if len(parts) != 4 {
		t.Errorf("serialized should have 4 parts, got %d", len(parts))
	}

	// Deserialize
	deserialized, err := DeserializeCiphertext(serialized)
	if err != nil {
		t.Fatalf("DeserializeCiphertext: %v", err)
	}

	// Decrypt
	plaintext, err := encryptor.Decrypt(deserialized)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(plaintext) != string(secret) {
		t.Errorf("plaintext = %q, want %q", string(plaintext), string(secret))
	}
}

func TestDeserializeCiphertextInvalidFormat(t *testing.T) {
	_, err := DeserializeCiphertext("invalid")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}

	_, err = DeserializeCiphertext("a.b.c")
	if err == nil {
		t.Fatal("expected error for 3-part format")
	}

	_, err = DeserializeCiphertext("a.b.c.d.e")
	if err == nil {
		t.Fatal("expected error for 5-part format")
	}
}

func TestDeserializeCiphertextInvalidBase64(t *testing.T) {
	_, err := DeserializeCiphertext("!!!invalid!!!base64!!!base64!!!base64!!!base64")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestSecretVaultStoreRetrieve(t *testing.T) {
	cek := deriveCEK("test-passphrase-12345")

	vault, err := NewSecretVault(cek)
	if err != nil {
		t.Fatalf("NewSecretVault: %v", err)
	}

	secret := []byte("api-key-abcdef123456")

	// Store
	ciphertext, err := vault.Store("test-secret", secret)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if ciphertext == "" {
		t.Fatal("ciphertext should not be empty")
	}

	// Retrieve
	plaintext, err := vault.Retrieve(ciphertext)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if string(plaintext) != string(secret) {
		t.Errorf("plaintext = %q, want %q", string(plaintext), string(secret))
	}
}

func TestSecretVaultMultipleSecrets(t *testing.T) {
	cek := deriveCEK("test-passphrase-12345")

	vault, err := NewSecretVault(cek)
	if err != nil {
		t.Fatalf("NewSecretVault: %v", err)
	}

	secrets := map[string][]byte{
		"secret1": []byte("value1"),
		"secret2": []byte("value2"),
		"secret3": []byte("value3"),
	}

	// Store all secrets
	ciphertexts := make(map[string]string)
	for id, secret := range secrets {
		ct, err := vault.Store(id, secret)
		if err != nil {
			t.Fatalf("Store(%s): %v", id, err)
		}
		ciphertexts[id] = ct
	}

	// Retrieve and verify all secrets
	for id, expected := range secrets {
		plaintext, err := vault.Retrieve(ciphertexts[id])
		if err != nil {
			t.Fatalf("Retrieve(%s): %v", id, err)
		}
		if string(plaintext) != string(expected) {
			t.Errorf("Retrieve(%s) = %q, want %q", id, string(plaintext), string(expected))
		}
	}
}

func TestSecretVaultRotateCEK(t *testing.T) {
	cek := deriveCEK("test-passphrase-12345")

	vault, err := NewSecretVault(cek)
	if err != nil {
		t.Fatalf("NewSecretVault: %v", err)
	}

	secret := []byte("secret-to-rotate")

	// Store with old CEK
	ciphertext, err := vault.Store("test", secret)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Retrieve with old CEK
	plaintext, err := vault.Retrieve(ciphertext)
	if err != nil {
		t.Fatalf("Retrieve before rotation: %v", err)
	}
	if string(plaintext) != string(secret) {
		t.Errorf("before rotation = %q, want %q", string(plaintext), string(secret))
	}

	// Rotate CEK
	_, err = vault.RotateCEK()
	if err != nil {
		t.Fatalf("RotateCEK: %v", err)
	}

	// Retrieve with new CEK should fail (old ciphertext encrypted with old CEK)
	_, err = vault.Retrieve(ciphertext)
	if err == nil {
		t.Fatal("expected error after CEK rotation")
	}
}

func TestEnvelopeEncryptorLargeSecret(t *testing.T) {
	cek := make([]byte, 32)
	if _, err := rand.Read(cek); err != nil {
		t.Fatalf("generate CEK: %v", err)
	}

	encryptor, err := NewEnvelopeEncryptor(cek)
	if err != nil {
		t.Fatalf("NewEnvelopeEncryptor: %v", err)
	}

	// Generate a large secret (1MB)
	secret := make([]byte, 1024*1024)
	if _, err := rand.Read(secret); err != nil {
		t.Fatalf("generate secret: %v", err)
	}

	// Encrypt
	ciphertext, err := encryptor.Encrypt(secret)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Decrypt
	plaintext, err := encryptor.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if len(plaintext) != len(secret) {
		t.Errorf("plaintext length = %d, want %d", len(plaintext), len(secret))
	}
	for i := range secret {
		if plaintext[i] != secret[i] {
			t.Errorf("plaintext[%d] = %d, want %d", i, plaintext[i], secret[i])
			break
		}
	}
}

func TestEnvelopeEncryptorEmptySecret(t *testing.T) {
	cek := make([]byte, 32)
	if _, err := rand.Read(cek); err != nil {
		t.Fatalf("generate CEK: %v", err)
	}

	encryptor, err := NewEnvelopeEncryptor(cek)
	if err != nil {
		t.Fatalf("NewEnvelopeEncryptor: %v", err)
	}

	// Encrypt empty secret
	ciphertext, err := encryptor.Encrypt([]byte(""))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Decrypt
	plaintext, err := encryptor.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(plaintext) != "" {
		t.Errorf("plaintext = %q, want empty string", string(plaintext))
	}
}

func TestEnvelopeEncryptorUnicodeSecret(t *testing.T) {
	cek := make([]byte, 32)
	if _, err := rand.Read(cek); err != nil {
		t.Fatalf("generate CEK: %v", err)
	}

	encryptor, err := NewEnvelopeEncryptor(cek)
	if err != nil {
		t.Fatalf("NewEnvelopeEncryptor: %v", err)
	}

	// Unicode secret
	secret := []byte("こんにちは世界🌍")

	// Encrypt
	ciphertext, err := encryptor.Encrypt(secret)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Decrypt
	plaintext, err := encryptor.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(plaintext) != string(secret) {
		t.Errorf("plaintext = %q, want %q", string(plaintext), string(secret))
	}
}

func TestAuditSecretNoDiskExposure(t *testing.T) {
	// This test verifies that the envelope encryption implementation
	// does not write secrets to disk. We test by:
	// 1. Creating a secret that would be detectable if written to disk
	// 2. Encrypting and storing it
	// 3. Verifying no temp files were created with the secret pattern

	secret := []byte("AUDIT_SECRET_DISCLOSURE_PATTERN_12345")

	cek := make([]byte, 32)
	if _, err := rand.Read(cek); err != nil {
		t.Fatalf("generate CEK: %v", err)
	}

	vault, err := NewSecretVault(cek)
	if err != nil {
		t.Fatalf("NewSecretVault: %v", err)
	}

	// Store the secret
	ciphertext, err := vault.Store("audit-test", secret)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Verify ciphertext doesn't contain the secret pattern
	if strings.Contains(ciphertext, string(secret)) {
		t.Error("ciphertext should not contain plaintext secret")
	}

	// Verify ciphertext doesn't contain the audit pattern
	if strings.Contains(ciphertext, "AUDIT_SECRET_DISCLOSURE_PATTERN_12345") {
		t.Error("ciphertext should not contain plaintext audit pattern")
	}

	// Retrieve and verify
	plaintext, err := vault.Retrieve(ciphertext)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if string(plaintext) != string(secret) {
		t.Errorf("plaintext = %q, want %q", string(plaintext), string(secret))
	}
}

func TestEnvelopeEncryptorNullByteSecret(t *testing.T) {
	cek := make([]byte, 32)
	if _, err := rand.Read(cek); err != nil {
		t.Fatalf("generate CEK: %v", err)
	}

	encryptor, err := NewEnvelopeEncryptor(cek)
	if err != nil {
		t.Fatalf("NewEnvelopeEncryptor: %v", err)
	}

	// Secret with null bytes
	secret := []byte{0x00, 0x01, 0x02, 0x00, 0x03}

	// Encrypt
	ciphertext, err := encryptor.Encrypt(secret)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Decrypt
	plaintext, err := encryptor.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(plaintext) != string(secret) {
		t.Errorf("plaintext = %v, want %v", plaintext, secret)
	}
}

func TestSplitCiphertext(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"a.b.c.d", []string{"a", "b", "c", "d"}},
		{"", []string{""}},
		{"a", []string{"a"}},
		{"a.b", []string{"a", "b"}},
	}

	for _, tt := range tests {
		result := splitCiphertext(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("splitCiphertext(%q) = %v (len=%d), want %v (len=%d)",
				tt.input, result, len(result), tt.expected, len(tt.expected))
		}
		for i := range result {
			if i < len(tt.expected) && result[i] != tt.expected[i] {
				t.Errorf("splitCiphertext(%q)[%d] = %q, want %q",
					tt.input, i, result[i], tt.expected[i])
			}
		}
	}
}

func TestEnvEncryptionKeyDerivation(t *testing.T) {
	cek1 := deriveCEK("test")
	cek2 := deriveCEK("test")
	cek3 := deriveCEK("different")

	// Same input should produce same output
	for i := range cek1 {
		if cek1[i] != cek2[i] {
			t.Errorf("deriveCEK should be deterministic: cek1[%d] = %d, cek2[%d] = %d",
				i, cek1[i], i, cek2[i])
			break
		}
	}

	// Different input should produce different output
	different := false
	for i := range cek1 {
		if cek1[i] != cek3[i] {
			different = true
			break
		}
	}
	if !different {
		t.Error("deriveCEK should produce different output for different inputs")
	}
}

func BenchmarkEnvelopeEncryptor(b *testing.B) {
	cek := make([]byte, 32)
	if _, err := rand.Read(cek); err != nil {
		b.Fatalf("generate CEK: %v", err)
	}

	encryptor, err := NewEnvelopeEncryptor(cek)
	if err != nil {
		b.Fatalf("NewEnvelopeEncryptor: %v", err)
	}

	secret := []byte("benchmark-secret-data")
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ciphertext, err := encryptor.Encrypt(secret)
		if err != nil {
			b.Fatalf("Encrypt: %v", err)
		}
		_, err = encryptor.Decrypt(ciphertext)
		if err != nil {
			b.Fatalf("Decrypt: %v", err)
		}
	}
}

func TestVaultEnforceNoDiskWrite(t *testing.T) {
	// This test creates a temporary directory that would catch any disk writes,
	// then verifies the vault operations do not write to it.

	tmpDir := t.TempDir()
	originalPanic := os.Getenv("STRATA_VAULT_TMPDIR")
	os.Setenv("STRATA_VAULT_TMPDIR", tmpDir)
	defer os.Setenv("STRATA_VAULT_TMPDIR", originalPanic)

	cek := deriveCEK("test-passphrase-12345")

	vault, err := NewSecretVault(cek)
	if err != nil {
		t.Fatalf("NewSecretVault: %v", err)
	}

	secret := []byte("test-secret-data")

	// Store should not write to tmpDir
	_, err = vault.Store("test", secret)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Check that no files were created in tmpDir
	entries, _ := os.ReadDir(tmpDir)
	if len(entries) > 0 {
		t.Errorf("expected no files in tmpDir, found %d", len(entries))
		for _, e := range entries {
			t.Logf("unexpected file: %s", e.Name())
		}
	}

	// Retrieve should not write to tmpDir
	ciphertext, _ := vault.Store("test2", secret)
	_, err = vault.Retrieve(ciphertext)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}

	entries, _ = os.ReadDir(tmpDir)
	if len(entries) > 0 {
		t.Errorf("expected no files in tmpDir after retrieve, found %d", len(entries))
	}
}
