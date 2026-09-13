// Package mail sends transactional email through Resend.
package mail

import (
	"context"
	"errors"
	"fmt"

	"github.com/resend/resend-go/v3"
	"github.com/sirupsen/logrus"
	"github.com/tylerhardison/race-condition-kingdom/pkg/config"
)

// ErrNotConfigured means outbound mail is disabled.
var ErrNotConfigured = errors.New("mail not configured")

// Sender sends transactional email.
type Sender interface {
	Configured() bool
	Send(ctx context.Context, to, subject, text, html string) error
}

// Resend sends mail through the Resend API.
type Resend struct {
	client *resend.Client
	from   string
	log    *logrus.Logger
}

// NewResend returns a Sender. An empty API key yields a disabled sender.
func NewResend(cfg *config.Config, log *logrus.Logger) Sender {
	if cfg.Mail.ResendAPIKey == "" || cfg.Mail.From == "" {
		return &Resend{log: log}
	}
	return &Resend{client: resend.NewClient(cfg.Mail.ResendAPIKey), from: cfg.Mail.From, log: log}
}

// Configured reports whether Resend credentials are present.
func (r *Resend) Configured() bool { return r != nil && r.client != nil && r.from != "" }

// Send delivers one email. It never logs the body.
func (r *Resend) Send(ctx context.Context, to, subject, text, html string) error {
	if !r.Configured() {
		return ErrNotConfigured
	}
	sent, err := r.client.Emails.SendWithContext(ctx, &resend.SendEmailRequest{
		From:    r.from,
		To:      []string{to},
		Subject: subject,
		Text:    text,
		Html:    html,
	})
	if err != nil {
		return fmt.Errorf("resend: %w", err)
	}
	if r.log != nil {
		r.log.WithField("resend_id", sent.Id).Info("Sent transactional email")
	}
	return nil
}
