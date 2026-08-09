package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repositoryScriptPath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "scripts", name))
}

func TestAlphaAcceptanceRequiresHTTPSCandidate(t *testing.T) {
	script := repositoryScriptPath(t, "alpha-acceptance.sh")
	cmd := exec.Command("bash", script, "preflight")
	cmd.Env = append(os.Environ(), "STRATA_ALPHA_URL=http://127.0.0.1:8080")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("preflight accepted an insecure HTTP Alpha URL")
	}
	if !strings.Contains(string(out), "must use https://") {
		t.Fatalf("unexpected error output: %s", out)
	}
}

func TestAlphaAcceptanceFaultsFailClosed(t *testing.T) {
	script := repositoryScriptPath(t, "alpha-acceptance.sh")
	cmd := exec.Command("bash", script, "fault", "postgres")
	cmd.Env = append(os.Environ(),
		"STRATA_ALPHA_URL=https://127.0.0.1",
		"STRATA_ALPHA_ALLOW_DESTRUCTIVE=0",
		"STRATA_ALPHA_FAULT_POSTGRES_CMD=true",
		"STRATA_ALPHA_RECOVER_POSTGRES_CMD=true",
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("fault injection ran without explicit destructive opt-in")
	}
	if !strings.Contains(string(out), "STRATA_ALPHA_ALLOW_DESTRUCTIVE=1") {
		t.Fatalf("unexpected error output: %s", out)
	}
}

func TestAlphaEvidenceMetadataIsImmutableAcrossFinalize(t *testing.T) {
	script := repositoryScriptPath(t, "alpha-acceptance.sh")
	evidenceDir := t.TempDir()
	run := func() {
		t.Helper()
		cmd := exec.Command("bash", script, "finalize")
		cmd.Env = append(os.Environ(),
			"STRATA_ALPHA_URL=https://127.0.0.1",
			"STRATA_ALPHA_EVIDENCE_DIR="+evidenceDir,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("finalize failed: %v\n%s", err, out)
		}
	}

	run()
	metadataPath := filepath.Join(evidenceDir, "metadata.env")
	before, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	run()
	after, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("finalize rewrote immutable candidate metadata\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestLoadHarnessDoesNotBypassEnrollmentOrTenancy(t *testing.T) {
	script := repositoryScriptPath(t, "loadtest.sh")
	body, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, forbidden := range []string{
		"nats pub",
		"tenant.00000000-0000-0000-0000-000000000001",
		"POST $API_URL/api/v1/enroll",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("load harness reintroduced trust-boundary bypass %q", forbidden)
		}
	}
	if !strings.Contains(text, "AUTH_TOKEN") {
		t.Fatal("authenticated load mode must require an explicit token")
	}
}
