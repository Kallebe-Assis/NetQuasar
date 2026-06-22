package oltcollect

import (
	"strconv"
	"strings"
)

func mergeTelnetFieldsIntoPonRow(row map[string]any, fields map[string]string, reportedAt string) {
	if row == nil || len(fields) == 0 {
		return
	}
	setFloat := func(key string, val string) {
		if strings.TrimSpace(val) == "" {
			return
		}
		if cur, ok := row[key]; ok && cur != nil && strings.TrimSpace(stringFromAny(cur)) != "" {
			return
		}
		if f := parseTelnetMetricFloat(val); f != nil {
			row[key] = *f
		}
	}
	setDbm := func(key string, val string) {
		if strings.TrimSpace(val) == "" {
			return
		}
		if cur, ok := row[key]; ok && cur != nil {
			switch cur.(type) {
			case float64, float32, int:
				return
			}
		}
		if f := parseDbmValue(val); f != nil {
			row[key] = *f
		}
	}
	if v := firstNonEmptyField(fields, "Temperatura", "Temperature", "Temp."); v != "" {
		setFloat("temperature", v)
	}
	if v := firstNonEmptyField(fields, "Voltagem", "Voltage", "VCC"); v != "" {
		setFloat("voltage", v)
	}
	if v := firstNonEmptyField(fields, "TX", "TX Power", "TxPower", "Potência TX"); v != "" {
		setDbm("tx_dbm", v)
	}
	if v := firstNonEmptyField(fields, "RX", "RX Power", "RxPower", "Potência RX"); v != "" {
		setDbm("rx_dbm", v)
	}
	if v := firstNonEmptyField(fields, "TX Bias", "Bias", "Corrente Bias", "Corrente TX", "Bias Current", "TXBias"); v != "" {
		setFloat("current", v)
	}
	row["pon_telnet_at"] = reportedAt
	row["pon_telnet_source"] = true
	if extra, ok := row["pon_telnet_fields"].(map[string]any); ok {
		for k, v := range fields {
			extra[k] = v
		}
	} else {
		extra := map[string]any{}
		for k, v := range fields {
			extra[k] = v
		}
		row["pon_telnet_fields"] = extra
	}
}

func firstNonEmptyField(fields map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(fields[k]); v != "" {
			return v
		}
	}
	return ""
}

func parseTelnetMetricFloat(s string) *float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	for _, suf := range []string{"V", "v", "A", "a", "mA", "ma", "C", "c", "°C"} {
		s = strings.TrimSuffix(s, suf)
	}
	s = strings.TrimSpace(s)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}

func ponTargetFromRow(row map[string]any) OnuReportTarget {
	pon := ponIndexFromRowMap(row)
	return OnuReportTarget{Pon: pon}
}

func ponIndexFromRowMap(row map[string]any) int {
	if row == nil {
		return 0
	}
	if n, ok := row["pon"].(int); ok && n > 0 {
		return n
	}
	if n := intFromRow(row, "pon"); n > 0 {
		return n
	}
	id := strings.TrimSpace(stringFromAny(row["id"]))
	if id == "" {
		id = strings.TrimSpace(stringFromAny(row["name"]))
	}
	id = strings.TrimPrefix(strings.ToLower(id), "pon-")
	id = strings.TrimPrefix(id, "gpon")
	id = strings.TrimLeft(id, "0/")
	n, err := strconv.Atoi(strings.TrimLeft(id, "0"))
	if err == nil && n > 0 {
		return n
	}
	return intFromRow(row, "id")
}
