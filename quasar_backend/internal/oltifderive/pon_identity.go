package oltifderive

import (
	"strings"
)

// PonIdentityNorm reduz chaves de PON para comparação estável:
// "04", "4", "GPON0/4" e "PON 4" → "4"; "1/1/04" → "1/1/4".
func PonIdentityNorm(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "<nil>" {
		return ""
	}
	if c := PonCompactFromPhy(s, s); c != "" {
		s = strings.ToLower(c)
	} else if sm := reVsolPonName.FindStringSubmatch(s); len(sm) == 2 {
		s = sm[1]
	}
	parts := strings.Split(s, "/")
	for i, p := range parts {
		p = strings.TrimSpace(p)
		trimmed := strings.TrimLeft(p, "0")
		if trimmed == "" {
			if strings.ContainsRune(p, '0') {
				trimmed = "0"
			} else {
				trimmed = p
			}
		}
		parts[i] = trimmed
	}
	return strings.Join(parts, "/")
}

func ponLastSegment(s string) string {
	s = PonIdentityNorm(s)
	if s == "" {
		return ""
	}
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// PonKeysEqual compara identificadores de PON ignorando zeros à esquerda e prefixos GPON/PON.
func PonKeysEqual(a, b string) bool {
	na, nb := PonIdentityNorm(a), PonIdentityNorm(b)
	if na == "" || nb == "" {
		return false
	}
	if na == nb {
		return true
	}
	// "04" ↔ "0/4" (compacto VSOL vs slot/porta).
	if !strings.Contains(na, "/") && strings.Count(nb, "/") == 1 && strings.HasPrefix(nb, "0/") {
		return na == ponLastSegment(nb)
	}
	if !strings.Contains(nb, "/") && strings.Count(na, "/") == 1 && strings.HasPrefix(na, "0/") {
		return nb == ponLastSegment(na)
	}
	return false
}

// FindPonRowByHints localiza a linha da PON no snapshot.
// 1) igualdade normalizada; 2) último segmento só se for único no snapshot.
func FindPonRowByHints(arr []map[string]any, hints []string) (map[string]any, bool) {
	var clean []string
	seen := map[string]struct{}{}
	for _, h := range hints {
		h = strings.TrimSpace(h)
		if h == "" || h == "<nil>" {
			continue
		}
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		clean = append(clean, h)
	}
	if len(clean) == 0 || len(arr) == 0 {
		return nil, false
	}

	for _, p := range arr {
		keys := ponRowIdentityKeys(p)
		for _, k := range keys {
			for _, h := range clean {
				if PonKeysEqual(k, h) {
					return p, true
				}
			}
		}
	}

	var lastWants []string
	lastSeen := map[string]struct{}{}
	for _, h := range clean {
		seg := ponLastSegment(h)
		if seg == "" {
			continue
		}
		if _, ok := lastSeen[seg]; ok {
			continue
		}
		lastSeen[seg] = struct{}{}
		lastWants = append(lastWants, seg)
	}
	if len(lastWants) == 0 {
		return nil, false
	}

	var unique map[string]any
	matches := 0
	for _, p := range arr {
		keys := ponRowIdentityKeys(p)
		hit := false
		for _, k := range keys {
			seg := ponLastSegment(k)
			for _, w := range lastWants {
				if seg == w {
					hit = true
					break
				}
			}
			if hit {
				break
			}
		}
		if hit {
			unique = p
			matches++
			if matches > 1 {
				return nil, false
			}
		}
	}
	if matches == 1 {
		return unique, true
	}
	return nil, false
}

func ponRowIdentityKeys(p map[string]any) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || s == "<nil>" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	add(StablePonRowKey(p))
	add(trimPonField(p["id"]))
	add(trimPonField(p["pon_compact"]))
	add(trimPonField(p["name"]))
	add(trimPonField(p["pon"]))
	return out
}
