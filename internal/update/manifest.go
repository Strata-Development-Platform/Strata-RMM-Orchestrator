package update

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const ReleaseManifestSchemaVersion = 1

type ReleaseManifest struct {
	SchemaVersion         int               `json:"schema_version"`
	Version               string            `json:"version"`
	SourceSHA             string            `json:"source_sha"`
	BuildTimestamp        string            `json:"build_timestamp"`
	SchemaCompatibility   string            `json:"schema_compatibility"`
	MinimumUpgradeVersion string            `json:"minimum_upgrade_version"`
	Channel               string            `json:"channel"`
	Artifacts             []ReleaseArtifact `json:"artifacts"`
	OCIImages             []ReleaseOCIImage `json:"oci_images"`
}

type ReleaseArtifact struct {
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	OS             string `json:"os,omitempty"`
	Arch           string `json:"arch,omitempty"`
	Size           int64  `json:"size"`
	SHA256         string `json:"sha256"`
	SigstoreBundle string `json:"sigstore_bundle"`
}

type ReleaseOCIImage struct {
	Reference string   `json:"reference"`
	Digest    string   `json:"digest"`
	Platforms []string `json:"platforms"`
	Signature string   `json:"signature"`
}

func (m ReleaseManifest) Validate() error {
	if m.SchemaVersion != ReleaseManifestSchemaVersion {
		return fmt.Errorf("unsupported release manifest schema version %d", m.SchemaVersion)
	}
	if _, err := parseSemanticVersion(m.Version); err != nil {
		return fmt.Errorf("invalid release manifest version: %w", err)
	}
	if _, err := parseSemanticVersion(m.MinimumUpgradeVersion); err != nil {
		return fmt.Errorf("invalid minimum upgrade version: %w", err)
	}
	if len(m.SourceSHA) != 40 || !isHex(m.SourceSHA) {
		return fmt.Errorf("source_sha must be a 40-character git commit SHA")
	}
	if _, err := time.Parse(time.RFC3339, m.BuildTimestamp); err != nil {
		return fmt.Errorf("invalid build_timestamp: %w", err)
	}
	if strings.TrimSpace(m.SchemaCompatibility) == "" {
		return fmt.Errorf("schema_compatibility is required")
	}
	if m.Channel != "stable" && m.Channel != "prerelease" {
		return fmt.Errorf("unsupported release channel %q", m.Channel)
	}
	if len(m.Artifacts) == 0 {
		return fmt.Errorf("release manifest contains no artifacts")
	}
	if len(m.OCIImages) != 1 {
		return fmt.Errorf("release manifest must contain exactly one authoritative OCI image")
	}

	seen := make(map[string]struct{}, len(m.Artifacts))
	for index, artifact := range m.Artifacts {
		if strings.TrimSpace(artifact.Name) == "" {
			return fmt.Errorf("artifact %d has no name", index)
		}
		if _, exists := seen[artifact.Name]; exists {
			return fmt.Errorf("duplicate artifact %q", artifact.Name)
		}
		seen[artifact.Name] = struct{}{}
		if strings.TrimSpace(artifact.Kind) == "" {
			return fmt.Errorf("artifact %q has no kind", artifact.Name)
		}
		if artifact.Size <= 0 {
			return fmt.Errorf("artifact %q has invalid size", artifact.Name)
		}
		if len(artifact.SHA256) != 64 || !isHex(artifact.SHA256) {
			return fmt.Errorf("artifact %q has invalid sha256", artifact.Name)
		}
		if artifact.SigstoreBundle != artifact.Name+".sigstore.json" {
			return fmt.Errorf("artifact %q has non-canonical Sigstore bundle name", artifact.Name)
		}
	}

	image := m.OCIImages[0]
	if strings.TrimSpace(image.Reference) == "" || strings.Contains(image.Reference, "://") || strings.Contains(image.Reference, "@") {
		return fmt.Errorf("OCI image reference must be an untagged registry/repository reference")
	}
	if !strings.HasPrefix(image.Digest, "sha256:") || len(image.Digest) != len("sha256:")+64 || !isHex(strings.TrimPrefix(image.Digest, "sha256:")) {
		return fmt.Errorf("OCI image digest must be an immutable sha256 digest")
	}
	if len(image.Platforms) != 2 || image.Platforms[0] != "linux/amd64" || image.Platforms[1] != "linux/arm64" {
		return fmt.Errorf("OCI image platforms must be linux/amd64 and linux/arm64")
	}
	if image.Signature != "sigstore-keyless" {
		return fmt.Errorf("OCI image must use the canonical Sigstore keyless signature")
	}
	return nil
}

func (m ReleaseManifest) AllowsUpgradeFrom(currentVersion string) (bool, error) {
	if err := m.Validate(); err != nil {
		return false, err
	}
	minimumComparison, err := compareSemanticVersions(currentVersion, m.MinimumUpgradeVersion)
	if err != nil {
		return false, fmt.Errorf("compare current version to minimum upgrade version: %w", err)
	}
	if minimumComparison < 0 {
		return false, nil
	}
	candidateComparison, err := compareSemanticVersions(m.Version, currentVersion)
	if err != nil {
		return false, fmt.Errorf("compare candidate version to current version: %w", err)
	}
	return candidateComparison > 0, nil
}

func (m ReleaseManifest) Artifact(name string) (ReleaseArtifact, bool) {
	for _, artifact := range m.Artifacts {
		if artifact.Name == name {
			return artifact, true
		}
	}
	return ReleaseArtifact{}, false
}

func isHex(value string) bool {
	_, err := hex.DecodeString(value)
	return err == nil
}
