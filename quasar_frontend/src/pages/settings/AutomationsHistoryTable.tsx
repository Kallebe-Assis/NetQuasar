import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { apiFetch } from "../../lib/api";
import { queryKeys } from "../../lib/queryKeys";

export type AutomationHistoryRow = {
  id: string;
  job_type: string;
  job_label: string;
  actor: string;
  trigger_type: string;
  origin_label?: string;
  started_at: string;
  finished_at: string;
  ok: boolean;
  status_message: string;
  error_message?: string | null;
  summary?: Record<string, unknown> | null;
  run_key?: string | null;
  triggered_by?: string;
  triggered_by_user_id?: string;
  triggered_by_email?: string;
  triggered_by_display_name?: string;
};

export const AUTOMATION_JOB_OPTIONS = [
  { value: "", label: "Todas as automações" },
  { value: "alerts_digest", label: "Resumo de alertas" },
  { value: "commercial_report", label: "Base comercial" },
  { value: "onu_monthly_report", label: "Relatório ONU mensal" },
  { value: "bng_stats_report", label: "Totais BNG" },
  { value: "database_backup", label: "Backup PostgreSQL (B2)" },
];

export function formatAutomationWhen(iso: string): string {
  try {
    return new Date(iso).toLocaleString("pt-BR", { dateStyle: "short", timeStyle: "medium" });
  } catch {
    return iso;
  }
}

export function automationOriginLabel(row: AutomationHistoryRow): string {
  if (row.origin_label) return row.origin_label;
  return row.trigger_type === "scheduled" ? "Sistema (agendado)" : "Manual";
}

export function automationActorLabel(row: AutomationHistoryRow): string {
  if (row.trigger_type === "scheduled" || row.actor === "SISTEMA" || row.actor === "scheduler") {
    return "Sistema";
  }
  const name = row.triggered_by_display_name?.trim() || row.triggered_by?.trim() || row.actor?.trim();
  const email = row.triggered_by_email?.trim();
  if (name && email && name !== email) return `${name} (${email})`;
  return name || email || "—";
}

function summaryTotals(row: AutomationHistoryRow): string {
  const s = row.summary;
  if (!s || typeof s !== "object") return "—";
  const parts: string[] = [];
  if (row.job_type === "alerts_digest") {
    if (s.alerts_open != null) parts.push(`alertas abertos: ${s.alerts_open}`);
    if (s.incidents_open != null) parts.push(`incidentes: ${s.incidents_open}`);
    if (s.alerts_closed_24h != null) parts.push(`resolvidos 24h: ${s.alerts_closed_24h}`);
  } else if (row.job_type === "commercial_report") {
    if (s.clients_total != null) parts.push(`clientes: ${s.clients_total}`);
    if (s.localities_count != null) parts.push(`localidades: ${s.localities_count}`);
    if (s.period) parts.push(`período: ${String(s.period)}`);
  } else if (row.job_type === "onu_monthly_report") {
    if (s.onu_total != null) parts.push(`ONUs: ${s.onu_total}`);
    if (s.onu_online != null) parts.push(`online: ${s.onu_online}`);
    if (s.olts_refreshed != null) parts.push(`OLTs OK: ${s.olts_refreshed}`);
  } else if (row.job_type === "database_backup") {
    if (s.object_key) parts.push(String(s.object_key));
    if (s.size_bytes != null) parts.push(`${(Number(s.size_bytes) / (1024 * 1024)).toFixed(1)} MB`);
  } else if (row.job_type === "bng_stats_report") {
    if (s.pppoe != null) parts.push(`PPPoE: ${s.pppoe}`);
    if (s.total != null) parts.push(`total: ${s.total}`);
  }
  return parts.length ? parts.join(" · ") : "—";
}

type Props = {
  jobType?: string;
  limit?: number;
  compact?: boolean;
  showJobColumn?: boolean;
  selectedId?: string | null;
  onSelect?: (row: AutomationHistoryRow) => void;
};

export function AutomationsHistoryTable({
  jobType = "",
  limit = 100,
  compact = false,
  showJobColumn = true,
  selectedId,
  onSelect,
}: Props) {
  const [triggerFilter, setTriggerFilter] = useState("");
  const [search, setSearch] = useState("");

  const params = useMemo(() => {
    const p = new URLSearchParams();
    if (jobType) p.set("job_type", jobType);
    if (triggerFilter) p.set("trigger_type", triggerFilter);
    if (search.trim()) p.set("q", search.trim());
    p.set("limit", String(limit));
    return p.toString();
  }, [jobType, triggerFilter, search, limit]);

  const hist = useQuery({
    queryKey: [...queryKeys.automationHistory, params],
    queryFn: () => apiFetch<{ items: AutomationHistoryRow[] }>(`/api/v1/settings/automation/history?${params}`),
    refetchInterval: 5000,
  });

  const items = hist.data?.items ?? [];

  return (
    <div className="automations-history">
      <div className="automations-history__filters">
        <select className="input" value={triggerFilter} onChange={(e) => setTriggerFilter(e.target.value)} aria-label="Origem">
          <option value="">Todas as origens</option>
          <option value="scheduled">Sistema (agendado)</option>
          <option value="manual">Manual</option>
        </select>
        <input
          className="input"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Pesquisar status, utilizador, erro…"
        />
      </div>
      {hist.isLoading ? (
        <p style={{ padding: 12, margin: 0, fontSize: 13 }}>A carregar…</p>
      ) : hist.isError ? (
        <div className="msg msg--err" style={{ margin: 8 }}>
          {(hist.error as Error).message}
        </div>
      ) : items.length === 0 ? (
        <p style={{ padding: 12, margin: 0, color: "var(--muted)", fontSize: 13 }}>Nenhuma execução encontrada.</p>
      ) : (
        <div className="automations-history__scroll">
          <table className={`table table--compact${compact ? " automations-history__table--compact" : ""}`}>
            <thead>
              <tr>
                <th>Data / hora</th>
                {showJobColumn ? <th>Automação</th> : null}
                <th>Origem</th>
                <th>Utilizador</th>
                <th>Status</th>
                {!compact ? <th>Detalhe</th> : null}
              </tr>
            </thead>
            <tbody>
              {items.map((row) => (
                <tr
                  key={row.id}
                  className={selectedId === row.id ? "is-selected" : undefined}
                  style={onSelect ? { cursor: "pointer" } : undefined}
                  onClick={() => onSelect?.(row)}
                >
                  <td style={{ whiteSpace: "nowrap" }}>{formatAutomationWhen(row.started_at)}</td>
                  {showJobColumn ? <td>{row.job_label || row.job_type}</td> : null}
                  <td>
                    <span className={`automations-pill automations-pill--${row.trigger_type === "scheduled" ? "system" : "manual"}`}>
                      {automationOriginLabel(row)}
                    </span>
                  </td>
                  <td style={{ fontSize: 12 }}>{automationActorLabel(row)}</td>
                  <td>
                    <span className={row.ok ? "badge badge--ok" : "badge badge--err"}>{row.ok ? "Sucesso" : "Erro"}</span>
                    <div style={{ fontSize: 11, marginTop: 4, color: "var(--muted)" }}>{row.status_message}</div>
                    {row.error_message ? (
                      <div style={{ fontSize: 11, color: "var(--danger, #c44)", marginTop: 2 }}>{row.error_message}</div>
                    ) : null}
                  </td>
                  {!compact ? <td style={{ fontSize: 12 }}>{summaryTotals(row)}</td> : null}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

export function AutomationsLogDetail({ row }: { row: AutomationHistoryRow | null }) {
  if (!row) {
    return <p style={{ color: "var(--muted)", fontSize: 13, margin: 0 }}>Seleccione uma execução no histórico para ver o log.</p>;
  }
  const lines = [
    `Início: ${formatAutomationWhen(row.started_at)}`,
    `Fim: ${formatAutomationWhen(row.finished_at)}`,
    `Origem: ${automationOriginLabel(row)}`,
    `Utilizador: ${automationActorLabel(row)}`,
    `Estado: ${row.ok ? "Sucesso" : "Erro"}`,
    `Mensagem: ${row.status_message || "—"}`,
  ];
  if (row.error_message) lines.push(`Erro: ${row.error_message}`);
  if (row.run_key) lines.push(`Chave: ${row.run_key}`);
  if (row.summary && typeof row.summary === "object") {
    lines.push(`Resumo: ${JSON.stringify(row.summary, null, 2)}`);
  }
  return (
    <pre className="automations-log-pre" aria-label="Log da execução">
      {lines.join("\n")}
    </pre>
  );
}
