package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/vsolparse"
)

// onuClientNameMap devolve serial (maiúsculas) -> nome do cliente vinculado.
func (s *Server) onuClientNameMap(ctx context.Context) (map[string]string, error) {
	rows, err := s.DB().Query(ctx, `SELECT serial, client_name FROM onu_client_links`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var serial, name string
		if err := rows.Scan(&serial, &name); err != nil {
			return nil, err
		}
		m[strings.ToUpper(strings.TrimSpace(serial))] = name
	}
	return m, rows.Err()
}

// allOltOnuSerials devolve o conjunto de seriais de ONU vistos em qualquer snapshot de
// OLT (todas as portas, online ou offline) — usado para validar o CSV de vínculo antes
// de gravar, para não aceitar seriais que não pertencem a nenhuma ONU conhecida.
func (s *Server) allOltOnuSerials(ctx context.Context) (map[string]bool, error) {
	rows, err := s.DB().Query(ctx, `
		SELECT o.summary::text
		FROM devices d
		JOIN olt_snapshots o ON o.device_id = d.id
		WHERE lower(trim(d.category)) = 'olt'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	set := map[string]bool{}
	for rows.Next() {
		var sum string
		if err := rows.Scan(&sum); err != nil {
			return nil, err
		}
		for _, raw := range vsolparse.VsolOnuRowsFromSummaryBlob([]byte(sum)) {
			row, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			serial := strings.ToUpper(strings.TrimSpace(stringFromAny(row["serial"])))
			if serial != "" {
				set[serial] = true
			}
		}
	}
	return set, rows.Err()
}

type onuClientLinkRow struct {
	Serial     string `json:"serial"`
	ClientName string `json:"client_name"`
}

type onuClientLinkImportRequest struct {
	Rows []onuClientLinkRow `json:"rows"`
}

// --- Correspondência aproximada de serial (o CSV vem de fora — planilha do técnico, etiqueta
// fotografada etc. — e diverge do serial visto pela OLT por erro de digitação/leitura, quase
// sempre nos últimos caracteres). Em vez de só rejeitar tudo que não bate exato, classifica o
// nível de confiança da aproximação para o utilizador decidir. Ordem do mais confiável ao
// menos confiável: exact > last5 (os 5 últimos caracteres batem, resto ignorado) >
// tail1diff (toda a string bate menos o último caractere) > last5_4of5 (dos 5 últimos
// caracteres, 4 batem e só o último diverge).
const (
	serialMatchExact     = "exact"
	serialMatchLast5     = "last5"
	serialMatchTail1Diff = "tail1diff"
	serialMatchLast5Of4  = "last5_4of5"
)

func serialMatchRank(kind string) int {
	switch kind {
	case serialMatchExact:
		return 4
	case serialMatchLast5:
		return 3
	case serialMatchTail1Diff:
		return 2
	case serialMatchLast5Of4:
		return 1
	default:
		return 0
	}
}

func serialTail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// classifySerialMatch compara dois seriais (já normalizados p/ maiúsculas) e devolve o nível
// de confiança da correspondência, ou "" se nenhum critério bate.
func classifySerialMatch(a, b string) string {
	if a == "" || b == "" {
		return ""
	}
	if a == b {
		return serialMatchExact
	}
	if len(a) >= 5 && len(b) >= 5 && serialTail(a, 5) == serialTail(b, 5) {
		return serialMatchLast5
	}
	if len(a) == len(b) && len(a) > 1 && a[:len(a)-1] == b[:len(b)-1] {
		return serialMatchTail1Diff
	}
	if len(a) >= 5 && len(b) >= 5 {
		ta, tb := serialTail(a, 5), serialTail(b, 5)
		if ta[:4] == tb[:4] && ta[4] != tb[4] {
			return serialMatchLast5Of4
		}
	}
	return ""
}

// bestSerialMatch procura, entre os seriais conhecidos, o de maior confiança para `target`
// (ver classifySerialMatch) — devolve serial="" se nada bateu em nenhum critério.
func bestSerialMatch(target string, known map[string]bool) (serial, matchType string) {
	bestRank := 0
	for candidate := range known {
		kind := classifySerialMatch(target, candidate)
		rank := serialMatchRank(kind)
		if rank > bestRank {
			bestRank = rank
			serial, matchType = candidate, kind
		}
	}
	return serial, matchType
}

type onuClientLinkSuggestion struct {
	Serial          string `json:"serial"` // o que veio na linha importada
	ClientName      string `json:"client_name"`
	SuggestedSerial string `json:"suggested_serial"` // serial real mais próximo entre as ONUs conhecidas
	MatchType       string `json:"match_type"`       // last5 | tail1diff | last5_4of5
}

// importOnuClientLinks recebe pares serial/cliente (já extraídos do CSV no frontend),
// confirma que cada serial pertence a uma ONU conhecida (vista em algum snapshot de
// OLT) e grava/actualiza o vínculo. Seriais desconhecidos são devolvidos em
// "not_found" em vez de rejeitar o pedido inteiro.
func (s *Server) importOnuClientLinks(w http.ResponseWriter, r *http.Request) {
	var body onuClientLinkImportRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if len(body.Rows) == 0 {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "nenhuma linha para importar", nil)
		return
	}

	ctx := r.Context()
	known, err := s.allOltOnuSerials(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}

	linked := 0
	var notFound []string
	var suggestions []onuClientLinkSuggestion
	seen := map[string]bool{}
	for _, row := range body.Rows {
		rawSerial := strings.TrimSpace(row.Serial)
		serial := strings.ToUpper(rawSerial)
		name := strings.TrimSpace(row.ClientName)
		if serial == "" || name == "" || seen[serial] {
			continue
		}
		seen[serial] = true
		if !known[serial] {
			// Não bateu exato — antes de rejeitar, procura a ONU conhecida mais parecida
			// (ver classifySerialMatch) para sugerir ao utilizador em vez de simplesmente
			// descartar a linha.
			if suggested, matchType := bestSerialMatch(serial, known); suggested != "" {
				suggestions = append(suggestions, onuClientLinkSuggestion{
					Serial: rawSerial, ClientName: name, SuggestedSerial: suggested, MatchType: matchType,
				})
			} else {
				notFound = append(notFound, rawSerial)
			}
			continue
		}
		_, err := s.DB().Exec(ctx, `
			INSERT INTO onu_client_links (serial, client_name)
			VALUES ($1, $2)
			ON CONFLICT (serial) DO UPDATE SET client_name = EXCLUDED.client_name, updated_at = now()
		`, serial, name)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		linked++
	}
	if notFound == nil {
		notFound = []string{}
	}
	if suggestions == nil {
		suggestions = []onuClientLinkSuggestion{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"linked":      linked,
		"not_found":   notFound,
		"suggestions": suggestions,
		"total":       len(body.Rows),
	})
}
