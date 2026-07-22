package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strconv"

	"github.com/rs/zerolog/log"
	"thuanle/cse-mark/internal/configs"
)

// SmtpSender implements Sender over plain SMTP. It authenticates with the
// configured credentials and sends a minimal OTP message. TLS is used when the
// server advertises STARTTLS (the common case for port 587); port 465 (implicit
// TLS) is handled by the custom tls.Dial path below.
type SmtpSender struct {
	host     string
	port     int
	username string
	password string
	from     string
}

// NewSmtpSender builds an SMTP sender from config. It does not open a
// connection; the connection is established per SendOTP, matching the lifecycle
// of a single OTP send.
func NewSmtpSender(config *configs.Config) *SmtpSender {
	return &SmtpSender{
		host:     config.SmtpHost,
		port:     config.SmtpPort,
		username: config.SmtpUsername,
		password: config.SmtpPassword,
		from:     config.SmtpFrom,
	}
}

// otpMessageBody builds the RFC 822 message. The OTP is the only dynamic value;
// the subject and headers are fixed. We keep it plain-text so mail clients do
// not reflow or hide the code.
func otpMessageBody(from, to, otp string) []byte {
	headers := fmt.Sprintf("From: %s\r\n", from) +
		fmt.Sprintf("To: %s\r\n", to) +
		"Subject: CSE Mark — Ma xac thuc (OTP)\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n"
	body := "Ma xac thuc cua ban la: " + otp + "\r\n"
	return []byte(headers + body)
}

// SendOTP delivers the OTP. Implicit-TLS port (465) dials a TLS connection
// directly; everything else uses net.Dial + smtp with STARTTLS upgrade.
func (s *SmtpSender) SendOTP(ctx context.Context, to string, otp string) error {
	addr := net.JoinHostPort(s.host, strconv.Itoa(s.port))
	msg := otpMessageBody(s.from, to, otp)

	// Respect context cancellation for the dial phase.
	d := &net.Dialer{}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if s.isImplicitTLS() {
		return s.sendImplicitTLS(conn, to, msg)
	}
	return s.sendStartTLS(conn, to, msg)
}

// isImplicitTLS treats port 465 as implicit-TLS (the legacy SMTPS port). All
// other ports (notably 587) use opportunistic STARTTLS.
func (s *SmtpSender) isImplicitTLS() bool {
	return s.port == 465
}

func (s *SmtpSender) sendStartTLS(conn net.Conn, to string, msg []byte) error {
	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer func() { _ = c.Quit() }()

	// STARTTLS when available, then authenticate.
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: s.host}); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}
	if err := s.auth(c); err != nil {
		return err
	}
	return s.mail(c, to, msg)
}

func (s *SmtpSender) sendImplicitTLS(conn net.Conn, to string, msg []byte) error {
	tlsConn := tls.Client(conn, &tls.Config{ServerName: s.host})
	if err := tlsConn.Handshake(); err != nil {
		return fmt.Errorf("smtp tls handshake: %w", err)
	}
	c, err := smtp.NewClient(tlsConn, s.host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer func() { _ = c.Quit() }()

	if err := s.auth(c); err != nil {
		return err
	}
	return s.mail(c, to, msg)
}

func (s *SmtpSender) auth(c *smtp.Client) error {
	if s.username == "" && s.password == "" {
		return nil
	}
	auth := smtp.PlainAuth("", s.username, s.password, s.host)
	if err := c.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}
	return nil
}

func (s *SmtpSender) mail(c *smtp.Client, to string, msg []byte) error {
	if err := c.Mail(s.from); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("smtp RCPT TO: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close body: %w", err)
	}
	log.Debug().Str("to", to).Msg("OTP email sent")
	return nil
}
