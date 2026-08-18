package update

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

const maxReleaseMetadataBytes = 4 << 20

func releaseAssetURL(release GitHubRelease, name string) string {
	for _, asset := range release.Assets {
		if asset.Name == name {
			return asset.BrowserDownloadURL
		}
	}
	return ""
}

func (u *OrchestratorUpdater) fetchReleaseAsset(ctx context.Context, url string, maxBytes int64) ([]byte, error) {
	if url == "" {
		return nil, fmt.Errorf("release asset URL is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create release asset request: %w", err)
	}
	req.Header.Set("User-Agent", "StrataRMM/1.0")
	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download release asset: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("release asset returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read release asset: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("release asset exceeds %d-byte metadata limit", maxBytes)
	}
	return body, nil
}

func (u *OrchestratorUpdater) verifySigstoreBytes(ctx context.Context, payload, bundle []byte, tag string) error {
	dir, err := os.MkdirTemp("", "strata-release-verify-*")
	if err != nil {
		return fmt.Errorf("create signature verification directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	payloadPath := filepath.Join(dir, "payload")
	bundlePath := filepath.Join(dir, "payload.sigstore.json")
	if err := os.WriteFile(payloadPath, payload, 0600); err != nil {
		return fmt.Errorf("write signature payload: %w", err)
	}
	if err := os.WriteFile(bundlePath, bundle, 0600); err != nil {
		return fmt.Errorf("write Sigstore bundle: %w", err)
	}
	return u.verifySigstoreFileWithBundle(ctx, payloadPath, bundlePath, tag)
}

func (u *OrchestratorUpdater) verifySigstoreFile(ctx context.Context, payloadPath, bundleURL, tag string) error {
	bundle, err := u.fetchReleaseAsset(ctx, bundleURL, maxReleaseMetadataBytes)
	if err != nil {
		return fmt.Errorf("download Sigstore bundle: %w", err)
	}
	dir, err := os.MkdirTemp("", "strata-release-bundle-*")
	if err != nil {
		return fmt.Errorf("create bundle verification directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	bundlePath := filepath.Join(dir, "artifact.sigstore.json")
	if err := os.WriteFile(bundlePath, bundle, 0600); err != nil {
		return fmt.Errorf("write Sigstore bundle: %w", err)
	}
	return u.verifySigstoreFileWithBundle(ctx, payloadPath, bundlePath, tag)
}

func (u *OrchestratorUpdater) verifySigstoreFileWithBundle(ctx context.Context, payloadPath, bundlePath, tag string) error {
	cosignPath, err := exec.LookPath("cosign")
	if err != nil {
		return fmt.Errorf("cosign verifier is required for release verification: %w", err)
	}
	identity := fmt.Sprintf("https://github.com/%s/%s/.github/workflows/publish-release.yml@refs/tags/%s", u.owner, u.repo, tag)
	cmd := exec.CommandContext(ctx, cosignPath,
		"verify-blob",
		"--bundle", bundlePath,
		"--certificate-identity", identity,
		"--certificate-oidc-issuer", "https://token.actions.githubusercontent.com",
		payloadPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sigstore verification failed: %w: %s", err, string(output))
	}
	return nil
}
