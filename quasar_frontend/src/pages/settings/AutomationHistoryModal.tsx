import { useMemo, useState } from "react";
import { apiFetch } from "../../lib/api";
import { useQuery } from "@tanstack/react-query";
import { queryKeys } from "../../lib/queryKeys";
import {
  AUTOMATION_JOB_OPTIONS,
  automationActorLabel,
  automationOriginLabel,
  formatAutomationWhen,
  type AutomationHistoryRow,
} from "./AutomationsHistoryTable";

function summaryTotals(row: AutomationHistoryRow): string {
  const s = row.summary;
  if (!s || typeof s !== "object") return "—";
  const parts: string[] = [];
  if (row.job_type === "alerts_digest") {
    if (s.alerts_open != null) parts.push(`alertas abertos: ${s.alerts_open}`);
    if (s.incidents_open != null) parts.push(`incidentes: ${s.incidents_open}`);
  } else if (row.job_type === "commercial_report") {
    if (s.clients_total != null) parts.push(`clientes: ${s.clients_total}`);
    if (s.period) parts.push(`período: ${String(s.period)}`);
  } else if (row.job_type === "onu_monthly_report") {
    if (s.onu_total != null) parts.push(`ONUs: ${s.onu_total}`);
  } else if (row.job_type === "database_backup") {
    if (s.object_key) parts.push(String(s.object_key));
  }
  return parts.length ? parts.join(" · ") : "—";
}

type Props = { open: boolean; onClose: () => void };

export function AutomationHistoryModal({ open, onClose }: Props) {
  const [jobFilter, setJobFilter] = useState("");
  const [triggerFilter, setTriggerFilter] = useState("");
  const [search, setSearch] = useState("");
  const [fromDate, setFromDate] = useState("");
  const [toDate, setToDate] = useState("");

  const params = useMemo(() => {
    const p = new URLSearchParams();
    if (jobFilter) p.set("job_type", jobFilter);
    if (triggerFilter) p.set("trigger_type", triggerFilter);
    if (search.trim()) p.set("q", search.trim());
    if (fromDate) {
      const d = new Date(`${fromDate}T00:00:00`);
      if (!Number.isNaN(d.getTime())) p.set("from", d.toISOString());
    }
    if (toDate) {
      const d = new Date(`${toDate}T23:59:59`);
      if (!Number.isNaN(d.getTime())) p.set("to", d.toISOString());
    }
    p.set("limit", "300");
    return p.toString();
  }, [jobFilter, triggerFilter, search, fromDate, toDate]);

  const hist = useQuery({
    queryKey: [...queryKeys.automationHistory, "modal", params],
    queryFn: () => apiFetch<{ items: AutomationHistoryRow[] }>(`/api/v1/settings/automation/history?${params}`),
    enabled: open,
    refetchInterval: open ? 5000 : false,
  });

  const items = hist.data?.items ?? [];

  if (!open) return null;

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="modal automation-history-modal"
        style={{ width: "75vw", height: "75vh", maxWidth: "none", maxHeight: "none", display: "flex", flexDirection: "column" }}
        onClick={(e) => e.stopPropagation()}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", flexShrink: 0 }}>
          <div>
            <h2 style={{ margin: 0 }}>Histórico de execuções</h2>
            <p style={{ margin: "6px 0 0", fontSize: 13, color: "var(--muted)" }}>
              Auditoria de execuções agendadas (sistema) e manuais (utilizador).
            </p>
          </div>
          <button type="button" className="btn" onClick={onClose}>
            Fechar
          </button>
        </div>

        <div className="settings-fields-grid" style={{ marginTop: 14, flexShrink: 0, gridTemplateColumns: "repeat(auto-fill, minmax(160px, 1fr))" }}>
          <label>
            <span className="label" style={{ display: "block", marginBottom: 4 }}>
              Automação
            </span>
            <select className="input" value={jobFilter} onChange={(e) => setJobFilter(e.target.value)}>
              {AUTOMATION_JOB_OPTIONS.map((o) => (
                <option key={o.value || "all"} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          </label>
          <label>
            <span className="label" style={{ display: "block", marginBottom: 4 }}>
              Origem
            </span>
            <select className="input" value={triggerFilter} onChange={(e) => setTriggerFilter(e.target.value)}>
              <option value="">Todas</option>
              <option value="scheduled">Sistema</option>
              <option value="manual">Manual</option>
            </select>
          </label>
          <label>
            <span className="label" style={{ display: "block", marginBottom: 4 }}>
              De (data)
            </span>
            <input className="input" type="date" value={fromDate} onChange={(e) => setFromDate(e.target.value)} />
          </label>
          <label>
            <span className="label" style={{ display: "block", marginBottom: 4 }}>
              Até (data)
            </span>
            <input className="input" type="date" value={toDate} onChange={(e) => setToDate(e.target.value)} />
          </label>
          <label style={{ gridColumn: "1 / -1" }}>
            <span className="label" style={{ display: "block", marginBottom: 4 }}>
              Pesquisar
            </span>
            <input
              className="input"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Status, utilizador, erro, período…"
            />
          </label>
        </div>

        <div style={{ flex: 1, minHeight: 0, marginTop: 12, overflow: "auto", border: "1px solid var(--border)", borderRadius: 8 }}>
          {hist.isLoading ? (
            <p style={{ padding: 16 }}>A carregar…</p>
          ) : hist.isError ? (
            <div className="msg msg--err" style={{ margin: 12 }}>
              {(hist.error as Error).message}
            </div>
          ) : items.length === 0 ? (
            <p style={{ padding: 16, color: "var(--muted)" }}>Nenhuma execução encontrada.</p>
          ) : (
            <table className="table table--compact" style={{ width: "100%" }}>
              <thead>
                <tr>
                  <th>Data / hora</th>
                  <th>Automação</th>
                  <th>Origem</th>
                  <th>Utilizador</th>
                  <th>Status</th>
                  <th>Totais</th>
                </tr>
              </thead>
              <tbody>
                {items.map((row) => (
                  <tr key={row.id}>
                    <td style={{ whiteSpace: "nowrap" }}>{formatAutomationWhen(row.started_at)}</td>
                    <td>{row.job_label || row.job_type}</td>
                    <td style={{ fontSize: 12 }}>{automationOriginLabel(row)}</td>
                    <td style={{ fontSize: 12 }}>{automationActorLabel(row)}</td>
                    <td>
                      <span className={row.ok ? "badge badge--ok" : "badge badge--err"}>{row.ok ? "Sucesso" : "Erro"}</span>
                      <div style={{ fontSize: 12, marginTop: 4 }}>{row.status_message}</div>
                      {row.error_message ? (
                        <div style={{ fontSize: 11, color: "var(--danger, #c44)", marginTop: 2 }}>{row.error_message}</div>
                      ) : null}
                    </td>
                    <td style={{ fontSize: 12 }}>{summaryTotals(row)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  );
}
