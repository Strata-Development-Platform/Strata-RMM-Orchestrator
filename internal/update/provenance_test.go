package update

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestVerifySigstoreBytesRequiresVerifier(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses POSIX PATH fixture")
	}
	t.Setenv("PATH", t.TempDir())
	u := NewOrchestratorUpdater("1.0.0", "Strata-Development-Platform", "Strata-RMM-Orchestrator")
	if err := u.verifySigstoreBytes(context.Background(), []byte("payload"), []byte("bundle"), "v1.1.0"); err == nil || !strings.Contains(err.Error(), "cosign verifier is required") {
		t.Fatalf("verifySigstoreBytes() error = %v", err)
	}
}

func TestVerifySigstoreBytesPinsWorkflowIdentity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses POSIX executable fixture")
	}
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	cosign := filepath.Join(dir, "cosign")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$STRATA_ARGS_FILE\"\nexit 0\n"
	if err := os.WriteFile(cosign, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("STRATA_ARGS_FILE", argsFile)

	u := NewOrchestratorUpdater("1.0.0", "Strata-Development-Platform", "Strata-RMM-Orchestrator")
	if err := u.verifySigstoreBytes(context.Background(), []byte("payload"), []byte(`{"bundle":true}`), "v1.1.0"); err != nil {
		t.Fatalf("verifySigstoreBytes() error = %v", err)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	got := string(args)
	for _, required := range []string{
		"verify-blob",
		"--bundle",
		"--certificate-identity",
		"https://github.com/Strata-Development-Platform/Strata-RMM-Orchestrator/.github/workflows/publish-release.yml@refs/tags/v1.1.0",
		"--certificate-oidc-issuer",
		"https://token.actions.githubusercontent.com",
	} {
		if !strings.Contains(got, required) {
			t.Fatalf("cosign args %q missing %q", got, required)
		}
	}
}
