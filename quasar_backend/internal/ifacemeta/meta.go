package ifacemeta

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LoadCustomDescriptionsByIndex devolve description do usuário indexada por if_index.
func LoadCustomDescriptionsByIndex(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID) map[int]string {
	out := map[int]string{}
	if pool == nil || deviceID == uuid.Nil {
		return out
	}
	rows, err := pool.Query(ctx, `
		SELECT if_index, description
		FROM device_interface_metadata
		WHERE device_id = $1 AND COALESCE(trim(description),'') <> ''
	`, deviceID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var idx int
		var desc string
		if rows.Scan(&idx, &desc) != nil || idx <= 0 {
			continue
		}
		if s := strings.TrimSpace(desc); s != "" {
			out[idx] = s
		}
	}
	return out
}

// Resolver resolve o «Nome da interface» que o usuário cadastrou (aba Interfaces do equipamento) para um alerta,
// guardando em cache o cadastro de cada equipamento durante o uso (uma consulta por equipamento).
type Resolver struct {
	pool    *pgxpool.Pool
	byIndex map[uuid.UUID]map[int]string
	byName  map[uuid.UUID]map[string]string
}

func NewResolver(pool *pgxpool.Pool) *Resolver {
	return &Resolver{pool: pool, byIndex: map[uuid.UUID]map[int]string{}, byName: map[uuid.UUID]map[string]string{}}
}

func (r *Resolver) load(ctx context.Context, deviceID uuid.UUID) {
	if _, ok := r.byIndex[deviceID]; ok {
		return
	}
	idx, names := map[int]string{}, map[string]string{}
	r.byIndex[deviceID], r.byName[deviceID] = idx, names
	if r.pool == nil {
		return
	}
	rows, err := r.pool.Query(ctx, `
		SELECT if_index, COALESCE(if_name,''), description FROM device_interface_metadata
		WHERE device_id = $1 AND COALESCE(trim(description),'') <> ''`, deviceID)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var i int
		var n, d string
		if rows.Scan(&i, &n, &d) != nil {
			continue
		}
		d = strings.TrimSpace(d)
		if i > 0 && d != "" {
			idx[i] = d
		}
		if k := strings.ToLower(strings.TrimSpace(n)); k != "" && d != "" {
			names[k] = d
			names["~"+alnumKey(k)] = d
		}
	}
}

func metaIfIndex(meta map[string]any) int {
	switch v := meta["if_index"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

// Resolve devolve o nome cadastrado da interface a que o alerta se refere, ou "". Procura primeiro pelo if_index
// do alerta e, se não houver, pelo nome da porta (if_name, display_name, pon…) no cadastro do equipamento.
func (r *Resolver) Resolve(ctx context.Context, deviceID uuid.UUID, metaRaw []byte) string {
	if r == nil || deviceID == uuid.Nil {
		return ""
	}
	r.load(ctx, deviceID)
	idx, names := r.byIndex[deviceID], r.byName[deviceID]
	if len(idx) == 0 {
		return ""
	}
	meta := map[string]any{}
	if json.Unmarshal(metaRaw, &meta) != nil {
		return ""
	}
	if s := strings.TrimSpace(fmt.Sprint(meta["custom_description"])); s != "" && s != "<nil>" {
		return s
	}
	if i := metaIfIndex(meta); i > 0 {
		if d, ok := idx[i]; ok {
			return d
		}
	}
	for _, key := range []string{"if_name", "display_name", "pon", "pon_name", "port", "interface"} {
		raw, _ := meta[key].(string)
		raw = strings.ToLower(strings.TrimSpace(raw))
		if raw == "" {
			continue
		}
		if d, ok := names[raw]; ok {
			return d
		}
		if d, ok := names["~"+alnumKey(raw)]; ok {
			return d
		}
		if p := strings.Index(raw, " ("); p > 0 { // «sfp9 (descrição)»
			if d, ok := names[strings.TrimSpace(raw[:p])]; ok {
				return d
			}
		}
	}
	return ""
}

// alnumKey — só letras e dígitos, para casar «GPON 0/1» com «gpon0/1».
func alnumKey(s string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, strings.ToLower(s))
}
