package api

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/integrationconsumer"
	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

const clientConnSelectCols = `id, display_number, client_name, address, neighborhood, login, password, ip_address,
	connection_kind, medium_type, sales_plan, onu_mac_sn, rx_dbm, tx_dbm, transmitter, cto, port,
	latitude, longitude, created_at, updated_at`

func scanClientConnection(rows interface {
	Scan(dest ...any) error
}) (map[string]any, error) {
	var id uuid.UUID
	var displayNumber int
	var clientName, login, connKind string
	var address, neighborhood, password, ipAddr, medium, salesPlan, onuMac, rx, tx, transmitter, cto, port *string
	var lat, lon *float64
	var created, updated time.Time
	err := rows.Scan(&id, &displayNumber, &clientName, &address, &neighborhood, &login, &password, &ipAddr, &connKind, &medium, &salesPlan,
		&onuMac, &rx, &tx, &transmitter, &cto, &port, &lat, &lon, &created, &updated)
	if err != nil {
		return nil, err
	}
	m := map[string]any{
		"id": id, "display_number": displayNumber, "client_name": clientName, "login": login, "connection_kind": connKind,
		"created_at": created, "updated_at": updated,
	}
	setOptionalStr(m, "address", address)
	setOptionalStr(m, "neighborhood", neighborhood)
	setOptionalStr(m, "password", password)
	setOptionalStr(m, "ip_address", ipAddr)
	setOptionalStr(m, "medium_type", medium)
	setOptionalStr(m, "sales_plan", salesPlan)
	setOptionalStr(m, "onu_mac_sn", onuMac)
	setOptionalStr(m, "rx_dbm", rx)
	setOptionalStr(m, "tx_dbm", tx)
	setOptionalStr(m, "transmitter", transmitter)
	setOptionalStr(m, "cto", cto)
	setOptionalStr(m, "port", port)
	if lat != nil {
		m["latitude"] = *lat
	}
	if lon != nil {
		m["longitude"] = *lon
	}
	return m, nil
}

func setOptionalStr(m map[string]any, key string, v *string) {
	if v != nil {
		m[key] = *v
	}
}

func normalizeConnectionKind(v string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "pppoe", "ppoe", "ppp":
		return "pppoe", true
	case "dhcp":
		return "dhcp", true
	default:
		return "", false
	}
}

func normalizeMediumType(v string) (*string, bool) {
	s := strings.ToLower(strings.TrimSpace(v))
	if s == "" {
		return nil, true
	}
	switch s {
	case "fibra", "fiber", "ftth":
		s := "fibra"
		return &s, true
	case "radio", "rádio", "wireless":
		s := "radio"
		return &s, true
	case "cabo_utp", "utp", "cabo", "cabo utp":
		s := "cabo_utp"
		return &s, true
	default:
		return nil, false
	}
}

type clientConnInput struct {
	ClientName     string   `json:"client_name"`
	Address        *string  `json:"address"`
	Neighborhood   *string  `json:"neighborhood"`
	Login          string   `json:"login"`
	Password       *string  `json:"password"`
	IPAddress      *string  `json:"ip_address"`
	ConnectionKind string   `json:"connection_kind"`
	MediumType     *string  `json:"medium_type"`
	SalesPlan      *string  `json:"sales_plan"`
	OnuMacSN       *string  `json:"onu_mac_sn"`
	RxDbm          *string  `json:"rx_dbm"`
	TxDbm          *string  `json:"tx_dbm"`
	Transmitter    *string  `json:"transmitter"`
	CTO            *string  `json:"cto"`
	Port           *string  `json:"port"`
	Latitude       *float64 `json:"latitude"`
	Longitude      *float64 `json:"longitude"`
}

func (in *clientConnInput) validate() error {
	if strings.TrimSpace(in.ClientName) == "" {
		return errors.New("client_name obrigatório")
	}
	if strings.TrimSpace(in.Login) == "" {
		return errors.New("login obrigatório")
	}
	kind, ok := normalizeConnectionKind(in.ConnectionKind)
	if !ok {
		return errors.New("connection_kind inválido (pppoe ou dhcp)")
	}
	in.ConnectionKind = kind
	if in.MediumType != nil && strings.TrimSpace(*in.MediumType) != "" {
		mt, ok := normalizeMediumType(*in.MediumType)
		if !ok {
			return errors.New("medium_type inválido (fibra, radio, cabo_utp)")
		}
		in.MediumType = mt
	} else {
		in.MediumType = nil
	}
	if (in.Latitude == nil) != (in.Longitude == nil) {
		return errors.New("latitude e longitude devem ser preenchidas juntas")
	}
	if in.Latitude != nil {
		if *in.Latitude < -90 || *in.Latitude > 90 || *in.Longitude < -180 || *in.Longitude > 180 {
			return errors.New("coordenadas fora do intervalo")
		}
	}
	return nil
}

type connConflictInfo struct {
	ID            uuid.UUID `json:"id"`
	DisplayNumber int       `json:"display_number"`
	ClientName    string    `json:"client_name"`
	Login         string    `json:"login"`
	IPAddress     *string   `json:"ip_address,omitempty"`
}

func normalizeDuplicatePolicy(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "replace", "ignore", "reject":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return "reject"
	}
}

func connIPNorm(ip *string) string {
	if ip == nil {
		return ""
	}
	return strings.TrimSpace(*ip)
}

func (s *Server) findConnByLogin(ctx context.Context, login string, excludeID *uuid.UUID) (*connConflictInfo, error) {
	login = strings.TrimSpace(login)
	if login == "" {
		return nil, nil
	}
	q := `SELECT id, display_number, client_name, login, ip_address FROM client_connections WHERE lower(trim(login))=lower(trim($1))`
	args := []any{login}
	if excludeID != nil {
		q += ` AND id <> $2`
		args = append(args, *excludeID)
	}
	q += ` LIMIT 1`
	var info connConflictInfo
	var ip *string
	err := s.DB().QueryRow(ctx, q, args...).Scan(&info.ID, &info.DisplayNumber, &info.ClientName, &info.Login, &ip)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	info.IPAddress = ip
	return &info, nil
}

func (s *Server) findConnByIP(ctx context.Context, ip string, excludeID *uuid.UUID) (*connConflictInfo, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return nil, nil
	}
	q := `SELECT id, display_number, client_name, login, ip_address FROM client_connections WHERE trim(ip_address)=trim($1)`
	args := []any{ip}
	if excludeID != nil {
		q += ` AND id <> $2`
		args = append(args, *excludeID)
	}
	q += ` LIMIT 1`
	var info connConflictInfo
	var ipVal *string
	err := s.DB().QueryRow(ctx, q, args...).Scan(&info.ID, &info.DisplayNumber, &info.ClientName, &info.Login, &ipVal)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	info.IPAddress = ipVal
	return &info, nil
}

func (s *Server) connDuplicateConflicts(ctx context.Context, body clientConnInput, excludeID *uuid.UUID) (loginC, ipC *connConflictInfo, err error) {
	loginC, err = s.findConnByLogin(ctx, body.Login, excludeID)
	if err != nil {
		return nil, nil, err
	}
	ip := connIPNorm(body.IPAddress)
	if ip != "" {
		ipC, err = s.findConnByIP(ctx, ip, excludeID)
		if err != nil {
			return nil, nil, err
		}
	}
	if loginC != nil && ipC != nil && loginC.ID != ipC.ID {
		return loginC, ipC, errors.New("login e IPv4 pertencem a conexões diferentes")
	}
	return loginC, ipC, nil
}

func (s *Server) upsertClientConnection(ctx context.Context, body clientConnInput, policy string, excludeID *uuid.UUID) (id uuid.UUID, skipped bool, err error) {
	policy = normalizeDuplicatePolicy(policy)
	loginC, ipC, err := s.connDuplicateConflicts(ctx, body, excludeID)
	if err != nil {
		return uuid.Nil, false, err
	}
	hasDup := loginC != nil || ipC != nil
	if !hasDup {
		err = s.DB().QueryRow(ctx, `
			INSERT INTO client_connections (
				client_name, address, neighborhood, login, password, ip_address,
				connection_kind, medium_type, sales_plan, onu_mac_sn, rx_dbm, tx_dbm,
				transmitter, cto, port, latitude, longitude
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
			RETURNING id`,
			body.ClientName, body.Address, body.Neighborhood, body.Login, body.Password, body.IPAddress,
			body.ConnectionKind, body.MediumType, body.SalesPlan, body.OnuMacSN, body.RxDbm, body.TxDbm,
			body.Transmitter, body.CTO, body.Port, body.Latitude, body.Longitude,
		).Scan(&id)
		return id, false, err
	}
	if policy == "ignore" {
		return uuid.Nil, true, nil
	}
	if policy == "reject" {
		details := map[string]any{}
		if loginC != nil {
			details["login_conflict"] = loginC
		}
		if ipC != nil {
			details["ip_conflict"] = ipC
		}
		return uuid.Nil, false, &duplicateConnError{details: details}
	}
	target := loginC
	if target == nil {
		target = ipC
	}
	_, err = s.DB().Exec(ctx, `
		UPDATE client_connections SET
			client_name=$2, address=$3, neighborhood=$4, login=$5, password=$6, ip_address=$7,
			connection_kind=$8, medium_type=$9, sales_plan=$10, onu_mac_sn=$11, rx_dbm=$12, tx_dbm=$13,
			transmitter=$14, cto=$15, port=$16, latitude=$17, longitude=$18, updated_at=now()
		WHERE id=$1`,
		target.ID, body.ClientName, body.Address, body.Neighborhood, body.Login, body.Password, body.IPAddress,
		body.ConnectionKind, body.MediumType, body.SalesPlan, body.OnuMacSN, body.RxDbm, body.TxDbm,
		body.Transmitter, body.CTO, body.Port, body.Latitude, body.Longitude,
	)
	return target.ID, false, err
}

type duplicateConnError struct {
	details map[string]any
}

func (e *duplicateConnError) Error() string {
	return "conexão duplicada"
}

func (s *Server) checkClientConnectionDuplicates(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Login     string  `json:"login"`
		IPAddress *string `json:"ip_address"`
		ExcludeID *string `json:"exclude_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	var excludeID *uuid.UUID
	if body.ExcludeID != nil && strings.TrimSpace(*body.ExcludeID) != "" {
		id, err := uuid.Parse(strings.TrimSpace(*body.ExcludeID))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "BAD_ID", "exclude_id inválido", nil)
			return
		}
		excludeID = &id
	}
	in := clientConnInput{Login: body.Login, IPAddress: body.IPAddress}
	loginC, ipC, err := s.connDuplicateConflicts(r.Context(), in, excludeID)
	if err != nil {
		writeErr(w, http.StatusConflict, "CONFLICT", err.Error(), map[string]any{
			"login_conflict": loginC,
			"ip_conflict":    ipC,
		})
		return
	}
	out := map[string]any{"has_duplicate": loginC != nil || ipC != nil}
	if loginC != nil {
		out["login_conflict"] = loginC
	}
	if ipC != nil {
		out["ip_conflict"] = ipC
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listClientConnections(w http.ResponseWriter, r *http.Request) {
	q := `SELECT ` + clientConnSelectCols + ` FROM client_connections WHERE 1=1`
	args := []any{}
	n := 1
	if v := strings.TrimSpace(r.URL.Query().Get("connection_kind")); v != "" {
		if k, ok := normalizeConnectionKind(v); ok {
			q += ` AND connection_kind = $` + strconv.Itoa(n)
			args = append(args, k)
			n++
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("q")); v != "" {
		pat := "%" + v + "%"
		q += ` AND (client_name ILIKE $` + strconv.Itoa(n) + ` OR login ILIKE $` + strconv.Itoa(n) + ` OR COALESCE(address,'') ILIKE $` + strconv.Itoa(n) + `)`
		args = append(args, pat)
		n++
		if num, err := strconv.Atoi(v); err == nil && num > 0 {
			q += ` OR display_number = $` + strconv.Itoa(n)
			args = append(args, num)
			n++
		}
	}
	q += ` ORDER BY display_number ASC LIMIT 5000`
	rows, err := s.DB().Query(r.Context(), q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []map[string]any
	for rows.Next() {
		m, err := scanClientConnection(rows)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		list = append(list, m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": list})
}

func (s *Server) createClientConnection(w http.ResponseWriter, r *http.Request) {
	var body struct {
		clientConnInput
		DuplicatePolicy string `json:"duplicate_policy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if body.ConnectionKind == "" {
		body.ConnectionKind = "pppoe"
	}
	if err := body.validate(); err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	}
	policy := normalizeDuplicatePolicy(body.DuplicatePolicy)
	id, skipped, err := s.upsertClientConnection(r.Context(), body.clientConnInput, policy, nil)
	if err != nil {
		var dup *duplicateConnError
		if errors.As(err, &dup) {
			writeErr(w, http.StatusConflict, "DUPLICATE", "login ou IPv4 já cadastrado", dup.details)
			return
		}
		if isUniqueViolation(err) {
			writeErr(w, http.StatusConflict, "DUPLICATE", "login já cadastrado", nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if skipped {
		writeJSON(w, http.StatusOK, map[string]any{"skipped": true, "reason": "duplicate"})
		return
	}
	s.appendAuditLog(r.Context(), "client_connection", id.String(), "create", actorFromRequest(r), nil, body.clientConnInput)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) getClientConnection(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	row := s.DB().QueryRow(r.Context(), `SELECT `+clientConnSelectCols+` FROM client_connections WHERE id=$1`, id)
	m, err := scanClientConnection(row)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) patchClientConnection(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var body map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	// Reutiliza get após patch parcial via COALESCE em campos enviados
	var cur clientConnInput
	err = s.DB().QueryRow(r.Context(), `
		SELECT client_name, address, neighborhood, login, password, ip_address,
			connection_kind, medium_type, sales_plan, onu_mac_sn, rx_dbm, tx_dbm,
			transmitter, cto, port, latitude, longitude
		FROM client_connections WHERE id=$1`, id).Scan(
		&cur.ClientName, &cur.Address, &cur.Neighborhood, &cur.Login, &cur.Password, &cur.IPAddress,
		&cur.ConnectionKind, &cur.MediumType, &cur.SalesPlan, &cur.OnuMacSN, &cur.RxDbm, &cur.TxDbm,
		&cur.Transmitter, &cur.CTO, &cur.Port, &cur.Latitude, &cur.Longitude,
	)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	applyConnPatch(&cur, body)
	if err := cur.validate(); err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	}
	ct, err := s.DB().Exec(r.Context(), `
		UPDATE client_connections SET
			client_name=$2, address=$3, neighborhood=$4, login=$5, password=$6, ip_address=$7,
			connection_kind=$8, medium_type=$9, sales_plan=$10, onu_mac_sn=$11, rx_dbm=$12, tx_dbm=$13,
			transmitter=$14, cto=$15, port=$16, latitude=$17, longitude=$18, updated_at=now()
		WHERE id=$1`,
		id, cur.ClientName, cur.Address, cur.Neighborhood, cur.Login, cur.Password, cur.IPAddress,
		cur.ConnectionKind, cur.MediumType, cur.SalesPlan, cur.OnuMacSN, cur.RxDbm, cur.TxDbm,
		cur.Transmitter, cur.CTO, cur.Port, cur.Latitude, cur.Longitude,
	)
	if err != nil {
		if isUniqueViolation(err) {
			writeErr(w, http.StatusConflict, "DUPLICATE", "login já cadastrado", nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	s.appendAuditLog(r.Context(), "client_connection", id.String(), "patch", actorFromRequest(r), nil, body)
	s.getClientConnection(w, r)
}

func applyConnPatch(cur *clientConnInput, body map[string]json.RawMessage) {
	if v, ok := body["client_name"]; ok {
		_ = json.Unmarshal(v, &cur.ClientName)
	}
	if v, ok := body["address"]; ok {
		var s *string
		_ = json.Unmarshal(v, &s)
		cur.Address = s
	}
	if v, ok := body["neighborhood"]; ok {
		var s *string
		_ = json.Unmarshal(v, &s)
		cur.Neighborhood = s
	}
	if v, ok := body["login"]; ok {
		_ = json.Unmarshal(v, &cur.Login)
	}
	if v, ok := body["password"]; ok {
		var s *string
		_ = json.Unmarshal(v, &s)
		cur.Password = s
	}
	if v, ok := body["ip_address"]; ok {
		var s *string
		_ = json.Unmarshal(v, &s)
		cur.IPAddress = s
	}
	if v, ok := body["connection_kind"]; ok {
		_ = json.Unmarshal(v, &cur.ConnectionKind)
	}
	if v, ok := body["medium_type"]; ok {
		var s *string
		_ = json.Unmarshal(v, &s)
		cur.MediumType = s
	}
	if v, ok := body["sales_plan"]; ok {
		var s *string
		_ = json.Unmarshal(v, &s)
		cur.SalesPlan = s
	}
	if v, ok := body["onu_mac_sn"]; ok {
		var s *string
		_ = json.Unmarshal(v, &s)
		cur.OnuMacSN = s
	}
	if v, ok := body["rx_dbm"]; ok {
		var s *string
		_ = json.Unmarshal(v, &s)
		cur.RxDbm = s
	}
	if v, ok := body["tx_dbm"]; ok {
		var s *string
		_ = json.Unmarshal(v, &s)
		cur.TxDbm = s
	}
	if v, ok := body["transmitter"]; ok {
		var s *string
		_ = json.Unmarshal(v, &s)
		cur.Transmitter = s
	}
	if v, ok := body["cto"]; ok {
		var s *string
		_ = json.Unmarshal(v, &s)
		cur.CTO = s
	}
	if v, ok := body["port"]; ok {
		var s *string
		_ = json.Unmarshal(v, &s)
		cur.Port = s
	}
	if v, ok := body["latitude"]; ok {
		_ = json.Unmarshal(v, &cur.Latitude)
	}
	if v, ok := body["longitude"]; ok {
		_ = json.Unmarshal(v, &cur.Longitude)
	}
}

func (s *Server) deleteClientConnection(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	ct, err := s.DB().Exec(r.Context(), `DELETE FROM client_connections WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	s.appendAuditLog(r.Context(), "client_connection", id.String(), "delete", actorFromRequest(r), nil, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) bulkClientConnections(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Connections     []clientConnInput `json:"connections"`
		DuplicatePolicy string            `json:"duplicate_policy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Connections) == 0 {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "connections obrigatório", nil)
		return
	}
	policy := normalizeDuplicatePolicy(body.DuplicatePolicy)
	if policy == "reject" {
		policy = "replace"
	}
	bulkRows := make([]connImportRow, len(body.Connections))
	for i, c := range body.Connections {
		bulkRows[i] = connImportRow{Line: i + 1, Input: c}
	}
	imported, skipped, failed := s.importClientConnectionRows(r.Context(), bulkRows, policy)
	s.appendAuditLog(r.Context(), "client_connection", "bulk", "import", actorFromRequest(r), nil, map[string]any{
		"imported": imported,
		"skipped":  skipped,
		"failed":   len(failed),
	})
	writeJSON(w, http.StatusOK, map[string]any{"imported": imported, "skipped": skipped, "failed": failed})
}

type connImportRow struct {
	Line  int
	Input clientConnInput
}

func connImportFailRow(line int, login, msg string, extra map[string]any) map[string]any {
	row := map[string]any{"error": msg}
	if line > 0 {
		row["line"] = line
	}
	if strings.TrimSpace(login) != "" {
		row["login"] = login
	}
	for k, v := range extra {
		row[k] = v
	}
	return row
}

func friendlyConnImportDBError(err error) string {
	if err == nil {
		return "erro desconhecido"
	}
	msg := err.Error()
	low := strings.ToLower(msg)
	if strings.Contains(low, "display_number") {
		return "coluna display_number em falta na base — execute a migração 049_client_connections_display_number.sql"
	}
	if strings.Contains(low, "client_connections") && strings.Contains(low, "does not exist") {
		return "tabela client_connections em falta — execute a migração 048_client_connections.sql"
	}
	return msg
}

func (s *Server) importClientConnectionRows(ctx context.Context, rows []connImportRow, policy string) (imported, skipped int, failed []map[string]any) {
	for i, row := range rows {
		c := row.Input
		line := row.Line
		if line <= 0 {
			line = i + 1
		}
		if c.ConnectionKind == "" {
			c.ConnectionKind = "pppoe"
		}
		if err := c.validate(); err != nil {
			failed = append(failed, connImportFailRow(line, c.Login, err.Error(), nil))
			continue
		}
		_, skip, err := s.upsertClientConnection(ctx, c, policy, nil)
		if err != nil {
			var dup *duplicateConnError
			if errors.As(err, &dup) {
				failed = append(failed, connImportFailRow(line, c.Login, err.Error(), dup.details))
			} else {
				failed = append(failed, connImportFailRow(line, c.Login, friendlyConnImportDBError(err), nil))
			}
			continue
		}
		if skip {
			skipped++
			continue
		}
		imported++
	}
	return imported, skipped, failed
}

func (s *Server) importClientConnectionsCSV(w http.ResponseWriter, r *http.Request) {
	policy := normalizeDuplicatePolicy(r.URL.Query().Get("duplicate_policy"))
	if policy == "reject" {
		policy = "replace"
	}
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
	col := connCSVColumnMap(headers)
	if _, ok := col["client_name"]; !ok {
		writeErr(w, http.StatusBadRequest, "CSV_HEADER", "cabeçalho inválido: coluna nome_cliente (ou nome/cliente) obrigatória", map[string]any{
			"headers_found": headers,
			"hint":          "use o modelo CSV (separador ;). Colunas: nome_cliente, login, ip, tipo_conexao, …",
		})
		return
	}
	if _, ok := col["login"]; !ok {
		writeErr(w, http.StatusBadRequest, "CSV_HEADER", "cabeçalho inválido: coluna login obrigatória", map[string]any{
			"headers_found": headers,
		})
		return
	}
	var rows []connImportRow
	var failed []map[string]any
	line := 1
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		line++
		if err != nil {
			failed = append(failed, map[string]any{"line": line, "error": err.Error()})
			break
		}
		if isCSVRowEmpty(rec) {
			continue
		}
		c, perr := parseConnCSVRow(rec, col)
		if perr != nil {
			failed = append(failed, connImportFailRow(line, "", perr.Error(), nil))
			continue
		}
		if c.ConnectionKind == "" {
			c.ConnectionKind = "pppoe"
		}
		if err := c.validate(); err != nil {
			failed = append(failed, connImportFailRow(line, c.Login, err.Error(), nil))
			continue
		}
		rows = append(rows, connImportRow{Line: line, Input: c})
	}
	imported, skipped, importFailed := s.importClientConnectionRows(r.Context(), rows, policy)
	for _, f := range importFailed {
		failed = append(failed, f)
	}
	s.appendAuditLog(r.Context(), "client_connection", "bulk", "import_csv", actorFromRequest(r), nil, map[string]any{
		"imported": imported,
		"skipped":  skipped,
		"failed":   len(failed),
		"policy":   policy,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": len(failed) == 0, "imported": imported, "skipped": skipped, "failed": failed})
}

func connCSVColumnMap(headers []string) map[string]int {
	m := map[string]int{}
	for i, h := range headers {
		key := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(h, " ", "_")))
		key = strings.ReplaceAll(key, "ã", "a")
		key = strings.ReplaceAll(key, "á", "a")
		key = strings.ReplaceAll(key, "é", "e")
		key = strings.ReplaceAll(key, "ó", "o")
		m[key] = i
	}
	aliases := map[string][]string{
		"client_name":     {"client_name", "nome", "nome_cliente", "cliente"},
		"address":         {"address", "endereco", "endereço"},
		"neighborhood":    {"neighborhood", "bairro"},
		"login":           {"login", "pppoe", "usuario", "user"},
		"password":        {"password", "senha"},
		"ip_address":      {"ip_address", "ip"},
		"connection_kind": {"connection_kind", "tipo_conexao", "tipo"},
		"medium_type":     {"medium_type", "meio", "tipo_meio"},
		"sales_plan":      {"sales_plan", "plano", "plano_venda"},
		"onu_mac_sn":      {"onu_mac_sn", "mac", "sn", "mac_sn"},
		"rx_dbm":          {"rx_dbm", "rx"},
		"tx_dbm":          {"tx_dbm", "tx"},
		"transmitter":     {"transmitter", "transmissor"},
		"cto":             {"cto"},
		"port":            {"port", "porta"},
		"latitude":        {"latitude", "lat"},
		"longitude":       {"longitude", "lon", "lng"},
	}
	out := map[string]int{}
	for canon, keys := range aliases {
		for _, k := range keys {
			if idx, ok := m[k]; ok {
				out[canon] = idx
				break
			}
		}
	}
	return out
}

func parseConnCSVRow(rec []string, col map[string]int) (clientConnInput, error) {
	get := func(k string) string {
		i, ok := col[k]
		if !ok || i >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[i])
	}
	c := clientConnInput{
		ClientName:     get("client_name"),
		Login:          get("login"),
		ConnectionKind: get("connection_kind"),
	}
	if v := get("address"); v != "" {
		c.Address = &v
	}
	if v := get("neighborhood"); v != "" {
		c.Neighborhood = &v
	}
	if v := get("password"); v != "" {
		c.Password = &v
	}
	if v := get("ip_address"); v != "" {
		c.IPAddress = &v
	}
	if v := get("medium_type"); v != "" {
		c.MediumType = &v
	}
	if v := get("sales_plan"); v != "" {
		c.SalesPlan = &v
	}
	if v := get("onu_mac_sn"); v != "" {
		c.OnuMacSN = &v
	}
	if v := get("rx_dbm"); v != "" {
		c.RxDbm = &v
	}
	if v := get("tx_dbm"); v != "" {
		c.TxDbm = &v
	}
	if v := get("transmitter"); v != "" {
		c.Transmitter = &v
	}
	if v := get("cto"); v != "" {
		c.CTO = &v
	}
	if v := get("port"); v != "" {
		c.Port = &v
	}
	latS, lonS := get("latitude"), get("longitude")
	if latS != "" || lonS != "" {
		lat, err1 := strconv.ParseFloat(strings.ReplaceAll(latS, ",", "."), 64)
		lon, err2 := strconv.ParseFloat(strings.ReplaceAll(lonS, ",", "."), 64)
		if err1 != nil || err2 != nil {
			return c, errors.New("latitude/longitude inválidas")
		}
		c.Latitude = &lat
		c.Longitude = &lon
	}
	if c.ClientName == "" || c.Login == "" {
		return c, errors.New("client_name e login obrigatórios")
	}
	return c, nil
}

func parseMapBBoxQuery(r *http.Request) (minLat, maxLat, minLng, maxLng float64, hasBBox bool) {
	parse := func(key string) (float64, bool) {
		s := strings.TrimSpace(r.URL.Query().Get(key))
		if s == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
		return f, err == nil
	}
	var okLatMin, okLatMax, okLngMin, okLngMax bool
	minLat, okLatMin = parse("min_lat")
	maxLat, okLatMax = parse("max_lat")
	minLng, okLngMin = parse("min_lng")
	maxLng, okLngMax = parse("max_lng")
	hasBBox = okLatMin && okLatMax && okLngMin && okLngMax && minLat <= maxLat && minLng <= maxLng
	return
}

func mapConnectionLimit(zoom float64, hasBBox bool) int {
	if !hasBBox {
		return 0
	}
	switch {
	case zoom < 9:
		return 120
	case zoom < 11:
		return 350
	case zoom < 13:
		return 800
	case zoom < 15:
		return 1500
	default:
		return 2500
	}
}

func parseMapZoomQuery(r *http.Request) float64 {
	raw := strings.TrimSpace(r.URL.Query().Get("zoom"))
	if raw == "" {
		return 0
	}
	z, err := strconv.ParseFloat(raw, 64)
	if err != nil || z < 0 || z > 22 {
		return 0
	}
	return z
}

func (s *Server) mapConnectionPoints(w http.ResponseWriter, r *http.Request) {
	minLat, maxLat, minLng, maxLng, hasBBox := parseMapBBoxQuery(r)
	zoom := parseMapZoomQuery(r)
	limit := mapConnectionLimit(zoom, hasBBox)

	var total int
	countQ := `SELECT COUNT(*) FROM client_connections WHERE latitude IS NOT NULL AND longitude IS NOT NULL`
	countArgs := []any{}
	if hasBBox {
		countQ += ` AND latitude >= $1 AND latitude <= $2 AND longitude >= $3 AND longitude <= $4`
		countArgs = append(countArgs, minLat, maxLat, minLng, maxLng)
	}
	_ = s.DB().QueryRow(r.Context(), countQ, countArgs...).Scan(&total)

	requiresBBox := !hasBBox && total > 800
	if requiresBBox {
		writeJSON(w, http.StatusOK, map[string]any{
			"points":        []any{},
			"total":         total,
			"truncated":     true,
			"requires_bbox": true,
			"limit":         0,
		})
		return
	}

	if !hasBBox {
		limit = 2500
	} else if limit <= 0 {
		limit = 2500
	}

	q := `
		SELECT id, client_name, login, connection_kind, latitude, longitude, address, neighborhood
		FROM client_connections
		WHERE latitude IS NOT NULL AND longitude IS NOT NULL`
	args := []any{}
	n := 1
	if hasBBox {
		q += fmt.Sprintf(` AND latitude >= $%d AND latitude <= $%d AND longitude >= $%d AND longitude <= $%d`, n, n+1, n+2, n+3)
		args = append(args, minLat, maxLat, minLng, maxLng)
		n += 4
	}
	q += ` ORDER BY client_name`
	q += fmt.Sprintf(` LIMIT $%d`, n)
	args = append(args, limit)

	rows, err := s.DB().Query(r.Context(), q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var pts []map[string]any
	for rows.Next() {
		var id uuid.UUID
		var name, login, kind string
		var lat, lon float64
		var addr, bairro *string
		if err := rows.Scan(&id, &name, &login, &kind, &lat, &lon, &addr, &bairro); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		pt := map[string]any{
			"id": id, "client_name": name, "login": login, "connection_kind": kind,
			"lat": lat, "lng": lon, "point_type": "connection",
		}
		if addr != nil {
			pt["address"] = *addr
		}
		if bairro != nil {
			pt["neighborhood"] = *bairro
		}
		pts = append(pts, pt)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"points":        pts,
		"total":         total,
		"truncated":     total > len(pts),
		"requires_bbox": false,
		"limit":         limit,
	})
}

func (s *Server) lookupConnectionLoginIntegrations(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Login          string   `json:"login"`
		IntegrationIDs []string `json:"integration_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	login := strings.TrimSpace(body.Login)
	if login == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "login é obrigatório", nil)
		return
	}

	filterIDs := map[uuid.UUID]bool{}
	for _, raw := range body.IntegrationIDs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "BAD_ID", "integration_ids inválido", nil)
			return
		}
		filterIDs[id] = true
	}
	useFilter := len(filterIDs) > 0

	rows, err := s.DB().Query(r.Context(), `
		SELECT id, name, enabled, consumer_config FROM integrations ORDER BY name`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var integID uuid.UUID
		var name string
		var enabled bool
		var consumerCfg []byte
		if err := rows.Scan(&integID, &name, &enabled, &consumerCfg); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		if useFilter && !filterIDs[integID] {
			continue
		}
		if !enabled {
			results = append(results, map[string]any{
				"integration_id": integID, "integration_name": name, "ok": false,
				"message": "integração inativa", "clients": []any{},
			})
			continue
		}
		cc := integrationconsumer.ConfigFromJSON(consumerCfg)
		if !cc.ClientSearch.Enabled {
			results = append(results, map[string]any{
				"integration_id": integID, "integration_name": name, "ok": false,
				"message": "consulta de cliente não configurada", "clients": []any{},
			})
			continue
		}
		item := s.integrationLookupLogin(r.Context(), integID, name, login, cc)
		results = append(results, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"login": login, "results": results})
}

func (s *Server) integrationLookupLogin(ctx context.Context, integID uuid.UUID, name, login string, cc integrationconsumer.Config) map[string]any {
	out := map[string]any{
		"integration_id": integID, "integration_name": name, "clients": []integrationconsumer.ClientCard{},
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	reqID, _, err := s.resolveClientSearchRequest(ctx, integID, cc)
	if err != nil || reqID == uuid.Nil {
		out["ok"] = false
		out["message"] = "requisição de consulta não configurada"
		return out
	}

	cfg, err := s.loadIntegrationRunner(ctx, integID)
	if err != nil {
		out["ok"] = false
		out["message"] = err.Error()
		return out
	}
	if err := s.ensureIntegrationSession(ctx, integID, cfg); err != nil {
		out["ok"] = false
		out["message"] = err.Error()
		return out
	}
	cfg, _ = s.loadIntegrationRunner(ctx, integID)

	var rr requestRow
	err = s.DB().QueryRow(ctx, `
		SELECT id, method, path, path_params, query_params, headers, body_template, body_type, extract_json_path, is_login
		FROM integration_requests WHERE id=$1 AND integration_id=$2 AND enabled=true
	`, reqID, integID).Scan(&rr.ID, &rr.Method, &rr.Path, &rr.PathParams, &rr.QueryParams, &rr.Headers,
		&rr.BodyTemplate, &rr.BodyType, &rr.ExtractJSONPath, &rr.IsLogin)
	if err != nil {
		out["ok"] = false
		out["message"] = "requisição de consulta não encontrada"
		return out
	}

	rc := s.requestRowToConfig(rr)
	profile := integrationconsumer.DetectClientSearchProfile(cc.ClientSearch.Provider, rc, cfg.BaseURL)
	busca := "login"
	termo := login

	cfgExec := cfg
	if cfgExec.Variables == nil {
		cfgExec.Variables = map[string]string{}
	} else {
		vars := make(map[string]string, len(cfgExec.Variables)+4)
		for k, v := range cfgExec.Variables {
			vars[k] = v
		}
		cfgExec.Variables = vars
	}
	for k, v := range integrationconsumer.ClientSearchVariables(busca, termo) {
		cfgExec.Variables[k] = v
	}

	execute := func(rc integrationhttp.RequestConfig) integrationhttp.RunResult {
		return integrationhttp.RunWithLoginRequest(ctx, cfgExec, rc, false)
	}

	var res integrationhttp.RunResult
	var parsed integrationconsumer.SearchResult
	if profile == integrationconsumer.ProviderIXC && integrationconsumer.IsLoginBusca(busca) {
		if !cc.ClientLogin.Enabled {
			out["ok"] = false
			out["message"] = "consulta por login requer API de logins (IXC)"
			return out
		}
		loginReqID, _, err := s.resolveClientLoginRequest(ctx, integID, cc)
		if err != nil || loginReqID == uuid.Nil {
			out["ok"] = false
			out["message"] = "requisição de logins não configurada"
			return out
		}
		var lrr requestRow
		if err := s.DB().QueryRow(ctx, `
			SELECT id, method, path, path_params, query_params, headers, body_template, body_type, extract_json_path, is_login
			FROM integration_requests WHERE id=$1 AND integration_id=$2 AND enabled=true
		`, loginReqID, integID).Scan(&lrr.ID, &lrr.Method, &lrr.Path, &lrr.PathParams, &lrr.QueryParams, &lrr.Headers,
			&lrr.BodyTemplate, &lrr.BodyType, &lrr.ExtractJSONPath, &lrr.IsLogin); err != nil {
			out["ok"] = false
			out["message"] = "requisição de logins não encontrada"
			return out
		}
		lrc := s.requestRowToConfig(lrr)
		contractRC := rc
		if contractReqID, _, err := s.resolveClientContractRequest(ctx, integID, cc); err == nil && contractReqID != uuid.Nil {
			var crr requestRow
			if err := s.DB().QueryRow(ctx, `
				SELECT id, method, path, path_params, query_params, headers, body_template, body_type, extract_json_path, is_login
				FROM integration_requests WHERE id=$1 AND integration_id=$2 AND enabled=true
			`, contractReqID, integID).Scan(&crr.ID, &crr.Method, &crr.Path, &crr.PathParams, &crr.QueryParams, &crr.Headers,
				&crr.BodyTemplate, &crr.BodyType, &crr.ExtractJSONPath, &crr.IsLogin); err == nil {
				contractRC = s.requestRowToConfig(crr)
			}
		}
		res, parsed = integrationconsumer.RunIXCClientSearchByLogin(lrc, rc, contractRC, busca, termo, cc.ClientLogin, cc.ClientSearch, execute)
	} else {
		rc = integrationconsumer.ApplyClientSearchContext(rc, profile, busca, termo, false, cc.ClientSearch)
		res = execute(rc)
		rawBody := integrationconsumer.ResponseBodyForParse([]byte(res.ResponsePreview))
		parsed, _ = integrationconsumer.ParseClientSearchBest(rawBody, profile)
	}

	out["ok"] = parsed.OK && res.OK
	out["message"] = parsed.Message
	if out["message"] == "" && !res.OK {
		out["message"] = res.ErrorMessage
	}
	out["clients"] = parsed.Clients
	return out
}
