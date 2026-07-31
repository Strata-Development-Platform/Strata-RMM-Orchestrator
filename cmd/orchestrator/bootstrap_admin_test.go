package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadBootstrapPassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(path, []byte("correct horse battery staple\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readBootstrapPassword(path)
	if err != nil {
		t.Fatal(err)
	}
	defer clearBytes(got)
	if string(got) != "correct horse battery staple" {
		t.Fatalf("unexpected password contents")
	}
}

func TestReadBootstrapPasswordRejectsPermissiveMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(path, []byte("correct horse battery staple"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readBootstrapPassword(path); err == nil {
		t.Fatal("expected permissive password file to be rejected")
	}
}

func TestReadBootstrapPasswordRejectsOversizedValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "password")
	value := make([]byte, 73)
	for i := range value {
		value[i] = 'x'
	}
	if err := os.WriteFile(path, value, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readBootstrapPassword(path); err == nil {
		t.Fatal("expected oversized password to be rejected")
	}
}
