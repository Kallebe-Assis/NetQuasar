package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/monitorworker"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpcatalog"
	"github.com/netquasar/netquasar/quasar_backend/internal/snmpdiscovery"
)

type deviceDTO struct {
	ID                    uuid.UUID       `json:"id"`
	PopID                 *uuid.UUID      `json:"pop_id"`
	LocalityID            *uuid.UUID      `json:"locality_id"`
	Category              string          `json:"category"`
	Description           string          `json:"description"`
	IP                    *string         `json:"ip"`
	NetworkStatus         string          `json:"network_status"`
	AccessMode            *string         `json:"access_mode"`
	TelemetryMode         *string         `json:"telemetry_mode"`
	PingEnabled           bool            `json:"ping_enabled"`
	TelemetryEnabled      bool            `json:"telemetry_enabled"`
	OperationalMode       string          `json:"operational_mode"`
	Latitude              *float64        `json:"latitude"`
	Longitude             *float64        `json:"longitude"`
	Brand                 *string         `json:"brand"`
	Model                 *string         `json:"model"`
	MAC                   *string         `json:"mac"`
	SerialNumber          *string         `json:"serial_number"`
	SoftwareVersion       *string         `json:"software_version"`
	HardwareVersion       *string         `json:"hardware_version"`
	AcquiredAt            *string         `json:"acquired_at"`
	SNMPCommunity         *string         `json:"snmp_community,omitempty"`
	MIBFolderPath         *string         `json:"mib_folder_path,omitempty"`
	TelemetryOIDStrategy  *string         `json:"telemetry_oid_strategy,omitempty"`
	TelemetryOIDOverrides json.RawMessage `json:"telemetry_oid_overrides,omitempty"`
	MaxPons               *int            `json:"max_pons,omitempty"`
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	q := `SELECT id, pop_id, locality_id, category, description, host(ip)::text, network_status, access_mode, telemetry_mode,
		ping_enabled, telemetry_enabled, operational_mode,
		latitude, longitude, brand, model, mac, serial_number, software_version, hardware_version, acquired_at::text, snmp_community, mib_folder_path,
		telemetry_oid_strategy, telemetry_oid_overrides::text, max_pons
		FROM devices ORDER BY description LIMIT 500`
	rows, err := s.DB().Query(r.Context(), q)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()
	var out []deviceDTO
	for rows.Next() {
		var d deviceDTO
		var ip *string
		var overrides []byte
		if err := rows.Scan(&d.ID, &d.PopID, &d.LocalityID, &d.Category, &d.Description, &ip, &d.NetworkStatus, &d.AccessMode, &d.TelemetryMode,
			&d.PingEnabled, &d.TelemetryEnabled, &d.OperationalMode,
			&d.Latitude, &d.Longitude, &d.Brand, &d.Model, &d.MAC, &d.SerialNumber, &d.SoftwareVersion, &d.HardwareVersion, &d.AcquiredAt, &d.SNMPCommunity, &d.MIBFolderPath,
			&d.TelemetryOIDStrategy, &overrides, &d.MaxPons); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		d.IP = ip
		d.TelemetryOIDOverrides = json.RawMessage(overrides)
		out = append(out, d)
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": out})
}

func networkStatusIsBridge(ns string) bool {
	return strings.EqualFold(strings.TrimSpace(ns), "Bridge")
}

func validateDeviceIP(networkStatus string, ip *string) error {
	ns := strings.TrimSpace(networkStatus)
	if ns == "" {
		ns = "Normal"
	}
	if networkStatusIsBridge(ns) {
		return nil
	}
	if ns == "Normal" {
		if ip == nil || strings.TrimSpace(*ip) == "" {
			return errValidation("IP obrigatório quando network_status=Normal")
		}
		if net.ParseIP(strings.TrimSpace(*ip)) == nil {
			return errValidation("IP inválido")
		}
	}
	return nil
}

func applyDeviceBridgeAndLocalityRules(d *deviceDTO) {
	if strings.TrimSpace(d.Category) != "OLT" {
		d.LocalityID = nil
		d.MaxPons = nil
	}
	if strings.TrimSpace(d.Category) != "Outros" {
		def := "default"
		d.TelemetryOIDStrategy = &def
		d.TelemetryOIDOverrides = json.RawMessage(`{}`)
	}
	if networkStatusIsBridge(d.NetworkStatus) {
		d.PingEnabled = false
		d.TelemetryEnabled = false
		d.TelemetryMode = nil
	}
}

type validationErr string

func (e validationErr) Error() string { return string(e) }

func errValidation(msg string) error { return validationErr(msg) }

func acquiredAtArg(s *string) any {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	return strings.TrimSpace(*s)
}

func (s *Server) createDevice(w http.ResponseWriter, r *http.Request) {
	var body deviceDTO
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if body.Description == "" || body.Category == "" {
		writeErr(w, 422, "VALIDATION", "description e category obrigatórios", nil)
		return
	}
	ns := body.NetworkStatus
	if ns == "" {
		ns = "Normal"
	}
	body.NetworkStatus = ns
	applyDeviceBridgeAndLocalityRules(&body)
	if err := validateDeviceIP(ns, body.IP); err != nil {
		writeErr(w, 422, "VALIDATION", err.Error(), nil)
		return
	}
	if body.TelemetryEnabled && !body.PingEnabled {
		writeErr(w, 422, "VALIDATION", "telemetria exige ping ativo", nil)
		return
	}
	op := body.OperationalMode
	if op == "" {
		op = "Ativo"
	}
	var id uuid.UUID
	var ipArg any
	if body.IP != nil {
		ipArg = strings.TrimSpace(*body.IP)
	}
	err := s.DB().QueryRow(r.Context(), `
		INSERT INTO devices (pop_id, locality_id, category, description, ip, network_status, access_mode, telemetry_mode,
			ping_enabled, telemetry_enabled, operational_mode,
			latitude, longitude, brand, model, mac, serial_number, software_version, hardware_version, acquired_at, snmp_community, mib_folder_path,
			telemetry_oid_strategy, telemetry_oid_overrides, max_pons)
		VALUES ($1,$2,$3,$4, NULLIF($5::text,'')::inet, $6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20::date, $21, $22,
			COALESCE($23,'default'), COALESCE($24::jsonb,'{}'::jsonb), $25)
		RETURNING id
	`, body.PopID, body.LocalityID, body.Category, body.Description, ipArg, ns, body.AccessMode, body.TelemetryMode, body.PingEnabled, body.TelemetryEnabled, op,
		body.Latitude, body.Longitude, body.Brand, body.Model, body.MAC, body.SerialNumber, body.SoftwareVersion, body.HardwareVersion, acquiredAtArg(body.AcquiredAt), body.SNMPCommunity, body.MIBFolderPath,
		body.TelemetryOIDStrategy, body.TelemetryOIDOverrides, body.MaxPons).Scan(&id)
	if err != nil {
		if strings.Contains(err.Error(), "telemetry_requires_ping") {
			writeErr(w, 422, "VALIDATION", "telemetria exige ping", nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if body.TelemetryEnabled {
		s.scheduleSNMPDiscovery(id)
	}
	s.appendAuditLog(r.Context(), "device", id.String(), "create", actorFromRequest(r), nil, body)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) importDevicesCSV(w http.ResponseWriter, r *http.Request) {
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
	comma := deviceCSVDetectComma(firstLine)
	cr := csv.NewReader(bytes.NewReader(raw))
	cr.Comma = comma
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1
	headers, err := cr.Read()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "CSV_HEADER", err.Error(), nil)
		return
	}
	colMap := deviceCSVBuildColumnMap(headers)
	useNamed := deviceCSVHasNamedColumns(colMap)
	line := 1
	imported := 0
	var failed []map[string]any
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
		var d deviceDTO
		var derr error
		if useNamed {
			d, derr = parseDeviceCSVRowNamed(r.Context(), s.DB(), rec, colMap)
		} else {
			d, derr = parseDeviceCSVRow(rec)
		}
		if derr != nil {
			failed = append(failed, map[string]any{"line": line, "error": derr.Error()})
			continue
		}
		applyDeviceBridgeAndLocalityRules(&d)
		if err := validateDeviceIP(d.NetworkStatus, d.IP); err != nil {
			failed = append(failed, map[string]any{"line": line, "error": err.Error()})
			continue
		}
		if d.TelemetryEnabled && !d.PingEnabled {
			failed = append(failed, map[string]any{"line": line, "error": "telemetria exige ping ativo"})
			continue
		}
		var id uuid.UUID
		var ipArg any
		if d.IP != nil {
			ipArg = strings.TrimSpace(*d.IP)
		}
		err = s.DB().QueryRow(r.Context(), `
			INSERT INTO devices (pop_id, locality_id, category, description, ip, network_status, access_mode, telemetry_mode,
				ping_enabled, telemetry_enabled, operational_mode,
				latitude, longitude, brand, model, mac, serial_number, software_version, hardware_version, acquired_at, snmp_community, mib_folder_path,
				telemetry_oid_strategy, telemetry_oid_overrides, max_pons)
			VALUES ($1,$2,$3,$4, NULLIF($5::text,'')::inet, $6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20::date, $21, $22,
				COALESCE($23,'default'), COALESCE($24::jsonb,'{}'::jsonb), $25)
			RETURNING id
		`, d.PopID, d.LocalityID, d.Category, d.Description, ipArg, d.NetworkStatus, d.AccessMode, d.TelemetryMode, d.PingEnabled, d.TelemetryEnabled, d.OperationalMode,
			d.Latitude, d.Longitude, d.Brand, d.Model, d.MAC, d.SerialNumber, d.SoftwareVersion, d.HardwareVersion, acquiredAtArg(d.AcquiredAt), d.SNMPCommunity, d.MIBFolderPath,
			d.TelemetryOIDStrategy, d.TelemetryOIDOverrides, d.MaxPons).Scan(&id)
		if err != nil {
			failed = append(failed, map[string]any{"line": line, "error": err.Error()})
			continue
		}
		imported++
		if d.TelemetryEnabled {
			s.scheduleSNMPDiscovery(id)
		}
		s.appendAuditLog(r.Context(), "device", id.String(), "create", actorFromRequest(r), nil, d)
	}
	s.DB().Exec(r.Context(), `NOTIFY devices_changed, 'reload'`)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       len(failed) == 0,
		"imported": imported,
		"failed":   failed,
	})
}

func isCSVRowEmpty(rec []string) bool {
	for _, c := range rec {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func parseDeviceCSVRow(rec []string) (deviceDTO, error) {
	field := func(idx int) string {
		if idx >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[idx])
	}
	required := field(0)
	if required == "" {
		return deviceDTO{}, errValidation("description obrigatório")
	}
	category := field(1)
	if category == "" {
		return deviceDTO{}, errValidation("category obrigatório")
	}
	ipVal := field(2)
	var ip *string
	if ipVal != "" {
		ip = &ipVal
	}
	networkStatus := field(3)
	if networkStatus == "" {
		networkStatus = "Normal"
	}
	pingEnabled := parseCSVBool(field(4), true)
	telemetryEnabled := parseCSVBool(field(5), false)
	opMode := field(6)
	if opMode == "" {
		opMode = "Ativo"
	}
	var popID *uuid.UUID
	if s := field(7); s != "" {
		v, err := uuid.Parse(s)
		if err != nil {
			return deviceDTO{}, errValidation("pop_id inválido")
		}
		popID = &v
	}
	var localityID *uuid.UUID
	if s := field(8); s != "" {
		v, err := uuid.Parse(s)
		if err != nil {
			return deviceDTO{}, errValidation("locality_id inválido")
		}
		localityID = &v
	}
	brand := nilIfBlank(field(9))
	model := nilIfBlank(field(10))
	accessMode := nilIfBlank(field(11))
	telemetryMode := nilIfBlank(field(12))
	var lat *float64
	if s := field(13); s != "" {
		v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
		if err != nil {
			return deviceDTO{}, errValidation("latitude inválida")
		}
		lat = &v
	}
	var lon *float64
	if s := field(14); s != "" {
		v, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
		if err != nil {
			return deviceDTO{}, errValidation("longitude inválida")
		}
		lon = &v
	}
	snmp := nilIfBlank(field(15))
	mibPath := nilIfBlank(field(16))
	out := deviceDTO{
		PopID:                 popID,
		LocalityID:            localityID,
		Category:              category,
		Description:           required,
		IP:                    ip,
		NetworkStatus:         networkStatus,
		AccessMode:            accessMode,
		TelemetryMode:         telemetryMode,
		PingEnabled:           pingEnabled,
		TelemetryEnabled:      telemetryEnabled,
		OperationalMode:       opMode,
		Latitude:              lat,
		Longitude:             lon,
		Brand:                 brand,
		Model:                 model,
		SNMPCommunity:         snmp,
		MIBFolderPath:         mibPath,
		TelemetryOIDStrategy:  strPtr("default"),
		TelemetryOIDOverrides: json.RawMessage(`{}`),
	}
	return out, nil
}

func parseCSVBool(v string, def bool) bool {
	s := strings.ToLower(strings.TrimSpace(v))
	if s == "" {
		return def
	}
	if s == "0" || s == "false" || s == "não" || s == "nao" || s == "no" || s == "n" {
		return false
	}
	return s == "1" || s == "true" || s == "sim" || s == "yes" || s == "y"
}

func deviceCSVFirstLine(data []byte) string {
	i := bytes.IndexByte(data, '\n')
	if i < 0 {
		return string(data)
	}
	return strings.TrimSuffix(string(data[:i]), "\r")
}

func deviceCSVDetectComma(firstLine string) rune {
	tab := strings.Count(firstLine, "\t")
	semi := strings.Count(firstLine, ";")
	comma := strings.Count(firstLine, ",")
	if tab > semi && tab > comma && tab > 0 {
		return '\t'
	}
	if semi >= comma && semi > 0 {
		return ';'
	}
	return ','
}

func deviceCSVHeaderKey(h string) string {
	h = strings.TrimSpace(h)
	var b strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(h) {
		if unicode.IsSpace(r) {
			if b.Len() > 0 && !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
			continue
		}
		lastUnderscore = false
		b.WriteRune(r)
	}
	return strings.Trim(b.String(), "_")
}

func deviceCSVBuildColumnMap(headers []string) map[string]int {
	aliasToKey := map[string]string{
		"nome": "description", "descricao": "description", "descrição": "description", "description": "description",
		"categoria": "category", "category": "category", "tipo": "category",
		"ip": "ip", "endereco_ip": "ip", "endereço_ip": "ip",
		"status": "network_status", "network_status": "network_status", "rede": "network_status",
		"modo_acesso": "access_mode", "access_mode": "access_mode",
		"monitorar": "ping_enabled", "ping": "ping_enabled", "ping_enabled": "ping_enabled",
		"snmp": "telemetry_enabled", "telemetria": "telemetry_enabled", "telemetry_enabled": "telemetry_enabled",
		"telemetry_mode": "telemetry_mode", "modo_telemetria": "telemetry_mode",
		"operacao": "operational_mode", "operação": "operational_mode", "operacional": "operational_mode", "operational_mode": "operational_mode",
		"marca": "brand", "brand": "brand",
		"modelo": "model", "model": "model",
		"mac": "mac", "endereco_mac": "mac",
		"s/n": "serial_number", "serial": "serial_number", "serial_number": "serial_number", "n_serie": "serial_number",
		"versao": "software_version", "versão": "software_version", "firmware": "software_version", "version": "software_version",
		"hardware": "hardware_version", "hardware_version": "hardware_version",
		"data_aquisicao": "acquired_at", "data_aquisição": "acquired_at", "acquired_at": "acquired_at",
		"pop": "pop", "pop_id": "pop", "pops": "pop",
		"locality_id": "locality_id", "localidade_id": "locality_id",
		"latitude": "latitude", "lat": "latitude",
		"longitude": "longitude", "lng": "longitude", "lon": "longitude",
		"snmp_community": "snmp_community", "community": "snmp_community",
		"mib_folder_path": "mib_folder_path", "mib": "mib_folder_path",
	}
	out := make(map[string]int)
	for i, h := range headers {
		k := deviceCSVHeaderKey(h)
		if k == "" {
			continue
		}
		key, ok := aliasToKey[k]
		if !ok {
			continue
		}
		if _, exists := out[key]; exists {
			continue
		}
		out[key] = i
	}
	return out
}

func deviceCSVHasNamedColumns(m map[string]int) bool {
	_, d := m["description"]
	_, c := m["category"]
	return d && c
}

func deviceCSVGetCol(rec []string, m map[string]int, key string) string {
	i, ok := m[key]
	if !ok || i < 0 || i >= len(rec) {
		return ""
	}
	return strings.TrimSpace(rec[i])
}

func normalizeNetworkStatusCSV(s string) string {
	x := strings.TrimSpace(strings.ToUpper(s))
	switch x {
	case "", "NORMAL":
		return "Normal"
	case "BRIDGE":
		return "Bridge"
	default:
		lo := strings.ToLower(x)
		if lo == "" {
			return "Normal"
		}
		return strings.ToUpper(lo[:1]) + lo[1:]
	}
}

func normalizeOperationalCSV(s string) string {
	x := strings.TrimSpace(strings.ToUpper(s))
	switch x {
	case "", "ATIVO", "ACTIVE":
		return "Ativo"
	case "INATIVO", "INACTIVE":
		return "Inativo"
	default:
		if strings.TrimSpace(s) == "" {
			return "Ativo"
		}
		low := strings.ToLower(strings.TrimSpace(s))
		return strings.ToUpper(low[:1]) + low[1:]
	}
}

func parseCSVFloatCoordOptional(s string) (*float64, bool) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	if s == "" {
		return nil, true
	}
	parts := strings.Split(s, ".")
	if len(parts) == 4 && isLikelyIPv4Octets(parts) {
		return nil, true
	}
	var combined string
	if len(parts) >= 3 {
		combined = parts[0] + "." + strings.Join(parts[1:], "")
	} else {
		combined = s
	}
	v, err := strconv.ParseFloat(combined, 64)
	if err != nil {
		return nil, true
	}
	return &v, true
}

func isLikelyIPv4Octets(parts []string) bool {
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if len(p) == 0 || len(p) > 3 {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

func resolvePopForImport(ctx context.Context, pool *pgxpool.Pool, s string) (*uuid.UUID, error) {
	s = strings.TrimSpace(s)
	if s == "" || pool == nil {
		return nil, nil
	}
	if id, err := uuid.Parse(s); err == nil {
		return &id, nil
	}
	var id uuid.UUID
	err := pool.QueryRow(ctx, `SELECT id FROM pops WHERE lower(trim(description)) = lower(trim($1)) LIMIT 1`, s).Scan(&id)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func parseDeviceCSVRowNamed(ctx context.Context, pool *pgxpool.Pool, rec []string, m map[string]int) (deviceDTO, error) {
	desc := deviceCSVGetCol(rec, m, "description")
	cat := deviceCSVGetCol(rec, m, "category")
	if desc == "" {
		return deviceDTO{}, errValidation("description obrigatório (coluna Nome ou description)")
	}
	if cat == "" {
		return deviceDTO{}, errValidation("category obrigatório (coluna Categoria ou category)")
	}
	ipVal := deviceCSVGetCol(rec, m, "ip")
	var ip *string
	if ipVal != "" {
		ip = &ipVal
	}
	ns := normalizeNetworkStatusCSV(deviceCSVGetCol(rec, m, "network_status"))
	pingEn := parseCSVBool(deviceCSVGetCol(rec, m, "ping_enabled"), true)
	telEn := parseCSVBool(deviceCSVGetCol(rec, m, "telemetry_enabled"), false)
	op := normalizeOperationalCSV(deviceCSVGetCol(rec, m, "operational_mode"))
	telMode := deviceCSVGetCol(rec, m, "telemetry_mode")
	if telEn && strings.TrimSpace(telMode) == "" {
		telMode = "SNMP"
	}
	var popID *uuid.UUID
	if popRaw := deviceCSVGetCol(rec, m, "pop"); popRaw != "" {
		pid, err := resolvePopForImport(ctx, pool, popRaw)
		if err != nil {
			return deviceDTO{}, err
		}
		popID = pid
	}
	var localityID *uuid.UUID
	if lid := deviceCSVGetCol(rec, m, "locality_id"); lid != "" {
		u, err := uuid.Parse(lid)
		if err != nil {
			return deviceDTO{}, errValidation("locality_id inválido")
		}
		localityID = &u
	}
	lat, _ := parseCSVFloatCoordOptional(deviceCSVGetCol(rec, m, "latitude"))
	lon, _ := parseCSVFloatCoordOptional(deviceCSVGetCol(rec, m, "longitude"))
	return deviceDTO{
		PopID:                 popID,
		LocalityID:            localityID,
		Category:              cat,
		Description:           desc,
		IP:                    ip,
		NetworkStatus:         ns,
		AccessMode:            nilIfBlank(deviceCSVGetCol(rec, m, "access_mode")),
		TelemetryMode:         nilIfBlank(telMode),
		PingEnabled:           pingEn,
		TelemetryEnabled:      telEn,
		OperationalMode:       op,
		Latitude:              lat,
		Longitude:             lon,
		Brand:                 nilIfBlank(deviceCSVGetCol(rec, m, "brand")),
		Model:                 nilIfBlank(deviceCSVGetCol(rec, m, "model")),
		MAC:                   nilIfBlank(deviceCSVGetCol(rec, m, "mac")),
		SerialNumber:          nilIfBlank(deviceCSVGetCol(rec, m, "serial_number")),
		SoftwareVersion:       nilIfBlank(deviceCSVGetCol(rec, m, "software_version")),
		HardwareVersion:       nilIfBlank(deviceCSVGetCol(rec, m, "hardware_version")),
		AcquiredAt:            nilIfBlank(deviceCSVGetCol(rec, m, "acquired_at")),
		SNMPCommunity:         nilIfBlank(deviceCSVGetCol(rec, m, "snmp_community")),
		MIBFolderPath:         nilIfBlank(deviceCSVGetCol(rec, m, "mib_folder_path")),
		TelemetryOIDStrategy:  strPtr("default"),
		TelemetryOIDOverrides: json.RawMessage(`{}`),
	}, nil
}

func nilIfBlank(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	v := strings.TrimSpace(s)
	return &v
}

func strPtr(s string) *string { return &s }

func (s *Server) getDevice(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	var d deviceDTO
	var ip *string
	err = s.DB().QueryRow(r.Context(), `
		SELECT id, pop_id, locality_id, category, description, host(ip)::text, network_status, access_mode, telemetry_mode,
			ping_enabled, telemetry_enabled, operational_mode,
			latitude, longitude, brand, model, mac, serial_number, software_version, hardware_version, acquired_at::text, snmp_community, mib_folder_path,
			telemetry_oid_strategy, telemetry_oid_overrides::text, max_pons
		FROM devices WHERE id=$1
	`, id).Scan(&d.ID, &d.PopID, &d.LocalityID, &d.Category, &d.Description, &ip, &d.NetworkStatus, &d.AccessMode, &d.TelemetryMode,
		&d.PingEnabled, &d.TelemetryEnabled, &d.OperationalMode,
		&d.Latitude, &d.Longitude, &d.Brand, &d.Model, &d.MAC, &d.SerialNumber, &d.SoftwareVersion, &d.HardwareVersion, &d.AcquiredAt, &d.SNMPCommunity, &d.MIBFolderPath,
		&d.TelemetryOIDStrategy, &d.TelemetryOIDOverrides, &d.MaxPons)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "equipamento não encontrado", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	d.IP = ip
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) patchDevice(w http.ResponseWriter, r *http.Request) {
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
	var d deviceDTO
	var ip *string
	err = s.DB().QueryRow(r.Context(), `
		SELECT id, pop_id, locality_id, category, description, host(ip)::text, network_status, access_mode, telemetry_mode,
			ping_enabled, telemetry_enabled, operational_mode,
			latitude, longitude, brand, model, mac, serial_number, software_version, hardware_version, acquired_at::text, snmp_community, mib_folder_path,
			telemetry_oid_strategy, telemetry_oid_overrides::text, max_pons
		FROM devices WHERE id=$1
	`, id).Scan(&d.ID, &d.PopID, &d.LocalityID, &d.Category, &d.Description, &ip, &d.NetworkStatus, &d.AccessMode, &d.TelemetryMode,
		&d.PingEnabled, &d.TelemetryEnabled, &d.OperationalMode,
		&d.Latitude, &d.Longitude, &d.Brand, &d.Model, &d.MAC, &d.SerialNumber, &d.SoftwareVersion, &d.HardwareVersion, &d.AcquiredAt, &d.SNMPCommunity, &d.MIBFolderPath,
		&d.TelemetryOIDStrategy, &d.TelemetryOIDOverrides, &d.MaxPons)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "equipamento não encontrado", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	d.IP = ip
	prevPing := d.PingEnabled
	prevTelemetry := d.TelemetryEnabled
	mergeDeviceJSON(&d, body)
	if strings.TrimSpace(d.NetworkStatus) == "" {
		d.NetworkStatus = "Normal"
	}
	applyDeviceBridgeAndLocalityRules(&d)
	if err := validateDeviceIP(d.NetworkStatus, d.IP); err != nil {
		writeErr(w, 422, "VALIDATION", err.Error(), nil)
		return
	}
	if d.TelemetryEnabled && !d.PingEnabled {
		writeErr(w, 422, "VALIDATION", "telemetria exige ping ativo", nil)
		return
	}
	var ipArg any
	if d.IP != nil {
		ipArg = strings.TrimSpace(*d.IP)
	}
	_, err = s.DB().Exec(r.Context(), `
		UPDATE devices SET pop_id=$2, locality_id=$3, category=$4, description=$5, ip=NULLIF($6::text,'')::inet, network_status=$7,
			access_mode=$8, telemetry_mode=$9,
			ping_enabled=$10, telemetry_enabled=$11, operational_mode=$12,
			latitude=$13, longitude=$14, brand=$15, model=$16, mac=$17, serial_number=$18, software_version=$19, hardware_version=$20,
			acquired_at=$21::date, snmp_community=$22, mib_folder_path=$23,
			telemetry_oid_strategy=COALESCE($24,'default'),
			telemetry_oid_overrides=COALESCE($25::jsonb,'{}'::jsonb),
			max_pons=$26,
			updated_at=now()
		WHERE id=$1
	`, id, d.PopID, d.LocalityID, d.Category, d.Description, ipArg, d.NetworkStatus, d.AccessMode, d.TelemetryMode,
		d.PingEnabled, d.TelemetryEnabled, d.OperationalMode,
		d.Latitude, d.Longitude, d.Brand, d.Model, d.MAC, d.SerialNumber, d.SoftwareVersion, d.HardwareVersion, acquiredAtArg(d.AcquiredAt), d.SNMPCommunity, d.MIBFolderPath,
		d.TelemetryOIDStrategy, d.TelemetryOIDOverrides, d.MaxPons)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if d.TelemetryEnabled && !prevTelemetry {
		s.scheduleSNMPDiscovery(id)
	}
	if prevPing && !d.PingEnabled {
		monitorworker.ClosePingUnreachableOnMonitoringDisabled(r.Context(), s.DB(), &s.Log, id)
	}
	s.appendAuditLog(r.Context(), "device", id.String(), "patch", actorFromRequest(r), nil, d)
	s.getDevice(w, r)
}

func mergeDeviceJSON(d *deviceDTO, body map[string]json.RawMessage) {
	if v, ok := body["pop_id"]; ok {
		var x *uuid.UUID
		_ = json.Unmarshal(v, &x)
		d.PopID = x
	}
	if v, ok := body["category"]; ok {
		_ = json.Unmarshal(v, &d.Category)
	}
	if v, ok := body["description"]; ok {
		_ = json.Unmarshal(v, &d.Description)
	}
	if v, ok := body["ip"]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil {
			d.IP = &s
		}
	}
	if v, ok := body["network_status"]; ok {
		_ = json.Unmarshal(v, &d.NetworkStatus)
	}
	if v, ok := body["ping_enabled"]; ok {
		_ = json.Unmarshal(v, &d.PingEnabled)
	}
	if v, ok := body["telemetry_enabled"]; ok {
		_ = json.Unmarshal(v, &d.TelemetryEnabled)
	}
	if v, ok := body["operational_mode"]; ok {
		_ = json.Unmarshal(v, &d.OperationalMode)
	}
	if v, ok := body["latitude"]; ok {
		_ = json.Unmarshal(v, &d.Latitude)
	}
	if v, ok := body["longitude"]; ok {
		_ = json.Unmarshal(v, &d.Longitude)
	}
	if v, ok := body["brand"]; ok {
		_ = json.Unmarshal(v, &d.Brand)
	}
	if v, ok := body["model"]; ok {
		_ = json.Unmarshal(v, &d.Model)
	}
	if v, ok := body["mac"]; ok {
		_ = json.Unmarshal(v, &d.MAC)
	}
	if v, ok := body["serial_number"]; ok {
		_ = json.Unmarshal(v, &d.SerialNumber)
	}
	if v, ok := body["software_version"]; ok {
		_ = json.Unmarshal(v, &d.SoftwareVersion)
	}
	if v, ok := body["hardware_version"]; ok {
		_ = json.Unmarshal(v, &d.HardwareVersion)
	}
	if v, ok := body["locality_id"]; ok {
		var x *uuid.UUID
		_ = json.Unmarshal(v, &x)
		d.LocalityID = x
	}
	if v, ok := body["access_mode"]; ok {
		_ = json.Unmarshal(v, &d.AccessMode)
	}
	if v, ok := body["telemetry_mode"]; ok {
		_ = json.Unmarshal(v, &d.TelemetryMode)
	}
	if v, ok := body["acquired_at"]; ok {
		_ = json.Unmarshal(v, &d.AcquiredAt)
	}
	if v, ok := body["snmp_community"]; ok {
		_ = json.Unmarshal(v, &d.SNMPCommunity)
	}
	if v, ok := body["mib_folder_path"]; ok {
		_ = json.Unmarshal(v, &d.MIBFolderPath)
	}
	if v, ok := body["telemetry_oid_strategy"]; ok {
		_ = json.Unmarshal(v, &d.TelemetryOIDStrategy)
	}
	if v, ok := body["telemetry_oid_overrides"]; ok {
		var raw json.RawMessage
		if json.Unmarshal(v, &raw) == nil {
			d.TelemetryOIDOverrides = raw
		}
	}
	if v, ok := body["max_pons"]; ok {
		var x *int
		if json.Unmarshal(v, &x) == nil {
			d.MaxPons = x
		}
	}
}

func (s *Server) scheduleSNMPDiscovery(deviceID uuid.UUID) {
	pool := s.DB()
	if pool == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		l := s.Log.With().Str("device", deviceID.String()).Logger()
		if err := snmpdiscovery.Run(ctx, pool, &l, deviceID); err != nil {
			l.Warn().Err(err).Msg("SNMP discovery automático falhou")
		}
	}()
}

func (s *Server) getDeviceSNMPInventory(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	if disc, err := snmpcatalog.LoadEquipment(id.String()); err == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"source":          "local_json",
			"device_id":       disc.DeviceID,
			"brand":           disc.Brand,
			"model":           disc.Model,
			"collected_at":    disc.CollectedAt,
			"root_oid":        disc.RootOID,
			"row_count":       disc.RowCount,
			"truncated":       disc.Truncated,
			"walk_note":       disc.WalkNote,
			"class_summary":   disc.ClassSummary,
			"collect_profile": disc.CollectProfile,
			"discovery_debug": disc.DiscoveryDebug,
			"rows":            disc.Rows,
		})
		return
	} else if err != nil && !os.IsNotExist(err) {
		writeErr(w, http.StatusInternalServerError, "FS", err.Error(), nil)
		return
	}
	var disc time.Time
	var root string
	var rc int
	var tr bool
	var walkRows, walkSum, prof []byte
	err = s.DB().QueryRow(r.Context(), `
		SELECT discovered_at, root_oid, row_count, truncated, walk_rows::text, walk_summary::text, collect_profile::text
		FROM device_snmp_inventory WHERE device_id=$1
	`, id).Scan(&disc, &root, &rc, &tr, &walkRows, &walkSum, &prof)
	if err != nil && errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusOK, map[string]any{
			"device_id": id,
			"note":      "ainda sem inventário SNMP — active a telemetria no equipamento ou POST /devices/{id}/telemetry/discover",
		})
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	out := map[string]any{
		"source":          "db",
		"device_id":       id,
		"discovered_at":   disc,
		"root_oid":        root,
		"row_count":       rc,
		"truncated":       tr,
		"walk_rows":       json.RawMessage(walkRows),
		"walk_summary":    json.RawMessage(walkSum),
		"collect_profile": json.RawMessage(prof),
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	ct, err := s.DB().Exec(r.Context(), `DELETE FROM devices WHERE id=$1`, id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if ct.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "equipamento não encontrado", nil)
		return
	}
	s.appendAuditLog(r.Context(), "device", id.String(), "delete", actorFromRequest(r), nil, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deviceStatusStub(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "UUID inválido", nil)
		return
	}
	var ipStr *string
	var pingEn bool
	err = s.DB().QueryRow(r.Context(), `SELECT host(ip)::text, ping_enabled FROM devices WHERE id=$1`, id).Scan(&ipStr, &pingEn)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "equipamento não encontrado", nil)
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if !pingEn {
		writeJSON(w, http.StatusOK, map[string]any{"device_id": id, "ping_enabled": false, "note": "ping desativado para este equipamento"})
		return
	}
	if ipStr == nil || strings.TrimSpace(*ipStr) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"device_id": id, "ok": false, "error": "sem IP para sondagem"})
		return
	}

	var monRunning bool
	var monMode string
	_ = s.DB().QueryRow(r.Context(), `SELECT is_running, monitoring_mode FROM monitoring_runtime WHERE id=1`).Scan(&monRunning, &monMode)

	var checkedAt *time.Time
	var cacheMode string
	var ok bool
	var lat sql.NullInt64
	var method sql.NullString
	var snmpOK sql.NullBool
	var detail []byte
	err = s.DB().QueryRow(r.Context(), `
		SELECT checked_at, monitoring_mode, ok, latency_ms, method, snmp_ok, detail::text
		FROM device_probe_cache WHERE device_id=$1
	`, id).Scan(&checkedAt, &cacheMode, &ok, &lat, &method, &snmpOK, &detail)
	if err == nil {
		var snmpOut any
		if snmpOK.Valid {
			snmpOut = snmpOK.Bool
		}
		out := map[string]any{
			"device_id":          id,
			"ping_enabled":       true,
			"source":             "worker_cache",
			"monitoring_running": monRunning,
			"monitoring_mode":    monMode,
			"checked_at":         checkedAt,
			"cache_mode":         cacheMode,
			"ok":                 ok,
			"snmp_ok":            snmpOut,
			"detail":             json.RawMessage(detail),
			"note":               "Resultado do worker em background; atualizado a cada ping_seconds enquanto is_running=true.",
		}
		if lat.Valid {
			out["latency_ms"] = lat.Int64
		}
		if method.Valid {
			out["method"] = method.String
		}
		writeJSON(w, http.StatusOK, out)
		return
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}

	// Sem cache ainda: sondagem pontual (útil antes do 1º ciclo ou com monitoramento parado).
	if monRunning {
		writeJSON(w, http.StatusOK, map[string]any{
			"device_id": id, "ping_enabled": true, "monitoring_running": true, "monitoring_mode": monMode,
			"source": "pending", "note": "Aguardando primeiro ciclo do worker; tente novamente em instantes.",
		})
		return
	}

	port := r.URL.Query().Get("tcp_port")
	if port == "" {
		port = "443"
	}
	var pingTimeoutMs, icmpPayloadBytes int
	if err := s.DB().QueryRow(r.Context(), `
		SELECT ping_timeout_ms, icmp_payload_bytes FROM monitoring_intervals WHERE id=1`).Scan(&pingTimeoutMs, &icmpPayloadBytes); err != nil {
		pingTimeoutMs = 5500
		icmpPayloadBytes = 32
	}
	if pingTimeoutMs < 1000 {
		pingTimeoutMs = 1000
	}
	if pingTimeoutMs > 30000 {
		pingTimeoutMs = 30000
	}
	icmpPayloadBytes = probing.ClampICMPPayloadBytes(icmpPayloadBytes)
	to := time.Duration(pingTimeoutMs) * time.Millisecond
	icmpPart := to * 2 / 3
	if icmpPart < 500*time.Millisecond {
		icmpPart = 500 * time.Millisecond
	}
	tcpPart := to - icmpPart
	if tcpPart < time.Second {
		tcpPart = time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), to+300*time.Millisecond)
	defer cancel()
	out := probing.HostReachability(ctx, strings.TrimSpace(*ipStr), port, icmpPart, tcpPart, icmpPayloadBytes)
	out["device_id"] = id
	out["ping_enabled"] = true
	out["source"] = "live_adhoc"
	out["monitoring_running"] = monRunning
	out["note"] = "Monitoramento parado: sondagem única sob demanda. Inicie o monitoramento para resultados contínuos no cache."
	writeJSON(w, http.StatusOK, out)
}
