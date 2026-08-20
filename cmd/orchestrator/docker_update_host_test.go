package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func TestNewProductCommandReplacesLegacyUpdateChild(t *testing.T) {
	cmd := NewProductCommand(context.Background(), "1.2.3", "deadbeef", zap.NewNop())
	updates := 0
	for _, child := range cmd.Commands() {
		if child.Name() != "update" {
			continue
		}
		updates++
		if child.Short != "Check and apply orchestrator updates" {
			t.Fatalf("unexpected shipped update child: %q", child.Short)
		}
	}
	if updates != 1 {
		t.Fatalf("shipped product must expose exactly one update command, got %d", updates)
	}
}

func TestValidateDockerHostUpdatePaths(t *testing.T) {
	dir := t.TempDir()
	compose := filepath.Join(dir, "docker-compose.install.yml")
	env := filepath.Join(dir, ".install.env")
	journal := filepath.Join(dir, "docker-upgrade.json")
	if err := os.WriteFile(compose, []byte("services: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env, []byte("STRATA_ORCHESTRATOR_IMAGE=ghcr.io/example/repo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n"), 0600); err != nil {
		t.Fatal(err)
	}
	paths, err := validateDockerHostUpdatePaths(compose, env, journal)
	if err != nil {
		t.Fatalf("valid protected paths rejected: %v", err)
	}
	if paths.compose != compose || paths.env != env || paths.journal != journal {
		t.Fatalf("unexpected normalized paths: %+v", paths)
	}
}

func TestValidateDockerHostUpdatePathsRejectsUnsafeInputs(t *testing.T) {
	dir := t.TempDir()
	compose := filepath.Join(dir, "docker-compose.install.yml")
	env := filepath.Join(dir, ".install.env")
	journal := filepath.Join(dir, "docker-upgrade.json")
	if err := os.WriteFile(compose, []byte("services: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env, []byte("x=y\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := validateDockerHostUpdatePaths(compose, env, journal); err == nil {
		t.Fatal("broad env-file permissions unexpectedly accepted")
	}
	if err := os.Chmod(env, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateDockerHostUpdatePaths("relative.yml", env, journal); err == nil {
		t.Fatal("relative compose path unexpectedly accepted")
	}
	link := filepath.Join(dir, "compose-link.yml")
	if err := os.Symlink(compose, link); err != nil {
		t.Fatal(err)
	}
	if _, err := validateDockerHostUpdatePaths(link, env, journal); err == nil {
		t.Fatal("symlink compose path unexpectedly accepted")
	}
}
