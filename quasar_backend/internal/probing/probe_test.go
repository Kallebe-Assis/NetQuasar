package probing

import (
	"context"
	"testing"
	"time"
)

func TestTCPProbe_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := TCPProbe(ctx, "192.0.2.1", "443", time.Second)
	if err == nil {
		t.Fatal("esperava erro após cancelamento")
	}
}
