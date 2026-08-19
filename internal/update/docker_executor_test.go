package update

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func writeFakeDocker(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "docker")
	script := `#!/bin/sh
set -eu
if [ "$1" = "inspect" ]; then
  printf '%s\n' "$FAKE_DOCKER_LIVE_IMAGE"
  exit 0
fi
if [ "$1" = "pull" ]; then
  printf '%s\n' "$2" >> "$FAKE_DOCKER_LOG"
  exit 0
fi
if [ "$1" != "compose" ]; then exit 9; fi
envfile=""
prev=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--env-file" ]; then envfile="$2"; shift 2; continue; fi
  if [ "$1" = "ps" ]; then printf 'container123\n'; exit 0; fi
  if [ "$1" = "exec" ]; then
    image="$(sed -n 's/^STRATA_ORCHESTRATOR_IMAGE=//p' "$envfile")"
    if [ "${FAKE_DOCKER_FAIL_IMAGE:-}" = "$image" ]; then exit 7; fi
    exit 0
  fi
  shift
done
exit 0
`
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
}

func dockerExecutorFixture(t *testing.T) (DockerUpgradeExecutor, OCIReleaseCandidate, PreflightResult, string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake Docker harness requires POSIX shell")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0755); err != nil { t.Fatal(err) }
	writeFakeDocker(t, bin)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	previous := "ghcr.io/strata-development-platform/strata-rmm-orchestrator@sha256:" + strings.Repeat("a", 64)
	candidateImage := "ghcr.io/strata-development-platform/strata-rmm-orchestrator@sha256:" + strings.Repeat("b", 64)
	t.Setenv("FAKE_DOCKER_LIVE_IMAGE", previous)
	t.Setenv("FAKE_DOCKER_LOG", filepath.Join(dir, "docker.log"))
	envFile := filepath.Join(dir, ".install.env")
	if err := os.WriteFile(envFile, []byte("STRATA_DOMAIN=rmm.example.test\nSTRATA_ORCHESTRATOR_IMAGE="+previous+"\n"), 0600); err != nil { t.Fatal(err) }
	composeFile := filepath.Join(dir, "docker-compose.install.yml")
	if err := os.WriteFile(composeFile, []byte("services: {}\n"), 0600); err != nil { t.Fatal(err) }
	executor := DockerUpgradeExecutor{
		ComposeFile: composeFile,
		EnvFile: envFile,
		Project: "strata-rmm",
		JournalFile: filepath.Join(dir, "upgrade", "transaction.json"),
		HealthTimeout: 50 * time.Millisecond,
	}
	candidate := OCIReleaseCandidate{
		Version: "1.10.0", SourceSHA: strings.Repeat("2", 40), SchemaCompatibility: "00096",
		Reference: "ghcr.io/strata-development-platform/strata-rmm-orchestrator",
		Digest: "sha256:"+strings.Repeat("b", 64), Image: candidateImage, ReleaseTag: "v1.10.0",
	}
	preflight := PreflightResult{Pass: true, SourceSchemaVersion: 95, TargetSchemaVersion: 96, Timestamp: time.Now().UTC()}
	return executor, candidate, preflight, previous, envFile
}

func TestDockerUpgradeExecutorSuccessCommitsImmutableCandidate(t *testing.T) {
	executor, candidate, preflight, _, envFile := dockerExecutorFixture(t)
	if err := executor.Apply(t.Context(), candidate, preflight, "1.9.9", strings.Repeat("1", 40), strings.Repeat("c", 64)); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	payload, err := os.ReadFile(envFile)
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(payload), "STRATA_ORCHESTRATOR_IMAGE="+candidate.Image) {
		t.Fatalf("candidate image was not persisted: %s", payload)
	}
	if _, err := os.Stat(executor.JournalFile); !os.IsNotExist(err) {
		t.Fatalf("successful transaction journal still present: %v", err)
	}
}

func TestDockerUpgradeExecutorCandidateFailureRestoresPreviousDigest(t *testing.T) {
	executor, candidate, preflight, previous, envFile := dockerExecutorFixture(t)
	t.Setenv("FAKE_DOCKER_FAIL_IMAGE", candidate.Image)
	err := executor.Apply(t.Context(), candidate, preflight, "1.9.9", strings.Repeat("1", 40), strings.Repeat("c", 64))
	if err == nil || !strings.Contains(err.Error(), "previous immutable image was restored") {
		t.Fatalf("Apply() error = %v", err)
	}
	payload, readErr := os.ReadFile(envFile)
	if readErr != nil { t.Fatal(readErr) }
	if !strings.Contains(string(payload), "STRATA_ORCHESTRATOR_IMAGE="+previous) {
		t.Fatalf("previous image was not restored: %s", payload)
	}
	journal, readErr := ReadDockerUpgradeJournal(executor.JournalFile)
	if readErr != nil { t.Fatal(readErr) }
	if journal.State != DockerUpgradeRolledBack {
		t.Fatalf("journal state = %q, want %q", journal.State, DockerUpgradeRolledBack)
	}
}

func TestDockerUpgradeExecutorBlocksCompetingUnresolvedTransaction(t *testing.T) {
	executor, candidate, preflight, previous, _ := dockerExecutorFixture(t)
	now := time.Now().UTC()
	journal := DockerUpgradeJournal{
		Schema: 1, TransactionID: "existing", State: DockerUpgradeApplying,
		CurrentVersion: "1.9.9", CandidateVersion: "1.10.0",
		CurrentSourceSHA: strings.Repeat("1", 40), CandidateSourceSHA: strings.Repeat("2", 40),
		CurrentImage: previous, CandidateImage: candidate.Image, ManifestSHA256: strings.Repeat("c", 64),
		SourceSchemaVersion: 95, TargetSchemaVersion: 96, ComposeProject: executor.Project,
		ComposeFile: executor.ComposeFile, CreatedAt: now, UpdatedAt: now,
	}
	if err := WriteDockerUpgradeJournal(executor.JournalFile, journal); err != nil { t.Fatal(err) }
	if err := executor.Apply(t.Context(), candidate, preflight, "1.9.9", strings.Repeat("1", 40), strings.Repeat("c", 64)); err == nil || !strings.Contains(err.Error(), "blocks competing apply") {
		t.Fatalf("Apply() error = %v", err)
	}
}
