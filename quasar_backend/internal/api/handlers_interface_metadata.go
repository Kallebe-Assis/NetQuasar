package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type interfaceMetadataInput struct {
	IfIndex     int    `json:"if_index"`
	IfName      string `json:"if_name"`
	Description string `json:"description"`
	Type        string `json:"type"`
}

type interfaceMetadataBody struct {
	Interfaces []interfaceMetadataInput `json:"interfaces"`
}

// enrichInterfaceTableMetadata aplica descrição e tipo configurados pelo utilizador.
func enrichInterfaceTableMetadata(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, payload map[string]any) error {
	if pool == nil || deviceID == uuid.Nil || payload == nil {
		return nil
	}
	tab, ok := payload["interface_table"].([]map[string]any)
	if !ok || len(tab) == 0 {
		return nil
	}

	rows, err := pool.Query(ctx, `
		SELECT if_index, if_name, description, interface_type
		FROM device_interface_metadata
		WHERE device_id = $1
	`, deviceID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type metadata struct {
		ifName      string
		description string
		typ         string
	}
	byIndex := make(map[int]metadata)
	for rows.Next() {
		var idx int
		var m metadata
		if err := rows.Scan(&idx, &m.ifName, &m.description, &m.typ); err != nil {
			return err
		}
		byIndex[idx] = m
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, row := range tab {
		idx, ok := interfaceRowIndex(row["if_index"])
		if !ok {
			continue
		}
		m, ok := byIndex[idx]
		if !ok {
			continue
		}
		// ifIndex é a chave; o nome fica exposto para auditoria e futuras reconciliações.
		if strings.TrimSpace(m.ifName) != "" {
			row["metadata_if_name"] = strings.TrimSpace(m.ifName)
		}
		if strings.TrimSpace(m.description) != "" {
			row["custom_description"] = strings.TrimSpace(m.description)
		}
		if strings.TrimSpace(m.typ) != "" {
			row["custom_type"] = strings.TrimSpace(m.typ)
		}
	}
	return nil
}

func interfaceRowIndex(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, x > 0
	case int32:
		return int(x), x > 0
	case int64:
		return int(x), x > 0
	case float64:
		n := int(x)
		return n, n > 0
	default:
		return 0, false
	}
}

func normalizeInterfaceMetadata(in interfaceMetadataInput) (interfaceMetadataInput, error) {
	in.IfName = strings.TrimSpace(in.IfName)
	in.Description = strings.TrimSpace(in.Description)
	in.Type = strings.ToLower(strings.TrimSpace(in.Type))
	if in.IfIndex <= 0 {
		return in, errValidation("if_index deve ser maior que zero")
	}
	if utf8.RuneCountInString(in.IfName) > 255 {
		return in, errValidation("nome da interface não pode ter mais de 255 caracteres")
	}
	if utf8.RuneCountInString(in.Description) > 120 {
		return in, errValidation("descrição da interface não pode ter mais de 120 caracteres")
	}
	if in.Type != "" && in.Type != "ether" && in.Type != "sfp" {
		return in, errValidation("tipo da interface deve ser ether ou sfp")
	}
	return in, nil
}

// putDeviceInterfaceMetadata substitui, em lote, os metadados editáveis das interfaces.
func (s *Server) putDeviceInterfaceMetadata(w http.ResponseWriter, r *http.Request) {
	deviceID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	var body interfaceMetadataBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if len(body.Interfaces) > 4096 {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "máximo de 4096 interfaces", nil)
		return
	}

	normalized := make([]interfaceMetadataInput, 0, len(body.Interfaces))
	seen := make(map[int]struct{}, len(body.Interfaces))
	for _, item := range body.Interfaces {
		item, err = normalizeInterfaceMetadata(item)
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", err.Error(), nil)
			return
		}
		if _, exists := seen[item.IfIndex]; exists {
			writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "if_index duplicado", nil)
			return
		}
		seen[item.IfIndex] = struct{}{}
		if item.Description == "" && item.Type == "" {
			continue
		}
		normalized = append(normalized, item)
	}

	tx, err := s.DB().BeginTx(r.Context(), pgx.TxOptions{})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer tx.Rollback(r.Context())

	var exists bool
	if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM devices WHERE id=$1)`, deviceID).Scan(&exists); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if !exists {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "equipamento não encontrado", nil)
		return
	}
	if _, err := tx.Exec(r.Context(), `DELETE FROM device_interface_metadata WHERE device_id=$1`, deviceID); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	for _, item := range normalized {
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO device_interface_metadata
				(device_id, if_index, if_name, description, interface_type, updated_at)
			VALUES ($1, $2, $3, $4, $5, now())
		`, deviceID, item.IfIndex, item.IfName, item.Description, item.Type); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}

	s.appendAuditLog(r.Context(), "device_interfaces", deviceID.String(), "replace_metadata", s.actorFromRequest(r), nil, normalized)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "count": len(normalized)})
}
