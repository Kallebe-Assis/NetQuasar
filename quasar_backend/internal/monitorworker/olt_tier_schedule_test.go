package monitorworker

import (
	"testing"
	"time"
)

func TestOltFullScheduleDue(t *testing.T) {
	now := mustParseLocal("2026-07-16T03:05:00")
	target := "03:00"
	if !oltFullScheduleDue(target, nil, now) {
		t.Fatal("expected due when never ran")
	}
	last := mustParseLocal("2026-07-16T03:01:00")
	if oltFullScheduleDue(target, &last, now) {
		t.Fatal("already ran today after schedule")
	}
	yesterday := mustParseLocal("2026-07-15T03:01:00")
	if !oltFullScheduleDue(target, &yesterday, now) {
		t.Fatal("should run again next day")
	}
	before := mustParseLocal("2026-07-16T02:50:00")
	if oltFullScheduleDue(target, nil, before) {
		t.Fatal("before schedule hour")
	}
}

func mustParseLocal(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02T15:04:05", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}
