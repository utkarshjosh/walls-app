// Package mail sends the one kind of message this service writes: a short
// plain-text note to one address, such as a sign-in code.
//
// It speaks SMTP to a relay the owner rents (any provider that gives a host,
// a user name and a password). There is no queue and no retry: the caller is
// answering a person who is waiting, and tells them if the message did not go.
package mail

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// Message is one plain-text message to one address.
type Message struct {
	To      string // a bare address, already checked
	Subject string
	Text    string
}

// Sender delivers a message or says why it could not.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// SMTP sends through a relay. Port 465 is spoken to with TLS from the first
// byte; any other port must offer STARTTLS, and the message is not sent
// without it, so neither the password nor a sign-in code crosses the network
// in the clear.
type SMTP struct {
	Host     string
	Port     int
	User     string // empty means the relay needs no sign-in
	Password string
	From     string // "Still <hello@example.com>" or a bare address

	roots *x509.CertPool // tests only: the certificate of a made-up relay
}

// sendTimeout bounds one whole delivery, so a relay that stops answering
// cannot hold a request open.
const sendTimeout = 20 * time.Second

// ParseFrom checks a From setting and returns the bare address inside it.
func ParseFrom(from string) (string, error) {
	a, err := mail.ParseAddress(from)
	if err != nil {
		return "", fmt.Errorf("not a mail address: %q", from)
	}
	return a.Address, nil
}

// Send delivers m, or returns the relay's reason for refusing it.
func (s *SMTP) Send(ctx context.Context, m Message) error {
	from, err := ParseFrom(s.From)
	if err != nil {
		return err
	}
	if strings.ContainsAny(m.To, "\r\n") || strings.ContainsAny(m.Subject, "\r\n") {
		return errors.New("mail: a header cannot contain a line break")
	}
	body, err := s.compose(from, m)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	tlsConfig := &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12, RootCAs: s.roots}

	var conn net.Conn
	if s.Port == 465 {
		conn, err = (&tls.Dialer{Config: tlsConfig}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("mail: connect to %s: %w", addr, err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return fmt.Errorf("mail: greeting: %w", err)
	}
	defer c.Close()
	if s.Port != 465 {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return fmt.Errorf("mail: %s does not offer an encrypted connection (STARTTLS); nothing was sent", addr)
		}
		if err := c.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("mail: starting encryption: %w", err)
		}
	}
	if s.User != "" {
		if err := c.Auth(smtp.PlainAuth("", s.User, s.Password, s.Host)); err != nil {
			return fmt.Errorf("mail: the relay refused the user name or password: %w", err)
		}
	}
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("mail: sender refused: %w", err)
	}
	if err := c.Rcpt(m.To); err != nil {
		return fmt.Errorf("mail: recipient refused: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: message refused: %w", err)
	}
	_ = c.Quit() // the relay has the message; how the goodbye goes does not matter
	return nil
}

// compose writes the message as it goes over the wire: headers, a blank line,
// the text, every line ending in CRLF.
func (s *SMTP) compose(from string, m Message) ([]byte, error) {
	id := make([]byte, 12)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	domain := from[strings.LastIndexByte(from, '@')+1:]
	var b strings.Builder
	for _, h := range [][2]string{
		{"From", s.From},
		{"To", m.To},
		{"Subject", mime.QEncoding.Encode("utf-8", m.Subject)},
		{"Date", time.Now().Format(time.RFC1123Z)},
		{"Message-ID", "<" + hex.EncodeToString(id) + "@" + domain + ">"},
		{"MIME-Version", "1.0"},
		{"Content-Type", `text/plain; charset="utf-8"`},
		{"Content-Transfer-Encoding", "8bit"},
		// Tells mail programs this was written by a program in answer to a
		// request, so they do not send holiday auto-replies to it.
		{"Auto-Submitted", "auto-generated"},
	} {
		b.WriteString(h[0] + ": " + h[1] + "\r\n")
	}
	b.WriteString("\r\n")
	text := strings.ReplaceAll(strings.ReplaceAll(m.Text, "\r\n", "\n"), "\n", "\r\n")
	b.WriteString(text)
	if !strings.HasSuffix(text, "\r\n") {
		b.WriteString("\r\n")
	}
	return []byte(b.String()), nil
}

// Log is a Sender for development: it sends nothing and writes the message to
// the server's log instead, so sign-in can be tried without a mail account.
// Never use it on a server other people can reach: anyone who can read the
// log can sign in as anyone.
type Log struct {
	Log *slog.Logger
}

// Send writes the message to the log.
func (l Log) Send(_ context.Context, m Message) error {
	l.Log.Warn("mail not sent (WALLS_MAIL_TO_LOG is on): the message follows", "to", m.To, "subject", m.Subject, "text", m.Text)
	return nil
}
