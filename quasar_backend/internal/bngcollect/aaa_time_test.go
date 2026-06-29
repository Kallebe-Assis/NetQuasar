package bngcollect

import (
	"testing"
	"time"
)

func TestFormatAAARecordTime_SevenByteHex(t *testing.T) {
	raw := "07:ea:06:1c:17:38:28"
	got := FormatAAARecordTime(raw)
	if b, ok := decodeSNMPDateTimeBytes(raw); ok {
		if parsed, ok := dateTimeFromBytes(b); ok && parsed.After(time.Now().Add(aaaFutureSkew)) {
			if got != raw {
				t.Fatalf("expected raw SNMP for future date, got %q", got)
			}
			return
		}
	}
	if got == "" || got == raw {
		t.Fatalf("expected parsed datetime, got %q", got)
	}
}

func TestFormatAAARecordTime_FakeIPv4(t *testing.T) {
	raw := "7.234.6.29"
	got := FormatAAARecordTime(raw)
	if parsed, ok := parseFakeIPv4AsDateTime(raw); ok && parsed.After(time.Now().Add(aaaFutureSkew)) {
		if got != raw {
			t.Fatalf("expected raw value for future fake IPv4, got %q", got)
		}
		return
	}
	if got == "" || got == raw {
		t.Fatalf("expected parsed date from fake IPv4, got %q", got)
	}
}

func TestFormatAuthLogMessage(t *testing.T) {
	rec := AuthAttemptLog{
		Kind:   "failure",
		Time:   "2026-06-28 21:34:30",
		Login:  "marianafernandes",
		MAC:    "c0:94:ad:4b:9e:3f",
		Port:   "1084446",
		Seq:    "8758605",
		Reason: "Login incorrect (Failed retrieving values required to evaluate condition)",
	}
	finalizeAuthRecord(&rec, "")
	if rec.Message == "" {
		t.Fatal("expected message")
	}
	if !containsAll(rec.Message, "marianafernandes", "C0:94:AD:4B:9E:3F", "Falha no login", "1084446") {
		t.Fatalf("unexpected message: %s", rec.Message)
	}
}

func TestAuthTimeAcceptable_RejectsFutureDateOnly(t *testing.T) {
	t.Parallel()
	now := time.Now()
	futureDay := now.AddDate(0, 0, 2)
	future := time.Date(futureDay.Year(), futureDay.Month(), futureDay.Day(), 0, 0, 0, 0, futureDay.Location())
	if authTimeAcceptable(future, authMaxAge) {
		t.Fatal("expected future date-only timestamp to be rejected")
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !containsFold(s, p) {
			return false
		}
	}
	return true
}

func containsFold(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			ok := true
			for j := 0; j < len(sub); j++ {
				a := s[i+j]
				b := sub[j]
				if a >= 'A' && a <= 'Z' {
					a += 'a' - 'A'
				}
				if b >= 'A' && b <= 'Z' {
					b += 'a' - 'A'
				}
				if a != b {
					ok = false
					break
				}
			}
			if ok {
				return true
			}
		}
		return false
	})())
}
