package modules

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
)

func TestVerifyPackageAcceptsTrustedSignedPackage(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	manifest := testPackageManifest()
	payload := []byte("signed module payload")
	packageBytes := makeSignedPackage(t, manifest, payload, "publisher-key-1", privateKey, nil)
	trust := StaticPublisherTrustStore{
		manifest.Publisher + "\x00publisher-key-1": {
			Publisher: manifest.Publisher,
			KeyID:     "publisher-key-1",
			PublicKey: publicKey,
		},
	}

	verified, err := VerifyPackage(packageBytes, trust)
	if err != nil {
		t.Fatalf("VerifyPackage returned error: %v", err)
	}
	if verified.Manifest.ID != manifest.ID {
		t.Fatalf("manifest ID = %q, want %q", verified.Manifest.ID, manifest.ID)
	}
	if !bytes.Equal(verified.Payload, payload) {
		t.Fatalf("payload = %q, want %q", verified.Payload, payload)
	}
	wantDigest := sha256.Sum256(payload)
	if verified.PayloadSHA256 != hex.EncodeToString(wantDigest[:]) {
		t.Fatalf("payload digest = %q, want %q", verified.PayloadSHA256, hex.EncodeToString(wantDigest[:]))
	}
}

func TestVerifyPackageRejectsTamperedPayload(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest := testPackageManifest()
	original := []byte("original")
	packageBytes := makeSignedPackage(t, manifest, original, "key-1", privateKey, func(entries map[string][]byte) {
		entries[packagePayloadName] = []byte("tampered")
	})
	trust := StaticPublisherTrustStore{
		manifest.Publisher + "\x00key-1": {Publisher: manifest.Publisher, KeyID: "key-1", PublicKey: publicKey},
	}

	_, err = VerifyPackage(packageBytes, trust)
	if err == nil || errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("VerifyPackage error = %v, want payload digest mismatch", err)
	}
}

func TestVerifyPackageRejectsUntrustedPublisher(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest := testPackageManifest()
	packageBytes := makeSignedPackage(t, manifest, []byte("payload"), "unknown-key", privateKey, nil)

	_, err = VerifyPackage(packageBytes, StaticPublisherTrustStore{})
	if !errors.Is(err, ErrUntrustedPublisher) {
		t.Fatalf("VerifyPackage error = %v, want ErrUntrustedPublisher", err)
	}
}

func TestVerifyPackageRejectsInvalidSignature(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, wrongPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest := testPackageManifest()
	packageBytes := makeSignedPackage(t, manifest, []byte("payload"), "key-1", wrongPrivateKey, nil)
	trust := StaticPublisherTrustStore{
		manifest.Publisher + "\x00key-1": {Publisher: manifest.Publisher, KeyID: "key-1", PublicKey: publicKey},
	}

	_, err = VerifyPackage(packageBytes, trust)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("VerifyPackage error = %v, want ErrInvalidSignature", err)
	}
}

func TestVerifyPackageRejectsUnsafeAndUnexpectedEntries(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest := testPackageManifest()

	tests := []struct {
		name   string
		mutate func(map[string][]byte)
	}{
		{
			name: "path traversal",
			mutate: func(entries map[string][]byte) {
				entries["../payload.tar.gz"] = entries[packagePayloadName]
				delete(entries, packagePayloadName)
			},
		},
		{
			name: "unexpected file",
			mutate: func(entries map[string][]byte) {
				entries["README.txt"] = []byte("unexpected")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packageBytes := makeSignedPackage(t, manifest, []byte("payload"), "key-1", privateKey, tt.mutate)
			_, err := VerifyPackage(packageBytes, StaticPublisherTrustStore{})
			if err == nil {
				t.Fatal("VerifyPackage succeeded, want rejection")
			}
		})
	}
}

func TestVerifyPackageRejectsUnknownSignatureFields(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest := testPackageManifest()
	packageBytes := makeSignedPackage(t, manifest, []byte("payload"), "key-1", privateKey, func(entries map[string][]byte) {
		var signature map[string]any
		if err := json.Unmarshal(entries[packageSignatureName], &signature); err != nil {
			t.Fatal(err)
		}
		signature["unexpected"] = true
		entries[packageSignatureName], err = json.Marshal(signature)
		if err != nil {
			t.Fatal(err)
		}
	})
	trust := StaticPublisherTrustStore{
		manifest.Publisher + "\x00key-1": {Publisher: manifest.Publisher, KeyID: "key-1", PublicKey: publicKey},
	}

	_, err = VerifyPackage(packageBytes, trust)
	if err == nil {
		t.Fatal("VerifyPackage succeeded, want strict JSON rejection")
	}
}

func TestPackageSigningMessageRejectsInvalidDigest(t *testing.T) {
	_, err := PackageSigningMessage(testPackageManifest(), "not-a-digest")
	if err == nil {
		t.Fatal("PackageSigningMessage succeeded, want invalid digest error")
	}
}

func testPackageManifest() Manifest {
	return Manifest{
		ID:         "example.publisher.module",
		Name:       "Example Module",
		Version:    "1.0.0",
		APIVersion: CurrentAPIVersion,
		Publisher:  "example.publisher",
		Permissions: []string{
			"devices.read",
		},
	}
}

func makeSignedPackage(t *testing.T, manifest Manifest, payload []byte, keyID string, privateKey ed25519.PrivateKey, mutate func(map[string][]byte)) []byte {
	t.Helper()

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	payloadDigest := sha256.Sum256(payload)
	payloadHex := hex.EncodeToString(payloadDigest[:])
	message, err := PackageSigningMessage(manifest, payloadHex)
	if err != nil {
		t.Fatal(err)
	}
	signature := PackageSignature{
		Algorithm:     "ed25519",
		KeyID:         keyID,
		PayloadSHA256: payloadHex,
		Signature:     hex.EncodeToString(ed25519.Sign(privateKey, message)),
	}
	signatureJSON, err := json.Marshal(signature)
	if err != nil {
		t.Fatal(err)
	}

	entries := map[string][]byte{
		packageManifestName:  manifestJSON,
		packagePayloadName:   payload,
		packageSignatureName: signatureJSON,
	}
	if mutate != nil {
		mutate(entries)
	}

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, data := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
