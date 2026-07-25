package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type ChannelType string

const (
	ChannelMail    ChannelType = "email"
	ChannelSlack   ChannelType = "slack"
	ChannelTeams   ChannelType = "teams"
	ChannelWebhook ChannelType = "webhook"
	ChannelPager   ChannelType = "pagerduty"
)

type Notifier struct {
	client  *http.Client
	senders map[ChannelType]Sender
}

type Sender interface {
	Send(ctx context.Context, alert *Alert) error
	Name() string
}

func NewNotifier() *Notifier {
	n := &Notifier{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
		senders: make(map[ChannelType]Sender),
	}
	n.senders[ChannelSlack] = &SlackSender{client: n.client}
	n.senders[ChannelWebhook] = &WebhookSender{client: n.client}
	n.senders[ChannelMail] = &LogSender{name: "mail"}
	n.senders[ChannelPager] = &LogSender{name: "pagerduty"}
	n.senders[ChannelTeams] = &LogSender{name: "teams"}
	return n
}

func (n *Notifier) Send(ctx context.Context, alert *Alert) error {
	sent := 0
	for _, sender := range n.senders {
		if err := sender.Send(ctx, alert); err != nil {
			continue
		}
		sent++
	}
	if sent == 0 {
		return fmt.Errorf("no notification channels delivered")
	}
	return nil
}

type SlackSender struct {
	client *http.Client
	url    string
}

func (s *SlackSender) Name() string { return "slack" }

func (s *SlackSender) Send(ctx context.Context, alert *Alert) error {
	if s.url == "" {
		return nil
	}
	color := "#ff0000"
	if alert.Severity == SeverityWarning {
		color = "#ffa500"
	} else if alert.Severity == SeverityInfo {
		color = "#3498db"
	}

	payload := map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"color": color,
				"title": fmt.Sprintf("[%s] %s", alert.Severity, alert.Message),
				"fields": []map[string]interface{}{
					{"title": "Rule", "value": alert.RuleID, "short": true},
					{"title": "Device", "value": alert.DeviceID, "short": true},
					{"title": "Metric", "value": alert.MetricName, "short": true},
					{"title": "Value", "value": fmt.Sprintf("%.2f", alert.Value), "short": true},
					{"title": "Status", "value": string(alert.Status), "short": true},
				},
				"ts": alert.FiredAt.Unix(),
			},
		},
	}
	return s.postJSON(ctx, payload)
}

func (s *SlackSender) postJSON(ctx context.Context, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", s.url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

type WebhookSender struct {
	client *http.Client
	url    string
}

func (w *WebhookSender) Name() string { return "webhook" }

func (w *WebhookSender) Send(ctx context.Context, alert *Alert) error {
	if w.url == "" {
		return nil
	}
	data, err := json.Marshal(alert)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", w.url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

type LogSender struct {
	name string
}

func (l *LogSender) Name() string { return l.name }

func (l *LogSender) Send(ctx context.Context, alert *Alert) error {
	return nil
}
