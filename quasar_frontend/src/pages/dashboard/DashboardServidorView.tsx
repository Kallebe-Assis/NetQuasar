import { useQuery } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";
import { formatAlertDateTimePt, formatRelativeTimeAgoPt } from "../../lib/alertLabels";
import { apiFetch } from "../../lib/api";
import { fmtInt, Section } from "./dashboardShared";

type HealthApp = {
  started_at?: string;
  uptime_seconds?: number;
  go_version?: string;
  goroutines?: number;
  mem_alloc_mb?: number;
  mem_sys_mb?: number;
  gc_runs?: number;
  cpu_num?: number;
};
type HealthDB = { acquired_conns?: number; idle_conns?: number; total_conns?: number; max_conns?: number };
type HealthCycles = {
  latency?: string | null;
  telemetry?: string | null;
  interface_snapshot?: string | null;
  olt_if_derived?: string | null;
  bng?: string | null;
  pipeline?: string | null;
};
type HealthMonitoring = {
  ok?: boolean;
  is_running?: boolean;
  monitoring_mode?: string;
  last_cycle_at?: string | null;
  last_cycle_ok_count?: number;
  last_cycle_fail_count?: number;
  current_activity?: string | null;
  cycles?: HealthCycles;
};
type HealthIntegration = {
  integration_id: string;
  name: string;
  slug: string;
  enabled: boolean;
  calls_24h: number;
  ok_24h: number;
  avg_latency_ms?: number | null;
  max_latency_ms?: number | null;
  last_run_at?: string | null;
  last_ok?: boolean | null;
};
type HealthAlerts = { ok?: boolean; critical?: number; warning?: number; info?: number };
type HealthPanel = {
  generated_at?: string;
  app?: HealthApp;
  db?: HealthDB;
  monitoring?: HealthMonitoring;
  integrations?: HealthIntegration[];
  alerts?: HealthAlerts;
};

function formatUptime(seconds?: number): string {
  const s = Number(seconds);
  if (!Number.isFinite(s) || s < 0) return "—";
  const days = Math.floor(s / 86400);
  const hours = Math.floor((s % 86400) / 3600);
  const mins = Math.floor((s % 3600) / 60);
  const parts: string[] = [];
  if (days > 0) parts.push(`${days}d`);
  if (hours > 0 || days > 0) parts.push(`${hours}h`);
  parts.push(`${mins}m`);
  return parts.join(" ");
}

const CYCLE_LABELS: Record<keyof HealthCycles, string> = {
  latency: "Latência (ping)",
  telemetry: "Telemetria",
  interface_snapshot: "Interfaces SNMP",
  olt_if_derived: "OLT (derivado)",
  bng: "BNG",
  pipeline: "Pipeline geral",
};

function CycleRow({ label, at }: { label: string; at?: string | null }) {
  const stale = at ? Date.now() - new Date(at).getTime() > 30 * 60 * 1000 : true;
  return (
    <div className="integration-consult-card__service-cell">
      <span className="integration-consult-card__label">{label}</span>
      <span className={`integration-consult-card__value${stale ? "" : ""}`} style={{ color: stale ? "var(--warn)" : "var(--text)" }}>
        {at ? formatRelativeTimeAgoPt(at) : "sem registo"}
      </span>
    </div>
  );
}

export function DashboardServidorView() {
  const q = useQuery({
    queryKey: ["system-health-panel"],
    queryFn: () => apiFetch<HealthPanel>("/api/v1/system/health-panel"),
    refetchInterval: 30_000,
    staleTime: 15_000,
  });

  if (q.isLoading && !q.data) return <p>A carregar saúde do sistema…</p>;
  if (q.isError) return <div className="msg msg--err">{(q.error as Error).message}</div>;

  const app = q.data?.app;
  const db = q.data?.db;
  const mon = q.data?.monitoring;
  const integrations = q.data?.integrations ?? [];
  const alerts = q.data?.alerts;

  return (
    <>
      <div className="row" style={{ justifyContent: "space-between", alignItems: "center", marginBottom: 8, flexWrap: "wrap", gap: 8 }}>
        <span className="mono" style={{ fontSize: 11, color: "var(--muted)" }}>
          {q.data?.generated_at ? `Gerado: ${formatAlertDateTimePt(q.data.generated_at)}` : ""}
          {q.isFetching ? " · a atualizar…" : ""}
        </span>
        <button type="button" className="btn" onClick={() => void q.refetch()} disabled={q.isFetching}>
          <RefreshCw size={14} style={{ marginRight: 6, verticalAlign: -2 }} />
          Atualizar agora
        </button>
      </div>

      <div className="dashboard-kpi-row">
        <div className="stat">
          <div className="stat__k">Tempo ativo (app)</div>
          <div className="stat__v" style={{ fontSize: 18 }}>{formatUptime(app?.uptime_seconds)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Goroutines</div>
          <div className="stat__v">{fmtInt(app?.goroutines)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Memória (alocada)</div>
          <div className="stat__v">{app?.mem_alloc_mb ?? "—"} MB</div>
        </div>
        <div className="stat">
          <div className="stat__k">Ligações à BD</div>
          <div className="stat__v">
            {fmtInt(db?.acquired_conns)} / {fmtInt(db?.max_conns)}
          </div>
        </div>
        <div className="stat">
          <div className="stat__k">Monitorização</div>
          <div className="stat__v" style={{ fontSize: 14 }}>
            {mon?.is_running ? <span className="badge badge--ok">A correr</span> : <span className="badge badge--off">Parada</span>}
          </div>
        </div>
        <div className="stat">
          <div className="stat__k">Alertas críticos</div>
          <div className="stat__v" style={{ color: (alerts?.critical ?? 0) > 0 ? "var(--err)" : undefined }}>
            {fmtInt(alerts?.critical)}
          </div>
        </div>
        <div className="stat">
          <div className="stat__k">Alertas de atenção</div>
          <div className="stat__v" style={{ color: (alerts?.warning ?? 0) > 0 ? "var(--warn)" : undefined }}>
            {fmtInt(alerts?.warning)}
          </div>
        </div>
      </div>

      <Section
        id="sec-servidor-runtime"
        title="Processo e base de dados"
        subtitle="Estado interno do binário Go que serve o NetQuasar — memória, goroutines e pool de ligações à base de dados."
      >
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))", gap: 14, fontSize: 12 }}>
          <div>
            <h3 style={{ fontSize: 12, color: "var(--muted)", marginBottom: 6 }}>Aplicação</h3>
            <div className="integration-consult-card__meta" style={{ flexDirection: "column", gap: 4 }}>
              <span>Versão Go: <span className="mono">{app?.go_version ?? "—"}</span></span>
              <span>CPUs: <span className="mono">{fmtInt(app?.cpu_num)}</span></span>
              <span>Ciclos GC: <span className="mono">{fmtInt(app?.gc_runs)}</span></span>
              <span>Memória do sistema: <span className="mono">{app?.mem_sys_mb ?? "—"} MB</span></span>
              <span>Iniciado: <span className="mono">{app?.started_at ? formatAlertDateTimePt(app.started_at) : "—"}</span></span>
            </div>
          </div>
          <div>
            <h3 style={{ fontSize: 12, color: "var(--muted)", marginBottom: 6 }}>Pool de ligações (BD)</h3>
            <div className="integration-consult-card__meta" style={{ flexDirection: "column", gap: 4 }}>
              <span>Em uso: <span className="mono">{fmtInt(db?.acquired_conns)}</span></span>
              <span>Ociosas: <span className="mono">{fmtInt(db?.idle_conns)}</span></span>
              <span>Total abertas: <span className="mono">{fmtInt(db?.total_conns)}</span></span>
              <span>Máximo configurado: <span className="mono">{fmtInt(db?.max_conns)}</span></span>
            </div>
          </div>
        </div>
      </Section>

      <Section
        id="sec-servidor-worker"
        title="Worker de monitorização"
        subtitle="Última execução de cada ciclo de recolha. Um ciclo sem registo há mais de 30 minutos aparece destacado — normalmente sinal de monitorização parada ou de um tipo de coleta desativado."
      >
        {mon?.ok === false ? (
          <div className="msg msg--err">Não foi possível ler o estado do worker.</div>
        ) : (
          <>
            <div className="row" style={{ gap: 12, marginBottom: 10, flexWrap: "wrap" }}>
              <span style={{ fontSize: 12 }}>
                Modo: <span className="mono">{mon?.monitoring_mode ?? "—"}</span>
              </span>
              <span style={{ fontSize: 12 }}>
                Último ciclo OK/falha: <span className="mono">{fmtInt(mon?.last_cycle_ok_count)}</span> / <span className="mono">{fmtInt(mon?.last_cycle_fail_count)}</span>
              </span>
              {mon?.current_activity ? (
                <span style={{ fontSize: 12 }}>
                  Atividade atual: <span className="mono">{mon.current_activity}</span>
                </span>
              ) : null}
            </div>
            <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))", gap: 8 }}>
              {(Object.keys(CYCLE_LABELS) as (keyof HealthCycles)[]).map((k) => (
                <CycleRow key={k} label={CYCLE_LABELS[k]} at={mon?.cycles?.[k]} />
              ))}
            </div>
          </>
        )}
      </Section>

      <Section
        id="sec-servidor-integrations"
        title="Integrações externas"
        subtitle="Últimas 24h de chamadas registadas por integração (Hubsoft, IXC, etc.) — taxa de sucesso e latência média/máxima."
      >
        {integrations.length === 0 ? (
          <p style={{ color: "var(--muted)", fontSize: 13 }}>Nenhuma integração configurada.</p>
        ) : (
          <div className="table-wrap">
            <table style={{ fontSize: 11 }}>
              <thead>
                <tr>
                  <th>Integração</th>
                  <th>Estado</th>
                  <th className="mono">Chamadas (24h)</th>
                  <th className="mono">Sucesso</th>
                  <th className="mono">Latência média</th>
                  <th className="mono">Latência máx.</th>
                  <th>Última execução</th>
                </tr>
              </thead>
              <tbody>
                {integrations.map((it) => {
                  const successPct = it.calls_24h > 0 ? Math.round((it.ok_24h / it.calls_24h) * 100) : null;
                  return (
                    <tr key={it.integration_id}>
                      <td>{it.name}</td>
                      <td>
                        {!it.enabled ? (
                          <span className="badge badge--off">Desligada</span>
                        ) : it.last_ok === false ? (
                          <span className="badge badge--err">Última falhou</span>
                        ) : (
                          <span className="badge badge--ok">OK</span>
                        )}
                      </td>
                      <td className="mono">{fmtInt(it.calls_24h)}</td>
                      <td className="mono">{successPct !== null ? `${successPct}%` : "—"}</td>
                      <td className="mono">{it.avg_latency_ms != null ? `${Math.round(it.avg_latency_ms)} ms` : "—"}</td>
                      <td className="mono">{it.max_latency_ms != null ? `${it.max_latency_ms} ms` : "—"}</td>
                      <td className="mono" style={{ fontSize: 10, color: "var(--muted)" }}>
                        {it.last_run_at ? formatRelativeTimeAgoPt(it.last_run_at) : "sem registo"}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </Section>
    </>
  );
}
