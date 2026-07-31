package update

import "testing"

func TestChecksumForFile(t *testing.T) {
	const checksum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	manifest := checksum + "  strata-rmm-orchestrator-1.2.3-linux-amd64\n"

	if got := checksumForFile(manifest, "strata-rmm-orchestrator-1.2.3-linux-amd64"); got != checksum {
		t.Fatalf("checksumForFile() = %q, want %q", got, checksum)
	}
}

func TestChecksumForFileRequiresExactFilename(t *testing.T) {
	const checksum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	manifest := checksum + "  attacker-strata-rmm-orchestrator-1.2.3-linux-amd64\n"

	if got := checksumForFile(manifest, "strata-rmm-orchestrator-1.2.3-linux-amd64"); got != "" {
		t.Fatalf("checksumForFile() accepted a partial filename match: %q", got)
	}
}

func TestChecksumForFileRejectsInvalidSHA256(t *testing.T) {
	if got := checksumForFile("not-a-sha  strata-rmm-orchestrator-1.2.3-linux-amd64\n", "strata-rmm-orchestrator-1.2.3-linux-amd64"); got != "" {
		t.Fatalf("checksumForFile() accepted invalid SHA-256: %q", got)
	}
}
