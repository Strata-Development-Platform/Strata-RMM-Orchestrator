package patch

import (
	"errors"
	"testing"
)

func TestNormalizeDeploymentIDs(t *testing.T) {
	got := normalizeDeploymentIDs([]string{" device-a ", "", "device-a", "device-b", "  ", "device-b"})
	want := []string{"device-a", "device-b"}
	if len(got) != len(want) {
		t.Fatalf("normalizeDeploymentIDs() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normalizeDeploymentIDs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestValidatePatchSelectionSnapshot(t *testing.T) {
	tests := []struct {
		name     string
		snapshot string
		patches  []string
		wantErr  error
	}{
		{
			name:     "all requested patches missing",
			snapshot: `{"installed":[],"missing":[{"id":"KB5001"},{"id":"pkg-2"}]}`,
			patches:  []string{"KB5001", "pkg-2"},
		},
		{
			name:     "requested patch not missing",
			snapshot: `{"installed":[],"missing":[{"id":"KB5001"}]}`,
			patches:  []string{"KB5001", "KB9999"},
			wantErr:  ErrPatchSelectionNotMissing,
		},
		{
			name:     "empty missing list",
			snapshot: `{"installed":[],"missing":[]}`,
			patches:  []string{"KB5001"},
			wantErr:  ErrPatchSelectionNotMissing,
		},
		{
			name:     "missing field absent",
			snapshot: `{"installed":[]}`,
			patches:  []string{"KB5001"},
			wantErr:  ErrPatchInventoryInvalid,
		},
		{
			name:     "malformed snapshot",
			snapshot: `{"missing":`,
			patches:  []string{"KB5001"},
			wantErr:  ErrPatchInventoryInvalid,
		},
		{
			name:     "malformed missing array",
			snapshot: `{"missing":"KB5001"}`,
			patches:  []string{"KB5001"},
			wantErr:  ErrPatchInventoryInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePatchSelectionSnapshot([]byte(tt.snapshot), tt.patches)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("validatePatchSelectionSnapshot() error = %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("validatePatchSelectionSnapshot() error = %v, want errors.Is(%v)", err, tt.wantErr)
			}
		})
	}
}
