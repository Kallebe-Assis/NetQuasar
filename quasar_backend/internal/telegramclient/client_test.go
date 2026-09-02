package telegramclient

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestBuildSendMessagePayload_TopicIDIsInteger cobre o bug real em produção: mensagens de
// "relatórios" (tópico configurado) caíam no tópico "Geral" do grupo do Telegram em vez do
// tópico certo, porque message_thread_id ia como string no JSON — a API do Telegram exige
// Integer e ignora o campo quando o tipo não bate, sem erro nenhum.
func TestBuildSendMessagePayload_TopicIDIsInteger(t *testing.T) {
	cfg := Config{BotToken: "x", ChatID: "-100123", TopicID: "4680"}
	payload := buildSendMessagePayload(cfg, "olá", SendOpts{})
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	v, ok := decoded["message_thread_id"]
	if !ok {
		t.Fatalf("message_thread_id ausente no payload: %s", raw)
	}
	if _, isNumber := v.(float64); !isNumber {
		t.Fatalf("message_thread_id devia ser Integer no JSON, veio %T (%v) — payload: %s", v, v, raw)
	}
	if !strings.Contains(string(raw), `"message_thread_id":4680`) {
		t.Fatalf("esperava message_thread_id sem aspas no JSON, veio: %s", raw)
	}
}

func TestBuildSendMessagePayload_NoTopicIDOmitsField(t *testing.T) {
	cfg := Config{BotToken: "x", ChatID: "-100123"}
	payload := buildSendMessagePayload(cfg, "olá", SendOpts{})
	if _, ok := payload["message_thread_id"]; ok {
		t.Fatalf("message_thread_id não devia aparecer sem TopicID: %+v", payload)
	}
}

func TestParseTelegramMessageResult(t *testing.T) {
	body := []byte(`{"ok":true,"result":{"message_id":8421,"chat":{"id":-1001234567890}}}`)
	res, err := parseTelegramMessageResult(body)
	if err != nil {
		t.Fatal(err)
	}
	if res.MessageID != 8421 {
		t.Fatalf("message_id=%d", res.MessageID)
	}
	if res.ChatID != "-1001234567890" {
		t.Fatalf("chat_id=%q", res.ChatID)
	}
}

func TestSplitMessage(t *testing.T) {
	short := "hello"
	parts := SplitMessage(short, 100)
	if len(parts) != 1 || parts[0] != short {
		t.Fatalf("short: %#v", parts)
	}
	var lines []string
	for i := 0; i < 50; i++ {
		lines = append(lines, "linha-com-conteudo-suficiente-para-testar-particao")
	}
	long := strings.Join(lines, "\n")
	parts = SplitMessage(long, 200)
	if len(parts) < 2 {
		t.Fatalf("expected multiple parts, got %d", len(parts))
	}
	for i, p := range parts {
		if len(p) > 200 {
			t.Fatalf("part %d len=%d > 200", i, len(p))
		}
	}
	joined := strings.Join(parts, "\n")
	if !strings.Contains(joined, lines[0]) || !strings.Contains(joined, lines[len(lines)-1]) {
		t.Fatal("missing content after split")
	}
}
