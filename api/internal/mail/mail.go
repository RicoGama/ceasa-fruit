// Package mail monta e envia e-mails. Todo envio do sistema é por SMTP.
//
// Ninguém chama Send direto numa requisição HTTP: as rotas colocam o e-mail
// na fila (pacote queue) e um worker envia em segundo plano. Assim a resposta
// ao usuário não espera o servidor SMTP, e se ele estiver fora do ar o envio
// é tentado de novo depois.
package mail

import (
	"context"
	"fmt"
	"net/mail"
	"time"

	gomail "github.com/wneessen/go-mail"

	"github.com/RicoGama/ceasa-fruit/api/internal/config"
)

// Message é um e-mail pronto para enviar. Os campos têm tags json porque a
// mensagem é guardada como argumento do job na fila (tabela river_job).
type Message struct {
	To      string `json:"to"`
	ToName  string `json:"to_name,omitempty"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
	Text    string `json:"text"` // versão em texto puro, para clientes sem HTML
}

// Mailer é qualquer coisa que envia uma Message. Em produção é o SMTP; nos
// testes, um falso que só guarda as mensagens.
type Mailer interface {
	Send(ctx context.Context, msg Message) error
}

// SMTPMailer envia pelo servidor SMTP configurado.
type SMTPMailer struct {
	cfg  config.SMTP
	from *mail.Address
}

// NewSMTPMailer valida a configuração (o remetente precisa ser um endereço
// válido) e devolve o mailer.
func NewSMTPMailer(cfg config.SMTP) (*SMTPMailer, error) {
	from, err := mail.ParseAddress(cfg.From)
	if err != nil {
		return nil, fmt.Errorf("invalid SMTP_FROM %q: %w", cfg.From, err)
	}
	switch cfg.TLS {
	case "none", "starttls", "tls":
	default:
		return nil, fmt.Errorf("invalid SMTP_TLS %q (use none, starttls or tls)", cfg.TLS)
	}
	return &SMTPMailer{cfg: cfg, from: from}, nil
}

func (m *SMTPMailer) Send(ctx context.Context, msg Message) error {
	email := gomail.NewMsg()
	if err := email.FromFormat(m.from.Name, m.from.Address); err != nil {
		return err
	}
	if err := email.AddToFormat(msg.ToName, msg.To); err != nil {
		return err
	}
	email.Subject(msg.Subject)
	email.SetDate()
	email.SetMessageID()
	email.SetBodyString(gomail.TypeTextPlain, msg.Text)
	email.AddAlternativeString(gomail.TypeTextHTML, msg.HTML)

	opts := []gomail.Option{
		gomail.WithPort(m.cfg.Port),
		gomail.WithTimeout(20 * time.Second),
	}
	switch m.cfg.TLS {
	case "none":
		opts = append(opts, gomail.WithTLSPolicy(gomail.NoTLS))
	case "starttls":
		opts = append(opts, gomail.WithTLSPolicy(gomail.TLSMandatory))
	case "tls":
		opts = append(opts, gomail.WithSSL())
	}
	if m.cfg.Username != "" {
		opts = append(opts,
			gomail.WithSMTPAuth(gomail.SMTPAuthAutoDiscover), // escolhe o melhor método que o servidor aceitar
			gomail.WithUsername(m.cfg.Username),
			gomail.WithPassword(m.cfg.Password),
		)
	}

	client, err := gomail.NewClient(m.cfg.Host, opts...)
	if err != nil {
		return err
	}
	if err := client.DialAndSendWithContext(ctx, email); err != nil {
		return fmt.Errorf("sending email to %s: %w", msg.To, err)
	}
	return nil
}
