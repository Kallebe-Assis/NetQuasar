package ifacemeta

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LoadCustomDescriptionsByIndex devolve description do utilizador indexada por if_index.
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

// CustomDescriptionForIndex devolve a descrição custom de um if_index (ou "").
func CustomDescriptionForIndex(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, ifIndex int) string {
	if pool == nil || deviceID == uuid.Nil || ifIndex <= 0 {
		return ""
	}
	var desc *string
	_ = pool.QueryRow(ctx, `
		SELECT description FROM device_interface_metadata
		WHERE device_id=$1 AND if_index=$2
	`, deviceID, ifIndex).Scan(&desc)
	if desc == nil {
		return ""
	}
	return strings.TrimSpace(*desc)
}
