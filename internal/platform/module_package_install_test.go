package platform

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/modules"
)

func TestPrepareSignedModulePackageRejectsRawManifestJSON(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(moduleRuntimeRootEnv, filepath.Join(t.TempDir(), "modules"))
	t.Setenv(modulePublisherTrustEnv, fmt.Sprintf(`[{"publisher":"example.publisher","key_id":"key-1","public_key":"%s"}]`, base64.StdEncoding.EncodeToString(publicKey)))

	r := httptest.NewRequest("POST", moduleLifecycleCollectionPath, bytes.NewBufferString(`{"id":"example.publisher.module"}`))
	w := httptest.NewRecorder()
	if _, err := prepareSignedModulePackage(w, r); err == nil {
		t.Fatal("raw manifest JSON was accepted as a signed module package")
	}
}

func TestPrepareSignedModulePackageFailsClosedWithoutConfiguration(t *testing.T) {
	r := httptest.NewRequest("POST", moduleLifecycleCollectionPath, bytes.NewBufferString("not-a-package"))
	w := httptest.NewRecorder()
	t.Setenv(moduleRuntimeRootEnv, "")
	t.Setenv(modulePublisherTrustEnv, "")
	_, err := prepareSignedModulePackage(w, r)
	if !errorsIsAny(err, errModuleRuntimeRootRequired, errModulePublisherTrust) {
		t.Fatalf("error=%v, want fail-closed configuration error", err)
	}
}

func TestValidateSignedModuleEntrypointRequiresPayloadFile(t *testing.T) {
	manifest := modules.Manifest{
		ID:         "example.publisher.module",
		Name:       "Example",
		Version:    "1.0.0",
		APIVersion: modules.CurrentAPIVersion,
		Publisher:  "example.publisher",
		Runtime: &modules.RuntimeSpec{
			Kind:           modules.RuntimeKindWASI,
			Entrypoint:     "bin/module.wasm",
			MemoryMB:       64,
			TimeoutSeconds: 15,
			MaxConcurrency: 2,
			Network:        modules.RuntimeNetworkNone,
		},
	}
	payload := modules.ValidatedPayload{Files: []modules.PayloadFile{{Path: "bin/other.wasm", Mode: 0o500, Data: []byte("wasm")}}}
	if err := validateSignedModuleEntrypoint(manifest, payload); err != errModuleEntrypointMissing {
		t.Fatalf("error=%v, want errModuleEntrypointMissing", err)
	}
	payload.Files[0].Path = "bin/module.wasm"
	if err := validateSignedModuleEntrypoint(manifest, payload); err != nil {
		t.Fatalf("declared entrypoint rejected: %v", err)
	}
}

func TestReadSignedModulePackageRejectsEmptyBody(t *testing.T) {
	r := httptest.NewRequest("POST", moduleLifecycleCollectionPath, bytes.NewReader(nil))
	w := httptest.NewRecorder()
	if _, err := readSignedModulePackage(w, r); err != errModulePackageBodyRequired {
		t.Fatalf("error=%v, want errModulePackageBodyRequired", err)
	}
}

func errorsIsAny(err error, targets ...error) bool {
	for _, target := range targets {
		if err != nil && target != nil && err.Error() == target.Error() {
			return true
		}
	}
	return false
}
