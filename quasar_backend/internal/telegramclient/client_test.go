package telegramclient

import (
	"strings"
	"testing"
)

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
