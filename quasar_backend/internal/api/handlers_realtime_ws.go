package api

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var realtimeUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (s *Server) realtimeWS(w http.ResponseWriter, r *http.Request) {
	if s.rt == nil {
		writeErr(w, http.StatusServiceUnavailable, "UNAVAILABLE", "tempo real indisponível", nil)
		return
	}
	c, err := realtimeUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	s.rt.addConn(c)
	defer s.rt.removeConn(c)
	s.rt.publish(r.Context(), "realtime.connected", map[string]any{"message": "connected"})

	for {
		_ = c.SetReadDeadline(time.Now().Add(90 * time.Second))
		mt, msg, err := c.ReadMessage()
		if err != nil {
			return
		}
		if mt == websocket.PingMessage {
			_ = c.WriteMessage(websocket.PongMessage, nil)
			continue
		}
		if string(msg) == "ping" {
			_ = c.WriteMessage(websocket.TextMessage, []byte(`{"type":"pong"}`))
		}
	}
}

