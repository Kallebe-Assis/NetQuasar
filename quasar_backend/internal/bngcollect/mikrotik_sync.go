package bngcollect

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/netquasar/netquasar/quasar_backend/internal/mikrotikcollect"
)

// Este ficheiro é o caminho MikroTik para "Sessões PPPoE" — a coleta em known_logins.go e o
// resto do pacote bngcollect foram construídos para OIDs proprietários Huawei (hwAccessTable,
// ver metrics.go), que simplesmente não existem num concentrador MikroTik. Aqui em vez disso
// coleta-se por telnet CLI RouterOS (/ppp active + /ppp secret, ver mikrotikcollect/ppp_telnet.go)
// e alimenta-se exactamente as mesmas tabelas (bng_known_logins/bng_login_events) — a aba Sessões
// PPPoE, a pesquisa híbrida e o filtro online/offline continuam a ser os mesmos, só a fonte muda.

// SessionRowFromMikrotikActive converte uma entrada de /ppp active print detail (RouterOS) no
// SessionRow genérico do pacote — usado tanto na sincronização periódica/manual (lista completa)
// como na pesquisa ao vivo de um único login (mikrotikSessionLookup, em handlers_bng.go).
func SessionRowFromMikrotikActive(e mikrotikcollect.PPPActiveEntry) SessionRow {
	row := SessionRow{
		// RouterOS reatribui o "#" de /ppp active print a cada leitura (não é estável entre
		// coletas) — usa-se o próprio login como índice, estável enquanto a sessão durar.
		Index:  e.Name,
		Login:  e.Name,
		MAC:    e.CallerID,
		IPv4:   e.Address,
		Status: "Up",
	}
	if e.UptimeSec > 0 {
		row.OnlineTimeSec = strconv.FormatInt(e.UptimeSec, 10)
	}
	if e.Uptime != "" {
		row.OnlineTime = e.Uptime
	}
	return row
}

func mikrotikActiveToSessionRows(entries []mikrotikcollect.PPPActiveEntry) []SessionRow {
	out := make([]SessionRow, 0, len(entries))
	for _, e := range entries {
		out = append(out, SessionRowFromMikrotikActive(e))
	}
	return out
}

// syncMikrotikSecretRoster regista o comentário/rótulo do cliente de cada login PPP configurado
// (/ppp secret print) — não existe por SNMP, só por CLI. Cria uma linha offline em
// bng_known_logins para login nunca visto online (dá o roster completo, mesmo sem nenhuma conexão
// ainda) e, em quem já existe, só toca no comentário — nunca em is_online/last_seen_at/etc., que
// pertencem ao sync online/offline (SyncKnownLogins, chamado antes desta função).
func syncMikrotikSecretRoster(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, secrets []mikrotikcollect.PPPSecretEntry) error {
	if pool == nil || len(secrets) == 0 {
		return nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	now := time.Now().UTC()
	for _, sec := range secrets {
		login := strings.TrimSpace(sec.Name)
		if login == "" {
			continue
		}
		comment := nullStr(sec.Comment)
		if _, err := tx.Exec(ctx, `
			INSERT INTO bng_known_logins (device_id, login, is_online, first_seen_at, last_seen_at, comment, updated_at)
			VALUES ($1, $2, false, $3, $3, $4, $3)
			ON CONFLICT (device_id, login) DO UPDATE SET comment = $4, updated_at = $3
			WHERE bng_known_logins.comment IS DISTINCT FROM $4
		`, deviceID, login, now, comment); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// CollectAndSyncMikrotikPPPoE consulta /ppp active e /ppp secret por telnet e sincroniza
// bng_known_logins/bng_login_events — equivalente MikroTik ao ciclo SNMP Huawei
// (CollectSessionsWalk + SyncKnownLogins). Usado quando o equipamento BNG é identificado como
// MikroTik (mikrotikcollect.IsMikrotikDevice): SNMP Huawei não existe nesse hardware, mas
// RouterOS expõe tudo por CLI — e ainda traz o comentário/rótulo do cliente, que não existe em
// SNMP nenhum. O roster (/ppp secret) é melhor-esforço: se falhar, ainda sincroniza as sessões
// activas — só o comentário/roster completo fica desactualizado até à próxima leitura.
func CollectAndSyncMikrotikPPPoE(ctx context.Context, pool *pgxpool.Pool, deviceID uuid.UUID, host string, creds mikrotikcollect.TelnetCredentials, timeout time.Duration) (activeCount, knownCount int, err error) {
	active, secrets, err := mikrotikcollect.CollectPPPoESessions(ctx, host, creds, timeout)
	if err != nil {
		return 0, 0, err
	}
	rows := mikrotikActiveToSessionRows(active)
	if err := SyncKnownLogins(ctx, pool, deviceID, rows, ""); err != nil {
		return len(rows), 0, err
	}
	if len(secrets) > 0 {
		if err := syncMikrotikSecretRoster(ctx, pool, deviceID, secrets); err != nil {
			return len(rows), len(secrets), err
		}
	}
	return len(rows), len(secrets), nil
}
