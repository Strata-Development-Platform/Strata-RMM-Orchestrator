package update

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// OCIReleaseCandidate is the immutable container candidate selected from the
// already-verified release manifest. Callers must never substitute a tag.
type OCIReleaseCandidate struct {
	Version             string
	SourceSHA           string
	SchemaCompatibility string
	Reference           string
	Digest              string
	Image               string
	ReleaseTag          string
}

// OCICandidate selects the single authoritative OCI image from a validated
// manifest and binds it to the same semantic upgrade decision used by native
// artifacts. expectedRepository is the canonical registry/repository configured
// by the deployment; a signed manifest cannot redirect upgrades elsewhere.
func (m ReleaseManifest) OCICandidate(currentVersion, expectedRepository string) (OCIReleaseCandidate, bool, error) {
	allowed, err := m.AllowsUpgradeFrom(currentVersion)
	if err != nil {
		return OCIReleaseCandidate{}, false, err
	}
	if !allowed {
		return OCIReleaseCandidate{}, false, nil
	}
	if len(m.OCIImages) != 1 {
		return OCIReleaseCandidate{}, false, fmt.Errorf("release manifest must contain exactly one authoritative OCI image")
	}
	image := m.OCIImages[0]
	platform := "linux/" + runtime.GOARCH
	if runtime.GOOS != "linux" {
		return OCIReleaseCandidate{}, false, fmt.Errorf("Docker promoted-release apply is supported only on linux hosts")
	}
	found := false
	for _, value := range image.Platforms {
		if value == platform {
			found = true
			break
		}
	}
	if !found {
		return OCIReleaseCandidate{}, false, fmt.Errorf("signed OCI image does not support platform %s", platform)
	}
	if image.Signature != "sigstore-keyless" {
		return OCIReleaseCandidate{}, false, fmt.Errorf("signed OCI image has unsupported signature policy %q", image.Signature)
	}
	if strings.Contains(image.Reference, "@") || strings.Contains(image.Reference, "://") {
		return OCIReleaseCandidate{}, false, fmt.Errorf("signed OCI repository reference is not canonical")
	}
	expectedRepository = strings.TrimSpace(strings.ToLower(expectedRepository))
	if expectedRepository == "" || strings.Contains(expectedRepository, "@") || strings.Contains(expectedRepository, "://") {
		return OCIReleaseCandidate{}, false, fmt.Errorf("expected OCI repository is not canonical")
	}
	if strings.ToLower(image.Reference) != expectedRepository {
		return OCIReleaseCandidate{}, false, fmt.Errorf("signed OCI repository %q does not match configured repository %q", image.Reference, expectedRepository)
	}
	return OCIReleaseCandidate{
		Version:             m.Version,
		SourceSHA:           m.SourceSHA,
		SchemaCompatibility: m.SchemaCompatibility,
		Reference:           image.Reference,
		Digest:              image.Digest,
		Image:               image.Reference + "@" + image.Digest,
		ReleaseTag:          "v" + strings.TrimPrefix(m.Version, "v"),
	}, true, nil
}

// VerifyOCICandidate proves the selected immutable digest was signed by the
// protected release workflow for the same semantic release tag.
func (u *OrchestratorUpdater) VerifyOCICandidate(ctx context.Context, candidate OCIReleaseCandidate) error {
	if candidate.Image == "" || candidate.ReleaseTag == "" || !strings.Contains(candidate.Image, "@sha256:") {
		return fmt.Errorf("fully bound immutable OCI candidate is required")
	}
	cosignPath, err := exec.LookPath("cosign")
	if err != nil {
		return fmt.Errorf("cosign verifier is required for OCI verification: %w", err)
	}
	identity := fmt.Sprintf("https://github.com/%s/%s/.github/workflows/publish-release.yml@refs/tags/%s", u.owner, u.repo, candidate.ReleaseTag)
	cmd := exec.CommandContext(ctx, cosignPath,
		"verify",
		"--certificate-identity", identity,
		"--certificate-oidc-issuer", "https://token.actions.githubusercontent.com",
		candidate.Image,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("OCI Sigstore verification failed: %w: %s", err, string(output))
	}
	return nil
}
