package telegramclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	BotToken string
	ChatID   string
	TopicID  string
}

func (c Config) Ready() bool {
	return strings.TrimSpace(c.BotToken) != "" && strings.TrimSpace(c.ChatID) != ""
}

func LoadConfig(ctx context.Context, pool *pgxpool.Pool, id string) (Config, error) {
	var tok, chat, topic *string
	err := pool.QueryRow(ctx, `SELECT bot_token, chat_id, topic_id FROM settings_telegram WHERE id=$1`, id).Scan(&tok, &chat, &topic)
	if err == pgx.ErrNoRows {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	return Config{
		BotToken: strings.TrimSpace(deref(tok)),
		ChatID:   strings.TrimSpace(deref(chat)),
		TopicID:  strings.TrimSpace(deref(topic)),
	}, nil
}

func SendMessage(ctx context.Context, cfg Config, text string) error {
	return SendMessageWithParseMode(ctx, cfg, text, "")
}

// SendMessageWithParseMode envia texto; parseMode pode ser "" (texto simples), "HTML" ou "MarkdownV2" (ver API Telegram).
func SendMessageWithParseMode(ctx context.Context, cfg Config, text string, parseMode string) error {
	if !cfg.Ready() {
		return fmt.Errorf("configuração incompleta (token/chat_id)")
	}
	payload := map[string]any{
		"chat_id": cfg.ChatID,
		"text":    text,
	}
	if parseMode != "" {
		payload["parse_mode"] = parseMode
	}
	if cfg.TopicID != "" {
		payload["message_thread_id"] = cfg.TopicID
	}
	raw, _ := json.Marshal(payload)
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", cfg.BotToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 12 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
	if res.StatusCode >= 300 {
		return fmt.Errorf("telegram %s: %s", res.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

