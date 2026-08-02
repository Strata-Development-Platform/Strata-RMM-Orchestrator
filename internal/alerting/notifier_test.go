package alerting

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNotifierHonorsSelectedChannels(t *testing.T) {
	var slackCalls, webhookCalls int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/slack":
			slackCalls++
		case "/webhook":
			webhookCalls++
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	n := &Notifier{senders: map[ChannelType]Sender{
		ChannelSlack:   &JSONSender{channel: ChannelSlack, client: server.Client(), url: server.URL + "/slack"},
		ChannelWebhook: &JSONSender{channel: ChannelWebhook, client: server.Client(), url: server.URL + "/webhook"},
	}}
	if err := n.Send(context.Background(), &Alert{Message: "disk full", Channels: []ChannelType{ChannelWebhook}}); err != nil {
		t.Fatal(err)
	}
	if slackCalls != 0 || webhookCalls != 1 {
		t.Fatalf("calls slack=%d webhook=%d", slackCalls, webhookCalls)
	}
}

func TestJSONSenderRejectsNonSuccessStatus(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "rejected", http.StatusBadGateway) }))
	defer server.Close()
	s := &JSONSender{channel: ChannelWebhook, client: server.Client(), url: server.URL}
	err := s.Send(context.Background(), &Alert{Message: "test"})
	if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("expected status error, got %v", err)
	}
}

func TestNotifierDisabledIsNotFalseDelivery(t *testing.T) {
	n, err := NewNotifier(NotifierConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Send(context.Background(), &Alert{Message: "test"}); err != nil {
		t.Fatalf("disabled delivery should not block alert state: %v", err)
	}
}

func TestNotifierReportsSelectedUnconfiguredChannel(t *testing.T) {
	n, err := NewNotifier(NotifierConfig{})
	if err != nil {
		t.Fatal(err)
	}
	err = n.Send(context.Background(), &Alert{Message: "test", Channels: []ChannelType{ChannelPager}})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected unconfigured channel error, got %v", err)
	}
}

func TestNotifierConfigurationFailsClosed(t *testing.T) {
	if _, err := NewNotifier(NotifierConfig{SlackURL: "http://insecure.example.test"}); err == nil {
		t.Fatal("accepted insecure Slack URL")
	}
	if _, err := NewNotifier(NotifierConfig{SMTPRecipients: []string{"ops@example.test"}}); err == nil {
		t.Fatal("accepted incomplete SMTP configuration")
	}
}
