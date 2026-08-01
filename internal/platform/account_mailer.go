package platform

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"net/url"
	"strings"
	"time"
)

// OwnerActivationMail contains the minimum data needed to deliver an MSP owner
// activation. The token is deliberately kept out of HTTP responses and logs.
type OwnerActivationMail struct {
	Recipient     string
	MSPName       string
	ActivationURL string
	Token         string
	ExpiresAt     time.Time
}

type AccountMailer interface {
	SendOwnerActivation(context.Context, OwnerActivationMail) error
}

type SMTPAccountMailerConfig struct {
	Address     string
	Username    string
	Password    string
	FromAddress string
	ImplicitTLS bool
}

// SMTPAccountMailer is a TLS-only SMTP adapter. Explicit TLS requires STARTTLS
// and fails closed if the server does not advertise it; implicit TLS performs a
// TLS handshake before any SMTP bytes or credentials are sent.
type SMTPAccountMailer struct {
	address     string
	serverName  string
	username    string
	password    string
	fromAddress string
	implicitTLS bool
}

func NewSMTPAccountMailer(config SMTPAccountMailerConfig) (*SMTPAccountMailer, error) {
	host, _, err := net.SplitHostPort(strings.TrimSpace(config.Address))
	if err != nil || host == "" {
		return nil, fmt.Errorf("SMTP address must include host and port")
	}
	from, err := mail.ParseAddress(strings.TrimSpace(config.FromAddress))
	if err != nil || from.Address == "" {
		return nil, fmt.Errorf("SMTP from address is invalid")
	}
	if (config.Username == "") != (config.Password == "") {
		return nil, fmt.Errorf("SMTP username and password must be configured together")
	}
	return &SMTPAccountMailer{
		address:     strings.TrimSpace(config.Address),
		serverName:  host,
		username:    config.Username,
		password:    config.Password,
		fromAddress: from.Address,
		implicitTLS: config.ImplicitTLS,
	}, nil
}

func (m *SMTPAccountMailer) SendOwnerActivation(ctx context.Context, activation OwnerActivationMail) error {
	recipient, err := mail.ParseAddress(activation.Recipient)
	if err != nil || recipient.Address == "" {
		return fmt.Errorf("activation recipient is invalid")
	}
	activationURL, err := url.Parse(activation.ActivationURL)
	if err != nil || activationURL.Scheme == "" || activationURL.Host == "" || activationURL.User != nil {
		return fmt.Errorf("activation URL is invalid")
	}
	if containsControl(activation.MSPName) || containsControl(activation.Token) {
		return fmt.Errorf("activation mail data is invalid")
	}
	for _, value := range []string{m.fromAddress, recipient.Address, activation.MSPName, activation.Token, activation.ActivationURL} {
		if containsCRLF(value) {
			return fmt.Errorf("activation mail data is invalid")
		}
	}

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	connection, err := dialer.DialContext(ctx, "tcp", m.address)
	if err != nil {
		return fmt.Errorf("SMTP connection failed")
	}
	defer func() { _ = connection.Close() }()
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	} else {
		_ = connection.SetDeadline(time.Now().Add(30 * time.Second))
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: m.serverName}
	if m.implicitTLS {
		tlsConnection := tls.Client(connection, tlsConfig)
		if err := tlsConnection.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("SMTP TLS handshake failed")
		}
		connection = tlsConnection
	}

	client, err := smtp.NewClient(connection, m.serverName)
	if err != nil {
		return fmt.Errorf("SMTP session failed")
	}
	defer func() { _ = client.Close() }()
	if !m.implicitTLS {
		if supported, _ := client.Extension("STARTTLS"); !supported {
			return fmt.Errorf("SMTP server does not support required TLS")
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("SMTP TLS negotiation failed")
		}
	}
	if m.username != "" {
		if err := client.Auth(smtp.PlainAuth("", m.username, m.password, m.serverName)); err != nil {
			return fmt.Errorf("SMTP authentication failed")
		}
	}
	if err := client.Mail(m.fromAddress); err != nil { // #nosec G707 -- fromAddress is mail.ParseAddress-validated in NewSMTPAccountMailer; addr-spec cannot contain CR/LF.
		return fmt.Errorf("SMTP sender rejected")
	}
	if err := client.Rcpt(recipient.Address); err != nil { // #nosec G707 -- recipient.Address is mail.ParseAddress-validated above; addr-spec cannot contain CR/LF.
		return fmt.Errorf("SMTP recipient rejected")
	}
	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP message transfer failed")
	}
	message := buildOwnerActivationMessage(m.fromAddress, recipient.Address, activation)
	if !validSMTPMessage(message) {
		_ = wc.Close()
		return fmt.Errorf("SMTP message transfer failed")
	}
	if _, err := wc.Write([]byte(message)); err != nil {
		_ = wc.Close()
		return fmt.Errorf("SMTP message transfer failed")
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("SMTP message transfer failed")
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("SMTP delivery confirmation failed")
	}
	return nil
}

func buildOwnerActivationMessage(from, recipient string, activation OwnerActivationMail) string {
	return "From: " + from + "\r\n" +
		"To: " + recipient + "\r\n" +
		"Subject: Activate your Strata RMM MSP owner account\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" +
		"You were invited to activate the owner account for " + activation.MSPName + ".\r\n\r\n" +
		"Open this link to activate your account:\r\n\r\n" +
		activation.ActivationURL + "\r\n\r\n" +
		"This invitation expires at " + activation.ExpiresAt.UTC().Format(time.RFC3339) + ".\r\n"
}
