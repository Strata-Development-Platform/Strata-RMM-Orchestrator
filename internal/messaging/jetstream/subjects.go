package jetstream

// Subject namespaces used throughout the platform.
const (
	SubjectMetrics        = "tenant.*.agent.*.metrics"
	SubjectEvents         = "tenant.*.agent.*.events"
	SubjectHeartbeat      = "tenant.*.agent.*.heartbeat"
	SubjectCmdAck         = "tenant.*.agent.*.ack"
	SubjectCmdResult      = "tenant.*.agent.*.result"
	SubjectProbeSNMP      = "tenant.*.probe.*.snmp"
	SubjectProbeFlow      = "tenant.*.probe.*.flow"
	SubjectProbeDiscovery = "tenant.*.probe.*.discovery"
	SubjectScriptResult   = "tenant.*.agent.*.script.result"
	SubjectSoftwareResult = "tenant.*.agent.*.software.result"
	SubjectAlerts         = "tenant.*.alerts"
	SubjectConfig         = "tenant.*.config.*"
	SubjectCmd            = "tenant.*.cmd.*"
	// SubjectCommands captures both the base endpoint command subject and
	// command-specific suffixes. Keeping SubjectCmd preserves the existing
	// public subject contract while the stream wildcard provides durable
	// offline delivery for endpoint command families.
	SubjectCommands = "tenant.*.cmd.>"
)

// Stream names for JetStream streams.
const (
	StreamMetrics         = "STRATA_METRICS"
	StreamEvents          = "STRATA_EVENTS"
	StreamHeartbeats      = "STRATA_HEARTBEATS"
	StreamCommands        = "STRATA_COMMANDS"
	StreamCmdResults      = "STRATA_CMD_RESULTS"
	// StreamAgentResults is the semantic name used by the durable job protocol.
	// It intentionally aliases the existing command-result stream so installs
	// have one canonical stream for tenant.*.agent.*.result and *.ack subjects.
	StreamAgentResults    = StreamCmdResults
	StreamEndpointResults = "STRATA_ENDPOINT_RESULTS"
	StreamProbes          = "STRATA_PROBES"
	StreamDiscovery       = "STRATA_DISCOVERY"
	StreamAgentSession    = "STRATA_AGENT_SESSION"
	StreamIntegrations    = "STRATA_INTEGRATIONS"
)

// Consumer groups for pull/push consumers.
const (
	ConsumerMetrics      = "ingestion"
	ConsumerEvents       = "ingestion"
	ConsumerHeartbeats   = "ingestion"
	ConsumerCmdResults   = "platform"
	ConsumerProbes       = "probe"
	ConsumerDiscovery    = "probe"
	ConsumerAgentReplay  = "replay"
	ConsumerIntegrations = "integration"
)

// ToStream maps a subject namespace to its JetStream stream name.
func ToStream(subject string) string {
	switch {
	case subjectContains(subject, ".metrics"):
		return StreamMetrics
	case subjectContains(subject, ".events"):
		return StreamEvents
	case subjectContains(subject, ".heartbeat"):
		return StreamHeartbeats
	case subjectContains(subject, ".cmd."):
		return StreamCommands
	case subjectContains(subject, ".software.result"), subjectContains(subject, ".script.result"):
		return StreamEndpointResults
	case subjectContains(subject, ".result"), subjectContains(subject, ".ack"):
		return StreamCmdResults
	case subjectContains(subject, ".probe."):
		if subjectContains(subject, ".snmp") {
			return StreamProbes
		}
		if subjectContains(subject, ".discovery") {
			return StreamDiscovery
		}
		return StreamProbes
	case subjectContains(subject, ".integration"):
		return StreamIntegrations
	default:
		return ""
	}
}

func subjectContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
