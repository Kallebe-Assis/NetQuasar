package bngcollect

import (
	"fmt"
	"strings"
)

var (
	authWeekdayPT = []string{"Dom", "Seg", "Ter", "Qua", "Qui", "Sex", "Sáb"}
	authMonthPT   = []string{"Jan", "Fev", "Mar", "Abr", "Mai", "Jun", "Jul", "Ago", "Set", "Out", "Nov", "Dez"}
)

// FormatAuthLogTimestamp formata data/hora estilo log RADIUS (pt).
func FormatAuthLogTimestamp(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "—"
	}
	if t, ok := parseAAARecordTime(raw); ok {
		wd := authWeekdayPT[int(t.Weekday())]
		mo := authMonthPT[int(t.Month())-1]
		return fmt.Sprintf("%s %d %s %d %02d:%02d:%02d", wd, t.Day(), mo, t.Year(), t.Hour(), t.Minute(), t.Second())
	}
	return raw
}

// FormatAuthLogMessage linha legível estilo FreeRADIUS (português).
func FormatAuthLogMessage(rec AuthAttemptLog) string {
	ts := FormatAuthLogTimestamp(rec.Time)
	status := "Falha no login"
	if rec.Kind == "success" {
		status = "Login OK"
	}
	login := strings.TrimSpace(rec.Login)
	if login == "" {
		login = "—"
	}
	mac := strings.ToUpper(strings.TrimSpace(rec.MAC))
	if mac == "" {
		mac = "—"
	}
	seq := strings.TrimSpace(rec.Seq)
	seqPart := ""
	if seq != "" {
		seqPart = fmt.Sprintf("(%s) ", seq)
	}
	port := strings.TrimSpace(rec.Port)
	portPart := ""
	if port != "" {
		portPart = fmt.Sprintf("porta %s · ", port)
	}
	reason := strings.TrimSpace(rec.Reason)
	if reason == "" {
		reason = strings.TrimSpace(rec.Detail)
	}
	line := fmt.Sprintf("%s : Auth: %s%s — [%s/] (%sMAC %s)", ts, seqPart, status, login, portPart, mac)
	if reason != "" && rec.Kind == "failure" {
		line += fmt.Sprintf(" — %s", reason)
	}
	return line
}

func finalizeAuthRecord(rec *AuthAttemptLog, stripSuffix string) {
	rec.Login = NormalizeSNMPLoginValue(rec.Login, stripSuffix)
	rec.MAC = normalizeMACAddress(rec.MAC)
	rec.Time = normalizeAuthRecordTime(rec.Time)
	if rec.Message == "" {
		rec.Message = FormatAuthLogMessage(*rec)
	}
}

func normalizeAuthRecordTime(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, " → ") {
		raw = strings.SplitN(raw, " → ", 2)[0]
	}
	if t, ok := parseAAARecordTime(raw); ok {
		if t2, ok2 := acceptAAARecordTime(t); ok2 {
			return t2.Format("2006-01-02 15:04:05")
		}
		return ""
	}
	formatted := FormatAAARecordTime(raw)
	if formatted == "" || formatted == raw {
		return formatted
	}
	if t, ok := parseAAARecordTime(formatted); ok {
		if t2, ok2 := acceptAAARecordTime(t); ok2 {
			return t2.Format("2006-01-02 15:04:05")
		}
		return ""
	}
	return formatted
}
