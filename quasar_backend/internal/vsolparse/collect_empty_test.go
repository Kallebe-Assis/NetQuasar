package vsolparse

import (
	"context"
	"strings"
	"testing"
)

func TestCollectOLT_noRefs(t *testing.T) {
	r := CollectOLT(context.Background(), "10.0.0.1", "public", nil, true)
	if !strings.Contains(r.Note, "sem_indices") {
		t.Fatalf("note %q", r.Note)
	}
}
