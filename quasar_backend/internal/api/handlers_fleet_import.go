package api

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func (s *Server) importFleetVehiclesCSV(w http.ResponseWriter, r *http.Request) {
	s.importFleetCSV(w, r, "vehicles")
}

func (s *Server) importFleetDriversCSV(w http.ResponseWriter, r *http.Request) {
	s.importFleetCSV(w, r, "drivers")
}

func (s *Server) importFleetExpensesCSV(w http.ResponseWriter, r *http.Request) {
	s.importFleetCSV(w, r, "expenses")
}

func (s *Server) importFleetCSV(w http.ResponseWriter, r *http.Request, kind string) {
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_FORM", "envie multipart/form-data com campo file", nil)
		return
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_FILE", "arquivo CSV obrigatório no campo file", nil)
		return
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 8<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "READ", err.Error(), nil)
		return
	}
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	firstLine := deviceCSVFirstLine(raw)
	cr := csv.NewReader(bytes.NewReader(raw))
	cr.Comma = deviceCSVDetectComma(firstLine)
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1
	headers, err := cr.Read()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "CSV_HEADER", err.Error(), nil)
		return
	}
	colMap := fleetCSVBuildColumnMap(headers)
	switch kind {
	case "vehicles":
		if !fleetCSVHas(colMap, "description") || !fleetCSVHas(colMap, "plate") {
			writeErr(w, http.StatusBadRequest, "CSV_HEADER", "cabeçalho inválido: colunas descricao e placa obrigatórias", nil)
			return
		}
	case "expenses":
		if !fleetCSVHas(colMap, "plate") {
			writeErr(w, http.StatusBadRequest, "CSV_HEADER", "cabeçalho inválido: coluna placa obrigatória", nil)
			return
		}
	default:
		if !fleetCSVHas(colMap, "name") {
			writeErr(w, http.StatusBadRequest, "CSV_HEADER", "cabeçalho inválido: coluna nome obrigatória", nil)
			return
		}
	}

	uid := s.userIDFromRequest(r)
	line := 1
	imported := 0
	failed := []map[string]any{}
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			failed = append(failed, map[string]any{"line": line + 1, "error": err.Error()})
			break
		}
		line++
		if isCSVRowEmpty(rec) {
			continue
		}
		var ierr error
		switch kind {
		case "vehicles":
			ierr = s.importFleetVehicleRow(r.Context(), rec, colMap, uid)
		case "expenses":
			ierr = s.importFleetExpenseRow(r.Context(), rec, colMap, uid)
		default:
			ierr = s.importFleetDriverRow(r.Context(), rec, colMap, uid)
		}
		if ierr != nil {
			failed = append(failed, map[string]any{"line": line, "error": ierr.Error()})
			continue
		}
		imported++
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       len(failed) == 0,
		"imported": imported,
		"failed":   failed,
	})
}

func fleetCSVHas(colMap map[string]int, key string) bool {
	_, ok := colMap[key]
	return ok
}

func fleetCSVGet(rec []string, colMap map[string]int, key string) string {
	i, ok := colMap[key]
	if !ok || i < 0 || i >= len(rec) {
		return ""
	}
	return strings.TrimSpace(rec[i])
}

func fleetCSVBuildColumnMap(headers []string) map[string]int {
	alias := map[string]string{
		"descricao": "description", "descrição": "description", "description": "description", "nome_veiculo": "description",
		"placa": "plate", "plate": "plate",
		"ano": "year", "year": "year",
		"modelo": "model", "model": "model",
		"cor": "color", "color": "color",
		"cidade": "city", "city": "city",
		"uf": "uf", "estado": "uf",
		"combustivel": "fuel", "combustível": "fuel", "fuel": "fuel", "tipo_combustivel": "fuel",
		"capacidade_tanque_l": "tank", "capacidade_tanque": "tank", "tanque": "tank",
		"consumo_esperado_km_l": "expected_kpl", "consumo_esperado": "expected_kpl",
		"consumo_min_km_l": "min_kpl", "consumo_min": "min_kpl",
		"consumo_max_km_l": "max_kpl", "consumo_max": "max_kpl",
		"hodometro": "odometer", "hodómetro": "odometer", "hodômetro": "odometer", "odometro": "odometer", "km": "odometer",
		"centro_custo": "cost_center", "centrodecusto": "cost_center", "cc": "cost_center",
		"status": "status", "situacao": "status", "situação": "status",
		"observacao": "notes", "observação": "notes", "obs": "notes", "notes": "notes",
		"nome": "name", "name": "name", "motorista": "name",
		"cpf": "cpf",
		"rg": "rg",
		"telefone": "phone", "phone": "phone", "celular": "phone",
		"email": "email", "e-mail": "email",
		"cnh": "license_number", "numero_cnh": "license_number",
		"categoria_cnh": "license_category", "categoria": "license_category",
		"validade_cnh": "license_expires", "validade": "license_expires",
		"usuario": "user", "usuário": "user", "login": "user", "user": "user",
		"valor_unitario": "unit_price", "valor_unit": "unit_price", "preco_unitario": "unit_price", "preço_unitario": "unit_price", "unit_price": "unit_price", "preco": "unit_price", "preço": "unit_price",
		"quantidade": "quantity", "qtd": "quantity", "qty": "quantity", "quantity": "quantity",
		"valor": "total", "total": "total", "valor_total": "total",
		"data": "occurred_at", "data_despesa": "occurred_at", "occurred_at": "occurred_at",
		"tipo_despesa": "expense_type", "tipo": "expense_type", "expense_type": "expense_type",
		"lancamento": "launch_kind", "lançamento": "launch_kind", "natureza": "launch_kind",
		"kind": "launch_kind", "tipo_lancamento": "launch_kind", "tipo_lançamento": "launch_kind",
	}
	out := make(map[string]int)
	for i, h := range headers {
		k := deviceCSVHeaderKey(h)
		k = strings.Map(func(r rune) rune {
			if r == '-' {
				return '_'
			}
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
				return r
			}
			return -1
		}, k)
		if canon, ok := alias[k]; ok {
			if _, exists := out[canon]; !exists {
				out[canon] = i
			}
		}
	}
	return out
}

func fleetParseFloat(s string) (*float64, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	if s == "" {
		return nil, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func fleetParseInt(s string) (*int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func fleetMapVehicleStatus(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "ç", "c")
	switch s {
	case "", "ativo", "active":
		return "active"
	case "inativo", "inactive":
		return "inactive"
	case "manutencao", "em manutencao", "maintenance":
		return "maintenance"
	case "parado", "stopped":
		return "stopped"
	case "vendido", "sold":
		return "sold"
	case "baixado", "written_off", "baixada":
		return "written_off"
	case "locado", "rented":
		return "rented"
	default:
		return "active"
	}
}

func fleetMapDriverStatus(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", "ativo", "active":
		return "active"
	case "inativo", "inactive":
		return "inactive"
	case "bloqueado", "blocked":
		return "blocked"
	default:
		return "active"
	}
}

func fleetParseDate(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	layouts := []string{"2006-01-02", "02/01/2006", "02-01-2006", "2006/01/02"}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return &t, nil
		}
	}
	return nil, errValidation("data inválida (use AAAA-MM-DD ou DD/MM/AAAA)")
}

func fleetFoldFuelKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer("ç", "c", "ã", "a", "á", "a", "â", "a", "é", "e", "ê", "e", "í", "i", "ó", "o", "ô", "o", "ú", "u").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

func fleetLookupFuelID(ctx context.Context, db *pgxpool.Pool, raw string) (*uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	key := fleetFoldFuelKey(raw)
	switch key {
	case "gasolina", "gas", "gasolina comum", "comum":
		key = "gasolina"
	case "gasolina aditivada", "aditivada", "gas ad":
		key = "gasolina aditivada"
	case "etanol", "alcool", "álcool":
		key = "etanol"
	case "diesel", "diesel s10", "s10":
		key = "diesel"
	case "diesel s500", "s500":
		key = "diesel s500"
	case "arla", "arla 32", "arla32":
		key = "arla"
	}
	var id uuid.UUID
	err := db.QueryRow(ctx, `
		SELECT id FROM fleet_fuels
		WHERE lower(trim(description)) = lower(trim($1))
		   OR lower(trim(COALESCE(code,''))) = lower(trim($1))
		   OR lower(trim(COALESCE(fuel_type,''))) = $2
		   OR lower(description) LIKE lower(trim($1)) || '%'
		   OR lower(description) LIKE '%' || lower(trim($1)) || '%'
		ORDER BY
			CASE
				WHEN lower(trim(description)) = lower(trim($1)) THEN 0
				WHEN lower(trim(COALESCE(code,''))) = lower(trim($1)) THEN 1
				WHEN lower(description) LIKE lower(trim($1)) || '%' THEN 2
				WHEN lower(trim(COALESCE(fuel_type,''))) = $2 THEN 3
				ELSE 4
			END,
			CASE WHEN description ILIKE '%comum%' THEN 0 ELSE 1 END,
			length(description)
		LIMIT 1
	`, raw, key).Scan(&id)
	if err == pgx.ErrNoRows {
		return nil, errValidation("combustível não encontrado: " + raw)
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func fleetLookupCostCenterID(ctx context.Context, db *pgxpool.Pool, raw string) (*uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var id uuid.UUID
	err := db.QueryRow(ctx, `
		SELECT id FROM fleet_cost_centers
		WHERE lower(trim(code)) = lower(trim($1))
		   OR lower(trim(description)) = lower(trim($1))
		LIMIT 1
	`, raw).Scan(&id)
	if err == pgx.ErrNoRows {
		return nil, errValidation("centro de custo não encontrado: " + raw)
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func fleetLookupUserID(ctx context.Context, db *pgxpool.Pool, raw string) (*uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var id uuid.UUID
	err := db.QueryRow(ctx, `SELECT id FROM users WHERE lower(login) = lower(trim($1)) LIMIT 1`, raw).Scan(&id)
	if err == pgx.ErrNoRows {
		return nil, errValidation("usuário não encontrado: " + raw)
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (s *Server) importFleetVehicleRow(ctx context.Context, rec []string, colMap map[string]int, uid *uuid.UUID) error {
	desc := fleetCSVGet(rec, colMap, "description")
	plate, err := fleetNormalizePlate(fleetCSVGet(rec, colMap, "plate"))
	if err != nil {
		return err
	}
	if desc == "" {
		return errValidation("descricao e placa obrigatórios")
	}
	year, err := fleetParseInt(fleetCSVGet(rec, colMap, "year"))
	if err != nil {
		return errValidation("ano inválido")
	}
	tank, err := fleetParseFloat(fleetCSVGet(rec, colMap, "tank"))
	if err != nil {
		return errValidation("capacidade do tanque inválida")
	}
	exp, err := fleetParseFloat(fleetCSVGet(rec, colMap, "expected_kpl"))
	if err != nil {
		return errValidation("consumo esperado inválido")
	}
	minK, err := fleetParseFloat(fleetCSVGet(rec, colMap, "min_kpl"))
	if err != nil {
		return errValidation("consumo mínimo inválido")
	}
	maxK, err := fleetParseFloat(fleetCSVGet(rec, colMap, "max_kpl"))
	if err != nil {
		return errValidation("consumo máximo inválido")
	}
	odo, err := fleetParseFloat(fleetCSVGet(rec, colMap, "odometer"))
	if err != nil {
		return errValidation("hodômetro inválido")
	}
	odoVal := 0.0
	if odo != nil {
		odoVal = *odo
	}
	fuelID, err := fleetLookupFuelID(ctx, s.DB(), fleetCSVGet(rec, colMap, "fuel"))
	if err != nil {
		return err
	}
	ccID, err := fleetLookupCostCenterID(ctx, s.DB(), fleetCSVGet(rec, colMap, "cost_center"))
	if err != nil {
		return err
	}
	model := emptyToNil(fleetCSVGet(rec, colMap, "model"))
	color := emptyToNil(fleetCSVGet(rec, colMap, "color"))
	city := emptyToNil(fleetCSVGet(rec, colMap, "city"))
	uf := emptyToNil(strings.ToUpper(fleetCSVGet(rec, colMap, "uf")))
	notes := emptyToNil(fleetCSVGet(rec, colMap, "notes"))
	status := fleetMapVehicleStatus(fleetCSVGet(rec, colMap, "status"))
	_, err = s.DB().Exec(ctx, `
		INSERT INTO fleet_vehicles (
			description, plate, year, model, color, city, uf, primary_fuel_id,
			tank_capacity_liters, expected_km_per_liter, min_km_per_liter, max_km_per_liter,
			odometer_current, cost_center_id, status, notes, created_by, updated_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$17)
	`, desc, plate, year, model, color, city, uf, fuelID, tank, exp, minK, maxK, odoVal, ccID, status, notes, uid)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "fleet_vehicles_plate_uq") {
			return errValidation("placa já cadastrada")
		}
		return err
	}
	return nil
}

func (s *Server) importFleetDriverRow(ctx context.Context, rec []string, colMap map[string]int, uid *uuid.UUID) error {
	name := fleetCSVGet(rec, colMap, "name")
	if name == "" {
		return errValidation("nome obrigatório")
	}
	expires, err := fleetParseDate(fleetCSVGet(rec, colMap, "license_expires"))
	if err != nil {
		return err
	}
	userID, err := fleetLookupUserID(ctx, s.DB(), fleetCSVGet(rec, colMap, "user"))
	if err != nil {
		return err
	}
	cpf := emptyToNil(fleetCSVGet(rec, colMap, "cpf"))
	rg := emptyToNil(fleetCSVGet(rec, colMap, "rg"))
	phone := emptyToNil(fleetCSVGet(rec, colMap, "phone"))
	email := emptyToNil(fleetCSVGet(rec, colMap, "email"))
	cnh := emptyToNil(fleetCSVGet(rec, colMap, "license_number"))
	cat := emptyToNil(fleetCSVGet(rec, colMap, "license_category"))
	city := emptyToNil(fleetCSVGet(rec, colMap, "city"))
	uf := emptyToNil(strings.ToUpper(fleetCSVGet(rec, colMap, "uf")))
	notes := emptyToNil(fleetCSVGet(rec, colMap, "notes"))
	status := fleetMapDriverStatus(fleetCSVGet(rec, colMap, "status"))
	_, err = s.DB().Exec(ctx, `
		INSERT INTO fleet_drivers (
			name, cpf, rg, phone, email, license_number, license_category, license_expires_on,
			city, uf, user_id, status, notes, created_by, updated_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$14)
	`, name, cpf, rg, phone, email, cnh, cat, expires, city, uf, userID, status, notes, uid)
	if err != nil {
		low := strings.ToLower(err.Error())
		if strings.Contains(low, "fleet_drivers_cpf_uq") {
			return errValidation("CPF já cadastrado")
		}
		if strings.Contains(low, "fleet_drivers_user_id") || strings.Contains(low, "user_id") {
			return errValidation("usuário já vinculado a outro motorista")
		}
		return err
	}
	return nil
}

func (s *Server) importFleetExpenseRow(ctx context.Context, rec []string, colMap map[string]int, uid *uuid.UUID) error {
	plate := fleetCSVGet(rec, colMap, "plate")
	vehicleID, err := fleetLookupVehicleID(ctx, s.DB(), plate)
	if err != nil {
		return err
	}
	unit, err := fleetParseFloat(fleetCSVGet(rec, colMap, "unit_price"))
	if err != nil {
		return errValidation("valor unitário inválido")
	}
	qty, err := fleetParseFloat(fleetCSVGet(rec, colMap, "quantity"))
	if err != nil {
		return errValidation("quantidade inválida")
	}
	totalIn, err := fleetParseFloat(fleetCSVGet(rec, colMap, "total"))
	if err != nil {
		return errValidation("valor inválido")
	}
	var unitV, qtyV float64
	if unit != nil && qty != nil && *qty > 0 {
		unitV, qtyV = *unit, *qty
	} else if totalIn != nil && *totalIn >= 0 {
		unitV, qtyV = *totalIn, 1
	} else {
		return errValidation("informe valor_unitario e quantidade")
	}
	occurred := time.Now()
	if raw := fleetCSVGet(rec, colMap, "occurred_at"); raw != "" {
		if d, derr := fleetParseDate(raw); derr == nil && d != nil {
			occurred = *d
		} else if t, terr := parseTimeFlexible(raw); terr == nil && !t.IsZero() {
			occurred = t
		} else {
			return errValidation("data inválida (use AAAA-MM-DD ou DD/MM/AAAA)")
		}
	}
	odo, err := fleetParseFloat(fleetCSVGet(rec, colMap, "odometer"))
	if err != nil {
		return errValidation("KM inválido")
	}
	notes := emptyToNil(fleetCSVGet(rec, colMap, "notes"))
	desc := fleetCSVGet(rec, colMap, "description")
	kind := fleetMapLaunchKind(fleetCSVGet(rec, colMap, "launch_kind"))
	if kind == "fueling" {
		fuelRaw := fleetCSVGet(rec, colMap, "fuel")
		if fuelRaw == "" {
			fuelRaw = fleetCSVGet(rec, colMap, "expense_type")
		}
		if fuelRaw == "" {
			fuelRaw = desc
		}
		fuelID, ferr := fleetLookupFuelID(ctx, s.DB(), fuelRaw)
		if ferr != nil {
			return ferr
		}
		if fuelID == nil {
			return errValidation("combustível obrigatório no abastecimento (tipo_despesa ou descricao)")
		}
		if desc == "" {
			desc = fuelRaw
		}
		return s.insertFleetFuelingImport(ctx, vehicleID, *fuelID, occurred, qtyV, unitV, odo, notes, uid)
	}
	typeID, typeLabel, err := fleetLookupExpenseTypeID(ctx, s.DB(), fleetCSVGet(rec, colMap, "expense_type"))
	if err != nil {
		return err
	}
	if desc == "" {
		desc = typeLabel
	}
	_, _, _, err = s.insertFleetExpense(ctx, fleetExpenseInsert{
		OccurredAt:    occurred,
		VehicleID:     vehicleID,
		ExpenseTypeID: typeID,
		Description:   desc,
		UnitPrice:     unitV,
		Quantity:      qtyV,
		Odometer:      odo,
		Notes:         notes,
		UserID:        uid,
		UpdateOdo:     false,
	})
	return err
}

func (s *Server) insertFleetFuelingImport(ctx context.Context, vehicleID, fuelID uuid.UUID, at time.Time, liters, price float64, odo *float64, notes *string, uid *uuid.UUID) error {
	if liters <= 0 || price < 0 {
		return errValidation("quantidade (litros) e valor unitário (preço/L) obrigatórios no abastecimento")
	}
	var prev float64
	if err := s.DB().QueryRow(ctx, `SELECT odometer_current FROM fleet_vehicles WHERE id=$1`, vehicleID).Scan(&prev); err != nil {
		return err
	}
	var km *float64
	var curr any
	if odo != nil {
		curr = *odo
		if *odo >= prev {
			d := *odo - prev
			km = &d
		}
	}
	total := liters * price
	_, err := s.DB().Exec(ctx, `
		INSERT INTO fleet_fuelings (
			fueled_at, vehicle_id, fuel_id, liters, price_per_liter, total_amount,
			odometer_previous, odometer_current, km_driven, notes, created_by, updated_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$11)
	`, at, vehicleID, fuelID, liters, price, total, prev, curr, km, notes, uid)
	return err
}
