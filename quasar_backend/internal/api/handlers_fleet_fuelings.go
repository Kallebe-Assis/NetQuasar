package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/fleetvalidate"
	"github.com/netquasar/netquasar/quasar_backend/internal/telegramclient"
)

type fleetFueling struct {
	ID               uuid.UUID  `json:"id"`
	Number           int64      `json:"number"`
	FueledAt         time.Time  `json:"fueled_at"`
	VehicleID        uuid.UUID  `json:"vehicle_id"`
	Plate            string     `json:"plate,omitempty"`
	VehicleDesc      string     `json:"vehicle_description,omitempty"`
	DriverID         *uuid.UUID `json:"driver_id"`
	DriverName       *string    `json:"driver_name,omitempty"`
	StationID        *uuid.UUID `json:"station_id"`
	StationName      *string    `json:"station_name,omitempty"`
	FuelID           uuid.UUID  `json:"fuel_id"`
	FuelName         string     `json:"fuel_name,omitempty"`
	CostCenterID     *uuid.UUID `json:"cost_center_id"`
	CostCenterName   *string    `json:"cost_center_name,omitempty"`
	Liters           float64    `json:"liters"`
	PricePerLiter    float64    `json:"price_per_liter"`
	TotalAmount      float64    `json:"total_amount"`
	OdometerPrevious *float64   `json:"odometer_previous"`
	OdometerCurrent  *float64   `json:"odometer_current"`
	KmDriven         *float64   `json:"km_driven"`
	HourmeterPrev    *float64   `json:"hourmeter_previous"`
	HourmeterCurr    *float64   `json:"hourmeter_current"`
	HoursWorked      *float64   `json:"hours_worked"`
	KmPerLiter       *float64   `json:"km_per_liter"`
	CostPerKm        *float64   `json:"cost_per_km"`
	LitersPer100Km   *float64   `json:"liters_per_100km"`
	PaymentMethod    *string    `json:"payment_method"`
	DocumentNumber   *string    `json:"document_number"`
	InvoiceNumber    *string    `json:"invoice_number"`
	Notes            *string    `json:"notes"`
}

func (s *Server) listFleetFuelings(w http.ResponseWriter, r *http.Request) {
	q := fleetQ(r)
	limit, offset := fleetLimitOffset(r)
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	vehicleID := strings.TrimSpace(r.URL.Query().Get("vehicle_id"))
	rows, err := s.DB().Query(r.Context(), `
		SELECT f.id, f.number, f.fueled_at, f.vehicle_id, COALESCE(v.plate, ''), v.description, f.driver_id, d.name,
			f.station_id, st.description, f.fuel_id, fu.description, f.cost_center_id, cc.description,
			f.liters, f.price_per_liter, f.total_amount, f.odometer_previous, f.odometer_current, f.km_driven,
			f.hourmeter_previous, f.hourmeter_current, f.hours_worked, f.km_per_liter, f.cost_per_km, f.liters_per_100km,
			f.payment_method, f.document_number, f.invoice_number, f.notes
		FROM fleet_fuelings f
		JOIN fleet_vehicles v ON v.id = f.vehicle_id
		JOIN fleet_fuels fu ON fu.id = f.fuel_id
		LEFT JOIN fleet_drivers d ON d.id = f.driver_id
		LEFT JOIN fleet_stations st ON st.id = f.station_id
		LEFT JOIN fleet_cost_centers cc ON cc.id = f.cost_center_id
		WHERE ($1 = '' OR v.plate ILIKE '%'||$1||'%' OR v.description ILIKE '%'||$1||'%' OR COALESCE(d.name,'') ILIKE '%'||$1||'%')
		  AND ($2 = '' OR f.vehicle_id::text = $2)
		  AND ($3 = '' OR f.fueled_at >= $3::timestamptz)
		  AND ($4 = '' OR f.fueled_at < ($4::date + interval '1 day'))
		ORDER BY f.fueled_at DESC
		LIMIT $5 OFFSET $6
	`, q, vehicleID, from, to, limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	list := []fleetFueling{}
	for rows.Next() {
		var it fleetFueling
		if err := rows.Scan(&it.ID, &it.Number, &it.FueledAt, &it.VehicleID, &it.Plate, &it.VehicleDesc, &it.DriverID, &it.DriverName,
			&it.StationID, &it.StationName, &it.FuelID, &it.FuelName, &it.CostCenterID, &it.CostCenterName,
			&it.Liters, &it.PricePerLiter, &it.TotalAmount, &it.OdometerPrevious, &it.OdometerCurrent, &it.KmDriven,
			&it.HourmeterPrev, &it.HourmeterCurr, &it.HoursWorked, &it.KmPerLiter, &it.CostPerKm, &it.LitersPer100Km,
			&it.PaymentMethod, &it.DocumentNumber, &it.InvoiceNumber, &it.Notes); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		it.Plate = fleetDisplayPlate(it.Plate)
		list = append(list, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

type fleetFuelingBody struct {
	FueledAt         string     `json:"fueled_at"`
	VehicleID        uuid.UUID  `json:"vehicle_id"`
	DriverID         *uuid.UUID `json:"driver_id"`
	StationID        *uuid.UUID `json:"station_id"`
	FuelID           uuid.UUID  `json:"fuel_id"`
	CostCenterID     *uuid.UUID `json:"cost_center_id"`
	Liters           float64    `json:"liters"`
	PricePerLiter    float64    `json:"price_per_liter"`
	OdometerPrevious *float64   `json:"odometer_previous"`
	OdometerCurrent  *float64   `json:"odometer_current"`
	HourmeterPrev    *float64   `json:"hourmeter_previous"`
	HourmeterCurr    *float64   `json:"hourmeter_current"`
	PaymentMethod    *string    `json:"payment_method"`
	DocumentNumber   *string    `json:"document_number"`
	InvoiceNumber    *string    `json:"invoice_number"`
	Notes            *string    `json:"notes"`
	Latitude         *float64   `json:"latitude"`
	Longitude        *float64   `json:"longitude"`
}

func (s *Server) loadFleetSettings(r *http.Request) fleetvalidate.Settings {
	settings := fleetvalidate.Settings{ConsumptionTolerancePct: 20, PriceTolerancePct: 15, MinMinutesBetween: 60}
	_ = s.DB().QueryRow(r.Context(), `
		SELECT consumption_tolerance_pct, price_tolerance_pct, min_minutes_between_fuelings FROM fleet_settings WHERE id=1
	`).Scan(&settings.ConsumptionTolerancePct, &settings.PriceTolerancePct, &settings.MinMinutesBetween)
	return settings
}

func (s *Server) createFleetFueling(w http.ResponseWriter, r *http.Request) {
	var body fleetFuelingBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if body.VehicleID == uuid.Nil || body.FuelID == uuid.Nil || body.Liters <= 0 || body.PricePerLiter < 0 {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "vehicle_id, fuel_id, liters e price_per_liter obrigatórios", nil)
		return
	}
	fueledAt := time.Now()
	if strings.TrimSpace(body.FueledAt) != "" {
		t, err := parseTimeFlexible(body.FueledAt)
		if err != nil || t.IsZero() {
			writeErr(w, http.StatusBadRequest, "VALIDATION", "fueled_at inválido", nil)
			return
		}
		fueledAt = t
	}

	var veh fleetvalidate.VehicleCtx
	var odo float64
	var costCenter *uuid.UUID
	var vehStatus string
	err := s.DB().QueryRow(r.Context(), `
		SELECT tank_capacity_liters, expected_km_per_liter, min_km_per_liter, max_km_per_liter, odometer_current, cost_center_id, status
		FROM fleet_vehicles WHERE id=$1
	`, body.VehicleID).Scan(&veh.TankCapacityLiters, &veh.ExpectedKmPerLiter, &veh.MinKmPerLiter, &veh.MaxKmPerLiter, &odo, &costCenter, &vehStatus)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "veículo não encontrado", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if fleetStatusBlocksLaunch(vehStatus) {
		writeErr(w, http.StatusBadRequest, "VALIDATION", fleetLaunchBlockedMsg("abastecimento"), nil)
		return
	}
	veh.OdometerCurrent = odo
	if body.OdometerPrevious == nil {
		body.OdometerPrevious = &odo
	}
	if body.CostCenterID == nil {
		body.CostCenterID = costCenter
	}

	var lastAt *time.Time
	_ = s.DB().QueryRow(r.Context(), `SELECT fueled_at FROM fleet_fuelings WHERE vehicle_id=$1 ORDER BY fueled_at DESC LIMIT 1`, body.VehicleID).Scan(&lastAt)

	var avgPrice *float64
	if body.StationID != nil {
		_ = s.DB().QueryRow(r.Context(), `
			SELECT AVG(price_per_liter) FROM fleet_fuelings
			WHERE station_id=$1 AND fuel_id=$2 AND fueled_at > now() - interval '90 days'
		`, *body.StationID, body.FuelID).Scan(&avgPrice)
	}

	in := fleetvalidate.Input{
		Liters:        body.Liters,
		PricePerLiter: body.PricePerLiter,
		OdometerPrev:  body.OdometerPrevious,
		OdometerCurr:  body.OdometerCurrent,
		HourmeterPrev: body.HourmeterPrev,
		HourmeterCurr: body.HourmeterCurr,
		LastFuelingAt: lastAt,
		FuelingAt:     fueledAt,
		AvgPrice:      avgPrice,
	}
	comp, err := fleetvalidate.Compute(in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	}
	settings := s.loadFleetSettings(r)
	alerts := fleetvalidate.EvaluateAlerts(veh, in, comp, settings)

	uid := s.userIDFromRequest(r)
	tx, err := s.DB().Begin(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer tx.Rollback(r.Context())

	var id uuid.UUID
	var number int64
	err = tx.QueryRow(r.Context(), `
		INSERT INTO fleet_fuelings (
			fueled_at, vehicle_id, driver_id, station_id, fuel_id, cost_center_id,
			liters, price_per_liter, total_amount, odometer_previous, odometer_current, km_driven,
			hourmeter_previous, hourmeter_current, hours_worked, km_per_liter, cost_per_km, liters_per_100km,
			payment_method, document_number, invoice_number, latitude, longitude, notes, created_by, updated_by
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$25
		) RETURNING id, number
	`, fueledAt, body.VehicleID, body.DriverID, body.StationID, body.FuelID, body.CostCenterID,
		body.Liters, body.PricePerLiter, comp.TotalAmount, body.OdometerPrevious, body.OdometerCurrent, comp.KmDriven,
		body.HourmeterPrev, body.HourmeterCurr, comp.HoursWorked, comp.KmPerLiter, comp.CostPerKm, comp.LitersPer100Km,
		ptrTrim(body.PaymentMethod), ptrTrim(body.DocumentNumber), ptrTrim(body.InvoiceNumber),
		body.Latitude, body.Longitude, ptrTrim(body.Notes), uid).Scan(&id, &number)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}

	if body.OdometerCurrent != nil {
		if _, err := tx.Exec(r.Context(), `
			UPDATE fleet_vehicles SET odometer_current=$2, hourmeter_current=COALESCE($3, hourmeter_current), updated_at=now(), updated_by=$4 WHERE id=$1
		`, body.VehicleID, *body.OdometerCurrent, body.HourmeterCurr, uid); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO fleet_odometer_readings (vehicle_id, reading_at, odometer, hourmeter, source, fueling_id, created_by)
			VALUES ($1,$2,$3,$4,'fueling',$5,$6)
		`, body.VehicleID, fueledAt, *body.OdometerCurrent, body.HourmeterCurr, id, uid); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
	}

	alertOut := make([]map[string]any, 0, len(alerts))
	for _, a := range alerts {
		var aid uuid.UUID
		if err := tx.QueryRow(r.Context(), `
			INSERT INTO fleet_alerts (severity, alert_type, title, message, vehicle_id, fueling_id)
			VALUES ($1,$2,$3,$4,$5,$6) RETURNING id
		`, a.Severity, a.AlertType, a.Title, a.Message, body.VehicleID, id).Scan(&aid); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		alertOut = append(alertOut, map[string]any{"id": aid, "severity": a.Severity, "alert_type": a.AlertType, "title": a.Title, "message": a.Message})
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": id, "number": number, "total_amount": comp.TotalAmount, "km_driven": comp.KmDriven,
		"km_per_liter": comp.KmPerLiter, "cost_per_km": comp.CostPerKm, "liters_per_100km": comp.LitersPer100Km,
		"alerts": alertOut,
	})
}

func (s *Server) listFleetAlerts(w http.ResponseWriter, r *http.Request) {
	onlyOpen := r.URL.Query().Get("open") != "0"
	rows, err := s.DB().Query(r.Context(), `
		SELECT a.id, a.severity, a.alert_type, a.title, a.message, a.vehicle_id, v.plate, a.fueling_id, a.acknowledged_at, a.created_at
		FROM fleet_alerts a
		LEFT JOIN fleet_vehicles v ON v.id = a.vehicle_id
		WHERE (NOT $1 OR a.acknowledged_at IS NULL)
		ORDER BY a.created_at DESC
		LIMIT 2000
	`, onlyOpen)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	type item struct {
		ID             uuid.UUID  `json:"id"`
		Severity       string     `json:"severity"`
		AlertType      string     `json:"alert_type"`
		Title          string     `json:"title"`
		Message        string     `json:"message"`
		VehicleID      *uuid.UUID `json:"vehicle_id"`
		Plate          *string    `json:"plate"`
		FuelingID      *uuid.UUID `json:"fueling_id"`
		AcknowledgedAt *time.Time `json:"acknowledged_at"`
		CreatedAt      time.Time  `json:"created_at"`
	}
	list := []item{}
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.Severity, &it.AlertType, &it.Title, &it.Message, &it.VehicleID, &it.Plate, &it.FuelingID, &it.AcknowledgedAt, &it.CreatedAt); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (s *Server) ackFleetAlert(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	tag, err := s.DB().Exec(r.Context(), `
		UPDATE fleet_alerts SET acknowledged_at=now(), acknowledged_by=$2 WHERE id=$1 AND acknowledged_at IS NULL
	`, id, s.userIDFromRequest(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "alerta não encontrado ou já reconhecido", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) getFleetSettings(w http.ResponseWriter, r *http.Request) {
	var tolC, tolP float64
	var minM int
	err := s.DB().QueryRow(r.Context(), `
		SELECT consumption_tolerance_pct, price_tolerance_pct, min_minutes_between_fuelings FROM fleet_settings WHERE id=1
	`).Scan(&tolC, &tolP, &minM)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"consumption_tolerance_pct":     tolC,
		"price_tolerance_pct":           tolP,
		"min_minutes_between_fuelings":  minM,
	})
}

func (s *Server) patchFleetSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ConsumptionTolerancePct    *float64 `json:"consumption_tolerance_pct"`
		PriceTolerancePct          *float64 `json:"price_tolerance_pct"`
		MinMinutesBetweenFuelings  *int     `json:"min_minutes_between_fuelings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	cur := s.loadFleetSettings(r)
	if body.ConsumptionTolerancePct != nil {
		cur.ConsumptionTolerancePct = *body.ConsumptionTolerancePct
	}
	if body.PriceTolerancePct != nil {
		cur.PriceTolerancePct = *body.PriceTolerancePct
	}
	if body.MinMinutesBetweenFuelings != nil {
		cur.MinMinutesBetween = *body.MinMinutesBetweenFuelings
	}
	_, err := s.DB().Exec(r.Context(), `
		UPDATE fleet_settings SET consumption_tolerance_pct=$1, price_tolerance_pct=$2, min_minutes_between_fuelings=$3, updated_at=now() WHERE id=1
	`, cur.ConsumptionTolerancePct, cur.PriceTolerancePct, cur.MinMinutesBetween)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) fleetDashboard(w http.ResponseWriter, r *http.Request) {
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	vehicleID := strings.TrimSpace(r.URL.Query().Get("vehicle_id"))
	if from == "" {
		from = time.Now().Format("2006-01") + "-01"
	}
	if to == "" {
		to = time.Now().Format("2006-01-02")
	}

	var vehiclesTotal, vehiclesActive int64
	if vehicleID != "" {
		_ = s.DB().QueryRow(r.Context(), `SELECT COUNT(*) FROM fleet_vehicles WHERE id::text=$1`, vehicleID).Scan(&vehiclesTotal)
		_ = s.DB().QueryRow(r.Context(), `SELECT COUNT(*) FROM fleet_vehicles WHERE id::text=$1 AND status='active'`, vehicleID).Scan(&vehiclesActive)
	} else {
		_ = s.DB().QueryRow(r.Context(), `SELECT COUNT(*) FROM fleet_vehicles`).Scan(&vehiclesTotal)
		_ = s.DB().QueryRow(r.Context(), `SELECT COUNT(*) FROM fleet_vehicles WHERE status='active'`).Scan(&vehiclesActive)
	}

	var liters, fuelAmount, expAmount float64
	var fuelCount, expCount int64
	_ = s.DB().QueryRow(r.Context(), `
		SELECT COALESCE(SUM(liters),0), COALESCE(SUM(total_amount),0), COUNT(*)
		FROM fleet_fuelings
		WHERE fueled_at >= $1::date AND fueled_at < ($2::date + interval '1 day')
		  AND ($3 = '' OR vehicle_id::text = $3)
	`, from, to, vehicleID).Scan(&liters, &fuelAmount, &fuelCount)
	_ = s.DB().QueryRow(r.Context(), `
		SELECT COALESCE(SUM(total_amount),0), COUNT(*)
		FROM fleet_expenses
		WHERE occurred_at >= $1::date AND occurred_at < ($2::date + interval '1 day')
		  AND ($3 = '' OR vehicle_id::text = $3)
	`, from, to, vehicleID).Scan(&expAmount, &expCount)

	var avgPrice, avgKPL, avgCPK *float64
	_ = s.DB().QueryRow(r.Context(), `
		SELECT AVG(price_per_liter),
			AVG(km_per_liter) FILTER (WHERE km_per_liter IS NOT NULL),
			AVG(cost_per_km) FILTER (WHERE cost_per_km IS NOT NULL)
		FROM fleet_fuelings
		WHERE fueled_at >= $1::date AND fueled_at < ($2::date + interval '1 day')
		  AND ($3 = '' OR vehicle_id::text = $3)
	`, from, to, vehicleID).Scan(&avgPrice, &avgKPL, &avgCPK)

	type rank struct {
		VehicleID   uuid.UUID `json:"vehicle_id"`
		Plate       string    `json:"plate"`
		Description string    `json:"description"`
		Value       float64   `json:"value"`
		Liters      float64   `json:"liters,omitempty"`
		Amount      float64   `json:"amount,omitempty"`
	}
	loadRank := func(sql string) []rank {
		rows, err := s.DB().Query(r.Context(), sql, from, to, vehicleID)
		if err != nil {
			return nil
		}
		defer rows.Close()
		out := []rank{}
		for rows.Next() {
			var it rank
			if err := rows.Scan(&it.VehicleID, &it.Plate, &it.Description, &it.Value, &it.Liters, &it.Amount); err != nil {
				continue
			}
			it.Plate = fleetDisplayPlate(it.Plate)
			out = append(out, it)
		}
		return out
	}

	eco := loadRank(`
		SELECT v.id, COALESCE(v.plate, ''), v.description, AVG(f.km_per_liter), COALESCE(SUM(f.liters),0), COALESCE(SUM(f.total_amount),0)
		FROM fleet_fuelings f JOIN fleet_vehicles v ON v.id=f.vehicle_id
		WHERE f.km_per_liter IS NOT NULL AND f.fueled_at >= $1::date AND f.fueled_at < ($2::date + interval '1 day')
		  AND ($3 = '' OR f.vehicle_id::text = $3)
		GROUP BY v.id, v.plate, v.description ORDER BY AVG(f.km_per_liter) DESC LIMIT 10`)
	thirsty := loadRank(`
		SELECT v.id, COALESCE(v.plate, ''), v.description, COALESCE(SUM(f.liters),0), COALESCE(SUM(f.liters),0), COALESCE(SUM(f.total_amount),0)
		FROM fleet_fuelings f JOIN fleet_vehicles v ON v.id=f.vehicle_id
		WHERE f.fueled_at >= $1::date AND f.fueled_at < ($2::date + interval '1 day')
		  AND ($3 = '' OR f.vehicle_id::text = $3)
		GROUP BY v.id, v.plate, v.description ORDER BY SUM(f.liters) DESC LIMIT 10`)
	costly := loadRank(`
		SELECT v.id, COALESCE(v.plate, ''), v.description,
			COALESCE(SUM(f.fuel_amt),0) + COALESCE(SUM(e.exp_amt),0),
			COALESCE(SUM(f.liters),0),
			COALESCE(SUM(f.fuel_amt),0) + COALESCE(SUM(e.exp_amt),0)
		FROM fleet_vehicles v
		LEFT JOIN (
			SELECT vehicle_id, SUM(liters) AS liters, SUM(total_amount) AS fuel_amt
			FROM fleet_fuelings
			WHERE fueled_at >= $1::date AND fueled_at < ($2::date + interval '1 day')
			GROUP BY vehicle_id
		) f ON f.vehicle_id = v.id
		LEFT JOIN (
			SELECT vehicle_id, SUM(total_amount) AS exp_amt
			FROM fleet_expenses
			WHERE occurred_at >= $1::date AND occurred_at < ($2::date + interval '1 day')
			GROUP BY vehicle_id
		) e ON e.vehicle_id = v.id
		WHERE ($3 = '' OR v.id::text = $3)
		  AND (COALESCE(f.fuel_amt,0) + COALESCE(e.exp_amt,0)) > 0
		GROUP BY v.id, v.plate, v.description
		ORDER BY 4 DESC LIMIT 10`)

	type stationRank struct {
		StationID   uuid.UUID `json:"station_id"`
		Description string    `json:"description"`
		AvgPrice    float64   `json:"avg_price"`
		Liters      float64   `json:"liters"`
	}
	stationRows, _ := s.DB().Query(r.Context(), `
		SELECT COALESCE(st.id, '00000000-0000-0000-0000-000000000000'::uuid),
			COALESCE(st.description, '(sem posto)'), AVG(f.price_per_liter), COALESCE(SUM(f.liters),0)
		FROM fleet_fuelings f JOIN fleet_stations st ON st.id=f.station_id
		WHERE f.fueled_at >= $1::date AND f.fueled_at < ($2::date + interval '1 day')
		  AND ($3 = '' OR f.vehicle_id::text = $3)
		GROUP BY 1, 2 ORDER BY AVG(f.price_per_liter) ASC LIMIT 10
	`, from, to, vehicleID)
	cheapStations := []stationRank{}
	if stationRows != nil {
		defer stationRows.Close()
		for stationRows.Next() {
			var it stationRank
			if err := stationRows.Scan(&it.StationID, &it.Description, &it.AvgPrice, &it.Liters); err == nil {
				cheapStations = append(cheapStations, it)
			}
		}
	}

	type dayPoint struct {
		Date          string  `json:"date"`
		Liters        float64 `json:"liters"`
		FuelAmount    float64 `json:"fuel_amount"`
		ExpenseAmount float64 `json:"expense_amount"`
		Amount        float64 `json:"amount"`
	}
	drows, _ := s.DB().Query(r.Context(), `
		WITH days AS (
			SELECT generate_series($1::date, $2::date, interval '1 day')::date AS d
		),
		fuel AS (
			SELECT fueled_at::date AS d, SUM(liters) AS liters, SUM(total_amount) AS amount
			FROM fleet_fuelings
			WHERE fueled_at >= $1::date AND fueled_at < ($2::date + interval '1 day')
			  AND ($3 = '' OR vehicle_id::text = $3)
			GROUP BY 1
		),
		exp AS (
			SELECT occurred_at::date AS d, SUM(total_amount) AS amount
			FROM fleet_expenses
			WHERE occurred_at >= $1::date AND occurred_at < ($2::date + interval '1 day')
			  AND ($3 = '' OR vehicle_id::text = $3)
			GROUP BY 1
		)
		SELECT to_char(days.d, 'YYYY-MM-DD'),
			COALESCE(fuel.liters, 0),
			COALESCE(fuel.amount, 0),
			COALESCE(exp.amount, 0),
			COALESCE(fuel.amount, 0) + COALESCE(exp.amount, 0)
		FROM days
		LEFT JOIN fuel ON fuel.d = days.d
		LEFT JOIN exp ON exp.d = days.d
		ORDER BY 1
	`, from, to, vehicleID)
	series := []dayPoint{}
	if drows != nil {
		defer drows.Close()
		for drows.Next() {
			var it dayPoint
			if err := drows.Scan(&it.Date, &it.Liters, &it.FuelAmount, &it.ExpenseAmount, &it.Amount); err == nil {
				series = append(series, it)
			}
		}
	}

	var openAlerts int64
	_ = s.DB().QueryRow(r.Context(), `
		SELECT COUNT(*) FROM fleet_alerts
		WHERE acknowledged_at IS NULL AND ($1 = '' OR vehicle_id::text = $1)
	`, vehicleID).Scan(&openAlerts)

	writeJSON(w, http.StatusOK, map[string]any{
		"period":     map[string]string{"from": from, "to": to},
		"vehicle_id": vehicleID,
		"fleet": map[string]any{"vehicles_total": vehiclesTotal, "vehicles_active": vehiclesActive},
		"fuel": map[string]any{
			"liters": liters, "amount": fuelAmount, "count": fuelCount,
			"avg_price_per_liter": avgPrice, "avg_km_per_liter": avgKPL, "avg_cost_per_km": avgCPK,
		},
		"expenses": map[string]any{"amount": expAmount, "count": expCount},
		"totals": map[string]any{
			"fuel_amount": fuelAmount, "expense_amount": expAmount, "grand_total": fuelAmount + expAmount,
			"fuel_count": fuelCount, "expense_count": expCount,
		},
		"rankings": map[string]any{
			"most_efficient": eco, "highest_consumption": thirsty, "highest_cost_per_km": costly, "cheapest_stations": cheapStations,
		},
		"daily_series": series,
		"open_alerts":  openAlerts,
	})
}

func (s *Server) fleetVehicleSummary(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	if from == "" {
		from = time.Now().Format("2006-01") + "-01"
	}
	if to == "" {
		to = time.Now().Format("2006-01-02")
	}
	var liters, amount, km float64
	var count int64
	var avgKPL, avgCPK, avgPrice *float64
	err = s.DB().QueryRow(r.Context(), `
		SELECT COALESCE(SUM(liters),0), COALESCE(SUM(total_amount),0), COALESCE(SUM(km_driven),0), COUNT(*),
			AVG(km_per_liter) FILTER (WHERE km_per_liter IS NOT NULL),
			AVG(cost_per_km) FILTER (WHERE cost_per_km IS NOT NULL),
			AVG(price_per_liter)
		FROM fleet_fuelings
		WHERE vehicle_id=$1 AND fueled_at >= $2::date AND fueled_at < ($3::date + interval '1 day')
	`, id, from, to).Scan(&liters, &amount, &km, &count, &avgKPL, &avgCPK, &avgPrice)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"vehicle_id": id, "from": from, "to": to,
		"liters": liters, "amount": amount, "km_driven": km, "count": count,
		"avg_km_per_liter": avgKPL, "avg_cost_per_km": avgCPK, "avg_price_per_liter": avgPrice,
	})
}

func (s *Server) fleetReportCSV(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	if from == "" {
		from = time.Now().AddDate(0, -1, 0).Format("2006-01-02")
	}
	if to == "" {
		to = time.Now().Format("2006-01-02")
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=fleet-%s.csv", kind))
	cw := csv.NewWriter(w)
	defer cw.Flush()

	switch kind {
	case "fuelings":
		_ = cw.Write([]string{"numero", "data", "placa", "motorista", "posto", "combustivel", "litros", "preco_litro", "total", "km", "km_l", "rs_km", "centro_custo"})
		rows, err := s.DB().Query(r.Context(), `
			SELECT f.number, f.fueled_at, COALESCE(v.plate, ''), COALESCE(d.name,''), COALESCE(st.description,''), fu.description,
				f.liters, f.price_per_liter, f.total_amount, COALESCE(f.km_driven,0), COALESCE(f.km_per_liter,0), COALESCE(f.cost_per_km,0), COALESCE(cc.description,'')
			FROM fleet_fuelings f
			JOIN fleet_vehicles v ON v.id=f.vehicle_id
			JOIN fleet_fuels fu ON fu.id=f.fuel_id
			LEFT JOIN fleet_drivers d ON d.id=f.driver_id
			LEFT JOIN fleet_stations st ON st.id=f.station_id
			LEFT JOIN fleet_cost_centers cc ON cc.id=f.cost_center_id
			WHERE f.fueled_at >= $1::date AND f.fueled_at < ($2::date + interval '1 day')
			ORDER BY f.fueled_at
		`, from, to)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var n int64
			var at time.Time
			var plate, driver, station, fuel, cc string
			var liters, price, total, km, kpl, cpk float64
			if err := rows.Scan(&n, &at, &plate, &driver, &station, &fuel, &liters, &price, &total, &km, &kpl, &cpk, &cc); err != nil {
				continue
			}
			_ = cw.Write([]string{
				strconv.FormatInt(n, 10), at.Format("2006-01-02 15:04"), fleetDisplayPlateOrUnknown(plate), driver, station, fuel,
				fmt.Sprintf("%.3f", liters), fmt.Sprintf("%.4f", price), fmt.Sprintf("%.2f", total),
				fmt.Sprintf("%.1f", km), fmt.Sprintf("%.2f", kpl), fmt.Sprintf("%.4f", cpk), cc,
			})
		}
	case "by-vehicle":
		_ = cw.Write([]string{"placa", "descricao", "litros", "valor", "km", "km_l_medio", "rs_km_medio"})
		rows, err := s.DB().Query(r.Context(), `
			SELECT COALESCE(v.plate, ''), v.description, COALESCE(SUM(f.liters),0), COALESCE(SUM(f.total_amount),0), COALESCE(SUM(f.km_driven),0),
				AVG(f.km_per_liter) FILTER (WHERE f.km_per_liter IS NOT NULL),
				AVG(f.cost_per_km) FILTER (WHERE f.cost_per_km IS NOT NULL)
			FROM fleet_fuelings f JOIN fleet_vehicles v ON v.id=f.vehicle_id
			WHERE f.fueled_at >= $1::date AND f.fueled_at < ($2::date + interval '1 day')
			GROUP BY v.plate, v.description ORDER BY SUM(f.total_amount) DESC
		`, from, to)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var plate, desc string
			var liters, amount, km float64
			var kpl, cpk *float64
			if err := rows.Scan(&plate, &desc, &liters, &amount, &km, &kpl, &cpk); err != nil {
				continue
			}
			_ = cw.Write([]string{fleetDisplayPlateOrUnknown(plate), desc, fmt.Sprintf("%.3f", liters), fmt.Sprintf("%.2f", amount), fmt.Sprintf("%.1f", km),
				fmtPtr(kpl), fmtPtr(cpk)})
		}
	case "by-driver":
		_ = cw.Write([]string{"motorista", "litros", "valor", "abastecimentos"})
		rows, err := s.DB().Query(r.Context(), `
			SELECT COALESCE(d.name,'(sem motorista)'), COALESCE(SUM(f.liters),0), COALESCE(SUM(f.total_amount),0), COUNT(*)
			FROM fleet_fuelings f LEFT JOIN fleet_drivers d ON d.id=f.driver_id
			WHERE f.fueled_at >= $1::date AND f.fueled_at < ($2::date + interval '1 day')
			GROUP BY 1 ORDER BY SUM(f.total_amount) DESC
		`, from, to)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			var liters, amount float64
			var n int64
			if err := rows.Scan(&name, &liters, &amount, &n); err != nil {
				continue
			}
			_ = cw.Write([]string{name, fmt.Sprintf("%.3f", liters), fmt.Sprintf("%.2f", amount), strconv.FormatInt(n, 10)})
		}
	case "by-station":
		_ = cw.Write([]string{"posto", "litros", "valor", "preco_medio"})
		rows, err := s.DB().Query(r.Context(), `
			SELECT COALESCE(st.description,'(sem posto)'), COALESCE(SUM(f.liters),0), COALESCE(SUM(f.total_amount),0), AVG(f.price_per_liter)
			FROM fleet_fuelings f LEFT JOIN fleet_stations st ON st.id=f.station_id
			WHERE f.fueled_at >= $1::date AND f.fueled_at < ($2::date + interval '1 day')
			GROUP BY 1 ORDER BY SUM(f.total_amount) DESC
		`, from, to)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			var liters, amount, avg float64
			if err := rows.Scan(&name, &liters, &amount, &avg); err != nil {
				continue
			}
			_ = cw.Write([]string{name, fmt.Sprintf("%.3f", liters), fmt.Sprintf("%.2f", amount), fmt.Sprintf("%.4f", avg)})
		}
	case "by-cost-center":
		_ = cw.Write([]string{"centro_custo", "litros", "valor", "abastecimentos"})
		rows, err := s.DB().Query(r.Context(), `
			SELECT COALESCE(cc.description,'(sem centro)'), COALESCE(SUM(f.liters),0), COALESCE(SUM(f.total_amount),0), COUNT(*)
			FROM fleet_fuelings f LEFT JOIN fleet_cost_centers cc ON cc.id=f.cost_center_id
			WHERE f.fueled_at >= $1::date AND f.fueled_at < ($2::date + interval '1 day')
			GROUP BY 1 ORDER BY SUM(f.total_amount) DESC
		`, from, to)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			var liters, amount float64
			var n int64
			if err := rows.Scan(&name, &liters, &amount, &n); err != nil {
				continue
			}
			_ = cw.Write([]string{name, fmt.Sprintf("%.3f", liters), fmt.Sprintf("%.2f", amount), strconv.FormatInt(n, 10)})
		}
	default:
		writeErr(w, http.StatusBadRequest, "VALIDATION", "kind inválido", nil)
	}
}

func fmtPtr(v *float64) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%.4f", *v)
}

func (s *Server) fleetReportTelegram(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	if from == "" {
		from = time.Now().AddDate(0, -1, 0).Format("2006-01-02")
	}
	if to == "" {
		to = time.Now().Format("2006-01-02")
	}
	cfg, err := telegramclient.LoadConfig(r.Context(), s.DB(), "reports")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if !cfg.Ready() {
		writeErr(w, http.StatusUnprocessableEntity, "VALIDATION", "Telegram de relatórios não configurado (bot_token/chat_id).", nil)
		return
	}
	text, err := s.composeFleetReportTelegram(r.Context(), kind, from, to)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	}
	if err := telegramclient.SendMessageChunks(r.Context(), cfg, text); err != nil {
		writeErr(w, http.StatusBadGateway, "TELEGRAM_SEND_FAILED", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "fleet_report", kind, "telegram_send", s.actorFromRequest(r), nil, map[string]any{"kind": kind, "from": from, "to": to})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "kind": kind})
}

func (s *Server) composeFleetReportTelegram(ctx context.Context, kind, from, to string) (string, error) {
	title := map[string]string{
		"fuelings":       "Abastecimentos",
		"by-vehicle":     "Por veículo",
		"by-driver":      "Por motorista",
		"by-station":     "Por posto",
		"by-cost-center": "Por centro de custo",
	}[kind]
	if title == "" {
		return "", fmt.Errorf("kind inválido")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Frota — %s\nPeríodo: %s a %s\n\n", title, from, to)

	switch kind {
	case "fuelings":
		rows, err := s.DB().Query(ctx, `
			SELECT f.fueled_at, COALESCE(v.plate, ''), fu.description, f.liters, f.total_amount
			FROM fleet_fuelings f
			JOIN fleet_vehicles v ON v.id=f.vehicle_id
			JOIN fleet_fuels fu ON fu.id=f.fuel_id
			WHERE f.fueled_at >= $1::date AND f.fueled_at < ($2::date + interval '1 day')
			ORDER BY f.fueled_at DESC LIMIT 80
		`, from, to)
		if err != nil {
			return "", err
		}
		defer rows.Close()
		n := 0
		for rows.Next() {
			var at time.Time
			var plate, fuel string
			var liters, total float64
			if err := rows.Scan(&at, &plate, &fuel, &liters, &total); err != nil {
				continue
			}
			fmt.Fprintf(&b, "%s  %s  %s  %.1f L  R$ %.2f\n", at.Format("02/01"), fleetDisplayPlateOrUnknown(plate), fuel, liters, total)
			n++
		}
		if n == 0 {
			b.WriteString("Sem abastecimentos no período.")
		}
	case "by-vehicle":
		rows, err := s.DB().Query(ctx, `
			SELECT COALESCE(v.plate, ''), v.description, COALESCE(SUM(f.liters),0), COALESCE(SUM(f.total_amount),0)
			FROM fleet_fuelings f JOIN fleet_vehicles v ON v.id=f.vehicle_id
			WHERE f.fueled_at >= $1::date AND f.fueled_at < ($2::date + interval '1 day')
			GROUP BY v.plate, v.description ORDER BY SUM(f.total_amount) DESC LIMIT 40
		`, from, to)
		if err != nil {
			return "", err
		}
		defer rows.Close()
		n := 0
		for rows.Next() {
			var plate, desc string
			var liters, amount float64
			if err := rows.Scan(&plate, &desc, &liters, &amount); err != nil {
				continue
			}
			fmt.Fprintf(&b, "%s — %s\n%.1f L · R$ %.2f\n\n", fleetDisplayPlateOrUnknown(plate), desc, liters, amount)
			n++
		}
		if n == 0 {
			b.WriteString("Sem dados no período.")
		}
	case "by-driver":
		rows, err := s.DB().Query(ctx, `
			SELECT COALESCE(d.name,'(sem motorista)'), COALESCE(SUM(f.liters),0), COALESCE(SUM(f.total_amount),0), COUNT(*)
			FROM fleet_fuelings f LEFT JOIN fleet_drivers d ON d.id=f.driver_id
			WHERE f.fueled_at >= $1::date AND f.fueled_at < ($2::date + interval '1 day')
			GROUP BY 1 ORDER BY SUM(f.total_amount) DESC LIMIT 40
		`, from, to)
		if err != nil {
			return "", err
		}
		defer rows.Close()
		n := 0
		for rows.Next() {
			var name string
			var liters, amount float64
			var cnt int64
			if err := rows.Scan(&name, &liters, &amount, &cnt); err != nil {
				continue
			}
			fmt.Fprintf(&b, "%s\n%.1f L · R$ %.2f · %d abast.\n\n", name, liters, amount, cnt)
			n++
		}
		if n == 0 {
			b.WriteString("Sem dados no período.")
		}
	case "by-station":
		rows, err := s.DB().Query(ctx, `
			SELECT COALESCE(st.description,'(sem posto)'), COALESCE(SUM(f.liters),0), COALESCE(SUM(f.total_amount),0), AVG(f.price_per_liter)
			FROM fleet_fuelings f LEFT JOIN fleet_stations st ON st.id=f.station_id
			WHERE f.fueled_at >= $1::date AND f.fueled_at < ($2::date + interval '1 day')
			GROUP BY 1 ORDER BY SUM(f.total_amount) DESC LIMIT 40
		`, from, to)
		if err != nil {
			return "", err
		}
		defer rows.Close()
		n := 0
		for rows.Next() {
			var name string
			var liters, amount, avg float64
			if err := rows.Scan(&name, &liters, &amount, &avg); err != nil {
				continue
			}
			fmt.Fprintf(&b, "%s\n%.1f L · R$ %.2f · média R$ %.3f/L\n\n", name, liters, amount, avg)
			n++
		}
		if n == 0 {
			b.WriteString("Sem dados no período.")
		}
	case "by-cost-center":
		rows, err := s.DB().Query(ctx, `
			SELECT COALESCE(cc.description,'(sem centro)'), COALESCE(SUM(f.liters),0), COALESCE(SUM(f.total_amount),0), COUNT(*)
			FROM fleet_fuelings f LEFT JOIN fleet_cost_centers cc ON cc.id=f.cost_center_id
			WHERE f.fueled_at >= $1::date AND f.fueled_at < ($2::date + interval '1 day')
			GROUP BY 1 ORDER BY SUM(f.total_amount) DESC LIMIT 40
		`, from, to)
		if err != nil {
			return "", err
		}
		defer rows.Close()
		n := 0
		for rows.Next() {
			var name string
			var liters, amount float64
			var cnt int64
			if err := rows.Scan(&name, &liters, &amount, &cnt); err != nil {
				continue
			}
			fmt.Fprintf(&b, "%s\n%.1f L · R$ %.2f · %d abast.\n\n", name, liters, amount, cnt)
			n++
		}
		if n == 0 {
			b.WriteString("Sem dados no período.")
		}
	}
	return strings.TrimSpace(b.String()), nil
}
