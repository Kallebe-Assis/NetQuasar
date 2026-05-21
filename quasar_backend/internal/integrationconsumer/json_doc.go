package integrationconsumer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const responseTruncatedSuffix = "\n… (resposta truncada)"

// ResponseBodyForParse normaliza o preview HTTP antes do parser (remove sufixo de truncagem, extrai JSON embutido).
func ResponseBodyForParse(raw []byte) []byte {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return raw
	}
	s := string(raw)
	if strings.HasSuffix(s, responseTruncatedSuffix) {
		s = strings.TrimSuffix(s, responseTruncatedSuffix)
		s = strings.TrimSpace(s)
	}
	if json.Valid([]byte(s)) {
		return []byte(s)
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		candidate := s[start : end+1]
		if json.Valid([]byte(candidate)) {
			return []byte(candidate)
		}
	}
	start = strings.Index(s, "[")
	end = strings.LastIndex(s, "]")
	if start >= 0 && end > start {
		candidate := s[start : end+1]
		if json.Valid([]byte(candidate)) {
			return []byte(candidate)
		}
	}
	return []byte(s)
}

// decodeJSONDocument aceita objeto JSON ou array na raiz (comum em APIs legadas).
// Se o JSON foi truncado no limite do preview HTTP, extrai objetos completos de «registros».
func decodeJSONDocument(raw []byte) (map[string]any, error) {
	raw = bytes.TrimSpace(raw)
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	if len(raw) == 0 {
		return nil, fmt.Errorf("vazio")
	}
	if raw[0] == '[' {
		var arr []any
		if err := json.Unmarshal(raw, &arr); err != nil {
			if items := extractCompleteJSONArrayObjects(string(raw)); len(items) > 0 {
				return map[string]any{"registros": items}, nil
			}
			return nil, err
		}
		return map[string]any{"registros": arr}, nil
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		if partial, ok := decodePartialIXCList(string(raw)); ok {
			return partial, nil
		}
		return nil, err
	}
	return doc, nil
}

func decodePartialIXCList(s string) (map[string]any, bool) {
	if !strings.Contains(s, `"registros"`) {
		return nil, false
	}
	items := extractRegistrosFromPartial(s)
	if len(items) == 0 {
		return nil, false
	}
	doc := map[string]any{"registros": items}
	for _, key := range []string{"page", "total", "type", "message", "msg"} {
		if v := extractJSONScalarField(s, key); v != "" {
			doc[key] = v
		}
	}
	return doc, true
}

func extractRegistrosFromPartial(s string) []any {
	idx := strings.Index(s, `"registros"`)
	if idx < 0 {
		return nil
	}
	rest := s[idx:]
	arrStart := strings.Index(rest, "[")
	if arrStart < 0 {
		return nil
	}
	return extractCompleteJSONArrayObjects(rest[arrStart+1:])
}

func extractCompleteJSONArrayObjects(s string) []any {
	var out []any
	i := 0
	for i < len(s) {
		for i < len(s) && (s[i] == ' ' || s[i] == '\n' || s[i] == '\r' || s[i] == '\t' || s[i] == ',') {
			i++
		}
		if i >= len(s) || s[i] == ']' {
			break
		}
		if s[i] != '{' {
			i++
			continue
		}
		end := matchJSONObjectEnd(s, i)
		if end < 0 {
			break
		}
		var item map[string]any
		if err := json.Unmarshal([]byte(s[i:end+1]), &item); err == nil {
			out = append(out, item)
		}
		i = end + 1
	}
	return out
}

func matchJSONObjectEnd(s string, start int) int {
	if start < 0 || start >= len(s) || s[start] != '{' {
		return -1
	}
	depth := 0
	inString := false
	escape := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inString {
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func extractJSONScalarField(s, key string) string {
	needle := `"` + key + `"`
	idx := strings.Index(s, needle)
	if idx < 0 {
		return ""
	}
	colon := strings.Index(s[idx:], ":")
	if colon < 0 {
		return ""
	}
	frag := strings.TrimSpace(s[idx+colon+1:])
	if len(frag) == 0 {
		return ""
	}
	if frag[0] == '"' {
		end := 1
		for end < len(frag) {
			if frag[end] == '\\' {
				end += 2
				continue
			}
			if frag[end] == '"' {
				var v string
				_ = json.Unmarshal([]byte(frag[:end+1]), &v)
				return v
			}
			end++
		}
		return ""
	}
	var n json.Number
	if err := json.Unmarshal([]byte(strings.SplitN(frag, ",", 2)[0]), &n); err == nil {
		return n.String()
	}
	return ""
}

// NonJSONResponseHint mensagem útil quando a API não devolve JSON (HTML, texto, etc.).
func NonJSONResponseHint(raw []byte, statusCode int) string {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		if statusCode >= 400 {
			return fmt.Sprintf("Resposta vazia (HTTP %d). Verifique autenticação e URL base.", statusCode)
		}
		return "Resposta vazia do servidor"
	}
	if strings.HasPrefix(s, "<") {
		return fmt.Sprintf(
			"O servidor devolveu HTML (HTTP %d), não JSON. Confirme POST /cliente, header ixcsoft:listar, corpo JSON e autenticação Basic.",
			statusCode,
		)
	}
	if strings.HasPrefix(s, "{") && strings.Contains(s, `"registros"`) {
		return fmt.Sprintf(
			"Resposta IXC truncada (HTTP %d): muitos campos por cliente. O NetQuasar passa a ler os registros completos do preview; refine a busca (CPF com operador =) se vierem milhares de linhas.",
			statusCode,
		)
	}
	if len(s) > 280 {
		s = s[:280] + "…"
	}
	if statusCode > 0 {
		return fmt.Sprintf("Resposta não é JSON (HTTP %d): %s", statusCode, s)
	}
	return "Resposta não é JSON: " + s
}
