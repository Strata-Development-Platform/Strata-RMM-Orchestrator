package modules

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

const (
	packageManifestName  = "manifest.json"
	packagePayloadName   = "payload.tar.gz"
	packageSignatureName = "signature.json"

	maxPackageBytes   = 64 << 20
	maxManifestBytes  = 1 << 20
	maxSignatureBytes = 64 << 10
	maxPayloadBytes   = 60 << 20
)

var (
	ErrUntrustedPublisher = errors.New("module publisher is not trusted")
	ErrInvalidSignature   = errors.New("module package signature is invalid")
)

type PublisherKey struct {
	Publisher string
	KeyID     string
	PublicKey ed25519.PublicKey
}

type PublisherTrustStore interface {
	LookupPublisherKey(publisher, keyID string) (PublisherKey, error)
}

type StaticPublisherTrustStore map[string]PublisherKey

func (s StaticPublisherTrustStore) LookupPublisherKey(publisher, keyID string) (PublisherKey, error) {
	key, ok := s[publisher+"\x00"+keyID]
	if !ok {
		return PublisherKey{}, ErrUntrustedPublisher
	}
	if key.Publisher != publisher || key.KeyID != keyID || len(key.PublicKey) != ed25519.PublicKeySize {
		return PublisherKey{}, ErrUntrustedPublisher
	}
	return key, nil
}

type PackageSignature struct {
	Algorithm     string `json:"algorithm"`
	KeyID         string `json:"key_id"`
	PayloadSHA256 string `json:"payload_sha256"`
	Signature     string `json:"signature"`
}

type VerifiedPackage struct {
	Manifest      Manifest
	Payload       []byte
	PayloadSHA256 string
	PublisherKey  PublisherKey
}

func VerifyPackage(data []byte, trust PublisherTrustStore) (VerifiedPackage, error) {
	if len(data) == 0 {
		return VerifiedPackage{}, errors.New("module package is empty")
	}
	if len(data) > maxPackageBytes {
		return VerifiedPackage{}, fmt.Errorf("module package exceeds %d bytes", maxPackageBytes)
	}
	if trust == nil {
		return VerifiedPackage{}, errors.New("publisher trust store is required")
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return VerifiedPackage{}, fmt.Errorf("open module package: %w", err)
	}

	entries := make(map[string]*zip.File, 3)
	for _, file := range zr.File {
		clean := path.Clean(file.Name)
		if clean != file.Name || strings.HasPrefix(clean, "/") || clean == "." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "\\") {
			return VerifiedPackage{}, fmt.Errorf("unsafe package entry %q", file.Name)
		}
		if file.FileInfo().IsDir() {
			return VerifiedPackage{}, fmt.Errorf("unexpected package directory %q", file.Name)
		}
		if _, duplicate := entries[file.Name]; duplicate {
			return VerifiedPackage{}, fmt.Errorf("duplicate package entry %q", file.Name)
		}
		entries[file.Name] = file
	}

	if len(entries) != 3 {
		return VerifiedPackage{}, errors.New("module package must contain exactly manifest.json, payload.tar.gz, and signature.json")
	}
	manifestFile, okManifest := entries[packageManifestName]
	payloadFile, okPayload := entries[packagePayloadName]
	signatureFile, okSignature := entries[packageSignatureName]
	if !okManifest || !okPayload || !okSignature {
		return VerifiedPackage{}, errors.New("module package must contain exactly manifest.json, payload.tar.gz, and signature.json")
	}

	manifestBytes, err := readZipEntry(manifestFile, maxManifestBytes)
	if err != nil {
		return VerifiedPackage{}, fmt.Errorf("read manifest: %w", err)
	}
	payload, err := readZipEntry(payloadFile, maxPayloadBytes)
	if err != nil {
		return VerifiedPackage{}, fmt.Errorf("read payload: %w", err)
	}
	signatureBytes, err := readZipEntry(signatureFile, maxSignatureBytes)
	if err != nil {
		return VerifiedPackage{}, fmt.Errorf("read signature: %w", err)
	}

	var manifest Manifest
	if err := decodeStrictJSON(manifestBytes, &manifest); err != nil {
		return VerifiedPackage{}, fmt.Errorf("decode manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return VerifiedPackage{}, fmt.Errorf("validate manifest: %w", err)
	}

	var signature PackageSignature
	if err := decodeStrictJSON(signatureBytes, &signature); err != nil {
		return VerifiedPackage{}, fmt.Errorf("decode package signature: %w", err)
	}
	if signature.Algorithm != "ed25519" {
		return VerifiedPackage{}, fmt.Errorf("unsupported package signature algorithm %q", signature.Algorithm)
	}
	if strings.TrimSpace(signature.KeyID) == "" {
		return VerifiedPackage{}, errors.New("package signature key_id is required")
	}

	payloadDigest := sha256.Sum256(payload)
	payloadHex := hex.EncodeToString(payloadDigest[:])
	if !strings.EqualFold(signature.PayloadSHA256, payloadHex) {
		return VerifiedPackage{}, errors.New("package payload digest does not match signature metadata")
	}

	key, err := trust.LookupPublisherKey(manifest.Publisher, signature.KeyID)
	if err != nil {
		if errors.Is(err, ErrUntrustedPublisher) {
			return VerifiedPackage{}, ErrUntrustedPublisher
		}
		return VerifiedPackage{}, fmt.Errorf("lookup publisher key: %w", err)
	}

	signatureRaw, err := hex.DecodeString(signature.Signature)
	if err != nil || len(signatureRaw) != ed25519.SignatureSize {
		return VerifiedPackage{}, ErrInvalidSignature
	}
	message, err := packageSigningMessage(manifest, payloadHex)
	if err != nil {
		return VerifiedPackage{}, fmt.Errorf("build signing message: %w", err)
	}
	if !ed25519.Verify(key.PublicKey, message, signatureRaw) {
		return VerifiedPackage{}, ErrInvalidSignature
	}

	return VerifiedPackage{
		Manifest:      manifest,
		Payload:       payload,
		PayloadSHA256: payloadHex,
		PublisherKey:  key,
	}, nil
}

func PackageSigningMessage(manifest Manifest, payloadSHA256 string) ([]byte, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return packageSigningMessage(manifest, strings.ToLower(payloadSHA256))
}

func packageSigningMessage(manifest Manifest, payloadSHA256 string) ([]byte, error) {
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	if len(payloadSHA256) != sha256.Size*2 {
		return nil, errors.New("payload SHA-256 must be a 64-character hexadecimal digest")
	}
	if _, err := hex.DecodeString(payloadSHA256); err != nil {
		return nil, errors.New("payload SHA-256 must be hexadecimal")
	}

	message := make([]byte, 0, len(manifestJSON)+1+len(payloadSHA256))
	message = append(message, manifestJSON...)
	message = append(message, '\n')
	message = append(message, payloadSHA256...)
	return message, nil
}

func readZipEntry(file *zip.File, limit int64) ([]byte, error) {
	if limit < 0 {
		return nil, errors.New("zip entry byte limit must not be negative")
	}

	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	data, err := io.ReadAll(io.LimitReader(reader, limit))
	if err != nil {
		return nil, err
	}

	var extra [1]byte
	n, err := reader.Read(extra[:])
	if n > 0 {
		return nil, fmt.Errorf("entry %q exceeds %d bytes", file.Name, limit)
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}

	return data, nil
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}
