package integrationconsumer

import (
	"encoding/json"
	"strings"
)

// ParseClientSearch interpreta a resposta conforme o perfil do ERP.
func ParseClientSearch(raw []byte, profile string) SearchResult {
	raw = ResponseBodyForParse(raw)
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case ProviderIXC:
		return ParseIXCClientSearch(raw)
	case ProviderHubsoft:
		return ParseHubsoftClientSearch(raw)
	default:
		return ParseGenericClientSearch(raw)
	}
}

// ParseClientSearchBest tenta vários parsers até obter clientes (consulta alinhada ao teste manual).
func ParseClientSearchBest(raw []byte, preferred string) (SearchResult, string) {
	raw = ResponseBodyForParse(raw)
	preferred = strings.ToLower(strings.TrimSpace(preferred))
	order := []string{preferred, ProviderIXC, ProviderGeneric, ProviderHubsoft}
	seen := map[string]struct{}{}
	var last SearchResult
	used := preferred
	for _, p := range order {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		r := ParseClientSearch(raw, p)
		last = r
		used = p
		if r.OK && len(r.Clients) > 0 {
			return r, p
		}
	}
	if last.Message == "" {
		last = ParseClientSearch(raw, preferred)
	}
	return last, used
}

// ParseGenericClientSearch tenta extrair clientes de JSON comum (registros, data, items).
func ParseGenericClientSearch(raw []byte) SearchResult {
	out := ParseHubsoftClientSearch(raw)
	if out.OK && len(out.Clients) > 0 {
		return out
	}
	// Reavalia sem exigir campo status Hubsoft
	return parseClientSearchLenient(raw)
}

func parseClientSearchLenient(raw []byte) SearchResult {
	out := SearchResult{Clients: []ClientCard{}}
	if len(raw) == 0 {
		out.Message = "Resposta vazia"
		return out
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		out.Message = "Resposta não é JSON válido"
		return out
	}
	if msg := ixcErrorMessage(doc); msg != "" {
		out.OK = false
		out.Message = msg
		return out
	}
	items := extractClientArray(doc)
	for _, it := range items {
		if card, ok := mapClientItem(it); ok {
			out.Clients = append(out.Clients, card)
		}
	}
	out.OK = true
	if len(out.Clients) == 0 {
		out.Message = "Nenhum cliente encontrado para este termo."
	}
	return out
}
