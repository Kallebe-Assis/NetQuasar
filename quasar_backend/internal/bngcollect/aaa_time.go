package bngcollect

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// FormatAAARecordTime normaliza data/hora de registos AAA Huawei (DateAndTime SNMP, texto ou epoch).
func FormatAAARecordTime(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, " → ") {
		parts := strings.SplitN(raw, " → ", 2)
		a := FormatAAARecordTime(parts[0])
		b := FormatAAARecordTime(parts[1])
		if a == "" {
			return b
		}
		if b == "" {
			return a
		}
		return a + " → " + b
	}
	if t, ok := parseAAARecordTime(raw); ok {
		if t2, ok2 := acceptAAARecordTime(t); ok2 {
			return t2.Format("02/01/2006 15:04:05")
		}
	}
	return raw
}

const aaaFutureSkew = 2 * time.Minute

func acceptAAARecordTime(t time.Time) (time.Time, bool) {
	if t.IsZero() {
		return t, false
	}
	if t.After(time.Now().Add(aaaFutureSkew)) {
		return time.Time{}, false
	}
	return t, true
}

func parseAAARecordTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	if b, ok := decodeSNMPDateTimeBytes(raw); ok {
		if t, ok := dateTimeFromBytes(b); ok {
			return acceptAAARecordTime(t)
		}
	}
	if t, ok := parseFakeIPv4AsDateTime(raw); ok {
		return acceptAAARecordTime(t)
	}
	if n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil {
		if n > 1_000_000_000_000 {
			n /= 1000
		}
		if n > 946684800 && n < 4102444800 {
			return acceptAAARecordTime(time.Unix(n, 0).In(time.Local))
		}
	}
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006/01/02 15:04:05",
		"2006-01-02 15:04",
		"20060102150405",
		"2006-01-02T15:04:05",
		time.RFC3339,
		"02/01/2006 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return acceptAAARecordTime(t)
		}
	}
	return time.Time{}, false
}

func parseFakeIPv4AsDateTime(raw string) (time.Time, bool) {
	parts := strings.Split(strings.TrimSpace(raw), ".")
	if len(parts) != 4 {
		return time.Time{}, false
	}
	b := make([]byte, 4)
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 0 || n > 255 {
			return time.Time{}, false
		}
		b[i] = byte(n)
	}
	return dateTimeFromBytes(b)
}

func decodeSNMPDateTimeBytes(raw string) ([]byte, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	if strings.Contains(raw, ":") && !strings.Contains(raw, "-") && !strings.Contains(raw, "/") && !strings.Contains(raw, ".") {
		parts := strings.Split(raw, ":")
		if len(parts) >= 7 {
			out := make([]byte, 0, len(parts))
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if len(p) == 0 || len(p) > 2 {
					out = nil
					break
				}
				if len(p) == 1 {
					p = "0" + p
				}
				var v byte
				if _, err := fmt.Sscanf(p, "%02x", &v); err != nil {
					out = nil
					break
				}
				out = append(out, v)
			}
			if len(out) >= 7 {
				return out, true
			}
		}
	}
	h := strings.ReplaceAll(raw, ":", "")
	h = strings.ReplaceAll(h, " ", "")
	h = strings.ToLower(h)
	if len(h) >= 14 && len(h)%2 == 0 {
		if b, err := hex.DecodeString(h); err == nil && len(b) >= 7 {
			return b, true
		}
	}
	return nil, false
}

func dateTimeFromBytes(b []byte) (time.Time, bool) {
	if len(b) < 4 {
		return time.Time{}, false
	}
	year := int(binary.BigEndian.Uint16(b[0:2]))
	month := int(b[2])
	day := int(b[3])
	if year < 1970 || year > 2100 || month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}, false
	}
	hour, min, sec := 0, 0, 0
	switch {
	case len(b) >= 8:
		hour = int(b[4])
		min = int(b[5])
		sec = int(b[6])
	case len(b) >= 7:
		hour = int(b[4])
		min = int(b[5])
		sec = int(b[6])
	case len(b) == 4:
		// só data
	default:
		return time.Time{}, false
	}
	if hour < 0 || hour > 23 || min < 0 || min > 59 || sec < 0 || sec > 60 {
		return time.Time{}, false
	}
	loc := time.Local
	if len(b) >= 11 {
		dir := b[8]
		utcH := int(b[9])
		utcM := int(b[10])
		offSec := utcH*3600 + utcM*60
		if dir == '-' {
			offSec = -offSec
		}
		loc = time.FixedZone("SNMP-UTC", offSec)
	}
	return time.Date(year, time.Month(month), day, hour, min, sec, 0, loc).In(time.Local), true
}

// ApproxLoginTimeFromOnlineSec estima hora de login a partir da duração online.
func ApproxLoginTimeFromOnlineSec(sec int) string {
	if sec <= 0 {
		return ""
	}
	return time.Now().Add(-time.Duration(sec) * time.Second).Format("2006-01-02 15:04:05")
}
