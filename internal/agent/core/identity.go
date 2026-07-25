package core

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/google/uuid"
)

type Identity struct {
	AgentID  string `json:"agent_id"`
	TenantID string `json:"tenant_id"`
	CertPEM  []byte `json:"cert_pem"`
	KeyPEM   []byte `json:"key_pem"`
	CAPEM    []byte `json:"ca_pem"`
}

type IdentityManager struct {
	dir string
}

func NewIdentityManager(dataDir string) *IdentityManager {
	return &IdentityManager{dir: filepath.Join(dataDir, "identity")}
}

func (im *IdentityManager) LoadOrCreate(tenantID, enrollmentToken string) (*Identity, error) {
	os.MkdirAll(im.dir, 0700)

	ident, err := im.load()
	if err == nil && ident != nil {
		return ident, nil
	}

	return im.enroll(tenantID, enrollmentToken)
}

func (im *IdentityManager) RegisterWithDeploymentID(registerURL, deploymentID string) (*Identity, error) {
	os.MkdirAll(im.dir, 0700)

	ident, err := im.load()
	if err == nil && ident != nil {
		return ident, nil
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating key: %w", err)
	}

	keyBytes, _ := x509.MarshalECPrivateKey(key)
	pubKeyBytes := elliptic.Marshal(elliptic.P256(), key.PublicKey.X, key.PublicKey.Y)

	hostname, _ := os.Hostname()

	body, _ := json.Marshal(map[string]string{
		"deployment_id": deploymentID,
		"hostname":      hostname,
		"os":            runtime.GOOS,
		"arch":          runtime.GOARCH,
		"public_key":    hex.EncodeToString(pubKeyBytes),
	})

	resp, err := http.Post(registerURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("register request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("registration failed: %d", resp.StatusCode)
	}

	var regResp struct {
		DeviceID  string `json:"device_id"`
		AgentID   string `json:"agent_id"`
		TenantID  string `json:"tenant_id"`
		Token     string `json:"token"`
		NatsURLs  []string `json:"nats_urls"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&regResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	ident = &Identity{
		AgentID:  regResp.AgentID,
		TenantID: regResp.TenantID,
		KeyPEM:   pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}),
	}

	certDER, _ := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
		Subject:      pkix.Name{CommonName: regResp.AgentID, Organization: []string{regResp.TenantID}},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
	}, &x509.Certificate{}, &key.PublicKey, key)
	ident.CertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	if err := im.save(ident); err != nil {
		return nil, fmt.Errorf("saving identity: %w", err)
	}

	return ident, nil
}

func (im *IdentityManager) load() (*Identity, error) {
	ident := &Identity{}

	certPath := filepath.Join(im.dir, "agent.crt")
	keyPath := filepath.Join(im.dir, "agent.key")
	caPath := filepath.Join(im.dir, "ca.crt")
	metaPath := filepath.Join(im.dir, "meta.json")

	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return nil, err
	}
	meta, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}

	ident.CertPEM = certPEM
	ident.KeyPEM = keyPEM
	ident.CAPEM = caPEM

	var metaData struct {
		AgentID  string `json:"agent_id"`
		TenantID string `json:"tenant_id"`
	}
	if err := json.Unmarshal(meta, &metaData); err != nil {
		return nil, err
	}
	ident.AgentID = metaData.AgentID
	ident.TenantID = metaData.TenantID

	return ident, nil
}

func (im *IdentityManager) enroll(tenantID, enrollmentToken string) (*Identity, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating key: %w", err)
	}

	agentID := uuid.New().String()
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   agentID,
			Organization: []string{tenantID},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("creating cert: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyBytes, _ := x509.MarshalECPrivateKey(key)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})

	ident := &Identity{
		AgentID:  agentID,
		TenantID: tenantID,
		CertPEM:  certPEM,
		KeyPEM:   keyPEM,
	}

	if err := im.save(ident); err != nil {
		return nil, fmt.Errorf("saving identity: %w", err)
	}

	return ident, nil
}

func (im *IdentityManager) save(ident *Identity) error {
	os.MkdirAll(im.dir, 0700)

	if err := os.WriteFile(filepath.Join(im.dir, "agent.crt"), ident.CertPEM, 0600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(im.dir, "agent.key"), ident.KeyPEM, 0600); err != nil {
		return err
	}
	if len(ident.CAPEM) > 0 {
		if err := os.WriteFile(filepath.Join(im.dir, "ca.crt"), ident.CAPEM, 0600); err != nil {
			return err
		}
	}

	meta := fmt.Sprintf(`{"agent_id":"%s","tenant_id":"%s"}`, ident.AgentID, ident.TenantID)
	return os.WriteFile(filepath.Join(im.dir, "meta.json"), []byte(meta), 0600)
}
