package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/emersion/go-smtp"
)

// The backend structure is called for authentication of a new session.
type SMTPBackend struct {
	smtp.Backend
}

// During the process of receiving an email, this session is called.
type SMTPSession struct {
	smtp.Session
	remoteAddr net.Addr
	from       string
	to         string
}

// On login, we do not care about authentication. So we just start a new session and provide it ;)
func (b *SMTPBackend) Login(state *smtp.ConnectionState, username, password string) (smtp.Session, error) {
	return &SMTPSession{
		remoteAddr: state.RemoteAddr,
	}, nil
}

// We want to receive all emails, including anonymous emails.
func (b *SMTPBackend) AnonymousLogin(state *smtp.ConnectionState) (smtp.Session, error) {
	return &SMTPSession{
		remoteAddr: state.RemoteAddr,
	}, nil //return nil, smtp.ErrAuthRequired
}

// The session has provided mail options and who the message is from.
func (s *SMTPSession) Mail(from string, opts smtp.MailOptions) error {
	// Store the from in the session for when we receive the final message data.
	s.from = from
	return nil
}

// The session has provided who the mail is to.
func (s *SMTPSession) Rcpt(to string) error {
	// Store who the mail is to for final message data.
	s.to = to
	return nil
}

// The session has provided the data for the message.
func (s *SMTPSession) Data(r io.Reader) error {
	// Save the message to the database.
	err := MailSaveMessage(s.remoteAddr.String(), s.from, s.to, r)
	if err != nil {
		log.Println("Unable to parse email:", err)
	}
	return nil
}

// When the SMTP session is requested to start over.
func (s *SMTPSession) Reset() {}

// When the session is done completely.
func (s *SMTPSession) Logout() error {
	return nil
}

// This function starts the SMTP server.
func SMTPServe() {
	// Get the configuration.
	smtpBindAddr := app.config.SMTPBindAddr
	smtpPort := app.config.SMTPPort
	smtpDomain := app.config.SMTPDomain
	if app.context.String("smtp-bind") != "" {
		smtpBindAddr = app.context.String("smtp-bind")
	}
	if app.context.Uint("smtp-port") != 0 {
		smtpPort = app.context.Uint("smtp-port")
	}
	if app.context.String("smtp-domain") != "" {
		smtpDomain = app.context.String("smtp-domain")
	}

	// Create the SMTP server with our custom backend.
	smtpBackend := &SMTPBackend{}
	smtpServer := smtp.NewServer(smtpBackend)
	app.smtpServer = smtpServer

	// Configure the SMTP server.
	smtpServer.Addr = fmt.Sprintf("%s:%d", smtpBindAddr, smtpPort)
	smtpServer.Domain = smtpDomain
	smtpServer.ReadTimeout = 10 * time.Second
	smtpServer.WriteTimeout = 10 * time.Second
	smtpServer.MaxMessageBytes = app.config.MaxMessageSize
	smtpServer.MaxRecipients = 50
	smtpServer.AllowInsecureAuth = true

	// Start the server.
	log.Println("Starting smtp server on port", smtpPort)
	if err := smtpServer.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
