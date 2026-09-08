import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "../../lib/api";
import { fmtInt, Section } from "./dashboardShared";

type DeviceRow = { device_id: string; device_name: string; total: number; online: number; offline: number };
type PppoeSessionsData = {
  days: number;
  totals: { known_logins: number; online: number; offline: number };
  by_device: DeviceRow[];
  events_period: { days: number; connects: number; disconnects: number };
  avg_online_session_sec?: number;
};

/** "2d 3h" / "3h 12min" / "45min" / "—" — mesmo padrão de leitura rápida usado no resto do
 * produto. Dias aparecem para sessões muito longas (ex.: monitoramento ficou desligado por um
 * tempo e o login nunca foi re-verificado — a média fica realmente grande, e mostrar em dias
 * ajuda a perceber isso de relance em vez de um número gigante de horas). */
function formatDurationShort(sec: number | undefined): string {
  if (sec == null || !Number.isFinite(sec) || sec <= 0) return "—";
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}min`;
  return `${m}min`;
}

/** Aba "Sessões PPPoE" do Dashboard — totais/médias do inventário online/offline dos BNGs (ver
 * bng_known_logins, alimentado pelo ciclo rápido de presença) + eventos de conexão/desconexão
 * num período próprio (independente do período geral do Dashboard — este é sobre live status). */
export function DashboardPppoeView() {
  const [days, setDays] = useState(7);

  const q = useQuery({
    queryKey: ["dashboard-pppoe-sessions", days],
    queryFn: () => apiFetch<PppoeSessionsData>(`/api/v1/dashboard/pppoe-sessions?days=${days}`),
    staleTime: 60_000,
  });

  const totals = q.data?.totals;
  const occupancyPct = totals && totals.known_logins > 0 ? (totals.online / totals.known_logins) * 100 : 0;

  return (
    <Section
      id="sec-pppoe"
      title="Sessões PPPoE"
      subtitle="Status online/offline dos logins conhecidos em todos os BNGs (aba BNG → Sessões PPPoE), actualizado pelo ciclo rápido de presença."
    >
      {q.isLoading ? (
        <p style={{ color: "var(--muted)", fontSize: 13 }}>A carregar…</p>
      ) : q.isError ? (
        <div className="msg msg--err">Falha ao carregar dados de sessões PPPoE.</div>
      ) : (totals?.known_logins ?? 0) === 0 ? (
        <p style={{ color: "var(--muted)", fontSize: 13 }}>
          Nenhum login conhecido ainda. Cadastre um BNG e aguarde o ciclo de coleta de sessões.
        </p>
      ) : (
        <>
          <div className="row" style={{ gap: 12, marginBottom: 14, flexWrap: "wrap" }}>
            <div className="stat" style={{ minWidth: 130 }}>
              <div className="stat__k">Logins conhecidos</div>
              <div className="stat__v">{fmtInt(totals?.known_logins)}</div>
            </div>
            <div className="stat" style={{ minWidth: 110 }}>
              <div className="stat__k">Online agora</div>
              <div className="stat__v" style={{ color: "var(--ok, #3fb950)" }}>
                {fmtInt(totals?.online)}
              </div>
            </div>
            <div className="stat" style={{ minWidth: 110 }}>
              <div className="stat__k">Offline agora</div>
              <div className="stat__v" style={{ color: "var(--err, #f85149)" }}>
                {fmtInt(totals?.offline)}
              </div>
            </div>
            <div className="stat" style={{ minWidth: 110 }}>
              <div className="stat__k">Ocupação</div>
              <div className="stat__v">{occupancyPct.toFixed(1)}%</div>
            </div>
            <div className="stat" style={{ minWidth: 150 }}>
              <div className="stat__k">Duração média da sessão (online agora)</div>
              <div className="stat__v">{formatDurationShort(q.data?.avg_online_session_sec)}</div>
            </div>
          </div>

          <div className="row" style={{ alignItems: "center", gap: 8, marginBottom: 10 }}>
            <span style={{ fontSize: 12, color: "var(--muted)" }}>Eventos de conexão/desconexão em</span>
            <select className="select" style={{ fontSize: 12 }} value={days} onChange={(e) => setDays(Number(e.target.value) || 7)}>
              {[1, 3, 7, 14, 30].map((d) => (
                <option key={d} value={d}>
                  {d === 1 ? "24h" : `${d} dias`}
                </option>
              ))}
            </select>
          </div>
          <div className="row" style={{ gap: 12, marginBottom: 16, flexWrap: "wrap" }}>
            <div className="stat" style={{ minWidth: 110 }}>
              <div className="stat__k">Conexões</div>
              <div className="stat__v">{fmtInt(q.data?.events_period.connects)}</div>
            </div>
            <div className="stat" style={{ minWidth: 110 }}>
              <div className="stat__k">Desconexões</div>
              <div className="stat__v">{fmtInt(q.data?.events_period.disconnects)}</div>
            </div>
          </div>

          <p style={{ fontSize: 12, color: "var(--muted)", margin: "0 0 8px" }}>Por BNG:</p>
          <div className="table-wrap">
            <table style={{ fontSize: 12 }}>
              <thead>
                <tr>
                  <th>BNG</th>
                  <th className="mono">Total</th>
                  <th className="mono">Online</th>
                  <th className="mono">Offline</th>
                  <th className="mono">% online</th>
                </tr>
              </thead>
              <tbody>
                {(q.data?.by_device ?? []).map((d) => (
                  <tr key={d.device_id}>
                    <td>{d.device_name}</td>
                    <td className="mono">{fmtInt(d.total)}</td>
                    <td className="mono">{fmtInt(d.online)}</td>
                    <td className="mono">{fmtInt(d.offline)}</td>
                    <td className="mono">{d.total > 0 ? `${((d.online / d.total) * 100).toFixed(1)}%` : "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </Section>
  );
}
