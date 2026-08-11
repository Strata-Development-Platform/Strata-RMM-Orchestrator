package software

import (
	"strings"
	"testing"
)

func TestSoftwareDurableNameIsStableAndIdentitySpecific(t *testing.T) {
	a := softwareDurableName("tenant-a", "agent-1")
	b := softwareDurableName("tenant-a", "agent-1")
	c := softwareDurableName("tenant-a", "agent-2")
	if a != b {
		t.Fatalf("durable name changed: %q != %q", a, b)
	}
	if a == c {
		t.Fatalf("distinct agent identity produced same durable name: %q", a)
	}
	if !strings.HasPrefix(a, "software_") {
		t.Fatalf("durable name = %q, want software_ prefix", a)
	}
}

func TestValidateSoftwareCommandFailsClosed(t *testing.T) {
	base := SoftwareCommand{
		Type:         "software_install",
		DeploymentID: "dep-1",
		Action:       "install",
		SourceURL:    "https://example.invalid/pkg",
		PackageType:  "exe",
		Timeout:      600,
	}
	if err := validateSoftwareCommand(base); err != nil {
		t.Fatalf("valid command rejected: %v", err)
	}

	cases := []SoftwareCommand{
		func() SoftwareCommand { c := base; c.DeploymentID = ""; return c }(),
		func() SoftwareCommand { c := base; c.Action = "execute"; return c }(),
		func() SoftwareCommand { c := base; c.Type = "software_uninstall"; return c }(),
		func() SoftwareCommand { c := base; c.SourceURL = ""; return c }(),
		func() SoftwareCommand { c := base; c.PackageType = "other"; return c }(),
		func() SoftwareCommand { c := base; c.Timeout = -1; return c }(),
		func() SoftwareCommand { c := base; c.Timeout = 7201; return c }(),
	}
	for i, cmd := range cases {
		if err := validateSoftwareCommand(cmd); err == nil {
			t.Fatalf("invalid command %d unexpectedly accepted: %+v", i, cmd)
		}
	}
}

func TestSplitSoftwareArgsDoesNotInterpretShellOperators(t *testing.T) {
	got := splitSoftwareArgs("--quiet ; touch /tmp/pwned && whoami")
	want := []string{"--quiet", ";", "touch", "/tmp/pwned", "&&", "whoami"}
	if len(got) != len(want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("args[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSoftwareCommandKeySeparatesActions(t *testing.T) {
	install := softwareCommandKey(SoftwareCommand{DeploymentID: "dep-1", Action: "install"})
	uninstall := softwareCommandKey(SoftwareCommand{DeploymentID: "dep-1", Action: "uninstall"})
	if install == uninstall {
		t.Fatal("install and uninstall must not share a dedupe key")
	}
}
