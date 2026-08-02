package alerting

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"strings"
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

type NotifierConfig struct {
	SlackURL        string
	TeamsURL        string
	WebhookURL      string
	PagerDutyKey    string
	SMTPAddress     string
	SMTPUsername    string
	SMTPPassword    string
	SMTPFrom        string
	SMTPRecipients  []string
	SMTPImplicitTLS bool
}

type Notifier struct {
	senders map[ChannelType]Sender
}

type Sender interface {
	Send(context.Context, *Alert) error
	Name() string
}

func NewNotifier(cfg NotifierConfig) (*Notifier, error) {
	n := &Notifier{senders: map[ChannelType]Sender{}}
	client := &http.Client{Timeout: 15 * time.Second}
	for channel, raw := range map[ChannelType]string{ChannelSlack: cfg.SlackURL, ChannelTeams: cfg.TeamsURL, ChannelWebhook: cfg.WebhookURL} {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		u, err := validateHTTPSURL(raw)
		if err != nil {
			return nil, fmt.Errorf("%s URL: %w", channel, err)
		}
		n.senders[channel] = &JSONSender{channel: channel, client: client, url: u}
	}
	if strings.TrimSpace(cfg.PagerDutyKey) != "" {
		n.senders[ChannelPager] = &PagerDutySender{client: client, routingKey: strings.TrimSpace(cfg.PagerDutyKey), url: "https://events.pagerduty.com/v2/enqueue"}
	}
	if len(cfg.SMTPRecipients) > 0 {
		s, err := NewSMTPSender(cfg)
		if err != nil {
			return nil, err
		}
		n.senders[ChannelMail] = s
	}
	return n, nil
}

func (n *Notifier) Send(ctx context.Context, alert *Alert) error {
	selected := map[ChannelType]bool{}
	for _, channel := range alert.Channels {
		selected[channel] = true
	}
	var errs []error
	delivered := 0
	for channel, sender := range n.senders {
		if len(selected) > 0 && !selected[channel] {
			continue
		}
		if err := sender.Send(ctx, alert); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", sender.Name(), err))
			continue
		}
		delivered++
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	if len(selected) > 0 && delivered == 0 {
		return fmt.Errorf("selected notification channels are not configured")
	}
	return nil // notification delivery is explicitly disabled or no selected channel is configured
}

func validateHTTPSURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return "", fmt.Errorf("must be an absolute HTTPS URL without credentials")
	}
	return u.String(), nil
}

type JSONSender struct {
	channel ChannelType
	client  *http.Client
	url     string
}

func (s *JSONSender) Name() string { return string(s.channel) }
func (s *JSONSender) Send(ctx context.Context, alert *Alert) error {
	var payload any = alert
	if s.channel == ChannelSlack {
		payload = map[string]any{"text": fmt.Sprintf("[%s] %s", alert.Severity, alert.Message)}
	}
	if s.channel == ChannelTeams {
		payload = map[string]any{
			"type": "message",
			"attachments": []any{map[string]any{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"content": map[string]any{
					"type": "AdaptiveCard", "version": "1.4",
					"body": []any{map[string]any{"type": "TextBlock", "text": fmt.Sprintf("[%s] %s", alert.Severity, alert.Message), "wrap": true}},
				},
			}},
		}
	}
	return postJSON(ctx, s.client, s.url, payload)
}

type PagerDutySender struct {
	client     *http.Client
	routingKey string
	url        string
}

func (s *PagerDutySender) Name() string { return "pagerduty" }
func (s *PagerDutySender) Send(ctx context.Context, alert *Alert) error {
	action := "trigger"
	if alert.Status == AlertResolved {
		action = "resolve"
	}
	payload := map[string]any{
		"routing_key": s.routingKey, "event_action": action,
		"dedup_key": alert.RuleID + ":" + alert.DeviceID,
	}
	if action == "trigger" {
		payload["payload"] = map[string]any{"summary": alert.Message, "severity": string(alert.Severity), "source": alert.DeviceID, "custom_details": alert}
	}
	return postJSON(ctx, s.client, s.url, payload)
}

func postJSON(ctx context.Context, client *http.Client, endpoint string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

type SMTPSender struct {
	address, host, username, password, from string
	recipients                              []string
	implicitTLS                             bool
}

func NewSMTPSender(cfg NotifierConfig) (*SMTPSender, error) {
	host, _, err := net.SplitHostPort(strings.TrimSpace(cfg.SMTPAddress))
	if err != nil || host == "" {
		return nil, fmt.Errorf("alert SMTP address must include host and port")
	}
	from, err := mail.ParseAddress(cfg.SMTPFrom)
	if err != nil || from.Address == "" {
		return nil, fmt.Errorf("alert SMTP from address is invalid")
	}
	if (cfg.SMTPUsername == "") != (cfg.SMTPPassword == "") {
		return nil, fmt.Errorf("alert SMTP username and password must be configured together")
	}
	if len(cfg.SMTPRecipients) == 0 {
		return nil, fmt.Errorf("alert SMTP requires at least one recipient")
	}
	recipients := make([]string, 0, len(cfg.SMTPRecipients))
	for _, raw := range cfg.SMTPRecipients {
		parsed, err := mail.ParseAddress(raw)
		if err != nil || parsed.Address == "" {
			return nil, fmt.Errorf("alert SMTP recipient is invalid")
		}
		recipients = append(recipients, parsed.Address)
	}
	return &SMTPSender{
		address: cfg.SMTPAddress, host: host, username: cfg.SMTPUsername,
		password: cfg.SMTPPassword, from: from.Address, recipients: recipients,
		implicitTLS: cfg.SMTPImplicitTLS,
	}, nil
}

func (s *SMTPSender) Name() string { return "email" }
func (s *SMTPSender) Send(ctx context.Context, alert *Alert) error {
	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", s.address)
	if err != nil {
		return fmt.Errorf("connection failed")
	}
	defer func() { _ = conn.Close() }()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: s.host}
	if s.implicitTLS {
		tlsConn := tls.Client(conn, tlsCfg)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("TLS handshake failed")
		}
		conn = tlsConn
	}
	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("session failed")
	}
	defer func() { _ = client.Close() }()
	if !s.implicitTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("server does not support required TLS")
		}
		if err := client.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("TLS negotiation failed")
		}
	}
	if s.username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.username, s.password, s.host)); err != nil {
			return fmt.Errorf("authentication failed")
		}
	}
	if err := client.Mail(s.from); err != nil {
		return fmt.Errorf("sender rejected")
	}
	for _, recipient := range s.recipients {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("recipient rejected")
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("message transfer failed")
	}
	subject := fmt.Sprintf("[%s] Strata RMM alert", alert.Severity)
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n", s.from, strings.Join(s.recipients, ", "), subject, strings.ReplaceAll(strings.ReplaceAll(alert.Message, "\r", " "), "\n", " "))
	if _, err = w.Write([]byte(message)); err != nil {
		_ = w.Close()
		return fmt.Errorf("message transfer failed")
	}
	if err = w.Close(); err != nil {
		return fmt.Errorf("message transfer failed")
	}
	if err = client.Quit(); err != nil {
		return fmt.Errorf("delivery confirmation failed")
	}
	return nil
}
