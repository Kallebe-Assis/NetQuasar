package mailclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	Enabled     bool
	Host        string
	Port        int
	Username    string
	Password    string
	FromAddress string
	UseTLS      bool
}

func (c Config) Ready() bool {
	return c.Enabled && strings.TrimSpace(c.Host) != "" && c.Port > 0 && strings.TrimSpace(c.FromAddress) != ""
}

func LoadConfig(ctx context.Context, pool *pgxpool.Pool) (Config, error) {
	var c Config
	if pool == nil {
		return c, fmt.Errorf("pool nil")
	}
	err := pool.QueryRow(ctx, `
		SELECT enabled, COALESCE(host,''), port, COALESCE(username,''), COALESCE(password,''),
			COALESCE(from_address,''), use_tls
		FROM settings_smtp WHERE id = 1
	`).Scan(&c.Enabled, &c.Host, &c.Port, &c.Username, &c.Password, &c.FromAddress, &c.UseTLS)
	return c, err
}

func Send(ctx context.Context, cfg Config, to []string, subject, body string) error {
	_ = ctx
	if !cfg.Ready() {
		return fmt.Errorf("SMTP não configurado")
	}
	recipients := make([]string, 0, len(to))
	for _, t := range to {
		t = strings.TrimSpace(t)
		if t != "" {
			recipients = append(recipients, t)
		}
	}
	if len(recipients) == 0 {
		return fmt.Errorf("destinatário de e-mail vazio")
	}
	from := strings.TrimSpace(cfg.FromAddress)
	msg := strings.Join([]string{
		"From: " + from,
		"To: " + strings.Join(recipients, ", "),
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")
	addr := fmt.Sprintf("%s:%d", strings.TrimSpace(cfg.Host), cfg.Port)
	auth := smtp.PlainAuth("", cfg.Username, cfg.Password, strings.TrimSpace(cfg.Host))
	if cfg.UseTLS {
		tlsCfg := &tls.Config{ServerName: strings.TrimSpace(cfg.Host)}
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return err
		}
		defer conn.Close()
		c, err := smtp.NewClient(conn, cfg.Host)
		if err != nil {
			return err
		}
		defer c.Close()
		if cfg.Username != "" {
			if err := c.Auth(auth); err != nil {
				return err
			}
		}
		if err := c.Mail(from); err != nil {
			return err
		}
		for _, rcpt := range recipients {
			if err := c.Rcpt(rcpt); err != nil {
				return err
			}
		}
		w, err := c.Data()
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte(msg)); err != nil {
			_ = w.Close()
			return err
		}
		return w.Close()
	}
	return smtp.SendMail(addr, auth, from, recipients, []byte(msg))
}

func ParseRecipients(raw string) []string {
	raw = strings.ReplaceAll(raw, ";", ",")
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
