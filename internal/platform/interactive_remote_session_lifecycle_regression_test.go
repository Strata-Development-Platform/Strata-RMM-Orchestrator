package platform

import (
	"os"
	"strings"
	"testing"
)

func isolateRemoteHandler(t *testing.T, source, startName, endName string) string {
	t.Helper()
	start := strings.Index(source, "func (s *APIServer) "+startName+"(")
	end := strings.Index(source, "func (s *APIServer) "+endName+"(")
	if start < 0 || end <= start {
		t.Fatalf("could not isolate %s", startName)
	}
	return source[start:end]
}

func TestInteractiveRemoteStartBindsAndRollsBackOnDispatchFailure(t *testing.T) {
	data, err := os.ReadFile("remote_handlers.go")
	if err != nil {
		t.Fatalf("read remote_handlers.go: %v", err)
	}
	handler := isolateRemoteHandler(t, string(data), "handleStartInteractiveSession", "handleInteractiveSessionInput")
	for _, required := range []string{
		"bindRemoteSession(sessionID",
		"TenantID: tenantID",
		"DeviceID: req.DeviceID",
		"AgentID: agentID",
		"if err := s.nats.Publish",
		"s.deleteRemoteSession(sessionID)",
	} {
		if !strings.Contains(handler, required) {
			t.Fatalf("interactive start is missing lifecycle contract %q", required)
		}
	}
}

func TestInteractiveRemoteStopDeletesBindingAfterSuccessfulDispatch(t *testing.T) {
	data, err := os.ReadFile("remote_handlers.go")
	if err != nil {
		t.Fatalf("read remote_handlers.go: %v", err)
	}
	handler := isolateRemoteHandler(t, string(data), "handleStopInteractiveSession", "handleListInteractiveSessions")
	publish := strings.Index(handler, "if err := s.nats.Publish")
	deleteBinding := strings.LastIndex(handler, "s.deleteRemoteSession(sessionID)")
	if publish < 0 || deleteBinding <= publish {
		t.Fatal("interactive stop must delete the binding only after successful stop dispatch")
	}
}

func TestInteractiveRemoteListPurgesExpiredBindings(t *testing.T) {
	data, err := os.ReadFile("remote_handlers.go")
	if err != nil {
		t.Fatalf("read remote_handlers.go: %v", err)
	}
	handler := isolateRemoteHandler(t, string(data), "handleListInteractiveSessions", "handleStartInteractiveRecording")
	for _, required := range []string{
		"s.mu.Lock()",
		"s.cleanupExpiredRemoteSessionsLocked(s.remoteSessionTime())",
		"s.mu.Unlock()",
	} {
		if !strings.Contains(handler, required) {
			t.Fatalf("interactive list is missing expiry cleanup contract %q", required)
		}
	}
}
