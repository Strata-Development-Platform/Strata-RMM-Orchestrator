package platform

import "testing"

func TestRemoteSessionBindingPreservesEndpointIsolation(t *testing.T) {
	server := &APIServer{remoteSessions: make(map[string]remoteSessionBinding)}
	want := remoteSessionBinding{TenantID: "tenant-a", DeviceID: "device-a", AgentID: "agent-a"}
	server.bindRemoteSession("session-a", want)

	got, ok := server.remoteSession("session-a")
	if !ok || got != want {
		t.Fatalf("remoteSession() = %#v, %v; want %#v, true", got, ok, want)
	}
	if got.TenantID == "tenant-b" || got.DeviceID == "device-b" || got.AgentID == "agent-b" {
		t.Fatal("session binding widened to a different endpoint")
	}
	server.deleteRemoteSession("session-a")
	if _, ok := server.remoteSession("session-a"); ok {
		t.Fatal("deleted remote session remained authorized")
	}
}
