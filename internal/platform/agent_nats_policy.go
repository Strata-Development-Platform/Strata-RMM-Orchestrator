package platform

import (
	"fmt"
	"strings"
	"time"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/auth"
)

// AgentNATSPermissions is the deny-by-default subject policy issued to a
// connected endpoint. It intentionally contains no tenant-wide wildcard.
type AgentNATSPermissions struct {
	Publish   []string
	Subscribe []string
}

type AgentNATSGrant struct {
	AgentID    string
	TenantID   string
	ExpiresAt  time.Time
	Permission AgentNATSPermissions
}

// AuthorizeAgentNATSCredential verifies the enrollment-issued application JWT
// before it is translated into a short-lived broker user JWT by the NATS auth
// callout adapter. User/session tokens are never accepted as endpoint identity.
func AuthorizeAgentNATSCredential(generator *auth.TokenGenerator, token string, now time.Time) (AgentNATSGrant, error) {
	if generator == nil || token == "" {
		return AgentNATSGrant{}, fmt.Errorf("agent credential is required")
	}
	claims, err := generator.Validate(token)
	if err != nil {
		return AgentNATSGrant{}, fmt.Errorf("invalid agent credential: %w", err)
	}
	if claims.TokenUse != "agent" || claims.AgentID == "" || claims.Subject != claims.AgentID {
		return AgentNATSGrant{}, fmt.Errorf("credential is not an agent identity")
	}
	permissions, err := BuildAgentNATSPermissions(claims.TenantID, claims.AgentID)
	if err != nil {
		return AgentNATSGrant{}, fmt.Errorf("invalid agent identity: %w", err)
	}
	expiresAt := time.Unix(claims.ExpiresAt, 0)
	if !expiresAt.After(now) {
		return AgentNATSGrant{}, fmt.Errorf("agent credential is expired")
	}
	return AgentNATSGrant{
		AgentID: claims.AgentID, TenantID: claims.TenantID,
		ExpiresAt: expiresAt, Permission: permissions,
	}, nil
}

// BuildAgentNATSPermissions binds an endpoint to its immutable tenant and agent
// identity. NATS subject metacharacters are forbidden in identity components so
// an attacker cannot widen the resulting permissions.
func BuildAgentNATSPermissions(tenantID, agentID string) (AgentNATSPermissions, error) {
	if err := validSubjectComponent("tenant ID", tenantID); err != nil {
		return AgentNATSPermissions{}, err
	}
	if err := validSubjectComponent("agent ID", agentID); err != nil {
		return AgentNATSPermissions{}, err
	}

	agent := fmt.Sprintf("tenant.%s.agent.%s", tenantID, agentID)
	command := fmt.Sprintf("tenant.%s.cmd.%s", tenantID, agentID)

	return AgentNATSPermissions{
		Publish: []string{
			agent + ".heartbeat",
			agent + ".metrics",
			agent + ".events",
			agent + ".ack",
			agent + ".result",
			agent + ".script.result",
			agent + ".software.result",
			agent + ".tunnel.*.frame",
			agent + ".tunnel.*.ctrl",
		},
		Subscribe: []string{
			command,
			command + ".cancel",
			agent + ".result.ack",
			agent + ".tunnel.*.input",
			agent + ".tunnel.*.ctrl",
		},
	}, nil
}

func validSubjectComponent(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if strings.ContainsAny(value, ".*>< \t\r\n") {
		return fmt.Errorf("%s contains an invalid NATS subject character", name)
	}
	return nil
}
