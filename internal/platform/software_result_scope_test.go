package platform

import (
	"strings"
	"testing"
)

func TestParseSoftwareResultSubject(t *testing.T) {
	tenantID, deviceID, ok := parseSoftwareResultSubject("tenant.tenant-a.agent.device-1.software.result")
	if !ok {
		t.Fatal("valid software result subject rejected")
	}
	if tenantID != "tenant-a" || deviceID != "device-1" {
		t.Fatalf("identity = %q/%q, want tenant-a/device-1", tenantID, deviceID)
	}
}

func TestParseSoftwareResultSubjectRejectsSpoofableForms(t *testing.T) {
	for _, subject := range []string{
		"",
		"tenant.tenant-a.agent.device-1.result",
		"tenant.*.agent.device-1.software.result",
		"tenant.tenant-a.agent.*.software.result",
		"tenant.tenant-a.agent.device-1.patch.result",
		"tenant.tenant-a.agent.device-1.software.result.extra",
	} {
		if _, _, ok := parseSoftwareResultSubject(subject); ok {
			t.Fatalf("subject %q unexpectedly accepted", subject)
		}
	}
}

func TestBoundedSoftwareResultError(t *testing.T) {
	input := strings.Repeat("x", maxSoftwareResultErrorBytes+100)
	if got := boundedSoftwareResultError(input); len(got) != maxSoftwareResultErrorBytes {
		t.Fatalf("bounded error length = %d, want %d", len(got), maxSoftwareResultErrorBytes)
	}
}

func TestValidSoftwarePackageTypeFailsClosed(t *testing.T) {
	for _, value := range []string{"msi", "exe", "deb", "rpm", "appimage", "script"} {
		if !validSoftwarePackageType(value) {
			t.Fatalf("supported package type %q rejected", value)
		}
	}
	for _, value := range []string{"", "other", "binary", "cmd"} {
		if validSoftwarePackageType(value) {
			t.Fatalf("unsupported package type %q accepted", value)
		}
	}
}
