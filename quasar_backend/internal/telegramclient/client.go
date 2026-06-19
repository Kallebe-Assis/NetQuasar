package telegramclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
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

// SendResult dados devolvidos pela API após sendMessage.
type SendResult struct {
	MessageID int64
	ChatID    string
}

// SendOpts opções adicionais de envio.
type SendOpts struct {
	ParseMode        string
	ReplyToMessageID int64
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
	_, err := SendMessageWithResult(ctx, cfg, text, SendOpts{})
	return err
}

// SendMessageWithParseMode envia texto; parseMode pode ser "" (texto simples), "HTML" ou "MarkdownV2".
func SendMessageWithParseMode(ctx context.Context, cfg Config, text string, parseMode string) error {
	_, err := SendMessageWithResult(ctx, cfg, text, SendOpts{ParseMode: parseMode})
	return err
}

// SendMessageWithResult envia mensagem e devolve message_id/chat_id para threading.
func SendMessageWithResult(ctx context.Context, cfg Config, text string, opts SendOpts) (SendResult, error) {
	if !cfg.Ready() {
		return SendResult{}, fmt.Errorf("configuração incompleta (token/chat_id)")
	}
	payload := map[string]any{
		"chat_id": cfg.ChatID,
		"text":    text,
	}
	if opts.ParseMode != "" {
		payload["parse_mode"] = opts.ParseMode
	}
	if cfg.TopicID != "" {
		payload["message_thread_id"] = cfg.TopicID
	}
	if opts.ReplyToMessageID > 0 {
		payload["reply_to_message_id"] = opts.ReplyToMessageID
	}
	return postTelegram(ctx, cfg, "sendMessage", payload)
}

// EditMessageText actualiza o texto de uma mensagem já enviada.
func EditMessageText(ctx context.Context, cfg Config, chatID string, messageID int64, text string) error {
	if !cfg.Ready() {
		return fmt.Errorf("configuração incompleta (token/chat_id)")
	}
	if messageID <= 0 {
		return fmt.Errorf("message_id inválido")
	}
	chat := strings.TrimSpace(chatID)
	if chat == "" {
		chat = cfg.ChatID
	}
	payload := map[string]any{
		"chat_id":    chat,
		"message_id": messageID,
		"text":       text,
	}
	_, err := postTelegram(ctx, cfg, "editMessageText", payload)
	return err
}

func postTelegram(ctx context.Context, cfg Config, method string, payload map[string]any) (SendResult, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return SendResult{}, err
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/%s", cfg.BotToken, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return SendResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 12 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return SendResult{}, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 8192))
	if res.StatusCode >= 300 {
		return SendResult{}, fmt.Errorf("telegram %s: %s", res.Status, strings.TrimSpace(string(body)))
	}
	return parseTelegramMessageResult(body)
}

func parseTelegramMessageResult(body []byte) (SendResult, error) {
	var envelope struct {
		OK          bool            `json:"ok"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return SendResult{}, fmt.Errorf("resposta telegram inválida: %w", err)
	}
	if !envelope.OK {
		if envelope.Description != "" {
			return SendResult{}, fmt.Errorf("telegram api: %s", envelope.Description)
		}
		return SendResult{}, fmt.Errorf("telegram api: ok=false")
	}
	if len(envelope.Result) == 0 {
		return SendResult{}, nil
	}
	var msg struct {
		MessageID int64 `json:"message_id"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	}
	if err := json.Unmarshal(envelope.Result, &msg); err != nil {
		return SendResult{}, nil
	}
	chatID := strconv.FormatInt(msg.Chat.ID, 10)
	if chatID == "0" {
		chatID = ""
	}
	return SendResult{MessageID: msg.MessageID, ChatID: chatID}, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
