package platform

import "testing"

func TestNormalizeOffboardingRequest(t *testing.T) {
	tests := []struct {
		name       string
		input      offboardingRequest
		wantDays   int
		wantReason string
		wantError  bool
	}{
		{
			name:       "defaults retention and trims reason",
			input:      offboardingRequest{Reason: "  contract ended  "},
			wantDays:   defaultOffboardingRetentionDays,
			wantReason: "contract ended",
		},
		{
			name:       "accepts bounded retention",
			input:      offboardingRequest{Reason: "requested by customer", RetentionDays: 365},
			wantDays:   365,
			wantReason: "requested by customer",
		},
		{
			name:      "rejects blank reason",
			input:     offboardingRequest{Reason: "  ", RetentionDays: 90},
			wantError: true,
		},
		{
			name:      "rejects short retention",
			input:     offboardingRequest{Reason: "test", RetentionDays: 29},
			wantError: true,
		},
		{
			name:      "rejects excessive retention",
			input:     offboardingRequest{Reason: "test", RetentionDays: 3651},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, validationError := normalizeOffboardingRequest(tt.input)
			if (validationError != "") != tt.wantError {
				t.Fatalf("validation error = %q, wantError %v", validationError, tt.wantError)
			}
			if tt.wantError {
				return
			}
			if got.RetentionDays != tt.wantDays {
				t.Fatalf("retention days = %d, want %d", got.RetentionDays, tt.wantDays)
			}
			if got.Reason != tt.wantReason {
				t.Fatalf("reason = %q, want %q", got.Reason, tt.wantReason)
			}
		})
	}
}
