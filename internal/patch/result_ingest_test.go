package patch

import (
	"strings"
	"testing"
)

func TestPatchResultTransportIdentityUsesSubject(t *testing.T) {
	tenantID, deviceID, err := patchResultTransportIdentity("tenant.tenant-a.agent.device-1.patch.result")
	if err != nil {
		t.Fatalf("parse subject: %v", err)
	}
	if tenantID != "tenant-a" || deviceID != "device-1" {
		t.Fatalf("identity = %q/%q, want tenant-a/device-1", tenantID, deviceID)
	}
}

func TestPatchInventoryTransportIdentityUsesSubject(t *testing.T) {
	tenantID, deviceID, err := patchInventoryTransportIdentity("tenant.tenant-a.agent.device-1.patch.inventory")
	if err != nil {
		t.Fatalf("parse inventory subject: %v", err)
	}
	if tenantID != "tenant-a" || deviceID != "device-1" {
		t.Fatalf("identity = %q/%q, want tenant-a/device-1", tenantID, deviceID)
	}
}

func TestPatchResultTransportIdentityRejectsMalformedSubjects(t *testing.T) {
	for _, subject := range []string{
		"",
		"tenant.tenant-a.agent.device-1.patch",
		"tenant.tenant-a.device.device-1.patch.result",
		"tenant.*.agent.device-1.patch.result",
		"tenant.tenant-a.agent.*.patch.result",
		"tenant.tenant-a.agent.device-1.software.result",
	} {
		if _, _, err := patchResultTransportIdentity(subject); err == nil {
			t.Fatalf("subject %q unexpectedly accepted", subject)
		}
	}
}

func TestPatchInventoryTransportIdentityRejectsResultSubject(t *testing.T) {
	if _, _, err := patchInventoryTransportIdentity("tenant.tenant-a.agent.device-1.patch.result"); err == nil {
		t.Fatal("patch result subject unexpectedly accepted as inventory")
	}
}

func TestNormalizePatchResultErrorIsBounded(t *testing.T) {
	input := strings.Repeat("x", maxPatchResultErrorBytes+100)
	got := normalizePatchResultError(input)
	if len(got) != maxPatchResultErrorBytes {
		t.Fatalf("bounded error length = %d, want %d", len(got), maxPatchResultErrorBytes)
	}
}

func TestMaxPatchAttemptsIncludesInitialAttemptAndBoundsInvalidPolicy(t *testing.T) {
	tests := []struct {
		maxRetries int
		want       int
	}{
		{maxRetries: -10, want: 1},
		{maxRetries: -1, want: 1},
		{maxRetries: 0, want: 1},
		{maxRetries: 1, want: 2},
		{maxRetries: 3, want: 4},
	}
	for _, test := range tests {
		if got := maxPatchAttempts(test.maxRetries); got != test.want {
			t.Fatalf("maxPatchAttempts(%d) = %d, want %d", test.maxRetries, got, test.want)
		}
	}
}

func TestValidPatchResultStatusAllowsOnlyAgentTerminalResults(t *testing.T) {
	for _, status := range []PatchStatus{StatusInstalled, StatusFailed, StatusRebootReq} {
		if !validPatchResultStatus(status) {
			t.Fatalf("status %q should be accepted", status)
		}
	}
	for _, status := range []PatchStatus{StatusPending, StatusApproved, StatusDeploying, "unknown"} {
		if validPatchResultStatus(status) {
			t.Fatalf("status %q should be rejected", status)
		}
	}
}
