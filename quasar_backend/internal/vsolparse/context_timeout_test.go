package vsolparse

import (
	"testing"
	"time"
)

func TestCollectTimeout_scalesWithOnuCount(t *testing.T) {
	small := CollectTimeout(50, false)
	large := CollectTimeout(500, true)
	if large < small {
		t.Fatalf("large %v small %v", large, small)
	}
	if small < 90*time.Second {
		t.Fatalf("small %v", small)
	}
}
