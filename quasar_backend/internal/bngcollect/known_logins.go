package bngcollect

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type knownLoginRow struct {
	ID             int64
	Login          string
	IsOnline       bool
	CurrentEventID *int64
}

// SyncKnownLogins sincroniza o inventário de logins com uma coleta completa de sessões.
// Presentes → online (abre evento se necessário); ausentes que estavam online → offline (fecha evento).
func SyncKnownLogins(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, sessions []SessionRow, stripSuffix string) error {
	if pool == nil {
		return nil
	}
	online := make(map[string]SessionRow, len(sessions))
	for _, s := range sessions {
		login := strings.TrimSpace(NormalizeSNMPLoginValue(s.Login, stripSuffix))
		if login == "" {
			continue
		}
		s.Login = login
		online[strings.ToLower(login)] = s
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	existing, err := loadKnownLoginsTx(ctx, tx, deviceID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	for key, row := range online {
		prev, ok := existing[key]
		if !ok || !prev.IsOnline {
			if err := openOrRefreshKnownLogin(ctx, tx, deviceID, row, now, prev); err != nil {
				return err
			}
			continue
		}
		if err := refreshKnownLoginOnline(ctx, tx, deviceID, row, now, prev); err != nil {
			return err
		}
	}

	for key, prev := range existing {
		if !prev.IsOnline {
			continue
		}
		if _, still := online[key]; still {
			continue
		}
		if err := markKnownLoginOfflineTx(ctx, tx, deviceID, prev, now); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// TouchKnownLoginOnline actualiza um login encontrado em lookup pontual (não marca outros offline).
func TouchKnownLoginOnline(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, row SessionRow, stripSuffix string) error {
	if pool == nil {
		return nil
	}
	row.Login = strings.TrimSpace(NormalizeSNMPLoginValue(row.Login, stripSuffix))
	if row.Login == "" {
		return nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	existing, err := loadKnownLoginsTx(ctx, tx, deviceID)
	if err != nil {
		return err
	}
	key := strings.ToLower(row.Login)
	prev := existing[key]
	now := time.Now().UTC()
	if prev.ID == 0 || !prev.IsOnline {
		if err := openOrRefreshKnownLogin(ctx, tx, deviceID, row, now, prev); err != nil {
			return err
		}
	} else if err := refreshKnownLoginOnline(ctx, tx, deviceID, row, now, prev); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// MarkKnownLoginOfflineIfExists marca offline um login já conhecido que não foi encontrado no BNG.
func MarkKnownLoginOfflineIfExists(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, login, stripSuffix string) error {
	if pool == nil {
		return nil
	}
	login = strings.TrimSpace(NormalizeSNMPLoginValue(login, stripSuffix))
	if login == "" {
		return nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	existing, err := loadKnownLoginsTx(ctx, tx, deviceID)
	if err != nil {
		return err
	}
	prev, ok := existing[strings.ToLower(login)]
	if !ok || !prev.IsOnline {
		return tx.Commit(ctx)
	}
	if err := markKnownLoginOfflineTx(ctx, tx, deviceID, prev, time.Now().UTC()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// EnsureKnownLoginsFromLatestSnapshot importa o snapshot mais recente se ainda não houver inventário.
func EnsureKnownLoginsFromLatestSnapshot(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, stripSuffix string) error {
	if pool == nil {
		return nil
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM bng_known_logins WHERE device_id=$1`, deviceID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	var raw []byte
	err := pool.QueryRow(ctx, `
		SELECT data::text FROM bng_session_snapshots
		WHERE device_id=$1 ORDER BY captured_at DESC LIMIT 1
	`, deviceID).Scan(&raw)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil
		}
		return err
	}
	sessions := parseSessionRowsFromSnapshotJSON(raw)
	if len(sessions) == 0 {
		return nil
	}
	return SyncKnownLogins(ctx, pool, deviceID, sessions, stripSuffix)
}

// ListKnownLoginsAsSessions devolve inventário no formato da lista de sessões da UI.
func ListKnownLoginsAsSessions(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID) ([]map[string]any, *time.Time, int, int, error) {
	if pool == nil {
		return nil, nil, 0, 0, nil
	}
	rows, err := pool.Query(ctx, `
		SELECT login, is_online, first_seen_at, last_seen_at, last_offline_at,
		       session_index, vlan, ipv4, ipv6, ipv6_pd, mac, interface_name, domain,
		       ip_type, ip_type_raw, car_up_cir_kbps, car_dn_cir_kbps, online_time_sec, auth_state, updated_at
		FROM bng_known_logins
		WHERE device_id=$1
		ORDER BY is_online DESC, login ASC
	`, deviceID)
	if err != nil {
		return nil, nil, 0, 0, err
	}
	defer rows.Close()

	out := make([]map[string]any, 0)
	onlineN := 0
	var latest *time.Time
	for rows.Next() {
		var (
			login                                                                                       string
			sessIdx, vlan, ipv4, ipv6, ipv6PD, mac, iface, domain, ipType, ipTypeRaw, upCIR, dnCIR, auth *string
			isOnline                                                                                    bool
			firstSeen, lastSeen, updated                                                                time.Time
			lastOffline                                                                                 *time.Time
			onlineSec                                                                                   *int64
		)
		if err := rows.Scan(
			&login, &isOnline, &firstSeen, &lastSeen, &lastOffline,
			&sessIdx, &vlan, &ipv4, &ipv6, &ipv6PD, &mac, &iface, &domain,
			&ipType, &ipTypeRaw, &upCIR, &dnCIR, &onlineSec, &auth, &updated,
		); err != nil {
			return nil, nil, 0, 0, err
		}
		if isOnline {
			onlineN++
		}
		if latest == nil || updated.After(*latest) {
			t := updated
			latest = &t
		}
		status := "Down"
		if isOnline {
			status = "Up"
		}
		onlineTimeSec := ""
		onlineTime := ""
		if onlineSec != nil && *onlineSec > 0 {
			onlineTimeSec = fmt.Sprintf("%d", *onlineSec)
			onlineTime = FormatDurationSeconds(int(*onlineSec))
		}
		deref := func(p *string) string {
			if p == nil {
				return ""
			}
			return *p
		}
		m := map[string]any{
			"index":           deref(sessIdx),
			"login":           login,
			"ipv4":            nullIfEmpty(deref(ipv4)),
			"ipv6":            nullIfEmpty(deref(ipv6)),
			"ipv6_pd":         nullIfEmpty(deref(ipv6PD)),
			"mac":             nullIfEmpty(deref(mac)),
			"vlan":            nullIfEmpty(deref(vlan)),
			"interface":       nullIfEmpty(deref(iface)),
			"domain":          nullIfEmpty(deref(domain)),
			"ip_type":         nullIfEmpty(deref(ipType)),
			"ip_type_raw":     nullIfEmpty(deref(ipTypeRaw)),
			"car_up_cir_kbps": nullIfEmpty(deref(upCIR)),
			"car_dn_cir_kbps": nullIfEmpty(deref(dnCIR)),
			"online_time_sec": nullIfEmpty(onlineTimeSec),
			"online_time":     nullIfEmpty(onlineTime),
			"auth_state":      nullIfEmpty(deref(auth)),
			"status":          status,
			"is_online":       isOnline,
			"first_seen_at":   firstSeen.UTC().Format(time.RFC3339Nano),
			"last_seen_at":    lastSeen.UTC().Format(time.RFC3339Nano),
			"known_login":     true,
		}
		if lastOffline != nil {
			m["last_offline_at"] = lastOffline.UTC().Format(time.RFC3339Nano)
		}
		out = append(out, EnrichSessionMaps([]map[string]any{m})[0])
	}
	return out, latest, onlineN, len(out), rows.Err()
}

// ListLoginEvents devolve histórico de conexões de um login.
func ListLoginEvents(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, login string, limit int) ([]map[string]any, error) {
	if pool == nil {
		return nil, nil
	}
	login = strings.TrimSpace(login)
	if login == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := pool.Query(ctx, `
		SELECT connected_at, disconnected_at, duration_sec, session_index, vlan, ipv4, ipv6, ipv6_pd,
		       mac, interface_name, domain, ip_type, car_up_cir_kbps, car_dn_cir_kbps, online_time_sec
		FROM bng_login_events
		WHERE device_id=$1 AND lower(login)=lower($2)
		ORDER BY connected_at DESC
		LIMIT $3
	`, deviceID, login, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]map[string]any, 0)
	for rows.Next() {
		var (
			connectedAt                                                                time.Time
			disconnectedAt                                                             *time.Time
			durationSec, onlineSec                                                     *int64
			sessIdx, vlan, ipv4, ipv6, ipv6PD, mac, iface, domain, ipType, upCIR, dnCIR *string
		)
		if err := rows.Scan(
			&connectedAt, &disconnectedAt, &durationSec, &sessIdx, &vlan, &ipv4, &ipv6, &ipv6PD,
			&mac, &iface, &domain, &ipType, &upCIR, &dnCIR, &onlineSec,
		); err != nil {
			return nil, err
		}
		m := map[string]any{
			"connected_at": connectedAt.UTC().Format(time.RFC3339Nano),
			"online":       disconnectedAt == nil,
		}
		if disconnectedAt != nil {
			m["disconnected_at"] = disconnectedAt.UTC().Format(time.RFC3339Nano)
		}
		dur := int64(0)
		if durationSec != nil && *durationSec > 0 {
			dur = *durationSec
		} else if disconnectedAt != nil {
			dur = int64(disconnectedAt.Sub(connectedAt).Seconds())
		} else {
			dur = int64(time.Since(connectedAt).Seconds())
		}
		if dur < 0 {
			dur = 0
		}
		m["duration_sec"] = dur
		m["duration_display"] = FormatDurationSeconds(int(dur))
		if onlineSec != nil && *onlineSec > 0 {
			m["online_time_sec"] = *onlineSec
			m["online_time_display"] = FormatDurationSeconds(int(*onlineSec))
		}
		setPtr := func(key string, p *string) {
			if p != nil && strings.TrimSpace(*p) != "" {
				m[key] = strings.TrimSpace(*p)
			}
		}
		setPtr("session_index", sessIdx)
		setPtr("vlan", vlan)
		setPtr("ipv4", ipv4)
		setPtr("ipv6", ipv6)
		setPtr("ipv6_pd", ipv6PD)
		setPtr("mac", mac)
		setPtr("interface", iface)
		setPtr("domain", domain)
		setPtr("ip_type", ipType)
		setPtr("car_up_cir_kbps", upCIR)
		setPtr("car_dn_cir_kbps", dnCIR)
		if upCIR != nil {
			if n, ok := parseIntMetric(*upCIR); ok {
				m["car_up_cir_display"] = FormatKbitRate(n)
			}
		}
		if dnCIR != nil {
			if n, ok := parseIntMetric(*dnCIR); ok {
				m["car_dn_cir_display"] = FormatKbitRate(n)
			}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func nullIfEmpty(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}

func loadKnownLoginsTx(ctx context.Context, tx pgx.Tx, deviceID uuid.UUID) (map[string]knownLoginRow, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, login, is_online, current_event_id FROM bng_known_logins WHERE device_id=$1
	`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]knownLoginRow)
	for rows.Next() {
		var r knownLoginRow
		if err := rows.Scan(&r.ID, &r.Login, &r.IsOnline, &r.CurrentEventID); err != nil {
			return nil, err
		}
		out[strings.ToLower(strings.TrimSpace(r.Login))] = r
	}
	return out, rows.Err()
}

func sessionAttrs(row SessionRow) (onlineSec *int64, idx, vlan, ipv4, ipv6, ipv6PD, mac, iface, domain, ipType, ipTypeRaw, upCIR, dnCIR, auth string) {
	idx = strings.TrimSpace(row.Index)
	vlan = strings.TrimSpace(row.VLAN)
	ipv4 = strings.TrimSpace(row.IPv4)
	ipv6 = strings.TrimSpace(row.IPv6)
	ipv6PD = strings.TrimSpace(row.IPv6PD)
	mac = strings.TrimSpace(row.MAC)
	iface = strings.TrimSpace(row.Interface)
	domain = strings.TrimSpace(row.Domain)
	ipType = strings.TrimSpace(row.IPType)
	ipTypeRaw = strings.TrimSpace(row.IPTypeRaw)
	upCIR = strings.TrimSpace(row.CarUpCIRKbps)
	dnCIR = strings.TrimSpace(row.CarDnCIRKbps)
	auth = strings.TrimSpace(row.AuthState)
	if n, ok := parseIntMetric(row.OnlineTimeSec); ok && n > 0 {
		v := int64(n)
		onlineSec = &v
	}
	return
}

func openOrRefreshKnownLogin(ctx context.Context, tx pgx.Tx, deviceID uuid.UUID, row SessionRow, now time.Time, prev knownLoginRow) error {
	onlineSec, idx, vlan, ipv4, ipv6, ipv6PD, mac, iface, domain, ipType, ipTypeRaw, upCIR, dnCIR, auth := sessionAttrs(row)
	var eventID int64
	err := tx.QueryRow(ctx, `
		INSERT INTO bng_login_events (
			device_id, login, connected_at, session_index, vlan, ipv4, ipv6, ipv6_pd, mac,
			interface_name, domain, ip_type, car_up_cir_kbps, car_dn_cir_kbps, online_time_sec
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING id
	`, deviceID, row.Login, now, nullStr(idx), nullStr(vlan), nullStr(ipv4), nullStr(ipv6), nullStr(ipv6PD),
		nullStr(mac), nullStr(iface), nullStr(domain), nullStr(ipType), nullStr(upCIR), nullStr(dnCIR), onlineSec,
	).Scan(&eventID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO bng_known_logins (
			device_id, login, is_online, first_seen_at, last_seen_at, session_index, vlan, ipv4, ipv6, ipv6_pd,
			mac, interface_name, domain, ip_type, ip_type_raw, car_up_cir_kbps, car_dn_cir_kbps, online_time_sec,
			auth_state, current_event_id, updated_at
		) VALUES ($1,$2,true,$3,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$3)
		ON CONFLICT (device_id, login) DO UPDATE SET
			is_online = true,
			last_seen_at = EXCLUDED.last_seen_at,
			session_index = EXCLUDED.session_index,
			vlan = EXCLUDED.vlan,
			ipv4 = EXCLUDED.ipv4,
			ipv6 = EXCLUDED.ipv6,
			ipv6_pd = EXCLUDED.ipv6_pd,
			mac = EXCLUDED.mac,
			interface_name = EXCLUDED.interface_name,
			domain = EXCLUDED.domain,
			ip_type = EXCLUDED.ip_type,
			ip_type_raw = EXCLUDED.ip_type_raw,
			car_up_cir_kbps = EXCLUDED.car_up_cir_kbps,
			car_dn_cir_kbps = EXCLUDED.car_dn_cir_kbps,
			online_time_sec = EXCLUDED.online_time_sec,
			auth_state = EXCLUDED.auth_state,
			current_event_id = EXCLUDED.current_event_id,
			updated_at = EXCLUDED.updated_at
	`, deviceID, row.Login, now, nullStr(idx), nullStr(vlan), nullStr(ipv4), nullStr(ipv6), nullStr(ipv6PD),
		nullStr(mac), nullStr(iface), nullStr(domain), nullStr(ipType), nullStr(ipTypeRaw), nullStr(upCIR), nullStr(dnCIR),
		onlineSec, nullStr(auth), eventID)
	_ = prev
	return err
}

func refreshKnownLoginOnline(ctx context.Context, tx pgx.Tx, deviceID uuid.UUID, row SessionRow, now time.Time, prev knownLoginRow) error {
	onlineSec, idx, vlan, ipv4, ipv6, ipv6PD, mac, iface, domain, ipType, ipTypeRaw, upCIR, dnCIR, auth := sessionAttrs(row)
	_, err := tx.Exec(ctx, `
		UPDATE bng_known_logins SET
			is_online = true,
			last_seen_at = $3,
			session_index = $4,
			vlan = $5,
			ipv4 = $6,
			ipv6 = $7,
			ipv6_pd = $8,
			mac = $9,
			interface_name = $10,
			domain = $11,
			ip_type = $12,
			ip_type_raw = $13,
			car_up_cir_kbps = $14,
			car_dn_cir_kbps = $15,
			online_time_sec = $16,
			auth_state = $17,
			updated_at = $3
		WHERE device_id=$1 AND id=$2
	`, deviceID, prev.ID, now, nullStr(idx), nullStr(vlan), nullStr(ipv4), nullStr(ipv6), nullStr(ipv6PD),
		nullStr(mac), nullStr(iface), nullStr(domain), nullStr(ipType), nullStr(ipTypeRaw), nullStr(upCIR), nullStr(dnCIR),
		onlineSec, nullStr(auth))
	if err != nil {
		return err
	}
	if prev.CurrentEventID == nil {
		return nil
	}
	_, err = tx.Exec(ctx, `
		UPDATE bng_login_events SET
			session_index = $2,
			vlan = $3,
			ipv4 = $4,
			ipv6 = $5,
			ipv6_pd = $6,
			mac = $7,
			interface_name = $8,
			domain = $9,
			ip_type = $10,
			car_up_cir_kbps = $11,
			car_dn_cir_kbps = $12,
			online_time_sec = $13
		WHERE id=$1 AND disconnected_at IS NULL
	`, *prev.CurrentEventID, nullStr(idx), nullStr(vlan), nullStr(ipv4), nullStr(ipv6), nullStr(ipv6PD),
		nullStr(mac), nullStr(iface), nullStr(domain), nullStr(ipType), nullStr(upCIR), nullStr(dnCIR), onlineSec)
	return err
}

func markKnownLoginOfflineTx(ctx context.Context, tx pgx.Tx, deviceID uuid.UUID, prev knownLoginRow, now time.Time) error {
	if prev.CurrentEventID != nil {
		var connectedAt time.Time
		var onlineSec *int64
		err := tx.QueryRow(ctx, `
			SELECT connected_at, online_time_sec FROM bng_login_events WHERE id=$1
		`, *prev.CurrentEventID).Scan(&connectedAt, &onlineSec)
		if err != nil && err != pgx.ErrNoRows {
			return err
		}
		if err == nil {
			d := int64(now.Sub(connectedAt).Seconds())
			if d < 0 {
				d = 0
			}
			if onlineSec != nil && *onlineSec > d {
				d = *onlineSec
			}
			_, err = tx.Exec(ctx, `
				UPDATE bng_login_events
				SET disconnected_at=$2, duration_sec=$3
				WHERE id=$1 AND disconnected_at IS NULL
			`, *prev.CurrentEventID, now, d)
			if err != nil {
				return err
			}
		}
	}
	_, err := tx.Exec(ctx, `
		UPDATE bng_known_logins SET
			is_online = false,
			last_offline_at = $3,
			current_event_id = NULL,
			updated_at = $3
		WHERE device_id=$1 AND id=$2
	`, deviceID, prev.ID, now)
	return err
}

func nullStr(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}
