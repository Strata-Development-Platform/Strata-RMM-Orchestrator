package update

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func validDockerJournal(path string) DockerUpgradeJournal {
	now := time.Now().UTC().Truncate(time.Second)
	return DockerUpgradeJournal{
		Schema:               1,
		TransactionID:        "tx-123",
		State:                DockerUpgradePrepared,
		CurrentVersion:       "1.9.9",
		CandidateVersion:     "1.10.0",
		CurrentSourceSHA:     "1111111111111111111111111111111111111111",
		CandidateSourceSHA:   "2222222222222222222222222222222222222222",
		CurrentImage:         "ghcr.io/strata-development-platform/strata-rmm-orchestrator@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		CandidateImage:       "ghcr.io/strata-development-platform/strata-rmm-orchestrator@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		ManifestSHA256:       "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		SourceSchemaVersion:  95,
		TargetSchemaVersion:  96,
		ComposeProject:       "strata-rmm",
		ComposeFile:          filepath.Join(path, "docker-compose.install.yml"),
		Attempt:              0,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
}

func TestDockerUpgradeJournalRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state", "upgrade.json")
	journal := validDockerJournal(dir)
	if err := WriteDockerUpgradeJournal(path, journal); err != nil {
		t.Fatalf("WriteDockerUpgradeJournal() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("journal mode = %o, want 600", got)
	}
	got, err := ReadDockerUpgradeJournal(path)
	if err != nil {
		t.Fatalf("ReadDockerUpgradeJournal() error = %v", err)
	}
	if got.CandidateImage != journal.CandidateImage || got.State != DockerUpgradePrepared {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

func TestDockerUpgradeJournalRejectsUnsafeState(t *testing.T) {
	dir := t.TempDir()
	base := validDockerJournal(dir)
	tests := []struct {
		name   string
		mutate func(*DockerUpgradeJournal)
	}{
		{name: "mutable current", mutate: func(j *DockerUpgradeJournal) { j.CurrentImage = "example/orchestrator:latest" }},
		{name: "mutable candidate", mutate: func(j *DockerUpgradeJournal) { j.CandidateImage = "example/orchestrator:v1" }},
		{name: "same image", mutate: func(j *DockerUpgradeJournal) { j.CandidateImage = j.CurrentImage }},
		{name: "bad source sha", mutate: func(j *DockerUpgradeJournal) { j.CandidateSourceSHA = "master" }},
		{name: "bad manifest digest", mutate: func(j *DockerUpgradeJournal) { j.ManifestSHA256 = "deadbeef" }},
		{name: "relative compose path", mutate: func(j *DockerUpgradeJournal) { j.ComposeFile = "docker-compose.yml" }},
		{name: "attempt overflow", mutate: func(j *DockerUpgradeJournal) { j.Attempt = 11 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			journal := base
			test.mutate(&journal)
			if err := journal.Validate(); err == nil {
				t.Fatal("Validate() accepted unsafe journal")
			}
		})
	}
}

func TestReadDockerUpgradeJournalRejectsBroadPermissionsAndSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "upgrade.json")
	journal := validDockerJournal(dir)
	if err := WriteDockerUpgradeJournal(path, journal); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadDockerUpgradeJournal(path); err == nil {
		t.Fatal("ReadDockerUpgradeJournal() accepted broad permissions")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "upgrade-link.json")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadDockerUpgradeJournal(link); err == nil {
		t.Fatal("ReadDockerUpgradeJournal() accepted symlink")
	}
}
