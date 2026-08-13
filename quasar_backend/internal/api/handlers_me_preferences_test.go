package api

import "testing"

func TestNormalizeUserPreferencesDefaults(t *testing.T) {
	got := normalizeUserPreferences(defaultUserPreferences())
	if got.Theme != "dark" {
		t.Fatalf("theme=%q", got.Theme)
	}
	if !got.AlertToastEverywhere || !got.AlertSoundEnabled {
		t.Fatalf("switches should default on: %+v", got)
	}
	if got.AlertSoundID != builtinAlertSoundID {
		t.Fatalf("sound=%q", got.AlertSoundID)
	}
	empty := normalizeUserPreferences(userPreferences{})
	if empty.Theme != "dark" || empty.AlertSoundID != builtinAlertSoundID {
		t.Fatalf("empty sanitize %+v", empty)
	}
}

func TestNormalizeUserPreferencesKeepsValid(t *testing.T) {
	got := normalizeUserPreferences(userPreferences{
		Theme:                "light",
		AlertToastEverywhere: false,
		AlertSoundEnabled:    false,
		AlertSoundID:         "builtin:chime",
	})
	if got.Theme != "light" || got.AlertToastEverywhere || got.AlertSoundEnabled || got.AlertSoundID != "builtin:chime" {
		t.Fatalf("got %+v", got)
	}
}

func TestLooksLikeMP3(t *testing.T) {
	if looksLikeMP3([]byte("ID3....")) != true {
		t.Fatal("ID3")
	}
	if looksLikeMP3([]byte{0xFF, 0xFB, 0x90}) != true {
		t.Fatal("frame")
	}
	if looksLikeMP3([]byte("RIFF")) {
		t.Fatal("wav should fail")
	}
}
