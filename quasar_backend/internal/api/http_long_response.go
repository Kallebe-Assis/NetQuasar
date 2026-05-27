package api

import (
	"net/http"
	"time"
)

// extendWriteDeadline alarga o prazo de escrita HTTP para respostas longas (SNMP OLT).
// Sem isto, o WriteTimeout global do servidor (ex.: 120s) corta refresh OLT a meio.
func extendWriteDeadline(w http.ResponseWriter, d time.Duration) {
	if d <= 0 {
		d = 20 * time.Minute
	}
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Now().Add(d))
}
