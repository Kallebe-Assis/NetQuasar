package alertnotify

import (
	"testing"
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

func TestTelegramRefFromMetaPrefersProblemRef(t *testing.T) {
	meta := map[string]any{
		"telegram": map[string]any{
			"ok": true, "chat_id": "-1001", "message_id": float64(999),
		},
		"telegram_problem_ref": map[string]any{
			"ok": true, "chat_id": "-1001", "message_id": float64(42),
		},
	}
	ref, ok := telegramRefFromMeta(meta)
	if !ok || ref.MessageID != 42 {
		t.Fatalf("ref=%+v ok=%v", ref, ok)
	}
}

func TestTelegramResolutionReplyToProblemOnly(t *testing.T) {
	text := telegramResolutionBlocks("latency_high", "Latência normalizada",
		"OLT (10.0.0.1): latência ICMP/TCP em 45 ms.", "OLT", "10.0.0.1", "45 ms", "Duração: 10 min")
	if !containsAll(text, "🟢 ALERTA RESOLVIDO", "Latência normalizada", "Latência = 45 ms", "OLT", "10.0.0.1", "Duração: 10 min") {
		t.Fatalf("resolution text:\n%s", text)
	}
}

func TestResolvedValueFromMetaLatency(t *testing.T) {
	meta := map[string]any{"resolved_value": "45 ms"}
	if got := resolvedValueFromMeta("latency_high", meta); got != "45 ms" {
		t.Fatalf("got %q", got)
	}
	meta = map[string]any{"curr_latency_ms": int64(32)}
	if got := resolvedValueFromMeta("latency_high", meta); got != "32 ms" {
		t.Fatalf("got %q", got)
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
