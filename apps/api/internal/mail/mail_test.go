package mail

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

// relay is a made-up mail relay: enough of SMTP to take one message, and to
// say what it was told along the way.
type relay struct {
	host     string
	port     int
	startTLS bool // whether it offers an encrypted connection
	cert     tls.Certificate
	roots    *x509.CertPool

	done chan struct{}
	auth string // the user name and password it was given, decoded
	from string
	to   string
	data string
}

func newRelay(t *testing.T, startTLS bool) *relay {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "made-up relay"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(der)
	roots := x509.NewCertPool()
	roots.AddCert(leaf)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	r := &relay{host: "127.0.0.1", port: ln.Addr().(*net.TCPAddr).Port, startTLS: startTLS,
		cert: tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, roots: roots, done: make(chan struct{})}
	go func() {
		defer close(r.done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		r.serve(conn)
	}()
	return r
}

func (r *relay) serve(conn net.Conn) {
	rd := bufio.NewReader(conn)
	say := func(line string) { conn.Write([]byte(line + "\r\n")) }
	say("220 relay ready")
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		verb := strings.ToUpper(strings.SplitN(line, " ", 2)[0])
		switch verb {
		case "EHLO":
			if _, encrypted := conn.(*tls.Conn); r.startTLS && !encrypted {
				say("250-relay")
				say("250 STARTTLS")
			} else {
				say("250-relay")
				say("250 AUTH PLAIN")
			}
		case "STARTTLS":
			say("220 go ahead")
			secure := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{r.cert}})
			if err := secure.Handshake(); err != nil {
				return
			}
			conn, rd = secure, bufio.NewReader(secure)
		case "AUTH":
			raw, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(line, "AUTH PLAIN "))
			r.auth = strings.ReplaceAll(string(raw), "\x00", "|")
			say("235 welcome")
		case "MAIL":
			r.from = line
			say("250 ok")
		case "RCPT":
			r.to = line
			say("250 ok")
		case "DATA":
			say("354 go ahead")
			var b strings.Builder
			for {
				l, err := rd.ReadString('\n')
				if err != nil || l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			r.data = b.String()
			say("250 queued")
		case "QUIT":
			say("221 bye")
			return
		default:
			say("500 what")
		}
	}
}

func TestSendThroughARelay(t *testing.T) {
	r := newRelay(t, true)
	s := &SMTP{Host: r.host, Port: r.port, User: "apikey", Password: "s3cret", From: "Still <hello@still.example>", roots: r.roots}
	err := s.Send(context.Background(), Message{To: "ana@example.com", Subject: "123456 is your Still sign-in code",
		Text: "Your sign-in code for Still is\n\n    123456\n\nÀ bientôt.\n"})
	if err != nil {
		t.Fatal(err)
	}
	<-r.done
	if r.auth != "|apikey|s3cret" {
		t.Errorf("the relay was given %q as the sign-in", r.auth)
	}
	if !strings.Contains(r.from, "<hello@still.example>") || !strings.Contains(r.to, "<ana@example.com>") {
		t.Errorf("envelope = %q / %q", r.from, r.to)
	}
	head, body, ok := strings.Cut(r.data, "\r\n\r\n")
	if !ok {
		t.Fatalf("no blank line between headers and text:\n%s", r.data)
	}
	for _, want := range []string{"From: Still <hello@still.example>\r\n", "To: ana@example.com\r\n",
		"Subject: 123456 is your Still sign-in code\r\n", "Content-Type: text/plain; charset=\"utf-8\"\r\n", "Message-ID: <", "Date: "} {
		if !strings.Contains(head+"\r\n", want) {
			t.Errorf("headers lack %q:\n%s", want, head)
		}
	}
	if body != "Your sign-in code for Still is\r\n\r\n    123456\r\n\r\nÀ bientôt.\r\n" {
		t.Errorf("text = %q", body)
	}
}

// A relay that cannot encrypt the connection is not given the password or the
// message.
func TestNothingIsSentInTheClear(t *testing.T) {
	r := newRelay(t, false)
	s := &SMTP{Host: r.host, Port: r.port, User: "apikey", Password: "s3cret", From: "hello@still.example", roots: r.roots}
	err := s.Send(context.Background(), Message{To: "ana@example.com", Subject: "code", Text: "123456"})
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("err = %v, want a refusal that names STARTTLS", err)
	}
	<-r.done
	if r.auth != "" || r.data != "" {
		t.Errorf("the relay still received auth %q, message %q", r.auth, r.data)
	}
}

func TestHeadersCannotBeInjected(t *testing.T) {
	s := &SMTP{Host: "127.0.0.1", Port: 1, From: "hello@still.example"}
	for _, m := range []Message{
		{To: "ana@example.com\r\nBcc: other@example.com", Subject: "code", Text: "x"},
		{To: "ana@example.com", Subject: "code\r\nBcc: other@example.com", Text: "x"},
	} {
		if err := s.Send(context.Background(), m); err == nil || !strings.Contains(err.Error(), "line break") {
			t.Errorf("Send(%q) = %v, want a refusal", m.To+m.Subject, err)
		}
	}
	if _, err := ParseFrom("not an address"); err == nil {
		t.Error("ParseFrom accepted nonsense")
	}
	if addr, err := ParseFrom("Still <hello@still.example>"); err != nil || addr != "hello@still.example" {
		t.Errorf("ParseFrom = %q, %v", addr, err)
	}
}
