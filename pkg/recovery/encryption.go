package recovery

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
)

// EnvelopeVersion is the current authenticated envelope format version.
const EnvelopeVersion = 1

// Envelope is the authenticated encryption format for backup artifacts.
// Layout:
//
//	[version:1 byte][nonce:12 bytes][associated_data_hash:32 bytes][ciphertext:N bytes][tag:16 bytes]
//
// The associated_data_hash authenticates metadata that must not be substituted.
type Envelope struct {
	Version          uint8   `json:"version"`
	Nonce            []byte  `json:"-"`
	AssociatedData   []byte  `json:"associated_data"` // metadata to authenticate (for first-write)
	ADHash           []byte  `json:"-"`               // pre-computed AD hash (for serialization)
	Ciphertext       []byte  `json:"-"`               // ciphertext + GCM tag appended
	EncryptedSize    int     `json:"encrypted_size"`
	OriginalSize     int     `json:"original_size"`
}

// Encrypt produces an authenticated envelope for the given plaintext and metadata.
// The nonce is randomly generated for each call, ensuring uniqueness.
func Encrypt(key []byte, plaintext []byte, associatedData []byte) (*Envelope, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes, got %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	// Hash associated data to bind metadata to the ciphertext
	adHash := sha256.Sum256(associatedData)

	// Build the envelope payload: [nonce][ad_hash][ciphertext+tag]
	payload := make([]byte, 0, 1+len(nonce)+len(adHash)+aesGCM.Overhead())
	payload = append(payload, EnvelopeVersion)
	payload = append(payload, nonce...)
	payload = append(payload, adHash[:]...)

	ciphertext := aesGCM.Seal(nil, nonce, plaintext, adHash[:])
	payload = append(payload, ciphertext...)

	enc := &Envelope{
		Version:        EnvelopeVersion,
		Nonce:          nonce,
		AssociatedData: associatedData,
		ADHash:         adHash[:],
		Ciphertext:     ciphertext,
		EncryptedSize:  len(payload),
		OriginalSize:   len(plaintext),
	}

	return enc, nil
}

// Decrypt verifies and returns the plaintext from an authenticated envelope.
// It verifies the nonce, associated data hash, and GCM tag.
func Decrypt(key []byte, enc *Envelope) ([]byte, error) {
	if enc.Version != EnvelopeVersion {
		return nil, fmt.Errorf("unsupported envelope version: %d", enc.Version)
	}

	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes, got %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}

	nonceSize := aesGCM.NonceSize()

	// Verify nonce length
	if len(enc.Nonce) < nonceSize {
		return nil, errors.New("nonce too short")
	}
	nonce := enc.Nonce[:nonceSize]

	// The ciphertext from Seal already includes the GCM tag appended.
	// We pass it directly to Open along with the associated data hash as AAD.
	if len(enc.Ciphertext) < aesGCM.Overhead() {
		return nil, errors.New("ciphertext too short")
	}

	// Use stored AD hash if available (from deserialization), otherwise compute
	adHash := enc.ADHash
	if len(adHash) == 0 {
		hash := sha256.Sum256(enc.AssociatedData)
		adHash = hash[:]
	}

	// Seal already appends the tag; Open expects [ciphertext||tag] as input
	plaintext, err := aesGCM.Open(nil, nonce, enc.Ciphertext, adHash)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w (key mismatch, metadata tampering, or ciphertext corruption)", err)
	}

	return plaintext, nil
}

// Marshal serializes the envelope to bytes for storage/transmission.
func (e *Envelope) Marshal() ([]byte, error) {
	buf := make([]byte, 0, 1+len(e.Nonce)+32+len(e.Ciphertext))
	buf = append(buf, e.Version)
	buf = append(buf, e.Nonce...)
	adHash := sha256.Sum256(e.AssociatedData)
	buf = append(buf, adHash[:]...)
	buf = append(buf, e.Ciphertext...)
	return buf, nil
}

// Unmarshal deserializes an envelope from bytes.
func Unmarshal(data []byte) (*Envelope, error) {
	if len(data) < 1+12+32 {
		return nil, errors.New("data too short for envelope")
	}

	version := data[0]
	if version != EnvelopeVersion {
		return nil, fmt.Errorf("unsupported envelope version: %d", version)
	}

	nonceSize := 12 // GCM standard nonce size
	nonce := data[1 : 1+nonceSize]
	// Extract the 32-byte AD hash stored between nonce and ciphertext
	adHash := data[1+nonceSize : 1+nonceSize+32]
	ciphertext := data[1+nonceSize+32:]

	return &Envelope{
		Version:        version,
		Nonce:          nonce,
		AssociatedData: nil,
		ADHash:         adHash,
		Ciphertext:     ciphertext,
		EncryptedSize:  len(data),
	}, nil
}

// --- Authenticated metadata builder ---

// MetadataAuth assembles the associated data bytes that bind metadata to encryption.
type MetadataAuth struct {
	mu           sync.Mutex
	buf          []byte
	backupSetID  string
	componentID  string
	environmentID string
	keyID        string
	tenantScope  string
}

// NewMetadataAuth creates a new metadata authenticator.
func NewMetadataAuth() *MetadataAuth {
	return &MetadataAuth{buf: make([]byte, 0, 128)}
}

// AppendString binds a string value to the authenticated metadata.
func (m *MetadataAuth) AppendString(tag string, value string) *MetadataAuth {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.buf = append(m.buf, []byte(tag+"=")...)
	m.buf = append(m.buf, []byte(value)...)
	m.buf = append(m.buf, '\n')
	return m
}

// Bytes returns the assembled associated data bytes.
func (m *MetadataAuth) Bytes() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(m.buf))
	copy(cp, m.buf)
	return cp
}

// --- Base64 encoding helpers ---

// EncodeEnvelope base64-encodes a marshaled envelope for text storage.
func EncodeEnvelope(enc *Envelope) (string, error) {
	data, err := enc.Marshal()
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// DecodeEnvelope decodes base64 and unmarshals an envelope.
func DecodeEnvelope(encoded string) (*Envelope, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	return Unmarshal(data)
}

// --- Deterministic nonce for testing ---

// nonceCounter provides monotonically increasing nonces for tests.
type nonceCounter struct {
	mu    sync.Mutex
	count uint64
}

var testNonce nonceCounter

// TestNonce returns a deterministic nonce for reproducible tests.
func TestNonce() []byte {
	testNonce.mu.Lock()
	defer testNonce.mu.Unlock()
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, testNonce.count)
	testNonce.count++
	nonce := make([]byte, 12)
	copy(nonce[4:], b)
	return nonce
}

// ResetTestNonce resets the deterministic nonce counter.
func ResetTestNonce() {
	testNonce.mu.Lock()
	defer testNonce.mu.Unlock()
	testNonce.count = 0
}

// EncryptWithNonce encrypts with a specific nonce (for testing).
func EncryptWithNonce(key []byte, plaintext []byte, associatedData []byte, nonce []byte) (*Envelope, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes, got %d", len(key))
	}
	if len(nonce) != 12 {
		return nil, fmt.Errorf("nonce must be 12 bytes, got %d", len(nonce))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}

	adHash := sha256.Sum256(associatedData)
	ciphertext := aesGCM.Seal(nil, nonce, plaintext, adHash[:])

	return &Envelope{
		Version:        EnvelopeVersion,
		Nonce:          nonce,
		AssociatedData: associatedData,
		ADHash:         adHash[:],
		Ciphertext:     ciphertext,
		EncryptedSize:  1 + len(nonce) + 32 + len(ciphertext),
		OriginalSize:   len(plaintext),
	}, nil
}

// --- Hex helpers for key material ---

// HexDecodeKey converts a hex-encoded key string to bytes.
func HexDecodeKey(s string) ([]byte, error) {
	return hex.DecodeString(s)
}

// HexEncodeKey converts key bytes to a hex-encoded string.
func HexEncodeKey(key []byte) string {
	return hex.EncodeToString(key)
}
