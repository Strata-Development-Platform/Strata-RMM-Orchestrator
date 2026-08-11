package modules

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSignedWASIReferenceUpgradeAndRollbackBothExecute(t *testing.T) {
	root := filepath.Join(t.TempDir(), "modules")
	v1Manifest := alphaReferenceManifest("1.0.0")
	v2Manifest := alphaReferenceManifest("2.0.0")
	v1Pkg, v1Payload := signedReferencePayload(t, v1Manifest, wasmEmpty)
	v2Pkg, v2Payload := signedReferencePayload(t, v2Manifest, wasmEmpty)
	if _, err := MaterializePayloadRetrySafe(v1Pkg, v1Payload, MaterializeOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := MaterializePayloadRetrySafe(v2Pkg, v2Payload, MaterializeOptions{Root: root}); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry()
	installed, err := registry.Install(v1Pkg.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	enabledV1, err := registry.Enable(installed.Manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ActivateMaterializedVersion(root, v1Manifest.ID, v1Manifest.Version); err != nil {
		t.Fatal(err)
	}
	runtime, err := NewWASIRuntime(WASIRuntimeOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Invoke(context.Background(), enabledV1, Invocation{}); err != nil {
		t.Fatalf("v1 invocation: %v", err)
	}

	if _, err := registry.Disable(v1Manifest.ID, "upgrade"); err != nil {
		t.Fatal(err)
	}
	upgraded, err := registry.Upgrade(v1Manifest.ID, v2Pkg.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	v2State, err := ActivateMaterializedVersionWithHealth(
		context.Background(), root, v2Manifest.ID, v2Manifest.Version, time.Second,
		func(ctx context.Context, candidate CandidateActivation) error {
			return runtime.HealthCandidate(ctx, upgraded, candidate)
		},
	)
	if err != nil {
		t.Fatalf("v2 health activation: %v", err)
	}
	if v2State.ActiveVersion != "2.0.0" || v2State.PreviousVersion != "1.0.0" {
		t.Fatalf("unexpected v2 activation state: %+v", v2State)
	}
	enabledV2, err := registry.Enable(v2Manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Invoke(context.Background(), enabledV2, Invocation{}); err != nil {
		t.Fatalf("v2 invocation: %v", err)
	}

	if _, err := registry.Disable(v2Manifest.ID, "rollback"); err != nil {
		t.Fatal(err)
	}
	rolledBack, err := registry.Rollback(v2Manifest.ID, v1Pkg.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	rollbackState, err := ActivateExpectedPreviousVersion(root, v2Manifest.ID, v2State)
	if err != nil {
		t.Fatalf("filesystem rollback: %v", err)
	}
	if rollbackState.ActiveVersion != "1.0.0" || rollbackState.PreviousVersion != "2.0.0" {
		t.Fatalf("unexpected rollback activation state: %+v", rollbackState)
	}
	enabledRollback, err := registry.Enable(v1Manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if enabledRollback.Manifest.Version != rolledBack.Manifest.Version || enabledRollback.Manifest.Version != "1.0.0" {
		t.Fatalf("registry rollback version = %q", enabledRollback.Manifest.Version)
	}
	if _, err := runtime.Invoke(context.Background(), enabledRollback, Invocation{}); err != nil {
		t.Fatalf("rolled-back v1 invocation: %v", err)
	}
}
