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
	"github.com/jackc/pgx/v5/pgxpool"
)

type fleetExpenseType struct {
	ID          uuid.UUID `json:"id"`
	Description string    `json:"description"`
	Code        *string   `json:"code"`
	Active      bool      `json:"active"`
	Notes       *string   `json:"notes"`
}

func (s *Server) listFleetExpenseTypes(w http.ResponseWriter, r *http.Request) {
	q := fleetQ(r)
	onlyActive := r.URL.Query().Get("active") == "1" || r.URL.Query().Get("active") == "true"
	rows, err := s.DB().Query(r.Context(), `
		SELECT id, description, code, active, notes
		FROM fleet_expense_types
		WHERE ($1 = '' OR description ILIKE '%'||$1||'%' OR COALESCE(code,'') ILIKE '%'||$1||'%')
		  AND (NOT $2 OR active)
		ORDER BY description
	`, q, onlyActive)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	list := []fleetExpenseType{}
	for rows.Next() {
		var it fleetExpenseType
		if err := rows.Scan(&it.ID, &it.Description, &it.Code, &it.Active, &it.Notes); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, it)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (s *Server) createFleetExpenseType(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Description string  `json:"description"`
		Code        *string `json:"code"`
		Active      *bool   `json:"active"`
		Notes       *string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Description) == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "description obrigatória", nil)
		return
	}
	active := true
	if body.Active != nil {
		active = *body.Active
	}
	var id uuid.UUID
	err := s.DB().QueryRow(r.Context(), `
		INSERT INTO fleet_expense_types (description, code, active, notes, created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$5) RETURNING id
	`, strings.TrimSpace(body.Description), ptrTrim(body.Code), active, ptrTrim(body.Notes), s.userIDFromRequest(r)).Scan(&id)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) patchFleetExpenseType(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	var cur fleetExpenseType
	if err := s.DB().QueryRow(r.Context(), `SELECT id, description, code, active, notes FROM fleet_expense_types WHERE id=$1`, id).
		Scan(&cur.ID, &cur.Description, &cur.Code, &cur.Active, &cur.Notes); err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "tipo de despesa não encontrado", nil)
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if v, ok := body["description"]; ok {
		_ = json.Unmarshal(v, &cur.Description)
	}
	if v, ok := body["code"]; ok {
		_ = json.Unmarshal(v, &cur.Code)
	}
	if v, ok := body["active"]; ok {
		_ = json.Unmarshal(v, &cur.Active)
	}
	if v, ok := body["notes"]; ok {
		_ = json.Unmarshal(v, &cur.Notes)
	}
	_, err = s.DB().Exec(r.Context(), `
		UPDATE fleet_expense_types SET description=$2, code=$3, active=$4, notes=$5, updated_at=now(), updated_by=$6 WHERE id=$1
	`, id, strings.TrimSpace(cur.Description), ptrTrim(cur.Code), cur.Active, ptrTrim(cur.Notes), s.userIDFromRequest(r))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func fleetExpenseTypeAlias(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.NewReplacer("ç", "c", "ã", "a", "á", "a", "â", "a", "é", "e", "ê", "e", "í", "i", "ó", "o", "ô", "o", "ú", "u").Replace(s)
	s = strings.Join(strings.Fields(s), " ")
	switch s {
	case "preventive_maintenance", "manutencao preventiva", "preventiva", "manutencao_preventiva":
		return "preventive_maintenance"
	case "corrective_maintenance", "manutencao corretiva", "corretiva", "manutencao_corretiva":
		return "corrective_maintenance"
	case "wash", "lavagem", "lava jato", "lavacao":
		return "wash"
	case "tire", "pneu", "pneus":
		return "tire"
	case "fine", "multa", "multas":
		return "fine"
	case "documentation", "documentacao", "documento", "documentos", "licenciamento", "ipva":
		return "documentation"
	default:
		return strings.TrimSpace(raw)
	}
}

func fleetLookupExpenseTypeID(ctx context.Context, db *pgxpool.Pool, raw string) (uuid.UUID, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return uuid.Nil, "", errValidation("tipo de despesa obrigatório")
	}
	if id, err := uuid.Parse(raw); err == nil {
		var desc string
		err = db.QueryRow(ctx, `SELECT description FROM fleet_expense_types WHERE id=$1`, id).Scan(&desc)
		if err == pgx.ErrNoRows {
			return uuid.Nil, "", errValidation("tipo de despesa não encontrado")
		}
		if err != nil {
			return uuid.Nil, "", err
		}
		return id, desc, nil
	}
	alias := fleetExpenseTypeAlias(raw)
	var id uuid.UUID
	var desc string
	err := db.QueryRow(ctx, `
		SELECT id, description FROM fleet_expense_types
		WHERE lower(trim(description)) = lower(trim($1))
		   OR lower(trim(COALESCE(code,''))) = lower(trim($1))
		   OR lower(trim(COALESCE(code,''))) = lower(trim($2))
		LIMIT 1
	`, raw, alias).Scan(&id, &desc)
	if err == pgx.ErrNoRows {
		return uuid.Nil, "", errValidation("tipo de despesa não encontrado: " + raw)
	}
	if err != nil {
		return uuid.Nil, "", err
	}
	return id, desc, nil
}

type fleetExpenseItem struct {
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	TotalAmount float64 `json:"total_amount"`
}

type fleetExpense struct {
	ID            uuid.UUID          `json:"id"`
	Number        int64              `json:"number"`
	OccurredAt    time.Time          `json:"occurred_at"`
	VehicleID     uuid.UUID          `json:"vehicle_id"`
	Plate         string             `json:"plate,omitempty"`
	VehicleDesc   string             `json:"vehicle_description,omitempty"`
	ExpenseTypeID uuid.UUID          `json:"expense_type_id"`
	ExpenseType   string             `json:"expense_type"`
	TypeLabel     string             `json:"type_label,omitempty"`
	Description   string             `json:"description"`
	UnitPrice     float64            `json:"unit_price"`
	Quantity      float64            `json:"quantity"`
	TotalAmount   float64            `json:"total_amount"`
	Odometer      *float64           `json:"odometer"`
	Notes         *string            `json:"notes"`
	Items         []fleetExpenseItem `json:"items,omitempty"`
}

type fleetExpenseBody struct {
	OccurredAt     string             `json:"occurred_at"`
	VehicleID      uuid.UUID          `json:"vehicle_id"`
	ExpenseTypeID  *uuid.UUID         `json:"expense_type_id"`
	ExpenseType    string             `json:"expense_type"`
	Description    string             `json:"description"`
	UnitPrice      float64            `json:"unit_price"`
	Quantity       float64            `json:"quantity"`
	Odometer       *float64           `json:"odometer"`
	UpdateOdometer *bool              `json:"update_odometer"`
	Notes          *string            `json:"notes"`
	Items          []fleetExpenseItem `json:"items"`
}

func (s *Server) listFleetExpenses(w http.ResponseWriter, r *http.Request) {
	q := fleetQ(r)
	limit, offset := fleetLimitOffset(r)
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	vehicleID := strings.TrimSpace(r.URL.Query().Get("vehicle_id"))
	typeID := strings.TrimSpace(r.URL.Query().Get("expense_type_id"))
	rows, err := s.DB().Query(r.Context(), `
		SELECT e.id, e.number, e.occurred_at, e.vehicle_id, COALESCE(v.plate, ''), v.description,
			e.expense_type_id, COALESCE(t.code, t.description), t.description,
			e.description, e.unit_price, e.quantity, e.total_amount, e.odometer, e.notes
		FROM fleet_expenses e
		JOIN fleet_vehicles v ON v.id = e.vehicle_id
		JOIN fleet_expense_types t ON t.id = e.expense_type_id
		WHERE ($1 = '' OR v.plate ILIKE '%'||$1||'%' OR v.description ILIKE '%'||$1||'%' OR e.description ILIKE '%'||$1||'%')
		  AND ($2 = '' OR e.vehicle_id::text = $2)
		  AND ($3 = '' OR e.occurred_at >= $3::timestamptz)
		  AND ($4 = '' OR e.occurred_at < ($4::date + interval '1 day'))
		  AND ($5 = '' OR e.expense_type_id::text = $5)
		ORDER BY e.occurred_at DESC
		LIMIT $6 OFFSET $7
	`, q, vehicleID, from, to, typeID, limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	list := []fleetExpense{}
	for rows.Next() {
		var it fleetExpense
		if err := rows.Scan(&it.ID, &it.Number, &it.OccurredAt, &it.VehicleID, &it.Plate, &it.VehicleDesc,
			&it.ExpenseTypeID, &it.ExpenseType, &it.TypeLabel, &it.Description, &it.UnitPrice, &it.Quantity,
			&it.TotalAmount, &it.Odometer, &it.Notes); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		it.Plate = fleetDisplayPlate(it.Plate)
		list = append(list, it)
	}
	s.attachFleetExpenseItems(r.Context(), list)
	writeJSON(w, http.StatusOK, map[string]any{"items": list})
}

func (s *Server) attachFleetExpenseItems(ctx context.Context, list []fleetExpense) {
	if len(list) == 0 {
		return
	}
	ids := make([]uuid.UUID, len(list))
	idx := map[uuid.UUID]int{}
	for i, it := range list {
		ids[i] = it.ID
		idx[it.ID] = i
		list[i].Items = []fleetExpenseItem{}
	}
	rows, err := s.DB().Query(ctx, `
		SELECT expense_id, description, quantity, unit_price, total_amount
		FROM fleet_expense_items WHERE expense_id = ANY($1)
		ORDER BY sort_order, id
	`, ids)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var eid uuid.UUID
		var it fleetExpenseItem
		if err := rows.Scan(&eid, &it.Description, &it.Quantity, &it.UnitPrice, &it.TotalAmount); err != nil {
			continue
		}
		if i, ok := idx[eid]; ok {
			list[i].Items = append(list[i].Items, it)
		}
	}
}

func (s *Server) createFleetExpense(w http.ResponseWriter, r *http.Request) {
	var body fleetExpenseBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	rawType := body.ExpenseType
	if body.ExpenseTypeID != nil {
		rawType = body.ExpenseTypeID.String()
	}
	typeID, typeLabel, err := fleetLookupExpenseTypeID(r.Context(), s.DB(), rawType)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	}
	items, headerDesc, unit, qty, err := normalizeFleetExpenseItems(body.Items, strings.TrimSpace(body.Description), body.UnitPrice, body.Quantity)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	}
	if body.VehicleID == uuid.Nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "vehicle_id, description, unit_price e quantity obrigatórios", nil)
		return
	}
	var vehStatus string
	if err := s.DB().QueryRow(r.Context(), `SELECT status FROM fleet_vehicles WHERE id=$1`, body.VehicleID).Scan(&vehStatus); err == pgx.ErrNoRows {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "veículo não encontrado", nil)
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if fleetStatusBlocksLaunch(vehStatus) {
		writeErr(w, http.StatusBadRequest, "VALIDATION", fleetLaunchBlockedMsg("despesa"), nil)
		return
	}
	updateOdo := true
	if body.UpdateOdometer != nil {
		updateOdo = *body.UpdateOdometer
	}
	desc := headerDesc
	occurred := time.Now()
	if strings.TrimSpace(body.OccurredAt) != "" {
		t, err := parseTimeFlexible(body.OccurredAt)
		if err != nil || t.IsZero() {
			if d, derr := fleetParseDate(body.OccurredAt); derr == nil && d != nil {
				t = *d
			} else {
				writeErr(w, http.StatusBadRequest, "VALIDATION", "occurred_at inválido", nil)
				return
			}
		}
		occurred = t
	}
	id, number, total, err := s.insertFleetExpense(r.Context(), fleetExpenseInsert{
		OccurredAt:    occurred,
		VehicleID:     body.VehicleID,
		ExpenseTypeID: typeID,
		Description:   desc,
		UnitPrice:     unit,
		Quantity:      qty,
		Odometer:      body.Odometer,
		Notes:         ptrTrim(body.Notes),
		UserID:        s.userIDFromRequest(r),
		UpdateOdo:     updateOdo,
		Items:         items,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			writeErr(w, http.StatusBadRequest, "VALIDATION", "veículo não encontrado", nil)
			return
		}
		if _, ok := err.(validationErr); ok {
			writeErr(w, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
			return
		}
		writeErr(w, http.StatusBadRequest, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "number": number, "total_amount": total, "type_label": typeLabel})
}

type fleetExpenseInsert struct {
	OccurredAt    time.Time
	VehicleID     uuid.UUID
	ExpenseTypeID uuid.UUID
	Description   string
	UnitPrice     float64
	Quantity      float64
	Odometer      *float64
	Notes         *string
	UserID        *uuid.UUID
	UpdateOdo     bool
	Items         []fleetExpenseItem
}

func normalizeFleetExpenseItems(raw []fleetExpenseItem, headerDesc string, unit, qty float64) ([]fleetExpenseItem, string, float64, float64, error) {
	items := make([]fleetExpenseItem, 0, len(raw))
	for _, it := range raw {
		desc := strings.TrimSpace(it.Description)
		if desc == "" && it.Quantity == 0 && it.UnitPrice == 0 {
			continue
		}
		if desc == "" || it.Quantity <= 0 || it.UnitPrice < 0 {
			return nil, "", 0, 0, errValidation("cada item precisa de descrição, quantidade e valor unitário")
		}
		items = append(items, fleetExpenseItem{
			Description: desc,
			Quantity:    it.Quantity,
			UnitPrice:   it.UnitPrice,
			TotalAmount: it.Quantity * it.UnitPrice,
		})
	}
	if len(items) == 0 {
		if headerDesc == "" || qty <= 0 || unit < 0 {
			return nil, "", 0, 0, errValidation("vehicle_id, description, unit_price e quantity obrigatórios")
		}
		items = []fleetExpenseItem{{Description: headerDesc, Quantity: qty, UnitPrice: unit, TotalAmount: unit * qty}}
	}
	var totalQty, totalAmt float64
	names := make([]string, 0, len(items))
	for _, it := range items {
		totalQty += it.Quantity
		totalAmt += it.TotalAmount
		names = append(names, it.Description)
	}
	desc := headerDesc
	if desc == "" {
		desc = strings.Join(names, ", ")
	}
	avgUnit := 0.0
	if totalQty > 0 {
		avgUnit = totalAmt / totalQty
	}
	return items, desc, avgUnit, totalQty, nil
}

// patchFleetExpense edita um lançamento de despesa já gravado (aba Frota → Despesas). Ao
// contrário de createFleetExpense, NÃO mexe no hodômetro actual do veículo nem em
// fleet_odometer_readings — editar um registo passado é uma correcção pontual, não deve
// arrastar efeitos colaterais no estado actual do veículo (quem quiser actualizar o hodômetro
// faz um novo lançamento). Substitui sempre os itens (apaga e recria) — mais simples e evita
// divergência entre o cabeçalho (total_amount) e os itens.
func (s *Server) patchFleetExpense(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	var body fleetExpenseBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	rawType := body.ExpenseType
	if body.ExpenseTypeID != nil {
		rawType = body.ExpenseTypeID.String()
	}
	typeID, _, err := fleetLookupExpenseTypeID(r.Context(), s.DB(), rawType)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	}
	items, headerDesc, unit, qty, err := normalizeFleetExpenseItems(body.Items, strings.TrimSpace(body.Description), body.UnitPrice, body.Quantity)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	}
	if body.VehicleID == uuid.Nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "vehicle_id, description, unit_price e quantity obrigatórios", nil)
		return
	}
	var vehStatus string
	if err := s.DB().QueryRow(r.Context(), `SELECT status FROM fleet_vehicles WHERE id=$1`, body.VehicleID).Scan(&vehStatus); err == pgx.ErrNoRows {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "veículo não encontrado", nil)
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	occurred := time.Now()
	if strings.TrimSpace(body.OccurredAt) != "" {
		t, terr := parseTimeFlexible(body.OccurredAt)
		if terr != nil || t.IsZero() {
			if d, derr := fleetParseDate(body.OccurredAt); derr == nil && d != nil {
				t = *d
			} else {
				writeErr(w, http.StatusBadRequest, "VALIDATION", "occurred_at inválido", nil)
				return
			}
		}
		occurred = t
	}
	var total float64
	for _, it := range items {
		total += it.TotalAmount
	}
	userID := s.userIDFromRequest(r)
	tx, err := s.DB().Begin(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer tx.Rollback(r.Context())
	ct, err := tx.Exec(r.Context(), `
		UPDATE fleet_expenses SET
			occurred_at=$1, vehicle_id=$2, expense_type_id=$3, description=$4, unit_price=$5, quantity=$6,
			total_amount=$7, odometer=$8, notes=$9, updated_by=$10, updated_at=now()
		WHERE id=$11
	`, occurred, body.VehicleID, typeID, headerDesc, unit, qty, total, body.Odometer, ptrTrim(body.Notes), userID, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "despesa não encontrada", nil)
		return
	}
	if _, err := tx.Exec(r.Context(), `DELETE FROM fleet_expense_items WHERE expense_id=$1`, id); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	for i, it := range items {
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO fleet_expense_items (expense_id, description, quantity, unit_price, total_amount, sort_order)
			VALUES ($1,$2,$3,$4,$5,$6)
		`, id, it.Description, it.Quantity, it.UnitPrice, it.TotalAmount, i); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	s.appendAuditLog(r.Context(), "fleet_expense", id.String(), "update", s.actorFromRequest(r), nil, map[string]any{"total_amount": total})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "total_amount": total})
}

// deleteFleetExpense elimina UM lançamento de despesa (fleet_expense_items é apagado em cascata —
// ver 101_fleet_expense_items.sql). Não mexe no hodômetro do veículo.
func (s *Server) deleteFleetExpense(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "id inválido", nil)
		return
	}
	ct, err := s.DB().Exec(r.Context(), `DELETE FROM fleet_expenses WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "despesa não encontrada", nil)
		return
	}
	s.appendAuditLog(r.Context(), "fleet_expense", id.String(), "delete", s.actorFromRequest(r), nil, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) insertFleetExpense(ctx context.Context, in fleetExpenseInsert) (uuid.UUID, int64, float64, error) {
	items := in.Items
	if len(items) == 0 {
		items = []fleetExpenseItem{{
			Description: in.Description,
			Quantity:    in.Quantity,
			UnitPrice:   in.UnitPrice,
			TotalAmount: in.UnitPrice * in.Quantity,
		}}
	}
	var totalQty, total float64
	for i := range items {
		items[i].TotalAmount = items[i].Quantity * items[i].UnitPrice
		totalQty += items[i].Quantity
		total += items[i].TotalAmount
	}
	if in.Quantity <= 0 {
		in.Quantity = totalQty
	}
	if in.UnitPrice == 0 && totalQty > 0 {
		in.UnitPrice = total / totalQty
	}
	tx, err := s.DB().Begin(ctx)
	if err != nil {
		return uuid.Nil, 0, 0, err
	}
	defer tx.Rollback(ctx)
	var currentOdo float64
	err = tx.QueryRow(ctx, `SELECT odometer_current FROM fleet_vehicles WHERE id=$1`, in.VehicleID).Scan(&currentOdo)
	if err != nil {
		return uuid.Nil, 0, 0, err
	}
	if in.UpdateOdo && in.Odometer != nil && *in.Odometer < currentOdo {
		return uuid.Nil, 0, 0, errValidation("hodômetro não pode ser menor que o atual do veículo")
	}
	var id uuid.UUID
	var number int64
	err = tx.QueryRow(ctx, `
		INSERT INTO fleet_expenses (
			occurred_at, vehicle_id, expense_type_id, description, unit_price, quantity, total_amount,
			odometer, notes, created_by, updated_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10) RETURNING id, number
	`, in.OccurredAt, in.VehicleID, in.ExpenseTypeID, in.Description, in.UnitPrice, in.Quantity, total,
		in.Odometer, in.Notes, in.UserID).Scan(&id, &number)
	if err != nil {
		return uuid.Nil, 0, 0, err
	}
	for i, it := range items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO fleet_expense_items (expense_id, description, quantity, unit_price, total_amount, sort_order)
			VALUES ($1,$2,$3,$4,$5,$6)
		`, id, it.Description, it.Quantity, it.UnitPrice, it.TotalAmount, i); err != nil {
			return uuid.Nil, 0, 0, err
		}
	}
	if in.UpdateOdo && in.Odometer != nil {
		if _, err := tx.Exec(ctx, `
			UPDATE fleet_vehicles SET odometer_current=$2, updated_at=now(), updated_by=$3 WHERE id=$1
		`, in.VehicleID, *in.Odometer, in.UserID); err != nil {
			return uuid.Nil, 0, 0, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO fleet_odometer_readings (vehicle_id, reading_at, odometer, source, notes, created_by)
			VALUES ($1,$2,$3,'expense',$4,$5)
		`, in.VehicleID, in.OccurredAt, *in.Odometer, in.Notes, in.UserID); err != nil {
			return uuid.Nil, 0, 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, 0, 0, err
	}
	return id, number, total, nil
}

func fleetLookupVehicleID(ctx context.Context, db *pgxpool.Pool, plate string) (uuid.UUID, error) {
	plate = strings.TrimSpace(plate)
	if plate == "" {
		return uuid.Nil, errValidation("placa obrigatória")
	}
	var id uuid.UUID
	err := db.QueryRow(ctx, `
		SELECT id FROM fleet_vehicles
		WHERE upper(replace(replace(trim(plate), '-', ''), ' ', '')) = upper(replace(replace(trim($1), '-', ''), ' ', ''))
		LIMIT 1
	`, plate).Scan(&id)
	if err == pgx.ErrNoRows {
		return uuid.Nil, errValidation("veículo não encontrado: " + plate)
	}
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func fleetMapLaunchKind(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.NewReplacer("ç", "c", "ã", "a", "á", "a", "í", "i", "é", "e", "ê", "e", "ó", "o", "ô", "o", "ú", "u").Replace(s)
	switch s {
	case "abastecimento", "abastecimentos", "fueling", "fuel", "combustivel":
		return "fueling"
	default:
		return "expense"
	}
}

func (s *Server) exportFleetExpensesCSV(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=backup_despesas_%s.csv", time.Now().Format("20060102")))
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	defer cw.Flush()
	_ = cw.Write([]string{"lancamento", "descricao", "placa", "data", "tipo_despesa", "valor_unitario", "quantidade", "valor", "km", "observacao"})

	fmtFloat := func(v float64) string {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	fmtKM := func(v *float64) string {
		if v == nil {
			return ""
		}
		return fmtFloat(*v)
	}

	erows, err := s.DB().Query(r.Context(), `
		SELECT t.description, COALESCE(v.plate, ''), e.occurred_at, e.description, e.unit_price, e.quantity, e.total_amount, e.odometer, COALESCE(e.notes,'')
		FROM fleet_expenses e
		JOIN fleet_vehicles v ON v.id = e.vehicle_id
		JOIN fleet_expense_types t ON t.id = e.expense_type_id
		ORDER BY e.occurred_at
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer erows.Close()
	for erows.Next() {
		var typ, plate, desc, notes string
		var at time.Time
		var unit, qty, total float64
		var km *float64
		if err := erows.Scan(&typ, &plate, &at, &desc, &unit, &qty, &total, &km, &notes); err != nil {
			continue
		}
		_ = cw.Write([]string{"despesa", desc, fleetDisplayPlateOrUnknown(plate), at.Format("02/01/2006"), typ, fmtFloat(unit), fmtFloat(qty), fmtFloat(total), fmtKM(km), notes})
	}

	frows, err := s.DB().Query(r.Context(), `
		SELECT fu.description, COALESCE(v.plate, ''), f.fueled_at, f.liters, f.price_per_liter, f.total_amount, f.odometer_current, COALESCE(f.notes,'')
		FROM fleet_fuelings f
		JOIN fleet_vehicles v ON v.id = f.vehicle_id
		JOIN fleet_fuels fu ON fu.id = f.fuel_id
		ORDER BY f.fueled_at
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer frows.Close()
	for frows.Next() {
		var fuel, plate, notes string
		var at time.Time
		var liters, price, total float64
		var km *float64
		if err := frows.Scan(&fuel, &plate, &at, &liters, &price, &total, &km, &notes); err != nil {
			continue
		}
		_ = cw.Write([]string{"abastecimento", fuel, fleetDisplayPlateOrUnknown(plate), at.Format("02/01/2006"), fuel, fmtFloat(price), fmtFloat(liters), fmtFloat(total), fmtKM(km), notes})
	}
}

func (s *Server) purgeFleetExpenses(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm string `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(strings.ToUpper(body.Confirm)) != "ZERAR" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "para apagar todas as despesas envie confirm=ZERAR", nil)
		return
	}
	tx, err := s.DB().Begin(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err := tx.Exec(r.Context(), `DELETE FROM fleet_odometer_readings WHERE source IN ('expense','fueling')`); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if _, err := tx.Exec(r.Context(), `DELETE FROM fleet_alerts`); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	var nExp, nFuel int64
	tag, err := tx.Exec(r.Context(), `DELETE FROM fleet_expenses`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	nExp = tag.RowsAffected()
	tag, err = tx.Exec(r.Context(), `DELETE FROM fleet_fuelings`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	nFuel = tag.RowsAffected()
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted_expenses": nExp, "deleted_fuelings": nFuel})
}
