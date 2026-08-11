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

func TestNormalizePatchResultErrorIsBounded(t *testing.T) {
	input := strings.Repeat("x", maxPatchResultErrorBytes+100)
	got := normalizePatchResultError(input)
	if len(got) != maxPatchResultErrorBytes {
		t.Fatalf("bounded error length = %d, want %d", len(got), maxPatchResultErrorBytes)
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
