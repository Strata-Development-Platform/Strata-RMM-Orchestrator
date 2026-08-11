package modules

import (
	"archive/tar"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"path/filepath"
	"testing"
	"time"
)

func TestSignedWASIReferenceCandidateHealthActivatesAndExecutes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "modules")
	manifest := alphaReferenceManifest("1.0.0")
	pkg, payload := signedReferencePayload(t, manifest, wasmEmpty)

	if _, err := MaterializePayloadRetrySafe(pkg, payload, MaterializeOptions{Root: root}); err != nil {
		t.Fatalf("materialize signed reference package: %v", err)
	}
	runtime, err := NewWASIRuntime(WASIRuntimeOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	module := InstalledModule{Manifest: pkg.Manifest, State: StateEnabled}

	state, err := ActivateMaterializedVersionWithHealth(
		context.Background(),
		root,
		manifest.ID,
		manifest.Version,
		time.Second,
		func(ctx context.Context, candidate CandidateActivation) error {
			return runtime.HealthCandidate(ctx, module, candidate)
		},
	)
	if err != nil {
		t.Fatalf("health-gated candidate activation: %v", err)
	}
	if state.ActiveVersion != manifest.Version {
		t.Fatalf("active version = %q, want %q", state.ActiveVersion, manifest.Version)
	}

	if err := runtime.Health(context.Background(), module); err != nil {
		t.Fatalf("active reference health: %v", err)
	}
	result, err := runtime.Invoke(context.Background(), module, Invocation{
		Method:     "GET",
		Path:       "/api/modules/alpha.reference/ping",
		Permission: "devices.read",
	})
	if err != nil {
		t.Fatalf("active reference invoke: %v", err)
	}
	if result.StatusCode != 200 || len(result.Body) != 0 {
		t.Fatalf("result = %+v, want status 200 and empty body", result)
	}
}

func TestSignedWASIReferenceBadCandidateDoesNotReplaceActiveVersion(t *testing.T) {
	root := filepath.Join(t.TempDir(), "modules")
	goodManifest := alphaReferenceManifest("1.0.0")
	goodPkg, goodPayload := signedReferencePayload(t, goodManifest, wasmEmpty)
	if _, err := MaterializePayloadRetrySafe(goodPkg, goodPayload, MaterializeOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := ActivateMaterializedVersion(root, goodManifest.ID, goodManifest.Version); err != nil {
		t.Fatal(err)
	}

	badManifest := alphaReferenceManifest("2.0.0")
	badPkg, badPayload := signedReferencePayload(t, badManifest, []byte("not-wasm"))
	if _, err := MaterializePayloadRetrySafe(badPkg, badPayload, MaterializeOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	runtime, err := NewWASIRuntime(WASIRuntimeOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	badModule := InstalledModule{Manifest: badPkg.Manifest, State: StateEnabled}
	if _, err := ActivateMaterializedVersionWithHealth(
		context.Background(), root, badManifest.ID, badManifest.Version, time.Second,
		func(ctx context.Context, candidate CandidateActivation) error {
			return runtime.HealthCandidate(ctx, badModule, candidate)
		},
	); err == nil {
		t.Fatal("malformed candidate unexpectedly passed health activation")
	}
	state, err := ReadActiveVersion(root, goodManifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.ActiveVersion != goodManifest.Version {
		t.Fatalf("failed candidate replaced active version: %+v", state)
	}
}

func alphaReferenceManifest(version string) Manifest {
	return Manifest{
		ID:          "alpha.reference",
		Name:        "Alpha Reference",
		Version:     version,
		APIVersion:  CurrentAPIVersion,
		Publisher:   "strata.alpha",
		Permissions: []string{"devices.read"},
		Runtime: &RuntimeSpec{
			Kind:           RuntimeKindWASI,
			Entrypoint:     "bin/module.wasm",
			MemoryMB:       16,
			TimeoutSeconds: 1,
			MaxConcurrency: 1,
			Network:        RuntimeNetworkNone,
		},
	}
}

func signedReferencePayload(t *testing.T, manifest Manifest, wasm []byte) (VerifiedPackage, ValidatedPayload) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	archive := makePayloadArchive(t, []payloadArchiveEntry{
		{name: "bin", typeflag: tar.TypeDir, mode: 0o750},
		{name: "bin/module.wasm", typeflag: tar.TypeReg, mode: 0o500, data: wasm},
	})
	packageBytes := makeSignedPackage(t, manifest, archive, "alpha-reference-key", privateKey, nil)
	trust := StaticPublisherTrustStore{
		manifest.Publisher + "\x00alpha-reference-key": {
			Publisher: manifest.Publisher,
			KeyID:     "alpha-reference-key",
			PublicKey: publicKey,
		},
	}
	pkg, err := VerifyPackage(packageBytes, trust)
	if err != nil {
		t.Fatalf("verify signed reference package: %v", err)
	}
	payload, err := ValidatePayload(pkg)
	if err != nil {
		t.Fatalf("validate signed reference payload: %v", err)
	}
	return pkg, payload
}
