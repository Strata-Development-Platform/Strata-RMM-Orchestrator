package core

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testIdentity(t *testing.T, tenantID, agentID string) *Identity {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: agentID, Organization: []string{tenantID}},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return &Identity{
		AgentID: agentID, TenantID: tenantID,
		CertPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		KeyPEM:  pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
	}
}

func TestValidateBootstrapAllowsOneTimeEnrollment(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agent.EnrollmentToken = "one-time-secret"
	cfg.Agent.RegisterURL = "https://rmm.example.test/api/v1/agent/register"

	if err := cfg.ValidateBootstrap(); err != nil {
		t.Fatalf("ValidateBootstrap() error = %v", err)
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "tenant_id") {
		t.Fatalf("Validate() error = %v, want tenant_id failure before enrollment", err)
	}
}

func TestRuntimeValidationRejectsPlaintextRemoteMessaging(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agent.TenantID = "tenant-1"
	cfg.NATS.URLs = []string{"nats://nats.example.test:4222"}
	cfg.NATS.Token = "issued-token"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "encrypted transport") {
		t.Fatalf("Validate() error = %v, want plaintext rejection", err)
	}
}

func TestRuntimeValidationAcceptsTLSWithToken(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agent.TenantID = "tenant-1"
	cfg.NATS.URLs = []string{"tls://nats.example.test:4222"}
	cfg.NATS.Token = "issued-token"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateBootstrapRejectsInsecureRemoteRegistration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agent.EnrollmentToken = "one-time-secret"
	cfg.Agent.RegisterURL = "http://rmm.example.test/api/v1/agent/register"

	if err := cfg.ValidateBootstrap(); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("ValidateBootstrap() error = %v, want HTTPS failure", err)
	}
}

func TestLoadAcceptsInstallerBootstrapConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	data := []byte(`agent:
  enrollment_token: one-time-secret
  register_url: https://rmm.example.test/api/v1/agent/register
collect:
  interval: 1m
`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	if err := cfg.Load(path); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestSaveRuntimeRemovesOneTimeEnrollmentMaterial(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.yaml")
	cfg := DefaultConfig()
	cfg.Agent.TenantID = "tenant-1"
	cfg.Agent.AgentID = "agent-1"
	cfg.Agent.EnrollmentToken = "one-time-secret"
	cfg.Agent.DeploymentID = "legacy-secret"
	cfg.NATS.URLs = []string{"tls://nats.example.test:4222"}
	cfg.NATS.Token = "runtime-agent-token"

	if err := cfg.SaveRuntime(path); err != nil {
		t.Fatalf("SaveRuntime() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "one-time-secret") || strings.Contains(text, "legacy-secret") {
		t.Fatalf("runtime config retained one-time enrollment material: %s", text)
	}
	if !strings.Contains(text, "runtime-agent-token") || !strings.Contains(text, "tenant-1") {
		t.Fatalf("runtime config omitted issued identity: %s", text)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("runtime config mode = %o, want 600", info.Mode().Perm())
	}
}

func TestRegistrationPersistsIssuedMessagingIdentity(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/api/v1/agent/register" {
			http.NotFound(w, r)
			return
		}
		var request map[string]string
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode registration request: %v", err)
		}
		if request["enrollment_token"] != "one-time-secret" || request["public_key"] == "" {
			t.Fatalf("unexpected registration request: %#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"device_id": "device-1",
			"agent_id":  "agent-1",
			"tenant_id": "tenant-1",
			"token":     "issued-agent-jwt",
			"nats_urls": []string{"tls://nats.example.test:4222"},
			"ca_pem":    "test-ca-pem",
		})
	}))
	defer server.Close()

	manager := NewIdentityManager(t.TempDir())
	identity, err := manager.RegisterWithDeploymentID(server.URL+"/api/v1/agent/register", "", "one-time-secret")
	if err != nil {
		t.Fatalf("RegisterWithDeploymentID() error = %v", err)
	}
	if identity.AgentID != "agent-1" || identity.TenantID != "tenant-1" || identity.Token != "issued-agent-jwt" {
		t.Fatalf("registration identity = %#v", identity)
	}
	if len(identity.NATSURLs) != 1 || identity.NATSURLs[0] != "tls://nats.example.test:4222" {
		t.Fatalf("registration NATS URLs = %#v", identity.NATSURLs)
	}
	if string(identity.CAPEM) != "test-ca-pem" {
		t.Fatalf("registration CA = %q", identity.CAPEM)
	}
	block, _ := pem.Decode(identity.CertPEM)
	if block == nil {
		t.Fatal("registration did not create a PEM certificate")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse endpoint certificate: %v", err)
	}
	if certificate.Subject.CommonName != "agent-1" || len(certificate.ExtKeyUsage) != 1 || certificate.ExtKeyUsage[0] != x509.ExtKeyUsageClientAuth {
		t.Fatalf("endpoint certificate identity = %#v", certificate)
	}

	reloaded, err := manager.RegisterWithDeploymentID(server.URL+"/api/v1/agent/register", "", "must-not-be-used")
	if err != nil {
		t.Fatalf("reloading identity error = %v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("registration requests = %d, want 1", requests.Load())
	}
	if reloaded.AgentID != identity.AgentID || reloaded.Token != identity.Token || len(reloaded.NATSURLs) != 1 {
		t.Fatalf("reloaded identity = %#v, want persisted issued identity", reloaded)
	}

	metaPath := filepath.Join(manager.dir, "meta.json")
	info, err := os.Stat(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("identity metadata mode = %o, want 600", info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(manager.dir, "ca.crt")); err != nil {
		t.Fatalf("persisted CA: %v", err)
	}
}

func TestRegistrationRejectsIncompleteMessagingResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"agent_id": "agent-1", "tenant_id": "tenant-1", "nats_urls": []string{"tls://nats.example.test:4222"},
		})
	}))
	defer server.Close()

	manager := NewIdentityManager(t.TempDir())
	_, err := manager.RegisterWithDeploymentID(server.URL, "", "one-time-secret")
	if err == nil || !strings.Contains(err.Error(), "missing agent identity") {
		t.Fatalf("RegisterWithDeploymentID() error = %v, want incomplete response failure", err)
	}
}

func TestValidateBootstrapRejectsSubsecondCollection(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agent.EnrollmentToken = "one-time-secret"
	cfg.Agent.RegisterURL = "https://rmm.example.test/api/v1/agent/register"
	cfg.Collect.Interval = time.Millisecond

	if err := cfg.ValidateBootstrap(); err == nil || !strings.Contains(err.Error(), "at least 1s") {
		t.Fatalf("ValidateBootstrap() error = %v, want interval failure", err)
	}
}

func TestIdentityManagerRejectsCorruptStoredIdentity(t *testing.T) {
	dataDir := t.TempDir()
	identityDir := filepath.Join(dataDir, "identity")
	if err := os.MkdirAll(identityDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(identityDir, "meta.json"), []byte("not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	manager := NewIdentityManager(dataDir)
	if _, err := manager.LoadExisting("tenant-a"); err == nil {
		t.Fatal("LoadExisting accepted corrupt identity")
	}
}

func TestIdentityManagerRejectsConfiguredTenantMismatch(t *testing.T) {
	dataDir := t.TempDir()
	manager := NewIdentityManager(dataDir)
	identity := testIdentity(t, "tenant-a", "agent-a")
	if err := manager.save(identity); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.LoadExisting("tenant-b"); err == nil {
		t.Fatal("LoadExisting accepted identity from another tenant")
	}
}

func TestValidateBootstrapRejectsTenantIDEnrollmentBypass(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agent.TenantID = "tenant-a"
	cfg.Agent.EnrollmentToken = "must-be-consumed-server-side"
	cfg.NATS.Token = "untrusted-local-token"
	if err := cfg.ValidateBootstrap(); err == nil || !strings.Contains(err.Error(), "orchestrator registration is required") {
		t.Fatalf("ValidateBootstrap() error = %v, want registration requirement", err)
	}
}

func TestLoadExistingPreservesIssuedIdentityAcrossRestart(t *testing.T) {
	manager := NewIdentityManager(t.TempDir())
	want := testIdentity(t, "tenant-a", "agent-a")
	want.Token = "issued-agent-token"
	want.NATSURLs = []string{"tls://nats.example.test:4222"}
	if err := manager.save(want); err != nil {
		t.Fatal(err)
	}
	got, err := manager.LoadExisting("tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentID != want.AgentID || got.TenantID != want.TenantID || got.Token != want.Token {
		t.Fatalf("restarted identity = %#v, want %#v", got, want)
	}
}
