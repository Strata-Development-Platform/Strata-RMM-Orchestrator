package core

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/google/uuid"
)

type Identity struct {
	AgentID  string   `json:"agent_id"`
	TenantID string   `json:"tenant_id"`
	Token    string   `json:"-"`
	NATSURLs []string `json:"-"`
	CertPEM  []byte   `json:"cert_pem"`
	KeyPEM   []byte   `json:"key_pem"`
	CAPEM    []byte   `json:"ca_pem"`
}

type IdentityManager struct {
	dir string
}

func NewIdentityManager(dataDir string) *IdentityManager {
	return &IdentityManager{dir: filepath.Join(dataDir, "identity")}
}

func (im *IdentityManager) LoadOrCreate(tenantID, enrollmentToken string) (*Identity, error) {
	if err := os.MkdirAll(im.dir, 0700); err != nil {
		return nil, fmt.Errorf("creating identity directory: %w", err)
	}

	ident, err := im.load()
	if err == nil && ident != nil {
		if tenantID != "" && ident.TenantID != tenantID {
			return nil, fmt.Errorf("stored identity tenant does not match configured tenant")
		}
		return ident, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("loading stored identity: %w", err)
	}
	if err != nil && identityStateExists(im.dir) {
		return nil, fmt.Errorf("stored identity is incomplete: %w", err)
	}
	if enrollmentToken == "" {
		return nil, fmt.Errorf("stored identity is missing and enrollment token is unavailable")
	}

	return im.enroll(tenantID, enrollmentToken)
}

func (im *IdentityManager) RegisterWithDeploymentID(registerURL, deploymentID, enrollmentToken string) (*Identity, error) {
	if err := os.MkdirAll(im.dir, 0700); err != nil {
		return nil, fmt.Errorf("creating identity directory: %w", err)
	}

	ident, err := im.load()
	if err == nil && ident != nil {
		return ident, nil
	}
	if err != nil && (!os.IsNotExist(err) || identityStateExists(im.dir)) {
		return nil, fmt.Errorf("loading stored identity: %w", err)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating key: %w", err)
	}

	keyBytes, _ := x509.MarshalECPrivateKey(key)
	pubKeyBytes := elliptic.Marshal(elliptic.P256(), key.X, key.Y) //nolint:staticcheck // SA1019: deprecated but safe for this encoding use

	hostname, _ := os.Hostname()

	ver := "0.0.0-dev"
	if v := os.Getenv("STRATA_AGENT_VERSION"); v != "" {
		ver = v
	}

	body, _ := json.Marshal(map[string]string{
		"deployment_id":    deploymentID,
		"enrollment_token": enrollmentToken,
		"hostname":         hostname,
		"os":               runtime.GOOS,
		"arch":             runtime.GOARCH,
		"version":          ver,
		"public_key":       hex.EncodeToString(pubKeyBytes),
	})

	request, err := http.NewRequest(http.MethodPost, registerURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build register request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("register request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("registration failed: %d", resp.StatusCode)
	}

	var regResp struct {
		DeviceID string   `json:"device_id"`
		AgentID  string   `json:"agent_id"`
		TenantID string   `json:"tenant_id"`
		Token    string   `json:"token"`
		NatsURLs []string `json:"nats_urls"`
		CAPEM    string   `json:"ca_pem"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&regResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if regResp.AgentID == "" || regResp.TenantID == "" || regResp.Token == "" || len(regResp.NatsURLs) == 0 {
		return nil, fmt.Errorf("registration response missing agent identity or messaging configuration")
	}

	ident = &Identity{
		AgentID:  regResp.AgentID,
		TenantID: regResp.TenantID,
		Token:    regResp.Token,
		NATSURLs: append([]string(nil), regResp.NatsURLs...),
		CAPEM:    []byte(regResp.CAPEM),
		KeyPEM:   pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}),
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generating certificate serial: %w", err)
	}
	certificate := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: regResp.AgentID, Organization: []string{regResp.TenantID}},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, certificate, certificate, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("creating endpoint certificate: %w", err)
	}
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
	if err != nil && !os.IsNotExist(err) {
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
		AgentID  string   `json:"agent_id"`
		TenantID string   `json:"tenant_id"`
		Token    string   `json:"token"`
		NATSURLs []string `json:"nats_urls"`
	}
	if err := json.Unmarshal(meta, &metaData); err != nil {
		return nil, err
	}
	ident.AgentID = metaData.AgentID
	ident.TenantID = metaData.TenantID
	ident.Token = metaData.Token
	ident.NATSURLs = append([]string(nil), metaData.NATSURLs...)
	if ident.AgentID == "" || ident.TenantID == "" {
		return nil, fmt.Errorf("identity metadata is missing endpoint binding")
	}
	certificate, err := tls.X509KeyPair(ident.CertPEM, ident.KeyPEM)
	if err != nil {
		return nil, fmt.Errorf("identity certificate and key are invalid: %w", err)
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil || leaf.Subject.CommonName != ident.AgentID {
		return nil, fmt.Errorf("identity certificate does not match endpoint")
	}

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
	if err := os.MkdirAll(im.dir, 0700); err != nil {
		return err
	}

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

	meta, err := json.Marshal(struct {
		AgentID  string   `json:"agent_id"`
		TenantID string   `json:"tenant_id"`
		Token    string   `json:"token,omitempty"`
		NATSURLs []string `json:"nats_urls,omitempty"`
	}{ident.AgentID, ident.TenantID, ident.Token, ident.NATSURLs})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(im.dir, "meta.json"), meta, 0600)
}

func identityStateExists(dir string) bool {
	for _, name := range []string{"agent.crt", "agent.key", "ca.crt", "meta.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}
