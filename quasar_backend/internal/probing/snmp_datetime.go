package probing

import (
	"encoding/binary"
	"time"
)

// formatSNMPDateAndTime detecta OctetString DateAndTime (RFC 2579) antes de IPv4/MAC.
func formatSNMPDateAndTime(b []byte) (string, bool) {
	b = trimTrailingNulls(append([]byte(nil), b...))
	if len(b) < 4 {
		return "", false
	}
	t, ok := snmpDateTimeFromBytes(b)
	if !ok {
		return "", false
	}
	return t.Format("2006-01-02 15:04:05"), true
}

func snmpDateTimeFromBytes(b []byte) (time.Time, bool) {
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
		// Só data (sem hora) — comum quando 4 octetos eram confundidos com IPv4.
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

