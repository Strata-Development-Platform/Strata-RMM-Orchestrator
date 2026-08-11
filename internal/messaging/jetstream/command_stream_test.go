package jetstream

import "testing"

func TestEndpointCommandSubjectsMapToDurableCommandStream(t *testing.T) {
	for _, subject := range []string{
		"tenant.tenant-a.cmd.agent-1",
		"tenant.tenant-a.cmd.agent-1.software",
	} {
		if got := ToStream(subject); got != StreamCommands {
			t.Fatalf("ToStream(%q) = %q, want %q", subject, got, StreamCommands)
		}
	}
}

func TestEndpointSpecificResultsUseIndependentStream(t *testing.T) {
	for _, subject := range []string{
		"tenant.tenant-a.agent.agent-1.software.result",
		"tenant.tenant-a.agent.agent-1.script.result",
	} {
		if got := ToStream(subject); got != StreamEndpointResults {
			t.Fatalf("ToStream(%q) = %q, want %q", subject, got, StreamEndpointResults)
		}
	}
	if got := ToStream("tenant.tenant-a.agent.agent-1.result"); got != StreamCmdResults {
		t.Fatalf("base command result mapped to %q, want %q", got, StreamCmdResults)
	}
}

func TestRequiredStreamsIncludeCommandsAndEndpointResults(t *testing.T) {
	m := &StreamManager{cfg: Default()}
	seen := map[string][]string{}
	for _, sc := range m.requiredStreams() {
		seen[sc.Name] = sc.Subjects
	}
	if len(seen[StreamCommands]) != 1 || seen[StreamCommands][0] != SubjectCommands {
		t.Fatalf("command stream subjects = %#v", seen[StreamCommands])
	}
	if len(seen[StreamEndpointResults]) != 2 {
		t.Fatalf("endpoint result subjects = %#v", seen[StreamEndpointResults])
	}
}
