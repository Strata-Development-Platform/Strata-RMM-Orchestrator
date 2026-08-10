package modules

import (
	"archive/tar"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestActivateMaterializedVersionWithHealthCommitsAfterSuccess(t *testing.T) {
	root := filepath.Join(t.TempDir(), "modules")
	materializeActivationVersion(t, root, "1.0.0")

	called := false
	state, err := ActivateMaterializedVersionWithHealth(context.Background(), root, testPackageManifest().ID, "1.0.0", time.Second, func(ctx context.Context, candidate CandidateActivation) error {
		called = true
		if candidate.Version != "1.0.0" || candidate.Path != filepath.Join(root, candidate.ModuleID, "1.0.0") {
			t.Fatalf("unexpected candidate: %#v", candidate)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ActivateMaterializedVersionWithHealth returned error: %v", err)
	}
	if !called {
		t.Fatal("health checker was not called")
	}
	if state.ActiveVersion != "1.0.0" || state.PreviousVersion != "" {
		t.Fatalf("unexpected activation state: %#v", state)
	}
	stored, err := ReadActiveVersion(root, testPackageManifest().ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored != state {
		t.Fatalf("stored state = %#v, want %#v", stored, state)
	}
}

func TestActivateMaterializedVersionWithHealthFailurePreservesPriorState(t *testing.T) {
	root := filepath.Join(t.TempDir(), "modules")
	materializeActivationVersion(t, root, "1.0.0")
	materializeActivationVersion(t, root, "2.0.0")
	if _, err := ActivateMaterializedVersion(root, testPackageManifest().ID, "1.0.0"); err != nil {
		t.Fatal(err)
	}

	_, err := ActivateMaterializedVersionWithHealth(context.Background(), root, testPackageManifest().ID, "2.0.0", time.Second, func(context.Context, CandidateActivation) error {
		return errors.New("not ready")
	})
	if !errors.Is(err, ErrCandidateUnhealthy) {
		t.Fatalf("activation error = %v, want ErrCandidateUnhealthy", err)
	}
	stored, err := ReadActiveVersion(root, testPackageManifest().ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ActiveVersion != "1.0.0" || stored.PreviousVersion != "" {
		t.Fatalf("failed health check changed activation state: %#v", stored)
	}
}

func TestActivateMaterializedVersionWithHealthTimeoutPreservesPriorState(t *testing.T) {
	root := filepath.Join(t.TempDir(), "modules")
	materializeActivationVersion(t, root, "1.0.0")
	materializeActivationVersion(t, root, "2.0.0")
	if _, err := ActivateMaterializedVersion(root, testPackageManifest().ID, "1.0.0"); err != nil {
		t.Fatal(err)
	}

	_, err := ActivateMaterializedVersionWithHealth(context.Background(), root, testPackageManifest().ID, "2.0.0", 20*time.Millisecond, func(ctx context.Context, _ CandidateActivation) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if !errors.Is(err, ErrCandidateHealthTimeout) {
		t.Fatalf("activation error = %v, want ErrCandidateHealthTimeout", err)
	}
	stored, err := ReadActiveVersion(root, testPackageManifest().ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ActiveVersion != "1.0.0" || stored.PreviousVersion != "" {
		t.Fatalf("timed-out health check changed activation state: %#v", stored)
	}
}

func TestActivateMaterializedVersionWithHealthRejectsMissingCandidateBeforeCheck(t *testing.T) {
	root := filepath.Join(t.TempDir(), "modules")
	materializeActivationVersion(t, root, "1.0.0")
	called := false
	_, err := ActivateMaterializedVersionWithHealth(context.Background(), root, testPackageManifest().ID, "9.9.9", time.Second, func(context.Context, CandidateActivation) error {
		called = true
		return nil
	})
	if err == nil {
		t.Fatal("missing candidate unexpectedly activated")
	}
	if called {
		t.Fatal("health checker ran for missing materialized candidate")
	}
}

func TestActivateMaterializedVersionWithHealthHonorsActivationLock(t *testing.T) {
	root := filepath.Join(t.TempDir(), "modules")
	materializeActivationVersion(t, root, "1.0.0")
	moduleDir := filepath.Join(root, testPackageManifest().ID)
	lockPath := filepath.Join(moduleDir, ".activation.lock")
	if err := os.WriteFile(lockPath, []byte("held"), 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	_, err := ActivateMaterializedVersionWithHealth(context.Background(), root, testPackageManifest().ID, "1.0.0", time.Second, func(context.Context, CandidateActivation) error {
		called = true
		return nil
	})
	if !errors.Is(err, ErrActivationInProgress) {
		t.Fatalf("activation error = %v, want ErrActivationInProgress", err)
	}
	if called {
		t.Fatal("health checker ran while activation lock was held")
	}
}

func TestActivateMaterializedVersionWithHealthRequiresBoundedChecker(t *testing.T) {
	root := filepath.Join(t.TempDir(), "modules")
	materializeActivationVersion(t, root, "1.0.0")
	if _, err := ActivateMaterializedVersionWithHealth(context.Background(), root, testPackageManifest().ID, "1.0.0", time.Second, nil); !errors.Is(err, ErrCandidateHealthCheckRequired) {
		t.Fatalf("nil checker error = %v", err)
	}
	if _, err := ActivateMaterializedVersionWithHealth(context.Background(), root, testPackageManifest().ID, "1.0.0", 0, func(context.Context, CandidateActivation) error { return nil }); err == nil {
		t.Fatal("zero timeout was accepted")
	}
	if _, err := ActivateMaterializedVersionWithHealth(context.Background(), root, testPackageManifest().ID, "1.0.0", maxCandidateHealthTimeout+time.Nanosecond, func(context.Context, CandidateActivation) error { return nil }); err == nil {
		t.Fatal("oversized timeout was accepted")
	}
}

func materializeActivationVersion(t *testing.T, root, version string) {
	t.Helper()
	pkg, payload := testVerifiedMaterializePayload(t, version, []payloadArchiveEntry{
		{name: "bin/module", typeflag: tar.TypeReg, mode: 0o755, data: []byte("binary-" + version)},
	})
	if _, err := MaterializePayload(pkg, payload, MaterializeOptions{Root: root}); err != nil {
		t.Fatalf("materialize %s: %v", version, err)
	}
}
