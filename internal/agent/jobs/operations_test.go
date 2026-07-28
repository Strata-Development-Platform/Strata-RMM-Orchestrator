package jobs

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRebootCommandByPlatform(t *testing.T) {
	tests := []struct {
		goos string
		name string
	}{
		{goos: "linux", name: "shutdown"},
		{goos: "darwin", name: "shutdown"},
		{goos: "windows", name: "shutdown.exe"},
	}
	for _, test := range tests {
		t.Run(test.goos, func(t *testing.T) {
			name, args, err := rebootCommand(test.goos, "maintenance")
			if err != nil {
				t.Fatalf("rebootCommand() error = %v", err)
			}
			if name != test.name || len(args) == 0 {
				t.Fatalf("rebootCommand() = %q %v", name, args)
			}
		})
	}
	if _, _, err := rebootCommand("plan9", "maintenance"); err == nil {
		t.Fatal("unsupported platform must fail")
	}
}

func TestServiceCommandDoesNotUseShell(t *testing.T) {
	name, args, err := serviceCommand("linux", "restart", "ssh.service")
	if err != nil {
		t.Fatalf("serviceCommand() error = %v", err)
	}
	if name != "systemctl" || !reflect.DeepEqual(args, []string{"restart", "ssh.service"}) {
		t.Fatalf("serviceCommand() = %q %v", name, args)
	}
	if _, _, err := serviceCommand("linux", "start", "bad\nservice"); err == nil {
		t.Fatal("newline in service identifier must be rejected")
	}
}

func TestProcessTerminationIsGracefulAndProtectsSystemPIDs(t *testing.T) {
	if _, _, err := processTerminateCommand("linux", 1); err == nil {
		t.Fatal("PID 1 must be protected")
	}
	name, args, err := processTerminateCommand("linux", 4242)
	if err != nil {
		t.Fatalf("processTerminateCommand() error = %v", err)
	}
	if name != "kill" || !reflect.DeepEqual(args, []string{"-TERM", "4242"}) {
		t.Fatalf("processTerminateCommand() = %q %v", name, args)
	}
}

func TestParseOperationRejectsUnknownFields(t *testing.T) {
	_, err := parseOperation(json.RawMessage(`{"action":"reboot","unknown":true}`))
	if err == nil {
		t.Fatal("unknown payload fields must be rejected")
	}
}
