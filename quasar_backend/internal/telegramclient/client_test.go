package telegramclient

import "testing"

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

func TestParseTelegramMessageResultError(t *testing.T) {
	_, err := parseTelegramMessageResult([]byte(`{"ok":false,"description":"bad"}`))
	if err == nil {
		t.Fatal("expected error")
	}
}
