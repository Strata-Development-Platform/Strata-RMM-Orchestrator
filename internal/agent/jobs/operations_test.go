package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
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

func TestPatchMutationRejectsCancellationBeforeMutation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	mutationCtx, mutationCancel, err := beginPatchMutationWithTimeout(ctx, time.Minute)
	if mutationCancel != nil {
		mutationCancel()
	}
	if mutationCtx != nil {
		t.Fatal("cancelled command must not enter patch mutation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestPatchMutationSurvivesRoutineParentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	mutationCtx, mutationCancel, err := beginPatchMutationWithTimeout(ctx, time.Second)
	if err != nil {
		t.Fatalf("beginPatchMutationWithTimeout() error = %v", err)
	}
	defer mutationCancel()

	cancel()
	select {
	case <-mutationCtx.Done():
		t.Fatalf("routine parent cancellation must not interrupt active mutation: %v", mutationCtx.Err())
	case <-time.After(20 * time.Millisecond):
	}
}

func TestPatchMutationSafetyCeilingProducesDeadline(t *testing.T) {
	mutationCtx, cancel, err := beginPatchMutationWithTimeout(context.Background(), 10*time.Millisecond)
	if err != nil {
		t.Fatalf("beginPatchMutationWithTimeout() error = %v", err)
	}
	defer cancel()
	<-mutationCtx.Done()
	if !errors.Is(mutationCtx.Err(), context.DeadlineExceeded) {
		t.Fatalf("expected safety ceiling deadline, got %v", mutationCtx.Err())
	}
}

func TestPatchMutationCeilingExceedsLegacyTenMinuteLimit(t *testing.T) {
	if patchMutationTimeout <= 10*time.Minute {
		t.Fatalf("patch mutation ceiling %s must exceed legacy 10 minute transaction kill", patchMutationTimeout)
	}
}

func TestLinuxRebootRequiredSignals(t *testing.T) {
	tests := []struct {
		name       string
		marker     bool
		pm         string
		exitCode   int
		required   bool
	}{
		{name: "debian marker", marker: true, pm: "apt", exitCode: 0, required: true},
		{name: "dnf needs restart", marker: false, pm: "dnf", exitCode: 1, required: true},
		{name: "yum needs restart", marker: false, pm: "yum", exitCode: 1, required: true},
		{name: "dnf clean", marker: false, pm: "dnf", exitCode: 0, required: false},
		{name: "apt without marker", marker: false, pm: "apt", exitCode: 1, required: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := linuxRebootRequiredFromSignals(test.marker, test.pm, test.exitCode); got != test.required {
				t.Fatalf("linuxRebootRequiredFromSignals() = %v, want %v", got, test.required)
			}
		})
	}
}
