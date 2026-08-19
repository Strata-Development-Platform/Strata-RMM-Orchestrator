package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// DockerPlan is the container equivalent of Plan. Candidate selection still
// flows through the same signed manifest, semantic-version policy, and runtime
// preflight used by the native update service.
type DockerPlan struct {
	CurrentVersion string               `json:"current_version"`
	Available      bool                 `json:"update_available"`
	Candidate      *OCIReleaseCandidate `json:"candidate,omitempty"`
	ManifestSHA256 string               `json:"manifest_sha256,omitempty"`
	Preflight      *PreflightResult     `json:"preflight,omitempty"`
}

func (u *OrchestratorUpdater) checkOCI(ctx context.Context, expectedRepository string) (*OCIReleaseCandidate, string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", u.owner, u.repo)
	req, err := newGitHubRequest(ctx, url)
	if err != nil {
		return nil, "", err
	}
	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("github api: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("github api returned %d", resp.StatusCode)
	}
	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, "", fmt.Errorf("decode release metadata: %w", err)
	}
	if release.TagName == "" {
		return nil, "", fmt.Errorf("release tag is missing")
	}
	manifestURL := releaseAssetURL(release, "release-manifest.json")
	bundleURL := releaseAssetURL(release, "release-manifest.json.sigstore.json")
	if manifestURL == "" || bundleURL == "" {
		return nil, "", fmt.Errorf("release is missing signed provenance manifest")
	}
	manifestBytes, err := u.fetchReleaseAsset(ctx, manifestURL, maxReleaseMetadataBytes)
	if err != nil {
		return nil, "", fmt.Errorf("fetch release manifest: %w", err)
	}
	bundle, err := u.fetchReleaseAsset(ctx, bundleURL, maxReleaseMetadataBytes)
	if err != nil {
		return nil, "", fmt.Errorf("fetch release manifest Sigstore bundle: %w", err)
	}
	if err := u.verifySigstoreBytes(ctx, manifestBytes, bundle, release.TagName); err != nil {
		return nil, "", fmt.Errorf("verify release manifest: %w", err)
	}
	var manifest ReleaseManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, "", fmt.Errorf("decode verified release manifest: %w", err)
	}
	if strings.TrimPrefix(release.TagName, "v") != manifest.Version {
		return nil, "", fmt.Errorf("release tag %q does not match signed manifest version %q", release.TagName, manifest.Version)
	}
	candidate, available, err := manifest.OCICandidate(u.currentVersion, expectedRepository)
	if err != nil {
		return nil, "", fmt.Errorf("select signed OCI candidate: %w", err)
	}
	if !available {
		return nil, "", nil
	}
	digest := sha256.Sum256(manifestBytes)
	return &candidate, hex.EncodeToString(digest[:]), nil
}

func newGitHubRequest(ctx context.Context, url string) (*httpRequest, error) {
	return makeHTTPRequest(ctx, url)
}

// The indirection below keeps Docker discovery using the same headers as native
// discovery without exporting HTTP details from the update package.
type httpRequest = http.Request

func makeHTTPRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "StrataRMM/1.0")
	return req, nil
}

func (s *Service) PlanDocker(ctx context.Context, expectedRepository string, includePreflight bool) (*DockerPlan, error) {
	candidate, manifestDigest, err := s.updater.checkOCI(ctx, expectedRepository)
	if err != nil {
		return nil, fmt.Errorf("discover verified Docker release: %w", err)
	}
	plan := &DockerPlan{CurrentVersion: s.updater.CurrentVersion(), Available: candidate != nil, Candidate: candidate, ManifestSHA256: manifestDigest}
	if candidate == nil || !includePreflight {
		return plan, nil
	}
	release := &OrchestratorRelease{Version: candidate.Version, SourceSHA: candidate.SourceSHA, SchemaCompatibility: candidate.SchemaCompatibility, ReleaseTag: candidate.ReleaseTag}
	result, err := s.RunPreflight(ctx, release)
	if err != nil {
		return nil, err
	}
	plan.Preflight = &result
	return plan, nil
}
