package alertnotify

import (
	"testing"
	"time"
)

func TestTelegramRefFromMeta(t *testing.T) {
	meta := map[string]any{
		"telegram": map[string]any{
			"ok":         true,
			"chat_id":    "-1001234567890",
			"message_id": float64(8421),
		},
	}
	ref, ok := telegramRefFromMeta(meta)
	if !ok {
		t.Fatal("expected ref")
	}
	if ref.MessageID != 8421 || ref.ChatID != "-1001234567890" {
		t.Fatalf("ref=%+v", ref)
	}
}

func TestTelegramRefFromMetaMissing(t *testing.T) {
	_, ok := telegramRefFromMeta(map[string]any{"telegram": map[string]any{"ok": false}})
	if ok {
		t.Fatal("expected no ref when send failed")
	}
}

func TestTelegramResolutionReplyShort(t *testing.T) {
	got := telegramResolutionReplyShort("latency_high", 28*time.Minute)
	if got != "✅ Resolvido após 28 min." {
		t.Fatalf("got %q", got)
	}
	got = telegramResolutionReplyShort("ping_unreachable", 28*time.Minute)
	if got != "✅ Equipamento online após 28 min." {
		t.Fatalf("got %q", got)
	}
}

func TestTelegramResolvedEditBlocks(t *testing.T) {
	start := time.Date(2026, 6, 18, 14, 3, 0, 0, time.UTC)
	end := time.Date(2026, 6, 18, 14, 31, 0, 0, time.UTC)
	text := telegramResolvedEditBlocks("ping_unreachable", "Offline", "OLT POP Sul (10.0.0.1): ping indisponível", "OLT POP Sul", start, &end)
	if !containsAll(text, "🟢 EQUIPAMENTO ONLINE", "OLT POP Sul", "Início: 14:03", "Fim: 14:31", "Duração: 28 min") {
		t.Fatalf("edit text:\n%s", text)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !contains(s, p) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
