package sysevents

import "testing"

func TestEventTypeConstants(t *testing.T) {
	t.Parallel()
	if TypeAlertOpened != "alert.opened" {
		t.Fatalf("opened: %q", TypeAlertOpened)
	}
	if TypeAlertClosed != "alert.closed" {
		t.Fatalf("closed: %q", TypeAlertClosed)
	}
	if TypeDeviceChecks != "device.checks" {
		t.Fatalf("checks: %q", TypeDeviceChecks)
	}
}
