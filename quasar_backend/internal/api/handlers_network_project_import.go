package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type projectImportPoint struct {
	Description string   `json:"description"`
	Latitude    *float64 `json:"latitude"`
	Longitude   *float64 `json:"longitude"`
	Splitter    *string  `json:"splitter,omitempty"`
	Transmitter *string  `json:"transmitter,omitempty"`
	FiberColor  *string  `json:"fiber_color,omitempty"`
	FiberCount  *int     `json:"fiber_count,omitempty"`
	BoxModel    *string  `json:"box_model,omitempty"`
	PoleType    *string  `json:"pole_type,omitempty"`
	Notes       *string  `json:"notes,omitempty"`
}

type projectImportCable struct {
	Description string           `json:"description"`
	Latitude    *float64         `json:"latitude"`
	Longitude   *float64         `json:"longitude"`
	Path        []map[string]any `json:"path"`
	CableType   *string          `json:"cable_type,omitempty"`
	FiberCount  *int             `json:"fiber_count,omitempty"`
	Status      string           `json:"status,omitempty"`
}

type projectImportElements struct {
	Ctos        []projectImportPoint `json:"ctos"`
	SpliceBoxes []projectImportPoint `json:"splice_boxes"`
	Poles       []projectImportPoint `json:"poles"`
	Cables      []projectImportCable `json:"cables"`
	Pops        []projectImportPoint `json:"pops"`
}

type projectImportBody struct {
	Description      string                `json:"description"`
	LocalityID       *string               `json:"locality_id"`
	Color            *string               `json:"color"`
	Status           string                `json:"status"`
	Latitude         *float64              `json:"latitude"`
	Longitude        *float64              `json:"longitude"`
	ReplaceProjectID *string               `json:"replace_project_id,omitempty"`
	Elements         projectImportElements `json:"elements"`
}

// importNetworkProject cria um projeto e importa CTOs, emendas, postes, cabos e POPs numa única transacção.
func (s *Server) importNetworkProject(w http.ResponseWriter, r *http.Request) {
	var body projectImportBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", "", nil)
		return
	}
	desc := strings.TrimSpace(body.Description)
	if desc == "" {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "description obrigatória", nil)
		return
	}
	st, ok := normalizeProjectStatus(body.Status)
	if !ok {
		st = "planejamento"
	}
	if err := validateCoords(body.Latitude, body.Longitude); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", err.Error(), nil)
		return
	}
	locID, err := optionalUUIDFromString(networkStrPtr(body.LocalityID))
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", err.Error(), nil)
		return
	}
	locLat, locLon := s.localityLatLng(r.Context(), locID)
	body.Latitude, body.Longitude = fillCoordsFromLocality(body.Latitude, body.Longitude, locLat, locLon)

	total := len(body.Elements.Ctos) + len(body.Elements.SpliceBoxes) + len(body.Elements.Poles) + len(body.Elements.Cables) + len(body.Elements.Pops)
	if total == 0 {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "nenhum elemento para importar", nil)
		return
	}
	if total > 5000 {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "máximo 5000 elementos por importação", nil)
		return
	}

	replaceID := ""
	if body.ReplaceProjectID != nil {
		replaceID = strings.TrimSpace(*body.ReplaceProjectID)
	}

	ctx := r.Context()
	tx, err := s.DB().Begin(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer tx.Rollback(ctx)

	var projectID uuid.UUID
	var displayNumber int
	if replaceID != "" {
		pid, perr := uuid.Parse(replaceID)
		if perr != nil {
			writeErr(w, http.StatusBadRequest, "BAD_ID", "replace_project_id inválido", nil)
			return
		}
		err = tx.QueryRow(ctx, `SELECT id, display_number FROM network_projects WHERE id=$1`, pid).Scan(&projectID, &displayNumber)
		if err == pgx.ErrNoRows {
			writeErr(w, http.StatusNotFound, "NOT_FOUND", "projeto a substituir não encontrado", nil)
			return
		}
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		_, err = tx.Exec(ctx, `
			UPDATE network_projects
			SET description=$2, locality_id=$3, color=$4, status=$5, latitude=$6, longitude=$7, updated_at=now()
			WHERE id=$1`,
			projectID, desc, locID, trimPtr(body.Color), st, body.Latitude, body.Longitude,
		)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		for _, table := range []string{"network_ctos", "network_splice_boxes", "network_cables", "network_poles"} {
			if _, err = tx.Exec(ctx, `DELETE FROM `+table+` WHERE project_id=$1`, projectID); err != nil {
				writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
				return
			}
		}
	} else {
		err = tx.QueryRow(ctx, `
			INSERT INTO network_projects (description, locality_id, color, status, latitude, longitude)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, display_number`,
			desc, locID, trimPtr(body.Color), st, body.Latitude, body.Longitude,
		).Scan(&projectID, &displayNumber)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
	}

	counts := map[string]int{"ctos": 0, "splice_boxes": 0, "poles": 0, "cables": 0, "pops": 0}
	var failed []networkImportFail

	for i, item := range body.Elements.Ctos {
		d := strings.TrimSpace(item.Description)
		if d == "" {
			d = "CTO importada"
		}
		if err := validateCoords(item.Latitude, item.Longitude); err != nil {
			failed = append(failed, networkImportFail{Index: i, Description: d, Error: err.Error()})
			continue
		}
		fiberColor := trimPtr(item.FiberColor)
		if fiberColor != nil {
			if c, ok := normalizeFiberColor(*fiberColor); ok {
				fiberColor = &c
			}
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO network_ctos (description, latitude, longitude, splitter, transmitter, fiber_color, notes, needs_maintenance, project_id, locality_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,false,$8,$9)`,
			d, item.Latitude, item.Longitude, trimPtr(item.Splitter), trimPtr(item.Transmitter), fiberColor, trimPtr(item.Notes), projectID, locID,
		)
		if err != nil {
			failed = append(failed, networkImportFail{Index: i, Description: d, Error: err.Error()})
			continue
		}
		counts["ctos"]++
	}

	for i, item := range body.Elements.SpliceBoxes {
		d := strings.TrimSpace(item.Description)
		if d == "" {
			d = "Emenda importada"
		}
		if err := validateCoords(item.Latitude, item.Longitude); err != nil {
			failed = append(failed, networkImportFail{Index: i, Description: d, Error: err.Error()})
			continue
		}
		boxModel := "emenda"
		if item.BoxModel != nil {
			bm := strings.ToLower(strings.TrimSpace(*item.BoxModel))
			if bm == "distribuicao" || bm == "emenda" {
				boxModel = bm
			}
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO network_splice_boxes (description, latitude, longitude, fiber_count, needs_maintenance, notes, project_id, box_model, splitter)
			VALUES ($1,$2,$3,$4,false,$5,$6,$7,$8)`,
			d, item.Latitude, item.Longitude, item.FiberCount, trimPtr(item.Notes), projectID, boxModel, trimPtr(item.Splitter),
		)
		if err != nil {
			failed = append(failed, networkImportFail{Index: i, Description: d, Error: err.Error()})
			continue
		}
		counts["splice_boxes"]++
	}

	for i, item := range body.Elements.Poles {
		d := strings.TrimSpace(item.Description)
		if d == "" {
			d = "Poste importado"
		}
		if err := validateCoords(item.Latitude, item.Longitude); err != nil {
			failed = append(failed, networkImportFail{Index: i, Description: d, Error: err.Error()})
			continue
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO network_poles (description, pole_type, project_id, locality_id, latitude, longitude)
			VALUES ($1,$2,$3,$4,$5,$6)`,
			d, trimPtr(item.PoleType), projectID, locID, item.Latitude, item.Longitude,
		)
		if err != nil {
			failed = append(failed, networkImportFail{Index: i, Description: d, Error: err.Error()})
			continue
		}
		counts["poles"]++
	}

	for i, item := range body.Elements.Cables {
		d := strings.TrimSpace(item.Description)
		if d == "" {
			d = "Cabo importado"
		}
		status := strings.TrimSpace(item.Status)
		if status == "" {
			status = "ativo"
		}
		stCable, ok := normalizeCableStatus(status)
		if !ok {
			failed = append(failed, networkImportFail{Index: i, Description: d, Error: "status inválido"})
			continue
		}
		lat, lon := item.Latitude, item.Longitude
		if len(item.Path) > 0 {
			if lat == nil || lon == nil {
				if la, ok := asFloat(item.Path[0]["lat"]); ok {
					if lo, ok2 := asFloat(item.Path[0]["lng"]); ok2 {
						lat, lon = &la, &lo
					}
				}
			}
		}
		if err := validateCoords(lat, lon); err != nil {
			failed = append(failed, networkImportFail{Index: i, Description: d, Error: err.Error()})
			continue
		}
		if len(item.Path) > 2000 {
			failed = append(failed, networkImportFail{Index: i, Description: d, Error: "path: máximo 2000 pontos"})
			continue
		}
		var pathJSON []byte
		if len(item.Path) >= 2 {
			pathJSON, err = json.Marshal(item.Path)
			if err != nil {
				failed = append(failed, networkImportFail{Index: i, Description: d, Error: "path inválido"})
				continue
			}
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO network_cables (description, cable_type, fiber_count, status, project_id, latitude, longitude, path)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			d, trimPtr(item.CableType), item.FiberCount, stCable, projectID, lat, lon, pathJSON,
		)
		if err != nil {
			failed = append(failed, networkImportFail{Index: i, Description: d, Error: err.Error()})
			continue
		}
		counts["cables"]++
	}

	for i, item := range body.Elements.Pops {
		d := strings.TrimSpace(item.Description)
		if d == "" {
			d = "POP importado"
		}
		if err := validateCoords(item.Latitude, item.Longitude); err != nil {
			failed = append(failed, networkImportFail{Index: i, Description: d, Error: err.Error()})
			continue
		}
		var existing uuid.UUID
		err = tx.QueryRow(ctx, `
			SELECT id FROM pops
			WHERE lower(trim(description)) = lower(trim($1))
			  AND locality_id IS NOT DISTINCT FROM $2
			LIMIT 1`, d, locID).Scan(&existing)
		if err == nil && existing != uuid.Nil {
			_, err = tx.Exec(ctx, `
				UPDATE pops SET latitude=$2, longitude=$3, updated_at=now()
				WHERE id=$1`, existing, item.Latitude, item.Longitude)
			if err != nil {
				failed = append(failed, networkImportFail{Index: i, Description: d, Error: err.Error()})
				continue
			}
			counts["pops"]++
			continue
		}
		if err != nil && err != pgx.ErrNoRows {
			failed = append(failed, networkImportFail{Index: i, Description: d, Error: err.Error()})
			continue
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO pops (description, latitude, longitude, locality_id)
			VALUES ($1,$2,$3,$4)`,
			d, item.Latitude, item.Longitude, locID,
		)
		if err != nil {
			failed = append(failed, networkImportFail{Index: i, Description: d, Error: err.Error()})
			continue
		}
		counts["pops"]++
	}

	imported := counts["ctos"] + counts["splice_boxes"] + counts["poles"] + counts["cables"] + counts["pops"]
	if imported == 0 {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "nenhum elemento válido para importar", nil)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}

	action := "import_kml"
	status := http.StatusCreated
	if replaceID != "" {
		action = "replace_kml"
		status = http.StatusOK
	}
	s.appendAuditLog(ctx, "network_project", projectID.String(), action, s.actorFromRequest(r), nil, map[string]any{
		"counts": counts, "failed": len(failed), "replaced": replaceID != "",
	})
	writeJSON(w, status, map[string]any{
		"id":             projectID,
		"display_number": displayNumber,
		"imported":       counts,
		"failed":         failed,
		"replaced":       replaceID != "",
	})
}
