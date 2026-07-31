package platform

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func newRemoteSessionTestServer(now *time.Time) *APIServer {
	return &APIServer{
		remoteSessions:   make(map[string]remoteSessionBinding),
		remoteSessionTTL: 5 * time.Minute,
		remoteSessionNow: func() time.Time { return *now },
	}
}

func TestRemoteSessionBindingPreservesEndpointIsolation(t *testing.T) {
	now := time.Unix(1700000000, 0)
	server := newRemoteSessionTestServer(&now)
	want := remoteSessionBinding{TenantID: "tenant-a", DeviceID: "device-a", AgentID: "agent-a"}
	server.bindRemoteSession("session-a", want)

	got, ok := server.remoteSessionFor("session-a", "tenant-a", "device-a", "agent-a")
	if !ok || got.TenantID != want.TenantID || got.DeviceID != want.DeviceID || got.AgentID != want.AgentID {
		t.Fatalf("remoteSessionFor() = %#v, %v; want endpoint binding", got, ok)
	}
	for name, identity := range map[string][3]string{
		"wrong tenant": {"tenant-b", "device-a", "agent-a"},
		"wrong device": {"tenant-a", "device-b", "agent-a"},
		"wrong agent":  {"tenant-a", "device-a", "agent-b"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := server.remoteSessionFor("session-a", identity[0], identity[1], identity[2]); ok {
				t.Fatalf("session authorized for %s/%s/%s", identity[0], identity[1], identity[2])
			}
		})
	}
}

func TestRemoteSessionExpiresAndIsRemoved(t *testing.T) {
	now := time.Unix(1700000000, 0)
	server := newRemoteSessionTestServer(&now)
	server.bindRemoteSession("expired", remoteSessionBinding{TenantID: "tenant-a", DeviceID: "device-a", AgentID: "agent-a"})
	now = now.Add(5 * time.Minute)
	if _, ok := server.remoteSessionFor("expired", "tenant-a", "device-a", "agent-a"); ok {
		t.Fatal("expired remote session remained authorized")
	}
	if len(server.remoteSessions) != 0 {
		t.Fatalf("expired session was not removed: %#v", server.remoteSessions)
	}
}

func TestRemoteSessionDeletionAndShutdownCleanup(t *testing.T) {
	now := time.Unix(1700000000, 0)
	server := newRemoteSessionTestServer(&now)
	binding := remoteSessionBinding{TenantID: "tenant-a", DeviceID: "device-a", AgentID: "agent-a"}
	server.bindRemoteSession("deleted", binding)
	server.deleteRemoteSession("deleted")
	if _, ok := server.remoteSessionFor("deleted", "tenant-a", "device-a", "agent-a"); ok {
		t.Fatal("deleted remote session remained authorized")
	}
	server.bindRemoteSession("shutdown", binding)
	server.clearRemoteSessions()
	if len(server.remoteSessions) != 0 {
		t.Fatal("shutdown cleanup retained remote sessions")
	}
}

func TestRemoteSessionConcurrentAccess(t *testing.T) {
	now := time.Unix(1700000000, 0)
	server := newRemoteSessionTestServer(&now)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("session-%d", i)
			binding := remoteSessionBinding{TenantID: "tenant-a", DeviceID: id, AgentID: id}
			server.bindRemoteSession(id, binding)
			if _, ok := server.remoteSessionFor(id, "tenant-a", id, id); !ok {
				t.Errorf("session %s missing during concurrent access", id)
			}
			server.deleteRemoteSession(id)
		}(i)
	}
	wg.Wait()
	if len(server.remoteSessions) != 0 {
		t.Fatalf("concurrent cleanup retained %d sessions", len(server.remoteSessions))
	}
}
