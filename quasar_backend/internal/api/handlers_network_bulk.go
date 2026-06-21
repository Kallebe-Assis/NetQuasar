package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type networkBulkBody[T any] struct {
	Items           []T    `json:"items"`
	DuplicatePolicy string `json:"duplicate_policy"`
}

type networkImportFail struct {
	Index       int    `json:"index,omitempty"`
	Line        int    `json:"line,omitempty"`
	Description string `json:"description,omitempty"`
	Error       string `json:"error"`
}

func networkImportPolicy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "ignore":
		return "ignore"
	default:
		return "replace"
	}
}

func networkResolveLocalityID(ctx context.Context, s *Server, name *string, id *string) (*uuid.UUID, error) {
	if id != nil && strings.TrimSpace(*id) != "" {
		return optionalUUIDFromString(*id)
	}
	if name == nil || strings.TrimSpace(*name) == "" {
		return nil, nil
	}
	var locID uuid.UUID
	err := s.DB().QueryRow(ctx, `
		SELECT id FROM commercial_localities
		WHERE lower(trim(name)) = lower(trim($1))
		LIMIT 1`, strings.TrimSpace(*name)).Scan(&locID)
	if err == pgx.ErrNoRows {
		return nil, errors.New("localidade não encontrada: " + strings.TrimSpace(*name))
	}
	if err != nil {
		return nil, err
	}
	return &locID, nil
}

func networkResolveProjectID(ctx context.Context, s *Server, number *int, id *string) (*uuid.UUID, error) {
	if id != nil && strings.TrimSpace(*id) != "" {
		return optionalUUIDFromString(*id)
	}
	if number == nil || *number <= 0 {
		return nil, nil
	}
	var projID uuid.UUID
	err := s.DB().QueryRow(ctx, `SELECT id FROM network_projects WHERE display_number = $1 LIMIT 1`, *number).Scan(&projID)
	if err == pgx.ErrNoRows {
		return nil, errors.New("projeto não encontrado: número " + strconv.Itoa(*number))
	}
	if err != nil {
		return nil, err
	}
	return &projID, nil
}

func networkFindByDescription(ctx context.Context, s *Server, table, description string) (*uuid.UUID, error) {
	desc := strings.TrimSpace(description)
	if desc == "" {
		return nil, nil
	}
	var id uuid.UUID
	err := s.DB().QueryRow(ctx, `
		SELECT id FROM `+table+`
		WHERE lower(trim(description)) = lower(trim($1))
		LIMIT 1`, desc).Scan(&id)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (s *Server) bulkNetworkCtos(w http.ResponseWriter, r *http.Request) {
	var body networkBulkBody[networkCtoInput]
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Items) == 0 {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "items obrigatório", nil)
		return
	}
	policy := networkImportPolicy(body.DuplicatePolicy)
	ctx := r.Context()
	var imported, skipped int
	var failed []networkImportFail

	for i, item := range body.Items {
		line := i + 1
		desc := strings.TrimSpace(item.Description)
		if err := item.validate(); err != nil {
			failed = append(failed, networkImportFail{Index: i, Line: line, Description: desc, Error: err.Error()})
			continue
		}
		locID, err := networkResolveLocalityID(ctx, s, item.LocalityName, item.LocalityID)
		if err != nil {
			failed = append(failed, networkImportFail{Index: i, Line: line, Description: desc, Error: err.Error()})
			continue
		}
		projID, err := networkResolveProjectID(ctx, s, item.ProjectNumber, item.ProjectID)
		if err != nil {
			failed = append(failed, networkImportFail{Index: i, Line: line, Description: desc, Error: err.Error()})
			continue
		}
		needsMaint := false
		if item.NeedsMaintenance != nil {
			needsMaint = *item.NeedsMaintenance
		}
		existingID, err := networkFindByDescription(ctx, s, "network_ctos", desc)
		if err != nil {
			failed = append(failed, networkImportFail{Index: i, Line: line, Description: desc, Error: err.Error()})
			continue
		}
		if existingID != nil {
			if policy == "ignore" {
				skipped++
				continue
			}
			_, err = s.DB().Exec(ctx, `
				UPDATE network_ctos SET
					description=$2, latitude=$3, longitude=$4, splitter=$5, transmitter=$6, fiber_color=$7,
					notes=$8, needs_maintenance=$9, project_id=$10, locality_id=$11, updated_at=now()
				WHERE id=$1`,
				*existingID, desc, item.Latitude, item.Longitude, trimPtr(item.Splitter), trimPtr(item.Transmitter), trimPtr(item.FiberColor),
				trimPtr(item.Notes), needsMaint, projID, locID,
			)
			if err != nil {
				failed = append(failed, networkImportFail{Index: i, Line: line, Description: desc, Error: err.Error()})
				continue
			}
			imported++
			continue
		}
		_, err = s.DB().Exec(ctx, `
			INSERT INTO network_ctos (description, latitude, longitude, splitter, transmitter, fiber_color, notes, needs_maintenance, project_id, locality_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			desc, item.Latitude, item.Longitude, trimPtr(item.Splitter), trimPtr(item.Transmitter), trimPtr(item.FiberColor),
			trimPtr(item.Notes), needsMaint, projID, locID,
		)
		if err != nil {
			failed = append(failed, networkImportFail{Index: i, Line: line, Description: desc, Error: err.Error()})
			continue
		}
		imported++
	}

	s.appendAuditLog(ctx, "network_cto", "bulk", "import", s.actorFromRequest(r), nil, map[string]any{
		"imported": imported, "skipped": skipped, "failed": len(failed),
	})
	writeJSON(w, http.StatusOK, map[string]any{"imported": imported, "skipped": skipped, "failed": failed})
}

func (s *Server) bulkNetworkSpliceBoxes(w http.ResponseWriter, r *http.Request) {
	var body networkBulkBody[networkSpliceBoxInput]
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Items) == 0 {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "items obrigatório", nil)
		return
	}
	policy := networkImportPolicy(body.DuplicatePolicy)
	ctx := r.Context()
	var imported, skipped int
	var failed []networkImportFail

	for i, item := range body.Items {
		line := i + 1
		desc := strings.TrimSpace(item.Description)
		if err := item.validate(); err != nil {
			failed = append(failed, networkImportFail{Index: i, Line: line, Description: desc, Error: err.Error()})
			continue
		}
		projID, err := networkResolveProjectID(ctx, s, item.ProjectNumber, item.ProjectID)
		if err != nil {
			failed = append(failed, networkImportFail{Index: i, Line: line, Description: desc, Error: err.Error()})
			continue
		}
		needsMaint := false
		if item.NeedsMaintenance != nil {
			needsMaint = *item.NeedsMaintenance
		}
		existingID, err := networkFindByDescription(ctx, s, "network_splice_boxes", desc)
		if err != nil {
			failed = append(failed, networkImportFail{Index: i, Line: line, Description: desc, Error: err.Error()})
			continue
		}
		if existingID != nil {
			if policy == "ignore" {
				skipped++
				continue
			}
			_, err = s.DB().Exec(ctx, `
				UPDATE network_splice_boxes SET
					description=$2, latitude=$3, longitude=$4, fiber_count=$5,
					needs_maintenance=$6, notes=$7, project_id=$8, updated_at=now()
				WHERE id=$1`,
				*existingID, desc, item.Latitude, item.Longitude, item.FiberCount, needsMaint, trimPtr(item.Notes), projID,
			)
			if err != nil {
				failed = append(failed, networkImportFail{Index: i, Line: line, Description: desc, Error: err.Error()})
				continue
			}
			imported++
			continue
		}
		_, err = s.DB().Exec(ctx, `
			INSERT INTO network_splice_boxes (description, latitude, longitude, fiber_count, needs_maintenance, notes, project_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			desc, item.Latitude, item.Longitude, item.FiberCount, needsMaint, trimPtr(item.Notes), projID,
		)
		if err != nil {
			failed = append(failed, networkImportFail{Index: i, Line: line, Description: desc, Error: err.Error()})
			continue
		}
		imported++
	}

	s.appendAuditLog(ctx, "network_splice_box", "bulk", "import", s.actorFromRequest(r), nil, map[string]any{
		"imported": imported, "skipped": skipped, "failed": len(failed),
	})
	writeJSON(w, http.StatusOK, map[string]any{"imported": imported, "skipped": skipped, "failed": failed})
}

func (s *Server) bulkNetworkCables(w http.ResponseWriter, r *http.Request) {
	var body networkBulkBody[networkCableInput]
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Items) == 0 {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "items obrigatório", nil)
		return
	}
	policy := networkImportPolicy(body.DuplicatePolicy)
	ctx := r.Context()
	var imported, skipped int
	var failed []networkImportFail

	for i, item := range body.Items {
		line := i + 1
		desc := strings.TrimSpace(item.Description)
		if err := item.validate(); err != nil {
			failed = append(failed, networkImportFail{Index: i, Line: line, Description: desc, Error: err.Error()})
			continue
		}
		projID, err := networkResolveProjectID(ctx, s, item.ProjectNumber, item.ProjectID)
		if err != nil {
			failed = append(failed, networkImportFail{Index: i, Line: line, Description: desc, Error: err.Error()})
			continue
		}
		existingID, err := networkFindByDescription(ctx, s, "network_cables", desc)
		if err != nil {
			failed = append(failed, networkImportFail{Index: i, Line: line, Description: desc, Error: err.Error()})
			continue
		}
		if existingID != nil {
			if policy == "ignore" {
				skipped++
				continue
			}
			_, err = s.DB().Exec(ctx, `
				UPDATE network_cables SET
					description=$2, cable_type=$3, fiber_count=$4, status=$5,
					project_id=$6, latitude=$7, longitude=$8, updated_at=now()
				WHERE id=$1`,
				*existingID, desc, trimPtr(item.CableType), item.FiberCount, item.Status, projID, item.Latitude, item.Longitude,
			)
			if err != nil {
				failed = append(failed, networkImportFail{Index: i, Line: line, Description: desc, Error: err.Error()})
				continue
			}
			imported++
			continue
		}
		_, err = s.DB().Exec(ctx, `
			INSERT INTO network_cables (description, cable_type, fiber_count, status, project_id, latitude, longitude)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			desc, trimPtr(item.CableType), item.FiberCount, item.Status, projID, item.Latitude, item.Longitude,
		)
		if err != nil {
			failed = append(failed, networkImportFail{Index: i, Line: line, Description: desc, Error: err.Error()})
			continue
		}
		imported++
	}

	s.appendAuditLog(ctx, "network_cable", "bulk", "import", s.actorFromRequest(r), nil, map[string]any{
		"imported": imported, "skipped": skipped, "failed": len(failed),
	})
	writeJSON(w, http.StatusOK, map[string]any{"imported": imported, "skipped": skipped, "failed": failed})
}

func (s *Server) bulkNetworkPoles(w http.ResponseWriter, r *http.Request) {
	var body networkBulkBody[networkPoleInput]
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Items) == 0 {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "items obrigatório", nil)
		return
	}
	policy := networkImportPolicy(body.DuplicatePolicy)
	ctx := r.Context()
	var imported, skipped int
	var failed []networkImportFail

	for i, item := range body.Items {
		line := i + 1
		desc := strings.TrimSpace(item.Description)
		if err := item.validate(); err != nil {
			failed = append(failed, networkImportFail{Index: i, Line: line, Description: desc, Error: err.Error()})
			continue
		}
		locID, err := networkResolveLocalityID(ctx, s, item.LocalityName, item.LocalityID)
		if err != nil {
			failed = append(failed, networkImportFail{Index: i, Line: line, Description: desc, Error: err.Error()})
			continue
		}
		projID, err := networkResolveProjectID(ctx, s, item.ProjectNumber, item.ProjectID)
		if err != nil {
			failed = append(failed, networkImportFail{Index: i, Line: line, Description: desc, Error: err.Error()})
			continue
		}
		existingID, err := networkFindByDescription(ctx, s, "network_poles", desc)
		if err != nil {
			failed = append(failed, networkImportFail{Index: i, Line: line, Description: desc, Error: err.Error()})
			continue
		}
		if existingID != nil {
			if policy == "ignore" {
				skipped++
				continue
			}
			_, err = s.DB().Exec(ctx, `
				UPDATE network_poles SET
					description=$2, pole_type=$3, project_id=$4, locality_id=$5,
					latitude=$6, longitude=$7, updated_at=now()
				WHERE id=$1`,
				*existingID, desc, trimPtr(item.PoleType), projID, locID, item.Latitude, item.Longitude,
			)
			if err != nil {
				failed = append(failed, networkImportFail{Index: i, Line: line, Description: desc, Error: err.Error()})
				continue
			}
			imported++
			continue
		}
		_, err = s.DB().Exec(ctx, `
			INSERT INTO network_poles (description, pole_type, project_id, locality_id, latitude, longitude)
			VALUES ($1,$2,$3,$4,$5,$6)`,
			desc, trimPtr(item.PoleType), projID, locID, item.Latitude, item.Longitude,
		)
		if err != nil {
			failed = append(failed, networkImportFail{Index: i, Line: line, Description: desc, Error: err.Error()})
			continue
		}
		imported++
	}

	s.appendAuditLog(ctx, "network_pole", "bulk", "import", s.actorFromRequest(r), nil, map[string]any{
		"imported": imported, "skipped": skipped, "failed": len(failed),
	})
	writeJSON(w, http.StatusOK, map[string]any{"imported": imported, "skipped": skipped, "failed": failed})
}
