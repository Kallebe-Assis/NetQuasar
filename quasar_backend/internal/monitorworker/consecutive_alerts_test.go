package monitorworker

import "testing"

func TestConsecutivePingsRequired(t *testing.T) {
	if consecutivePingsRequired(0) != 3 {
		t.Fatal("min 3")
	}
	if consecutivePingsRequired(2) != 3 {
		t.Fatal("2 -> 3")
	}
	if consecutivePingsRequired(3) != 3 {
		t.Fatal("3 -> 3")
	}
	if consecutivePingsRequired(5) != 5 {
		t.Fatal("5 -> 5")
	}
}

func TestConsecutiveLatencyRequired(t *testing.T) {
	if consecutiveLatencyRequired(0) != 2 {
		t.Fatal("want 2")
	}
	if consecutiveLatencyRequired(5) != 2 {
		t.Fatal("latency always 2 coletas")
	}
}
