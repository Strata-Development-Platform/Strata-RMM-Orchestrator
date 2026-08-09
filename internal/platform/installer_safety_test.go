package platform

import (
	"os"
	"strings"
	"testing"
)

func TestSourceTreeAgentInstallerUsesScopedServerEnrollment(t *testing.T) {
	data, err := os.ReadFile("../../scripts/install.sh")
	if err != nil {
		t.Fatalf("read scripts/install.sh: %v", err)
	}
	script := string(data)

	required := []string{
		"$SERVER_URL/install.sh",
		"RMM_ENROLLMENT_TOKEN",
		"RMM_SERVER_URL",
		"--proto",
		"--tlsv1.2",
	}
	for _, token := range required {
		if !strings.Contains(script, token) {
			t.Errorf("installer is missing required safety/enrollment token %q", token)
		}
	}
}

func TestSourceTreeAgentInstallerHasNoLegacyDirectNATSOrRebootFlow(t *testing.T) {
	data, err := os.ReadFile("../../scripts/install.sh")
	if err != nil {
		t.Fatalf("read scripts/install.sh: %v", err)
	}
	script := string(data)

	forbidden := []string{
		"--nats-url",
		"NATS server address",
		"reboot now",
		"\nreboot\n",
		"go build -o",
	}
	lower := strings.ToLower(script)
	for _, token := range forbidden {
		if strings.Contains(lower, strings.ToLower(token)) {
			t.Errorf("installer contains forbidden legacy behavior %q", token)
		}
	}
}
