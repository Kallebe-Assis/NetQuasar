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
