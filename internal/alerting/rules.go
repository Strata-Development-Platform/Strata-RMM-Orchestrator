package alerting

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type RuleType string

const (
	RuleTypeThreshold RuleType = "threshold"
	RuleTypeHeartbeat RuleType = "heartbeat"
)

type Condition string

const (
	ConditionGT  Condition = "gt"
	ConditionGTE Condition = "gte"
	ConditionLT  Condition = "lt"
	ConditionLTE Condition = "lte"
	ConditionEQ  Condition = "eq"
	ConditionNEQ Condition = "neq"
)

type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

type Rule struct {
	ID       string   `json:"id"`
	TenantID string   `json:"tenant_id"`
	Name     string   `json:"name"`
	Type     RuleType `json:"type"`
	Enabled  bool     `json:"enabled"`
	Severity Severity `json:"severity"`

	MetricName string    `json:"metric_name,omitempty"`
	Condition  Condition `json:"condition,omitempty"`
	Threshold  float64   `json:"threshold,omitempty"`

	Timeout time.Duration `json:"timeout,omitempty"`

	DeviceID string        `json:"device_id,omitempty"`
	Cooldown time.Duration `json:"cooldown"`

	Channels []ChannelType `json:"channels"`
	Template string        `json:"template,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (r *Rule) Validate() error {
	var problems []string
	if r.ID != "" {
		if _, err := uuid.Parse(r.ID); err != nil {
			problems = append(problems, "id must be a UUID")
		}
	}
	if _, err := uuid.Parse(r.TenantID); err != nil {
		problems = append(problems, "tenant_id must be a UUID")
	}
	if strings.TrimSpace(r.Name) == "" || len(r.Name) > 200 {
		problems = append(problems, "name must contain 1-200 characters")
	}
	switch r.Type {
	case RuleTypeThreshold:
		if strings.TrimSpace(r.MetricName) == "" {
			problems = append(problems, "threshold rules require metric_name")
		}
		switch r.Condition {
		case ConditionGT, ConditionGTE, ConditionLT, ConditionLTE, ConditionEQ, ConditionNEQ:
		default:
			problems = append(problems, "threshold rules require a valid condition")
		}
	case RuleTypeHeartbeat:
		if r.Timeout <= 0 {
			problems = append(problems, "heartbeat rules require a positive timeout")
		}
		if r.DeviceID != "" {
			if _, err := uuid.Parse(r.DeviceID); err != nil {
				problems = append(problems, "device_id must be a UUID")
			}
		}
	default:
		problems = append(problems, "type must be threshold or heartbeat")
	}
	switch r.Severity {
	case SeverityCritical, SeverityWarning, SeverityInfo:
	default:
		problems = append(problems, "severity must be critical, warning, or info")
	}
	if r.Cooldown < 0 {
		problems = append(problems, "cooldown must not be negative")
	}
	seenChannels := map[ChannelType]bool{}
	for _, channel := range r.Channels {
		switch channel {
		case ChannelSlack, ChannelWebhook, ChannelMail, ChannelTeams, ChannelPager:
		default:
			problems = append(problems, fmt.Sprintf("unsupported notification channel %q", channel))
		}
		if seenChannels[channel] {
			problems = append(problems, fmt.Sprintf("duplicate notification channel %q", channel))
		}
		seenChannels[channel] = true
	}
	if len(r.Template) > 4000 {
		problems = append(problems, "template must not exceed 4000 characters")
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid alert rule: %s", strings.Join(problems, "; "))
	}
	return nil
}

func (r *Rule) MatchesMetric(name string) bool {
	if r.MetricName == "" {
		return false
	}
	if strings.HasSuffix(r.MetricName, "*") {
		prefix := strings.TrimSuffix(r.MetricName, "*")
		return strings.HasPrefix(name, prefix)
	}
	return r.MetricName == name
}

func (r *Rule) FormatMessage(deviceID, metricName string, value float64) string {
	if r.Template != "" {
		msg := r.Template
		msg = strings.ReplaceAll(msg, "{device_id}", deviceID)
		msg = strings.ReplaceAll(msg, "{metric_name}", metricName)
		msg = strings.ReplaceAll(msg, "{threshold}", fmt.Sprintf("%.2f", r.Threshold))
		msg = strings.ReplaceAll(msg, "{value}", fmt.Sprintf("%.2f", value))
		msg = strings.ReplaceAll(msg, "{severity}", string(r.Severity))
		msg = strings.ReplaceAll(msg, "{condition}", string(r.Condition))
		return msg
	}
	return fmt.Sprintf("[%s] %s = %.2f (%s %s) on %s",
		strings.ToUpper(string(r.Severity)),
		metricName, value,
		string(r.Condition), fmt.Sprintf("%.2f", r.Threshold),
		deviceID,
	)
}

type AlertState struct {
	RuleID           string
	TenantID         string
	DeviceID         string
	State            State
	LastFired        time.Time
	LastHeard        time.Time
	ConsecutiveFires int
	AlertID          string
	MetricName       string
}

type State string

const (
	StateOK     State = "ok"
	StateFiring State = "firing"
)

type AlertStatus string

const (
	AlertFiring       AlertStatus = "firing"
	AlertResolved     AlertStatus = "resolved"
	AlertAcknowledged AlertStatus = "acknowledged"
)

type Alert struct {
	ID             string        `json:"id"`
	RuleID         string        `json:"rule_id"`
	TenantID       string        `json:"tenant_id"`
	DeviceID       string        `json:"device_id"`
	MetricName     string        `json:"metric_name,omitempty"`
	Value          float64       `json:"value,omitempty"`
	Severity       Severity      `json:"severity"`
	Message        string        `json:"message"`
	Status         AlertStatus   `json:"status"`
	FiredAt        time.Time     `json:"fired_at"`
	ResolvedAt     *time.Time    `json:"resolved_at,omitempty"`
	AcknowledgedAt *time.Time    `json:"acknowledged_at,omitempty"`
	CorrelationKey string        `json:"correlation_key,omitempty"`
	Channels       []ChannelType `json:"-"`
}
