package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/netquasar/netquasar/quasar_backend/internal/oltcollect"
	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
	"github.com/netquasar/netquasar/quasar_backend/internal/vsolparse"
)

type oltOnuSearchRequest struct {
	Q          string   `json:"q"`
	Serial     string   `json:"serial"`
	Model      string   `json:"model"`
	Online     *bool    `json:"online"`
	RxDbmMin   *float64 `json:"rx_dbm_min"`
	RxDbmMax   *float64 `json:"rx_dbm_max"`
	TxDbmMin   *float64 `json:"tx_dbm_min"`
	TxDbmMax   *float64 `json:"tx_dbm_max"`
	TempMin    *float64 `json:"temp_min"`
	TempMax    *float64 `json:"temp_max"`
	VoltageMin *float64 `json:"voltage_min"`
	VoltageMax *float64 `json:"voltage_max"`
	OltID      string   `json:"olt_id"`
	Pon        int      `json:"pon"`
}

type oltOnuReportRequest struct {
	Pon     int    `json:"pon"`
	Onu     int    `json:"onu"`
	Serial  string `json:"serial"`
	IfIndex int    `json:"if_index"`
	IfName  string `json:"if_name"`
}

type oltOnuSerialSearchRequest struct {
	Serial string `json:"serial"`
	Pon    int    `json:"pon"` // 0 = todas as portas
}

type oltTelnetSession struct {
	DeviceID          uuid.UUID
	Desc, Brand, Model string
	Host              string
	Cfg               oltcollect.OnuReportConfig
	User, Password, Enable string
}

func (s *Server) loadOLTTelnetSession(ctx context.Context, id uuid.UUID) (oltTelnetSession, error) {
	var sess oltTelnetSession
	sess.DeviceID = id
	var ip *string
	err := s.DB().QueryRow(ctx, `
		SELECT description, brand, model, host(ip)::text
		FROM devices WHERE id=$1 AND lower(trim(category))='olt'
	`, id).Scan(&sess.Desc, &sess.Brand, &sess.Model, &ip)
	if err == pgx.ErrNoRows {
		return sess, err
	}
	if err != nil {
		return sess, err
	}
	if ip != nil {
		sess.Host = strings.TrimSpace(*ip)
	}
	if sess.Host == "" {
		return sess, errValidation("OLT sem IP configurado")
	}

	var reportRaw []byte
	err = s.DB().QueryRow(ctx, `
		SELECT coalesce(onu_report_commands::text, '{}')
		FROM olt_vendor_models
		WHERE upper(trim(brand)) = upper(trim($1)) AND upper(trim(model)) = upper(trim($2))
	`, sess.Brand, sess.Model).Scan(&reportRaw)
	if err != nil && err != pgx.ErrNoRows {
		return sess, err
	}
	sess.Cfg = oltcollect.ParseOnuReportConfig(reportRaw)

	var telUser, telPass, telEnable *string
	_ = s.DB().QueryRow(ctx, `SELECT telnet_user, telnet_password, telnet_enable FROM settings_connection_defaults WHERE id=1`).Scan(&telUser, &telPass, &telEnable)
	if telUser != nil {
		sess.User = strings.TrimSpace(*telUser)
	}
	if telPass != nil {
		sess.Password = strings.TrimSpace(*telPass)
	}
	if telEnable != nil {
		sess.Enable = strings.TrimSpace(*telEnable)
	}
	if sess.User == "" || sess.Password == "" {
		return sess, errValidation("credenciais telnet não configuradas em Definições → Rede e SNMP")
	}
	return sess, nil
}

func (s *Server) searchOLTOnus(w http.ResponseWriter, r *http.Request) {
	var body oltOnuSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	var oltFilter *uuid.UUID
	if id := strings.TrimSpace(body.OltID); id != "" {
		parsed, err := uuid.Parse(id)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "BAD_ID", "olt_id inválido", nil)
			return
		}
		oltFilter = &parsed
	}

	rows, err := s.DB().Query(r.Context(), `
		SELECT d.id, d.description, host(d.ip)::text, d.brand, d.model, l.name,
			COALESCE(o.summary::text, '{}'), o.updated_at
		FROM devices d
		LEFT JOIN commercial_localities l ON l.id = d.locality_id
		LEFT JOIN olt_snapshots o ON o.device_id = d.id
		WHERE lower(trim(d.category)) = 'olt'
		ORDER BY d.description
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	defer rows.Close()

	q := strings.ToLower(strings.TrimSpace(body.Q))
	serialQ := strings.ToLower(strings.TrimSpace(body.Serial))
	modelQ := strings.ToLower(strings.TrimSpace(body.Model))
	if serialQ == "" && strings.Contains(q, ":") == false {
		// allow q to match serial or model
	}
	if serialQ == "" && q != "" {
		serialQ = q
		modelQ = q
	}

	var results []map[string]any
	for rows.Next() {
		var id uuid.UUID
		var desc, ip, brand, model, locName *string
		var sum string
		var snapAt *time.Time
		if err := rows.Scan(&id, &desc, &ip, &brand, &model, &locName, &sum, &snapAt); err != nil {
			writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
			return
		}
		if oltFilter != nil && id != *oltFilter {
			continue
		}
		onuRows := vsolparse.VsolOnuRowsFromSummaryBlob([]byte(sum))
		for _, raw := range onuRows {
			row, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if !oltOnuRowMatchesFilters(row, body, serialQ, modelQ) {
				continue
			}
			item := map[string]any{
				"olt_id":          id,
				"olt_description": desc,
				"olt_ip":          ip,
				"olt_brand":       brand,
				"olt_model":       model,
				"locality_name":   locName,
				"pon":             row["pon"],
				"onu":             row["onu"],
				"serial":          row["serial"],
				"model":           row["model"],
				"online":          row["online"],
				"rx_dbm":          row["rx_dbm"],
				"rx_pwr":          firstNonNil(row["rx_pwr"], row["rx"]),
				"tx_pwr":          firstNonNil(row["tx_pwr"], row["tx"]),
				"temp":            row["temp"],
				"voltage":         firstNonNil(row["voltage"], row["volt"]),
				"if_index":        row["if_index"],
				"if_name":         firstNonNil(row["if_name"], row["if_descr"]),
				"vlan":            row["vlan"],
				"data_source_telnet": row["data_source_telnet"],
				"telnet_report_at":   row["telnet_report_at"],
				"phase_sta":          row["phase_sta"],
				"channel":            row["channel"],
			}
			if snapAt != nil {
				item["snapshot_at"] = snapAt.UTC().Format(time.RFC3339)
			}
			results = append(results, item)
		}
	}
	if results == nil {
		results = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"results": results,
		"total":   len(results),
	})
}

func (s *Server) reportOLTOnu(w http.ResponseWriter, r *http.Request) {
	extendWriteDeadline(w, 2*time.Minute)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var body oltOnuReportRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	if body.Pon <= 0 && body.Onu <= 0 && strings.TrimSpace(body.Serial) == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "informe pon/onu ou serial", nil)
		return
	}

	ctx := r.Context()
	sess, err := s.loadOLTTelnetSession(ctx, id)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		if strings.Contains(err.Error(), "credenciais telnet") || strings.Contains(err.Error(), "sem IP") {
			writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED", err.Error(), nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	cfg := sess.Cfg
	if !cfg.HasCommands() {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED",
			"Configure os comandos telnet de relatório ONU em Definições → Perfis OLT para "+sess.Brand+" / "+sess.Model, nil)
		return
	}
	if cfg.NeedsEnablePassword() && sess.Enable == "" {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED",
			"configure a palavra-passe enable (telnet enable) em Definições → Rede e SNMP para os pré-comandos deste perfil", nil)
		return
	}

	secrets := oltcollect.TelnetSecrets{Password: sess.Password, Enable: sess.Enable}

	target := oltcollect.OnuReportTarget{
		Pon:     body.Pon,
		Onu:     body.Onu,
		Serial:  body.Serial,
		IfIndex: body.IfIndex,
		IfName:  body.IfName,
	}
	target.GponOnu = oltcollect.ResolveGponOnu(target)

	if target.GponOnu == "" && cfg.NeedsGponOnu() && strings.TrimSpace(body.Serial) != "" {
		if g := s.lookupGponOnuBySerial(ctx, sess, target, secrets); g != "" {
			target.GponOnu = g
		}
	}

	preRendered := cfg.RenderPreCommands(target, secrets)
	cmds := cfg.RenderCommands(target, secrets)
	if len(cmds) == 0 {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "nenhum comando gerado a partir do template", nil)
		return
	}

	script := probing.TelnetRunScript(ctx, probing.TelnetRunScriptParams{
		Host: sess.Host, Port: "23", Timeout: 90 * time.Second,
		User: sess.User, Password: sess.Password, Enable: sess.Enable,
		PreCommands: preRendered, RawPreCommands: cfg.PreCommands,
		Commands: cmds, MaxReadBytes: 280000,
	})

	var outputs []map[string]any
	for _, step := range script.Steps {
		entry := map[string]any{
			"command": step.Command,
			"ok":      step.OK,
			"output":  step.Output,
		}
		if step.Error != "" {
			entry["error"] = step.Error
		}
		outputs = append(outputs, entry)
	}
	if outputs == nil {
		outputs = []map[string]any{}
	}

	if !script.OK {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":              false,
			"olt_id":          id,
			"olt_description": sess.Desc,
			"commands":        outputs,
			"output":          script.Output,
			"error":           script.Error,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"olt_id":          id,
		"olt_description": sess.Desc,
		"commands":        outputs,
		"output":          script.Output,
	})
}

func (s *Server) searchOLTOnuBySerial(w http.ResponseWriter, r *http.Request) {
	extendWriteDeadline(w, 3*time.Minute)
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_ID", "", nil)
		return
	}
	var body oltOnuSerialSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "BAD_JSON", err.Error(), nil)
		return
	}
	serial := strings.TrimSpace(body.Serial)
	if serial == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION", "informe o número de série", nil)
		return
	}

	ctx := r.Context()
	sess, err := s.loadOLTTelnetSession(ctx, id)
	if err == pgx.ErrNoRows {
		writeErr(w, http.StatusNotFound, "NOT_FOUND", "", nil)
		return
	}
	if err != nil {
		if strings.Contains(err.Error(), "credenciais telnet") || strings.Contains(err.Error(), "sem IP") {
			writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED", err.Error(), nil)
			return
		}
		writeErr(w, http.StatusInternalServerError, "DB", err.Error(), nil)
		return
	}
	if !sess.Cfg.HasCommands() && strings.TrimSpace(sess.Cfg.SerialSearchCommand) == "" {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED",
			"Configure os comandos telnet do perfil OLT em Definições → Perfis OLT para "+sess.Brand+" / "+sess.Model, nil)
		return
	}
	if sess.Cfg.NeedsEnablePassword() && sess.Enable == "" {
		writeErr(w, http.StatusBadRequest, "NOT_CONFIGURED",
			"configure a palavra-passe enable (telnet enable) em Definições → Rede e SNMP para os pré-comandos deste perfil", nil)
		return
	}

	secrets := oltcollect.TelnetSecrets{Password: sess.Password, Enable: sess.Enable}
	ponIndexes := oltcollect.LoadOLTPonIndexesFromSnapshot(ctx, s.DB(), id)
	telTO := 90 * time.Second
	if sess.Cfg.SerialSearchUsesPonPlaceholder() && body.Pon <= 0 && len(ponIndexes) > 4 {
		telTO = time.Duration(len(ponIndexes)) * 12 * time.Second
		if telTO > 3*time.Minute {
			telTO = 3 * time.Minute
		}
	}
	telCtx, cancel := context.WithTimeout(ctx, telTO)
	defer cancel()

	searchRes := oltcollect.RunSerialSearchTelnet(
		telCtx, sess.Host, sess.User, sess.Password, sess.Enable,
		sess.Cfg, secrets,
		oltcollect.SerialSearchRunOpts{Serial: serial, Pon: body.Pon},
		ponIndexes, telTO,
	)

	first := oltcollect.FirstSerialSearchMatch(searchRes)
	parsed := oltcollect.ExtractTelnetKVFieldsPublic(searchRes.Output)
	if first.Serial != "" {
		parsed["SN"] = first.Serial
	}
	if first.Model != "" {
		parsed["Modelo"] = first.Model
	}

	var matches []map[string]any
	for _, m := range searchRes.Matches {
		matches = append(matches, oltcollect.SerialSearchEntryToMap(m))
	}
	if matches == nil {
		matches = []map[string]any{}
	}

	out := map[string]any{
		"ok":              searchRes.OK,
		"mode":            searchRes.Mode,
		"olt_id":          id,
		"olt_description": sess.Desc,
		"serial":          serial,
		"pon_filter":      body.Pon,
		"command":         searchRes.Command,
		"commands":        searchRes.Commands,
		"output":          searchRes.Output,
		"matches":         matches,
		"parsed":          parsed,
		"gpon_onu":        nilIfBlankStr(first.GponOnu),
	}
	if first.Pon > 0 {
		out["pon"] = first.Pon
	}
	if first.Onu > 0 {
		out["onu"] = first.Onu
	}
	if searchRes.Error != "" {
		out["error"] = searchRes.Error
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) lookupGponOnuBySerial(ctx context.Context, sess oltTelnetSession, target oltcollect.OnuReportTarget, secrets oltcollect.TelnetSecrets) string {
	ponIndexes := oltcollect.LoadOLTPonIndexesFromSnapshot(ctx, s.DB(), sess.DeviceID)
	searchRes := oltcollect.RunSerialSearchTelnet(
		ctx, sess.Host, sess.User, sess.Password, sess.Enable,
		sess.Cfg, secrets,
		oltcollect.SerialSearchRunOpts{Serial: target.Serial, Pon: target.Pon},
		ponIndexes, 45*time.Second,
	)
	first := oltcollect.FirstSerialSearchMatch(searchRes)
	if first.GponOnu != "" {
		return first.GponOnu
	}
	if first.Pon > 0 && first.Onu > 0 {
		return fmt.Sprintf("gpon_onu-1/1/%d:%d", first.Pon, first.Onu)
	}
	return ""
}

func nilIfBlankStr(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func oltOnuRowMatchesFilters(row map[string]any, f oltOnuSearchRequest, serialQ, modelQ string) bool {
	serial := strings.ToLower(strings.TrimSpace(stringFromAny(row["serial"])))
	model := strings.ToLower(strings.TrimSpace(stringFromAny(row["model"])))
	if serialQ != "" && !strings.Contains(serial, serialQ) && !strings.Contains(model, serialQ) {
		if strings.TrimSpace(f.Serial) != "" {
			return false
		}
	}
	if strings.TrimSpace(f.Serial) != "" {
		sq := strings.ToLower(strings.TrimSpace(f.Serial))
		if !strings.Contains(serial, sq) {
			return false
		}
	}
	if strings.TrimSpace(f.Model) != "" {
		mq := strings.ToLower(strings.TrimSpace(f.Model))
		if !strings.Contains(model, mq) {
			return false
		}
	} else if modelQ != "" && serialQ == modelQ {
		if !strings.Contains(serial, modelQ) && !strings.Contains(model, modelQ) {
			return false
		}
	}
	if f.Online != nil {
		on, ok := row["online"].(bool)
		if !ok || on != *f.Online {
			return false
		}
	}
	if f.Pon > 0 {
		pon := intFromOnuSearchRow(row, "pon")
		if pon != f.Pon {
			return false
		}
	}
	rx := floatFromOnuRow(row, "rx_dbm", "rx_pwr", "rx")
	if !rangeOK(rx, f.RxDbmMin, f.RxDbmMax) {
		return false
	}
	tx := floatFromOnuRow(row, "tx_pwr", "tx")
	if !rangeOK(tx, f.TxDbmMin, f.TxDbmMax) {
		return false
	}
	temp := floatFromOnuRow(row, "temp")
	if !rangeOK(temp, f.TempMin, f.TempMax) {
		return false
	}
	volt := floatFromOnuRow(row, "voltage", "volt")
	if !rangeOK(volt, f.VoltageMin, f.VoltageMax) {
		return false
	}
	return true
}

func intFromOnuSearchRow(row map[string]any, key string) int {
	if row == nil {
		return 0
	}
	switch v := row[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	default:
		n, _ := strconv.Atoi(strings.TrimSpace(stringFromAny(row[key])))
		return n
	}
}

func stringFromAny(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	default:
		return ""
	}
}

func floatFromOnuRow(row map[string]any, keys ...string) *float64 {
	for _, k := range keys {
		if v, ok := row[k]; ok && v != nil {
			if f := parseFloatAny(v); f != nil {
				return f
			}
		}
	}
	return nil
}

func parseFloatAny(v any) *float64 {
	switch x := v.(type) {
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return nil
		}
		return &x
	case float32:
		f := float64(x)
		return &f
	case int:
		f := float64(x)
		return &f
	case string:
		s := strings.TrimSpace(strings.ReplaceAll(x, ",", "."))
		s = strings.TrimSuffix(s, "C")
		s = strings.TrimSuffix(s, "c")
		s = strings.TrimSuffix(s, "V")
		s = strings.TrimSuffix(s, "v")
		s = strings.TrimSuffix(s, "dBm")
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil
		}
		return &f
	default:
		return nil
	}
}

func rangeOK(val *float64, min, max *float64) bool {
	if min == nil && max == nil {
		return true
	}
	if val == nil {
		return false
	}
	if min != nil && *val < *min {
		return false
	}
	if max != nil && *val > *max {
		return false
	}
	return true
}

func firstNonNil(vals ...any) any {
	for _, v := range vals {
		if v != nil && strings.TrimSpace(stringFromAny(v)) != "" {
			return v
		}
	}
	return nil
}
