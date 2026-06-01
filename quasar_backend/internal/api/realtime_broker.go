package api

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

type realtimeEvent struct {
	Type string         `json:"type"`
	At   string         `json:"at"`
	Data map[string]any `json:"data"`
}

type realtimeBroker struct {
	log     zerolog.Logger
	redis   *redis.Client
	channel string
	mu      sync.RWMutex
	conns   map[*websocket.Conn]struct{}
}

func newRealtimeBroker(log zerolog.Logger, redisURL string) *realtimeBroker {
	b := &realtimeBroker{
		log:     log.With().Str("component", "realtime_broker").Logger(),
		channel: "netquasar:realtime",
		conns:   map[*websocket.Conn]struct{}{},
	}
	if strings.TrimSpace(redisURL) == "" {
		return b
	}
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		b.log.Warn().Err(err).Msg("NETQUASAR_REDIS_URL inválida; seguindo sem Redis")
		return b
	}
	b.redis = redis.NewClient(opt)
	return b
}

func (b *realtimeBroker) Start(ctx context.Context) {
	if b == nil || b.redis == nil {
		return
	}
	sub := b.redis.Subscribe(ctx, b.channel)
	go func() {
		defer func() { _ = sub.Close() }()
		ch := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok || strings.TrimSpace(msg.Payload) == "" {
					continue
				}
				b.broadcastRaw([]byte(msg.Payload))
			}
		}
	}()
}

func (b *realtimeBroker) publish(ctx context.Context, typ string, data map[string]any) {
	if b == nil {
		return
	}
	ev := realtimeEvent{
		Type: strings.TrimSpace(typ),
		At:   time.Now().UTC().Format(time.RFC3339),
		Data: data,
	}
	raw, _ := json.Marshal(ev)
	b.broadcastRaw(raw)
	if b.redis != nil {
		_ = b.redis.Publish(ctx, b.channel, string(raw)).Err()
	}
}

func (b *realtimeBroker) addConn(c *websocket.Conn) {
	if b == nil || c == nil {
		return
	}
	b.mu.Lock()
	b.conns[c] = struct{}{}
	b.mu.Unlock()
}

func (b *realtimeBroker) removeConn(c *websocket.Conn) {
	if b == nil || c == nil {
		return
	}
	b.mu.Lock()
	delete(b.conns, c)
	b.mu.Unlock()
	_ = c.Close()
}

func (b *realtimeBroker) broadcastRaw(raw []byte) {
	if b == nil || len(raw) == 0 {
		return
	}
	b.mu.RLock()
	conns := make([]*websocket.Conn, 0, len(b.conns))
	for c := range b.conns {
		conns = append(conns, c)
	}
	b.mu.RUnlock()
	for _, c := range conns {
		_ = c.SetWriteDeadline(time.Now().Add(4 * time.Second))
		if err := c.WriteMessage(websocket.TextMessage, raw); err != nil {
			b.removeConn(c)
		}
	}
}

