package alerting

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func testTLSClient() *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

func TestChannelTypeConstants(t *testing.T) {
	if ChannelMail != "email" {
		t.Fatalf("ChannelMail = %q", ChannelMail)
	}
	if ChannelSlack != "slack" {
		t.Fatalf("ChannelSlack = %q", ChannelSlack)
	}
	if ChannelTeams != "teams" {
		t.Fatalf("ChannelTeams = %q", ChannelTeams)
	}
	if ChannelWebhook != "webhook" {
		t.Fatalf("ChannelWebhook = %q", ChannelWebhook)
	}
	if ChannelPager != "pagerduty" {
		t.Fatalf("ChannelPager = %q", ChannelPager)
	}
}

func TestSeverityConstants(t *testing.T) {
	if SeverityCritical != "critical" {
		t.Fatalf("SeverityCritical = %q", SeverityCritical)
	}
	if SeverityWarning != "warning" {
		t.Fatalf("SeverityWarning = %q", SeverityWarning)
	}
	if SeverityInfo != "info" {
		t.Fatalf("SeverityInfo = %q", SeverityInfo)
	}
}

func TestAlertStatusConstants(t *testing.T) {
	if AlertFiring != "firing" {
		t.Fatalf("AlertFiring = %q", AlertFiring)
	}
	if AlertResolved != "resolved" {
		t.Fatalf("AlertResolved = %q", AlertResolved)
	}
	if AlertAcknowledged != "acknowledged" {
		t.Fatalf("AlertAcknowledged = %q", AlertAcknowledged)
	}
}

func TestAlertStructNotificationFields(t *testing.T) {
	now := time.Now()
	resolvedAt := now.Add(time.Minute)
	alert := Alert{
		ID:               "alert-1",
		RuleID:           "rule-1",
		TenantID:         "tenant-1",
		DeviceID:         "device-1",
		MetricName:       "cpu_usage",
		Value:            95.0,
		Severity:         SeverityCritical,
		Message:          "CPU usage critical",
		Status:           AlertFiring,
		FiredAt:          now,
		ResolvedAt:       &resolvedAt,
		AcknowledgedAt:   nil,
		CorrelationKey:   "cpu-critical",
		Channels:         []ChannelType{ChannelSlack, ChannelPager},
	}

	if alert.ID != "alert-1" {
		t.Fatalf("Alert.ID = %q", alert.ID)
	}
	if alert.Severity != SeverityCritical {
		t.Fatalf("Alert.Severity = %v", alert.Severity)
	}
	if alert.Status != AlertFiring {
		t.Fatalf("Alert.Status = %v", alert.Status)
	}
	if alert.Message != "CPU usage critical" {
		t.Fatalf("Alert.Message = %q", alert.Message)
	}
	if len(alert.Channels) != 2 {
		t.Fatalf("Alert.Channels length = %d", len(alert.Channels))
	}
}

func TestAlertPayloadSerializesNotificationFields(t *testing.T) {
	now := time.Now()
	alert := Alert{
		ID:       "alert-1",
		RuleID:   "rule-1",
		DeviceID: "device-1",
		Severity: SeverityWarning,
		Message:  "disk usage warning",
		Status:   AlertFiring,
		FiredAt:  now,
	}
	data, err := json.Marshal(alert)
	if err != nil {
		t.Fatalf("marshal alert: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal alert: %v", err)
	}
	if got["severity"] != "warning" {
		t.Fatalf("severity = %v", got["severity"])
	}
	if got["status"] != "firing" {
		t.Fatalf("status = %v", got["status"])
	}
}

func TestNotifierConfigFields(t *testing.T) {
	cfg := NotifierConfig{
		SlackURL:        "https://hooks.slack.example.test",
		TeamsURL:        "https://teams.example.test/webhook",
		WebhookURL:      "https://monitoring.example.test/alerts",
		PagerDutyKey:    "pd-key",
		SMTPAddress:     "smtp.example.test:587",
		SMTPUsername:    "user",
		SMTPPassword:    "pass",
		SMTPFrom:        "alerts@example.test",
		SMTPRecipients:  []string{"ops@example.test"},
		SMTPImplicitTLS: false,
	}
	if cfg.SlackURL != "https://hooks.slack.example.test" {
		t.Fatalf("SlackURL = %q", cfg.SlackURL)
	}
	if cfg.PagerDutyKey != "pd-key" {
		t.Fatalf("PagerDutyKey = %q", cfg.PagerDutyKey)
	}
}

func TestNotifierDisabledNoChannelsSucceeds(t *testing.T) {
	n, err := NewNotifier(NotifierConfig{})
	if err != nil {
		t.Fatal(err)
	}
	err = n.Send(context.Background(), &Alert{Message: "test"})
	if err != nil {
		t.Fatalf("disabled notifier should not return error: %v", err)
	}
}

func TestNotifierSelectedChannelsDeliversOnlySelected(t *testing.T) {
	var calls int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	n, err := NewNotifier(NotifierConfig{
		SlackURL:   server.URL,
		TeamsURL:   server.URL,
		WebhookURL: server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	n.senders[ChannelSlack] = &JSONSender{channel: ChannelSlack, client: testTLSClient(), url: server.URL}
	n.senders[ChannelTeams] = &JSONSender{channel: ChannelTeams, client: testTLSClient(), url: server.URL}
	n.senders[ChannelWebhook] = &JSONSender{channel: ChannelWebhook, client: testTLSClient(), url: server.URL}
	alert := &Alert{Message: "test", Channels: []ChannelType{ChannelSlack}}
	if err := n.Send(context.Background(), alert); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 sender call, got %d", calls)
	}
}

func TestNotifierUnconfiguredSelectedChannelFails(t *testing.T) {
	n, err := NewNotifier(NotifierConfig{})
	if err != nil {
		t.Fatal(err)
	}
	err = n.Send(context.Background(), &Alert{Message: "test", Channels: []ChannelType{ChannelPager}})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected unconfigured channel error: %v", err)
	}
}

func TestNotifierConfigurationRejectsInsecureSlackURL(t *testing.T) {
	_, err := NewNotifier(NotifierConfig{SlackURL: "http://insecure.example.test/webhook"})
	if err == nil {
		t.Fatal("expected error for insecure Slack URL")
	}
}

func TestNotifierConfigurationRejectsInsecureTeamsURL(t *testing.T) {
	_, err := NewNotifier(NotifierConfig{TeamsURL: "http://insecure.example.test/webhook"})
	if err == nil {
		t.Fatal("expected error for insecure Teams URL")
	}
}

func TestNotifierConfigurationRejectsInsecureWebhookURL(t *testing.T) {
	_, err := NewNotifier(NotifierConfig{WebhookURL: "http://insecure.example.test"})
	if err == nil {
		t.Fatal("expected error for insecure webhook URL")
	}
}

func TestNotifierConfigurationRejectsMissingSMTPFrom(t *testing.T) {
	_, err := NewNotifier(NotifierConfig{
		SMTPAddress:    "smtp.example.test:587",
		SMTPRecipients: []string{"ops@example.test"},
	})
	if err == nil {
		t.Fatal("expected error for missing SMTP From address")
	}
}

func TestNotifierConfigurationRejectsMissingSMTPAddress(t *testing.T) {
	_, err := NewNotifier(NotifierConfig{
		SMTPFrom:       "alerts@example.test",
		SMTPRecipients: []string{"ops@example.test"},
	})
	if err == nil {
		t.Fatal("expected error for missing SMTP address")
	}
}

func TestNotifierConfigurationDoesNotCreateSMTPSenderWithoutRecipients(t *testing.T) {
	n, err := NewNotifier(NotifierConfig{
		SMTPAddress:    "smtp.example.test:587",
		SMTPFrom:       "alerts@example.test",
		SMTPRecipients: []string{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := n.senders[ChannelMail]; ok {
		t.Fatal("SMTP sender should not be created without recipients")
	}
}

func TestNotifierConfigurationRejectsInvalidSMTPRecipient(t *testing.T) {
	_, err := NewNotifier(NotifierConfig{
		SMTPAddress:    "smtp.example.test:587",
		SMTPFrom:       "alerts@example.test",
		SMTPRecipients: []string{"not-an-email"},
	})
	if err == nil {
		t.Fatal("expected error for invalid SMTP recipient")
	}
}

func TestNotifierConfigurationRejectsUnbalancedSMTPCredentials(t *testing.T) {
	_, err := NewNotifier(NotifierConfig{
		SMTPAddress:    "smtp.example.test:587",
		SMTPFrom:       "alerts@example.test",
		SMTPRecipients: []string{"ops@example.test"},
		SMTPUsername:   "user",
		SMTPPassword:   "",
	})
	if err == nil {
		t.Fatal("expected error for username without password")
	}
	_, err = NewNotifier(NotifierConfig{
		SMTPAddress:    "smtp.example.test:587",
		SMTPFrom:       "alerts@example.test",
		SMTPRecipients: []string{"ops@example.test"},
		SMTPUsername:   "",
		SMTPPassword:   "pass",
	})
	if err == nil {
		t.Fatal("expected error for password without username")
	}
}

func TestNotifierConfigurationAcceptsSMTPWithNoAuth(t *testing.T) {
	n, err := NewNotifier(NotifierConfig{
		SMTPAddress:    "smtp.example.test:587",
		SMTPFrom:       "alerts@example.test",
		SMTPRecipients: []string{"ops@example.test"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n == nil {
		t.Fatal("expected notifier, got nil")
	}
}

func TestNotifierConfigurationRejectsMissingSMTPPort(t *testing.T) {
	_, err := NewNotifier(NotifierConfig{
		SMTPAddress:    "smtp.example.test",
		SMTPFrom:       "alerts@example.test",
		SMTPRecipients: []string{"ops@example.test"},
	})
	if err == nil {
		t.Fatal("expected error for SMTP address without port")
	}
}

func TestJSONSenderNameReturnsChannelType(t *testing.T) {
	for _, channel := range []ChannelType{ChannelSlack, ChannelTeams, ChannelWebhook} {
		s := &JSONSender{channel: channel}
		if s.Name() != string(channel) {
			t.Errorf("JSONSender{Name=%q} = %q", channel, s.Name())
		}
	}
}

func TestPagerDutySenderNameReturnsPagerDuty(t *testing.T) {
	s := &PagerDutySender{routingKey: "key"}
	if s.Name() != "pagerduty" {
		t.Fatalf("PagerDutySender.Name = %q", s.Name())
	}
}

func TestSMTPSenderNameReturnsEmail(t *testing.T) {
	s, err := NewSMTPSender(NotifierConfig{
		SMTPAddress:    "smtp.example.test:587",
		SMTPFrom:       "alerts@example.test",
		SMTPRecipients: []string{"ops@example.test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.Name() != "email" {
		t.Fatalf("SMTPSender.Name = %q", s.Name())
	}
}

func TestJSONSenderSlackPayloadFormat(t *testing.T) {
	var receivedText string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		receivedText, _ = body["text"].(string)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	s := &JSONSender{channel: ChannelSlack, client: testTLSClient(), url: server.URL}
	err := s.Send(context.Background(), &Alert{
		Severity: SeverityCritical,
		Message:  "disk full",
	})
	if err != nil {
		t.Fatal(err)
	}
	if receivedText != "[critical] disk full" {
		t.Fatalf("Slack text = %q", receivedText)
	}
}

func TestJSONSenderTeamsPayloadFormat(t *testing.T) {
	var receivedType string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		receivedType, _ = body["type"].(string)
		attachments, _ := body["attachments"].([]interface{})
		if len(attachments) > 0 {
			attachment, _ := attachments[0].(map[string]interface{})
			if content, ok := attachment["content"].(map[string]interface{}); ok {
				if bodyArr, ok := content["body"].([]interface{}); ok && len(bodyArr) > 0 {
					if textBlock, ok := bodyArr[0].(map[string]interface{}); ok {
						receivedText, _ := textBlock["text"].(string)
						t.Logf("Teams body text: %q", receivedText)
					}
				}
			}
			_, _ = attachment["body"].([]interface{})
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	s := &JSONSender{channel: ChannelTeams, client: testTLSClient(), url: server.URL}
	err := s.Send(context.Background(), &Alert{
		Severity: SeverityWarning,
		Message:  "high CPU",
	})
	if err != nil {
		t.Fatal(err)
	}
	if receivedType != "message" {
		t.Fatalf("Teams type = %q", receivedType)
	}
}

func TestJSONSenderWebhookPayloadIsRawAlert(t *testing.T) {
	var receivedSeverity string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		receivedSeverity, _ = body["severity"].(string)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	s := &JSONSender{channel: ChannelWebhook, client: testTLSClient(), url: server.URL}
	err := s.Send(context.Background(), &Alert{
		Severity: SeverityInfo,
		Message:  "info alert",
	})
	if err != nil {
		t.Fatal(err)
	}
	if receivedSeverity != "info" {
		t.Fatalf("Webhook severity = %q", receivedSeverity)
	}
}

func TestJSONSenderNon200ReturnsError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	s := &JSONSender{channel: ChannelWebhook, client: testTLSClient(), url: server.URL}
	err := s.Send(context.Background(), &Alert{Message: "test"})
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPagerDutySenderTriggerPayload(t *testing.T) {
	var receivedAction string
	var receivedSeverity string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		receivedAction, _ = body["event_action"].(string)
		payload, _ := body["payload"].(map[string]interface{})
		receivedSeverity, _ = payload["severity"].(string)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	s := &PagerDutySender{routingKey: "key", client: testTLSClient(), url: server.URL}
	err := s.Send(context.Background(), &Alert{
		DeviceID: "device-1",
		Severity: SeverityCritical,
		Message:  "outage",
		Status:   AlertFiring,
		RuleID:   "rule-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if receivedAction != "trigger" {
		t.Fatalf("PagerDuty action = %q", receivedAction)
	}
	if receivedSeverity != "critical" {
		t.Fatalf("PagerDuty severity = %q", receivedSeverity)
	}
}

func TestPagerDutySenderResolvePayload(t *testing.T) {
	var receivedAction string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		receivedAction, _ = body["event_action"].(string)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	s := &PagerDutySender{routingKey: "key", client: testTLSClient(), url: server.URL}
	err := s.Send(context.Background(), &Alert{
		DeviceID: "device-1",
		Status:   AlertResolved,
		RuleID:   "rule-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if receivedAction != "resolve" {
		t.Fatalf("PagerDuty action = %q", receivedAction)
	}
}

func TestPagerDutySenderDedupKeyIncludesRuleAndDevice(t *testing.T) {
	var receivedDedupKey string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		receivedDedupKey, _ = body["dedup_key"].(string)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	s := &PagerDutySender{routingKey: "key", client: testTLSClient(), url: server.URL}
	err := s.Send(context.Background(), &Alert{
		DeviceID: "device-1",
		RuleID:   "rule-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "rule-1:device-1"
	if receivedDedupKey != want {
		t.Fatalf("PagerDuty dedup_key = %q, want %q", receivedDedupKey, want)
	}
}

func TestPagerDutySenderNon200ReturnsError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()
	s := &PagerDutySender{routingKey: "key", client: testTLSClient(), url: server.URL}
	err := s.Send(context.Background(), &Alert{Message: "test"})
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNotifierSendHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	n, err := NewNotifier(NotifierConfig{
		WebhookURL: "https://example.test/webhook",
	})
	if err != nil {
		t.Fatal(err)
	}
	err = n.Send(ctx, &Alert{Message: "cancelled"})
	if err == nil {
		t.Fatal("expected error after context cancellation")
	}
}

func TestNotifierSendReportsAllChannelErrors(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "failure", http.StatusInternalServerError)
	}))
	defer server.Close()
	n, err := NewNotifier(NotifierConfig{
		SlackURL:   server.URL,
		TeamsURL:   server.URL,
		WebhookURL: server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	n.senders[ChannelSlack] = &JSONSender{channel: ChannelSlack, client: testTLSClient(), url: server.URL}
	n.senders[ChannelTeams] = &JSONSender{channel: ChannelTeams, client: testTLSClient(), url: server.URL}
	n.senders[ChannelWebhook] = &JSONSender{channel: ChannelWebhook, client: testTLSClient(), url: server.URL}
	err = n.Send(context.Background(), &Alert{
		Message:  "test",
		Channels: []ChannelType{ChannelSlack, ChannelTeams},
	})
	if err == nil {
		t.Fatal("expected error when selected channels fail")
	}
	if !strings.Contains(err.Error(), "slack") {
		t.Fatalf("error missing channel: %v", err)
	}
}

func TestNotifierSendSkipsUnconfiguredChannels(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	n, err := NewNotifier(NotifierConfig{
		SlackURL:   server.URL,
		TeamsURL:   server.URL,
		WebhookURL: server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	n.senders[ChannelSlack] = &JSONSender{channel: ChannelSlack, client: testTLSClient(), url: server.URL}
	n.senders[ChannelTeams] = &JSONSender{channel: ChannelTeams, client: testTLSClient(), url: server.URL}
	n.senders[ChannelWebhook] = &JSONSender{channel: ChannelWebhook, client: testTLSClient(), url: server.URL}
	err = n.Send(context.Background(), &Alert{
		Message:  "test",
		Channels: []ChannelType{ChannelSlack, ChannelWebhook},
	})
	if err != nil {
		t.Fatalf("expected success when unconfigured channel is not selected: %v", err)
	}
}

func TestNotifierConfigurationAcceptsOnlyHTTPS(t *testing.T) {
	_, err := NewNotifier(NotifierConfig{WebhookURL: "http://example.test"})
	if err == nil {
		t.Fatal("expected error for HTTP webhook URL")
	}
}

func TestNotifierConfigurationAcceptsHTTPSWithPath(t *testing.T) {
	n, err := NewNotifier(NotifierConfig{WebhookURL: "https://example.test/alerts/v1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n == nil {
		t.Fatal("expected notifier")
	}
}

func TestNotifierConfigurationRejectsHTTPSURLWithCredentials(t *testing.T) {
	u, err := url.Parse("https://example.test/webhook")
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword("trufflehog-test-user", "trufflehog-test-password")
	_, err = NewNotifier(NotifierConfig{WebhookURL: u.String()})
	if err == nil {
		t.Fatal("expected error for webhook URL with credentials")
	}
}

func TestNotifierConfigurationAcceptsPagerDutyWithoutOtherChannels(t *testing.T) {
	n, err := NewNotifier(NotifierConfig{PagerDutyKey: "pd-key"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n == nil {
		t.Fatal("expected notifier")
	}
}

func TestNotifierConfigurationAcceptsTeamsOnly(t *testing.T) {
	n, err := NewNotifier(NotifierConfig{TeamsURL: "https://teams.example.test/webhook"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n == nil {
		t.Fatal("expected notifier")
	}
}

func TestNotifierConfigurationAcceptsSlackOnly(t *testing.T) {
	n, err := NewNotifier(NotifierConfig{SlackURL: "https://hooks.slack.example.test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n == nil {
		t.Fatal("expected notifier")
	}
}

func TestNotifierSendNoChannelsSelectedDeliversAll(t *testing.T) {
	var calls int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	n, err := NewNotifier(NotifierConfig{
		SlackURL:   server.URL,
		TeamsURL:   server.URL,
		WebhookURL: server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	n.senders[ChannelSlack] = &JSONSender{channel: ChannelSlack, client: testTLSClient(), url: server.URL}
	n.senders[ChannelTeams] = &JSONSender{channel: ChannelTeams, client: testTLSClient(), url: server.URL}
	n.senders[ChannelWebhook] = &JSONSender{channel: ChannelWebhook, client: testTLSClient(), url: server.URL}
	alert := &Alert{Message: "test"}
	if err := n.Send(context.Background(), alert); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 delivery calls, got %d", calls)
	}
}

func TestNotifierSendSelectedChannelDeliversOnlyThatChannel(t *testing.T) {
	var calls int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	n, err := NewNotifier(NotifierConfig{
		SlackURL:   server.URL,
		TeamsURL:   server.URL,
		WebhookURL: server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	n.senders[ChannelSlack] = &JSONSender{channel: ChannelSlack, client: testTLSClient(), url: server.URL}
	n.senders[ChannelTeams] = &JSONSender{channel: ChannelTeams, client: testTLSClient(), url: server.URL}
	n.senders[ChannelWebhook] = &JSONSender{channel: ChannelWebhook, client: testTLSClient(), url: server.URL}
	alert := &Alert{Message: "test", Channels: []ChannelType{ChannelTeams}}
	if err := n.Send(context.Background(), alert); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 delivery call, got %d", calls)
	}
}

func TestNotifierSendSelectedUnconfiguredChannelFailsWithChannelName(t *testing.T) {
	n, err := NewNotifier(NotifierConfig{SlackURL: "https://slack.example.test"})
	if err != nil {
		t.Fatal(err)
	}
	n.senders[ChannelSlack] = &JSONSender{channel: ChannelSlack, client: testTLSClient(), url: "https://slack.example.test"}
	err = n.Send(context.Background(), &Alert{
		Message:  "test",
		Channels: []ChannelType{ChannelTeams},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected unconfigured channel message: %v", err)
	}
}

func TestNotifierConfigurationFailsOnInvalidSMTPFrom(t *testing.T) {
	_, err := NewNotifier(NotifierConfig{
		SMTPAddress:    "smtp.example.test:587",
		SMTPRecipients: []string{"ops@example.test"},
		SMTPFrom:       "invalid-from",
	})
	if err == nil {
		t.Fatal("expected error for invalid SMTP From address")
	}
}

func TestNotifierConfigurationFailsOnMultipleInvalidRecipients(t *testing.T) {
	_, err := NewNotifier(NotifierConfig{
		SMTPAddress:    "smtp.example.test:587",
		SMTPFrom:       "alerts@example.test",
		SMTPRecipients: []string{"bad", "also-bad"},
	})
	if err == nil {
		t.Fatal("expected error for invalid SMTP recipients")
	}
}

func TestNotifierConfigurationAcceptsMultipleRecipients(t *testing.T) {
	n, err := NewNotifier(NotifierConfig{
		SMTPAddress:    "smtp.example.test:587",
		SMTPFrom:       "alerts@example.test",
		SMTPRecipients: []string{"ops@example.test", "oncall@example.test"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n == nil {
		t.Fatal("expected notifier")
	}
}

func TestNotifierConfigurationAcceptsImplicitTLS(t *testing.T) {
	n, err := NewNotifier(NotifierConfig{
		SMTPAddress:     "smtp.example.test:465",
		SMTPFrom:        "alerts@example.test",
		SMTPRecipients:  []string{"ops@example.test"},
		SMTPImplicitTLS: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n == nil {
		t.Fatal("expected notifier")
	}
}

func TestJSONSenderSlackAlertFieldsIncluded(t *testing.T) {
	var receivedText string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		receivedText, _ = body["text"].(string)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	s := &JSONSender{channel: ChannelSlack, client: testTLSClient(), url: server.URL}
	err := s.Send(context.Background(), &Alert{
		Severity: SeverityWarning,
		Message:  "memory usage high",
		Status:   AlertFiring,
		DeviceID: "host-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if receivedText != "[warning] memory usage high" {
		t.Fatalf("Slack text = %q", receivedText)
	}
}

func TestJSONSenderTeamsAdaptiveCardContent(t *testing.T) {
	var receivedType string
	var receivedVersion string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		receivedType, _ = body["type"].(string)
		attachments, _ := body["attachments"].([]interface{})
		if len(attachments) > 0 {
			attachment, _ := attachments[0].(map[string]interface{})
			content, _ := attachment["content"].(map[string]interface{})
			receivedVersion, _ = content["version"].(string)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	s := &JSONSender{channel: ChannelTeams, client: testTLSClient(), url: server.URL}
	err := s.Send(context.Background(), &Alert{
		Severity: SeverityInfo,
		Message:  "scheduled maintenance",
	})
	if err != nil {
		t.Fatal(err)
	}
	if receivedType != "message" {
		t.Fatalf("Teams type = %q", receivedType)
	}
	if receivedVersion != "1.4" {
		t.Fatalf("Teams card version = %q", receivedVersion)
	}
}

func TestPagerDutySenderTriggerPayloadIncludesSummaryAndSource(t *testing.T) {
	var receivedSummary string
	var receivedSource string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		payload, _ := body["payload"].(map[string]interface{})
		receivedSummary, _ = payload["summary"].(string)
		receivedSource, _ = payload["source"].(string)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	s := &PagerDutySender{routingKey: "key", client: testTLSClient(), url: server.URL}
	err := s.Send(context.Background(), &Alert{
		DeviceID: "device-1",
		Message:  "disk failure",
		Status:   AlertFiring,
		RuleID:   "rule-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if receivedSummary != "disk failure" {
		t.Fatalf("PagerDuty summary = %q", receivedSummary)
	}
	if receivedSource != "device-1" {
		t.Fatalf("PagerDuty source = %q", receivedSource)
	}
}

func TestPagerDutySenderResolvePayloadDoesNotIncludeTriggerPayloadFields(t *testing.T) {
	var receivedPayload map[string]interface{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		receivedPayload, _ = body["payload"].(map[string]interface{})
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	s := &PagerDutySender{routingKey: "key", client: testTLSClient(), url: server.URL}
	err := s.Send(context.Background(), &Alert{
		DeviceID: "device-1",
		Status:   AlertResolved,
		RuleID:   "rule-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if receivedPayload != nil {
		t.Fatalf("PagerDuty resolve payload should be nil, got %v", receivedPayload)
	}
}

func TestNotifierConfigurationRejectsEmptySlackURL(t *testing.T) {
	_, err := NewNotifier(NotifierConfig{SlackURL: "   "})
	if err != nil {
		t.Fatalf("whitespace-only SlackURL should be ignored, error: %v", err)
	}
}

func TestNotifierConfigurationRejectsEmptyTeamsURL(t *testing.T) {
	_, err := NewNotifier(NotifierConfig{TeamsURL: "   "})
	if err != nil {
		t.Fatalf("whitespace-only TeamsURL should be ignored, error: %v", err)
	}
}

func TestNotifierConfigurationRejectsEmptyWebhookURL(t *testing.T) {
	_, err := NewNotifier(NotifierConfig{WebhookURL: "   "})
	if err != nil {
		t.Fatalf("whitespace-only WebhookURL should be ignored, error: %v", err)
	}
}

func TestNotifierConfigurationRejectsEmptyPagerDutyKey(t *testing.T) {
	_, err := NewNotifier(NotifierConfig{PagerDutyKey: "   "})
	if err != nil {
		t.Fatalf("whitespace-only PagerDutyKey should be ignored, error: %v", err)
	}
}

func TestNotifierSendMissingRequiredFieldsStillDispatches(t *testing.T) {
	var receivedSeverity string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		receivedSeverity, _ = body["severity"].(string)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	n, err := NewNotifier(NotifierConfig{WebhookURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	n.senders[ChannelWebhook] = &JSONSender{channel: ChannelWebhook, client: testTLSClient(), url: server.URL}
	err = n.Send(context.Background(), &Alert{})
	if err != nil {
		t.Fatal(err)
	}
	if receivedSeverity != "" {
		t.Fatalf("expected empty severity for empty alert: %q", receivedSeverity)
	}
}
