package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type fleetVehicle struct {
	ID                 uuid.UUID  `json:"id"`
	Description        string     `json:"description"`
	Plate              string     `json:"plate"`
	Year               *int       `json:"year"`
	Model              *string    `json:"model"`
	Color              *string    `json:"color"`
	City               *string    `json:"city"`
	UF                 *string    `json:"uf"`
	VehicleType        *string    `json:"vehicle_type"`
	Category           *string    `json:"category"`
	PrimaryFuelID      *uuid.UUID `json:"primary_fuel_id"`
	PrimaryFuelName    *string    `json:"primary_fuel_name,omitempty"`
	TankCapacityLiters *float64   `json:"tank_capacity_liters"`
	ExpectedKmPerLiter *float64   `json:"expected_km_per_liter"`
	MinKmPerLiter      *float64   `json:"min_km_per_liter"`
	MaxKmPerLiter      *float64   `json:"max_km_per_liter"`
	OdometerCurrent    float64    `json:"odometer_current"`
	HourmeterCurrent   *float64   `json:"hourmeter_current"`
	CostCenterID       *uuid.UUID `json:"cost_center_id"`
	CostCenterName     *string    `json:"cost_center_name,omitempty"`
	Status             string     `json:"status"`
	Notes              *string    `json:"notes"`
}

func (s *Server) listFleetVehicles(w http.ResponseWriter, r *http.Request) {
	q := fleetQ(r)
	st := fleetStatusFilter(r)
	rows, err := s.DB().Query(r.Context(), `
		SELECT v.id, v.description, v.plate, v.year, v.model, v.color, v.city, v.uf, v.vehicle_type, v.category,
			v.primary_fuel_id, f.description, v.tank_capacity_liters, v.expected_km_per_liter, v.min_km_per_liter, v.max_km_per_liter,
			v.odometer_current, v.hourmeter_current, v.cost_center_id, cc.description, v.status, v.notes
		FROM fleet_vehicles v
		LEFT JOIN fleet_fuels f ON f.id = v.primary_fuel_id
		LEFT JOIN fleet_cost_centers cc ON cc.id = v.cost_center_id
		WHERE ($1 = '' OR v.description ILIKE '%'||$1||'%' OR v.plate ILIKE '%'||$1||'%' OR COALESCE(v.model,'') ILIKE '%'||$1||'%')
		  AND ($2 = '' OR v.status = $2)
		ORDER BY v.plate
	`, q, st)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	list := []fleetVehicle{}
	for rows.Next() {
		var it fleetVehicle
		if err := rows.Scan(&it.ID, &it.Description, &it.Plate, &it.Year, &it.Model, &it.Color, &it.City, &it.UF,
			&it.VehicleType, &it.Category, &it.PrimaryFuelID, &it.PrimaryFuelName, &it.TankCapacityLiters,
			&it.ExpectedKmPerLiter, &it.MinKmPerLiter, &it.MaxKmPerLiter, &it.OdometerCurrent, &it.HourmeterCurrent,
			&it.CostCenterID, &it.CostCenterName, &it.Status, &it.Notes); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		it.Plate = fleetDisplayPlate(it.Plate)
		list = append(list, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (s *Server) getFleetVehicle(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	var it fleetVehicle
	err = s.DB().QueryRow(r.Context(), `
		SELECT v.id, v.description, v.plate, v.year, v.model, v.color, v.city, v.uf, v.vehicle_type, v.category,
			v.primary_fuel_id, f.description, v.tank_capacity_liters, v.expected_km_per_liter, v.min_km_per_liter, v.max_km_per_liter,
			v.odometer_current, v.hourmeter_current, v.cost_center_id, cc.description, v.status, v.notes
		FROM fleet_vehicles v
		LEFT JOIN fleet_fuels f ON f.id = v.primary_fuel_id
		LEFT JOIN fleet_cost_centers cc ON cc.id = v.cost_center_id
		WHERE v.id=$1
	`, id).Scan(&it.ID, &it.Description, &it.Plate, &it.Year, &it.Model, &it.Color, &it.City, &it.UF,
		&it.VehicleType, &it.Category, &it.PrimaryFuelID, &it.PrimaryFuelName, &it.TankCapacityLiters,
		&it.ExpectedKmPerLiter, &it.MinKmPerLiter, &it.MaxKmPerLiter, &it.OdometerCurrent, &it.HourmeterCurrent,
		&it.CostCenterID, &it.CostCenterName, &it.Status, &it.Notes)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "veículo não encontrado", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	it.Plate = fleetDisplayPlate(it.Plate)
	writeJSON(w, http.StatusOK, it)
}

func (s *Server) createFleetVehicle(w http.ResponseWriter, r *http.Request) {
	var body fleetVehicle
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Description) == "" || strings.TrimSpace(body.Plate) == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "description e plate obrigatórios", nil)
		return
	}
	plate, err := fleetNormalizePlate(body.Plate)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	}
	if body.Status == "" {
		body.Status = "active"
	}
	var id uuid.UUID
	err = s.DB().QueryRow(r.Context(), `
		INSERT INTO fleet_vehicles (
			description, plate, year, model, color, city, uf, vehicle_type, category, primary_fuel_id,
			tank_capacity_liters, expected_km_per_liter, min_km_per_liter, max_km_per_liter,
			odometer_current, hourmeter_current, cost_center_id, status, notes, created_by, updated_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$20) RETURNING id
	`, strings.TrimSpace(body.Description), plate, body.Year, ptrTrim(body.Model),
		ptrTrim(body.Color), ptrTrim(body.City), ptrTrim(body.UF), ptrTrim(body.VehicleType), ptrTrim(body.Category),
		body.PrimaryFuelID, body.TankCapacityLiters, body.ExpectedKmPerLiter, body.MinKmPerLiter, body.MaxKmPerLiter,
		body.OdometerCurrent, body.HourmeterCurrent, body.CostCenterID, body.Status, ptrTrim(body.Notes), s.userIDFromRequest(r)).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			writeErr(w, http.StatusConflict, "CONFLICT", "placa já cadastrada", nil)
			return
		}
		writeErr(w, http.StatusBadRequest, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) patchFleetVehicle(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	var body fleetVehicle
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	plate := strings.TrimSpace(body.Plate)
	if plate != "" {
		norm, nerr := fleetNormalizePlate(plate)
		if nerr != nil {
			writeErr(w, http.StatusBadRequest, "VALIDATION", nerr.Error(), nil)
			return
		}
		plate = norm
	}
	tag, err := s.DB().Exec(r.Context(), `
		UPDATE fleet_vehicles SET
			description=COALESCE(NULLIF(trim($2),''), description),
			plate=COALESCE(NULLIF($3,''), plate),
			year=$4, model=$5, color=$6, city=$7, uf=$8, vehicle_type=$9, category=$10, primary_fuel_id=$11,
			tank_capacity_liters=$12, expected_km_per_liter=$13, min_km_per_liter=$14, max_km_per_liter=$15,
			odometer_current=COALESCE($16, odometer_current), hourmeter_current=COALESCE($17, hourmeter_current), cost_center_id=$18,
			status=COALESCE(NULLIF($19,''), status), notes=$20, updated_at=now(), updated_by=$21
		WHERE id=$1
	`, id, body.Description, plate, body.Year, ptrTrim(body.Model), ptrTrim(body.Color), ptrTrim(body.City), ptrTrim(body.UF),
		ptrTrim(body.VehicleType), ptrTrim(body.Category), body.PrimaryFuelID, body.TankCapacityLiters, body.ExpectedKmPerLiter,
		body.MinKmPerLiter, body.MaxKmPerLiter, body.OdometerCurrent, body.HourmeterCurrent, body.CostCenterID,
		body.Status, ptrTrim(body.Notes), s.userIDFromRequest(r))
	if err != nil {
		if isUniqueViolation(err) {
			writeErr(w, http.StatusConflict, "CONFLICT", "placa já cadastrada", nil)
			return
		}
		writeErr(w, http.StatusBadRequest, "DB", err.Error(), nil)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "veículo não encontrado", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) fleetVehicleAutofill(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	var it fleetVehicle
	err = s.DB().QueryRow(r.Context(), `
		SELECT v.id, v.description, v.plate, v.year, v.model, v.color, v.city, v.uf, v.vehicle_type, v.category,
			v.primary_fuel_id, f.description, v.tank_capacity_liters, v.expected_km_per_liter, v.min_km_per_liter, v.max_km_per_liter,
			v.odometer_current, v.hourmeter_current, v.cost_center_id, cc.description, v.status, v.notes
		FROM fleet_vehicles v
		LEFT JOIN fleet_fuels f ON f.id = v.primary_fuel_id
		LEFT JOIN fleet_cost_centers cc ON cc.id = v.cost_center_id
		WHERE v.id=$1
	`, id).Scan(&it.ID, &it.Description, &it.Plate, &it.Year, &it.Model, &it.Color, &it.City, &it.UF,
		&it.VehicleType, &it.Category, &it.PrimaryFuelID, &it.PrimaryFuelName, &it.TankCapacityLiters,
		&it.ExpectedKmPerLiter, &it.MinKmPerLiter, &it.MaxKmPerLiter, &it.OdometerCurrent, &it.HourmeterCurrent,
		&it.CostCenterID, &it.CostCenterName, &it.Status, &it.Notes)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "veículo não encontrado", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	var primaryDriverID *uuid.UUID
	var primaryDriverName *string
	_ = s.DB().QueryRow(r.Context(), `
		SELECT d.id, d.name FROM fleet_driver_vehicles dv
		JOIN fleet_drivers d ON d.id = dv.driver_id
		WHERE dv.vehicle_id=$1 AND (dv.ends_on IS NULL OR dv.ends_on >= CURRENT_DATE)
		ORDER BY dv.is_primary DESC, dv.starts_on NULLS LAST
		LIMIT 1
	`, id).Scan(&primaryDriverID, &primaryDriverName)

	writeJSON(w, http.StatusOK, map[string]any{
		"vehicle":             it,
		"odometer_previous":   it.OdometerCurrent,
		"hourmeter_previous":  it.HourmeterCurrent,
		"primary_driver_id":   primaryDriverID,
		"primary_driver_name": primaryDriverName,
		"primary_fuel_id":     it.PrimaryFuelID,
		"cost_center_id":      it.CostCenterID,
	})
}

// --- Drivers ---

type fleetDriver struct {
	ID              uuid.UUID  `json:"id"`
	Name            string     `json:"name"`
	CPF             *string    `json:"cpf"`
	RG              *string    `json:"rg"`
	Phone           *string    `json:"phone"`
	Email           *string    `json:"email"`
	LicenseNumber   *string    `json:"license_number"`
	LicenseCategory *string    `json:"license_category"`
	LicenseExpires  *string    `json:"license_expires_on"`
	City            *string    `json:"city"`
	UF              *string    `json:"uf"`
	UserID          *uuid.UUID `json:"user_id"`
	UserLogin       *string    `json:"user_login,omitempty"`
	Status          string     `json:"status"`
	Notes           *string    `json:"notes"`
}

func (s *Server) listFleetDrivers(w http.ResponseWriter, r *http.Request) {
	q := fleetQ(r)
	st := fleetStatusFilter(r)
	rows, err := s.DB().Query(r.Context(), `
		SELECT d.id, d.name, d.cpf, d.rg, d.phone, d.email, d.license_number, d.license_category,
			to_char(d.license_expires_on, 'YYYY-MM-DD'), d.city, d.uf, d.user_id, u.login, d.status, d.notes
		FROM fleet_drivers d
		LEFT JOIN users u ON u.id = d.user_id
		WHERE ($1 = '' OR d.name ILIKE '%'||$1||'%' OR COALESCE(d.cpf,'') ILIKE '%'||$1||'%')
		  AND ($2 = '' OR d.status = $2)
		ORDER BY d.name
	`, q, st)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	list := []fleetDriver{}
	for rows.Next() {
		var it fleetDriver
		if err := rows.Scan(&it.ID, &it.Name, &it.CPF, &it.RG, &it.Phone, &it.Email, &it.LicenseNumber, &it.LicenseCategory,
			&it.LicenseExpires, &it.City, &it.UF, &it.UserID, &it.UserLogin, &it.Status, &it.Notes); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (s *Server) createFleetDriver(w http.ResponseWriter, r *http.Request) {
	var body fleetDriver
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "name obrigatório", nil)
		return
	}
	if body.Status == "" {
		body.Status = "active"
	}
	var expires *time.Time
	if body.LicenseExpires != nil && strings.TrimSpace(*body.LicenseExpires) != "" {
		t, err := time.Parse("2006-01-02", strings.TrimSpace(*body.LicenseExpires))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "VALIDATION", "license_expires_on inválida", nil)
			return
		}
		expires = &t
	}
	var id uuid.UUID
	err := s.DB().QueryRow(r.Context(), `
		INSERT INTO fleet_drivers (
			name, cpf, rg, phone, email, license_number, license_category, license_expires_on,
			city, uf, user_id, status, notes, created_by, updated_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$14) RETURNING id
	`, strings.TrimSpace(body.Name), ptrTrim(body.CPF), ptrTrim(body.RG), ptrTrim(body.Phone), ptrTrim(body.Email),
		ptrTrim(body.LicenseNumber), ptrTrim(body.LicenseCategory), expires, ptrTrim(body.City), ptrTrim(body.UF),
		body.UserID, body.Status, ptrTrim(body.Notes), s.userIDFromRequest(r)).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) patchFleetDriver(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	var body fleetDriver
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	var expires *time.Time
	if body.LicenseExpires != nil && strings.TrimSpace(*body.LicenseExpires) != "" {
		t, err := time.Parse("2006-01-02", strings.TrimSpace(*body.LicenseExpires))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "VALIDATION", "license_expires_on inválida", nil)
			return
		}
		expires = &t
	}
	tag, err := s.DB().Exec(r.Context(), `
		UPDATE fleet_drivers SET
			name=COALESCE(NULLIF(trim($2),''), name),
			cpf=$3, rg=$4, phone=$5, email=$6, license_number=$7, license_category=$8, license_expires_on=$9,
			city=$10, uf=$11, user_id=$12, status=COALESCE(NULLIF($13,''), status), notes=$14,
			updated_at=now(), updated_by=$15
		WHERE id=$1
	`, id, body.Name, ptrTrim(body.CPF), ptrTrim(body.RG), ptrTrim(body.Phone), ptrTrim(body.Email),
		ptrTrim(body.LicenseNumber), ptrTrim(body.LicenseCategory), expires, ptrTrim(body.City), ptrTrim(body.UF),
		body.UserID, body.Status, ptrTrim(body.Notes), s.userIDFromRequest(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "DB", err.Error(), nil)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "motorista não encontrado", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) fleetMeDriver(w http.ResponseWriter, r *http.Request) {
	uid := s.userIDFromRequest(r)
	if uid == nil {
		writeJSON(w, http.StatusOK, map[string]any{"driver": nil})
		return
	}
	var it fleetDriver
	err := s.DB().QueryRow(r.Context(), `
		SELECT d.id, d.name, d.cpf, d.rg, d.phone, d.email, d.license_number, d.license_category,
			to_char(d.license_expires_on, 'YYYY-MM-DD'), d.city, d.uf, d.user_id, u.login, d.status, d.notes
		FROM fleet_drivers d
		LEFT JOIN users u ON u.id = d.user_id
		WHERE d.user_id=$1
	`, *uid).Scan(&it.ID, &it.Name, &it.CPF, &it.RG, &it.Phone, &it.Email, &it.LicenseNumber, &it.LicenseCategory,
		&it.LicenseExpires, &it.City, &it.UF, &it.UserID, &it.UserLogin, &it.Status, &it.Notes)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusOK, map[string]any{"driver": nil})
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"driver": it})
}

type fleetDriverVehicle struct {
	ID         uuid.UUID  `json:"id"`
	DriverID   uuid.UUID  `json:"driver_id"`
	DriverName string     `json:"driver_name,omitempty"`
	VehicleID  uuid.UUID  `json:"vehicle_id"`
	Plate      string     `json:"plate,omitempty"`
	StartsOn   *string    `json:"starts_on"`
	EndsOn     *string    `json:"ends_on"`
	IsPrimary  bool       `json:"is_primary"`
	Notes      *string    `json:"notes"`
}

func (s *Server) listFleetDriverVehicles(w http.ResponseWriter, r *http.Request) {
	vid := strings.TrimSpace(r.URL.Query().Get("vehicle_id"))
	did := strings.TrimSpace(r.URL.Query().Get("driver_id"))
	rows, err := s.DB().Query(r.Context(), `
		SELECT dv.id, dv.driver_id, d.name, dv.vehicle_id, v.plate,
			to_char(dv.starts_on,'YYYY-MM-DD'), to_char(dv.ends_on,'YYYY-MM-DD'), dv.is_primary, dv.notes
		FROM fleet_driver_vehicles dv
		JOIN fleet_drivers d ON d.id = dv.driver_id
		JOIN fleet_vehicles v ON v.id = dv.vehicle_id
		WHERE ($1 = '' OR dv.vehicle_id::text = $1)
		  AND ($2 = '' OR dv.driver_id::text = $2)
		ORDER BY dv.is_primary DESC, d.name
	`, vid, did)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	list := []fleetDriverVehicle{}
	for rows.Next() {
		var it fleetDriverVehicle
		if err := rows.Scan(&it.ID, &it.DriverID, &it.DriverName, &it.VehicleID, &it.Plate, &it.StartsOn, &it.EndsOn, &it.IsPrimary, &it.Notes); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (s *Server) createFleetDriverVehicle(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DriverID  uuid.UUID `json:"driver_id"`
		VehicleID uuid.UUID `json:"vehicle_id"`
		StartsOn  *string   `json:"starts_on"`
		EndsOn    *string   `json:"ends_on"`
		IsPrimary bool      `json:"is_primary"`
		Notes     *string   `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.DriverID == uuid.Nil || body.VehicleID == uuid.Nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "driver_id e vehicle_id obrigatórios", nil)
		return
	}
	var starts, ends *time.Time
	if body.StartsOn != nil && strings.TrimSpace(*body.StartsOn) != "" {
		t, err := time.Parse("2006-01-02", strings.TrimSpace(*body.StartsOn))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "VALIDATION", "starts_on inválida", nil)
			return
		}
		starts = &t
	}
	if body.EndsOn != nil && strings.TrimSpace(*body.EndsOn) != "" {
		t, err := time.Parse("2006-01-02", strings.TrimSpace(*body.EndsOn))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "VALIDATION", "ends_on inválida", nil)
			return
		}
		ends = &t
	}
	var id uuid.UUID
	err := s.DB().QueryRow(r.Context(), `
		INSERT INTO fleet_driver_vehicles (driver_id, vehicle_id, starts_on, ends_on, is_primary, notes)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id
	`, body.DriverID, body.VehicleID, starts, ends, body.IsPrimary, ptrTrim(body.Notes)).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) deleteFleetDriverVehicle(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	tag, err := s.DB().Exec(r.Context(), `DELETE FROM fleet_driver_vehicles WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "vínculo não encontrado", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) listFleetUsersLite(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB().Query(r.Context(), `SELECT id, login FROM users ORDER BY login`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	type u struct {
		ID    uuid.UUID `json:"id"`
		Login string    `json:"login"`
	}
	list := []u{}
	for rows.Next() {
		var it u
		if err := rows.Scan(&it.ID, &it.Login); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}
