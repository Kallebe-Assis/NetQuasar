import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { AuditLogTable } from "../../components/AuditLogTable";
import { apiFetch } from "../../lib/api";
import { formatAuditAction, formatAuditEntityType } from "../../lib/auditPresentation";
import type { AuditRowView } from "../../lib/auditPresentation";

const ENTITY_FILTER_OPTIONS = [
  "",
  "device",
  "pop",
  "monitoring_runtime",
  "client_connection",
  "commercial_monthly_record",
  "network_tool",
  "integration",
  "user",
  "automation_onu_report",
  "nightly_collection",
];

const ACTION_FILTER_OPTIONS = [
  "",
  "create",
  "patch",
  "delete",
  "start",
  "stop",
  "import_csv",
  "import",
  "refresh_olt",
  "refresh_interfaces",
  "collect_telemetry",
  "ping_run",
  "executed",
  "run",
];

export function AuditingPanel() {
  const [sub, setSub] = useState<"ops" | "db">("ops");
  const [lim, setLim] = useState("300");
  const [searchQ, setSearchQ] = useState("");
  const [entityType, setEntityType] = useState("");
  const [actionFilter, setActionFilter] = useState("");
  const [actorFilter, setActorFilter] = useState("");
  const [applied, setApplied] = useState({ q: "", entityType: "", action: "", actor: "" });

  const opsUrl = useMemo(() => {
    const p = new URLSearchParams();
    p.set("limit", lim.trim() || "300");
    if (applied.q) p.set("q", applied.q);
    if (applied.entityType) p.set("entity_type", applied.entityType);
    if (applied.action) p.set("action", applied.action);
    if (applied.actor) p.set("actor", applied.actor);
    return `/api/v1/ops/audit?${p.toString()}`;
  }, [lim, applied]);

  const ops = useQuery({
    queryKey: ["ops-audit-settings", opsUrl],
    queryFn: () => apiFetch<{ items: AuditRowView[] }>(opsUrl),
    enabled: sub === "ops",
  });
  const db = useQuery({
    queryKey: ["settings-db-logs", lim],
    queryFn: () =>
      apiFetch<{ logs: { id: number; created_at: string; ok: boolean; phase: string; message: string }[] }>(
        `/api/v1/settings/database/logs?limit=${encodeURIComponent(lim)}`,
      ),
    enabled: sub === "db",
  });
  const q = sub === "ops" ? ops : db;

  const applyFilters = () => {
    setApplied({
      q: searchQ.trim(),
      entityType: entityType.trim(),
      action: actionFilter.trim(),
      actor: actorFilter.trim(),
    });
  };

  const clearFilters = () => {
    setSearchQ("");
    setEntityType("");
    setActionFilter("");
    setActorFilter("");
    setApplied({ q: "", entityType: "", action: "", actor: "" });
  };

  if (sub === "ops" && ops.isLoading && !ops.data) return <p>A carregar auditoria…</p>;
  if (sub === "db" && db.isLoading && !db.data) return <p>A carregar ligações…</p>;
  if (q.isError) return <div className="msg msg--err">{(q.error as Error).message}</div>;

  return (
    <div className="card">
      <h2>Auditoria</h2>
      <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>
        Histórico completo de alterações e operações no sistema. Use os filtros para consultar equipamento, utilizador ou ação específica.
      </p>
      <div className="tabs" style={{ marginBottom: 12 }}>
        <button type="button" className={sub === "ops" ? "active" : ""} onClick={() => setSub("ops")}>
          Operações
        </button>
        <button type="button" className={sub === "db" ? "active" : ""} onClick={() => setSub("db")}>
          Ligações à base de dados
        </button>
      </div>
      {sub === "ops" ? (
        <>
          <div className="row" style={{ marginBottom: 10, gap: 8, flexWrap: "wrap", alignItems: "flex-end" }}>
            <label style={{ display: "flex", flexDirection: "column", gap: 4, flex: "1 1 200px" }}>
              <span style={{ fontSize: 11, color: "var(--muted)" }}>Buscar (equipamento, utilizador, ação…)</span>
              <input
                className="input"
                value={searchQ}
                onChange={(e) => setSearchQ(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && applyFilters()}
                placeholder="Ex.: OLT central, collect_telemetry, key:ab…"
              />
            </label>
            <label style={{ display: "flex", flexDirection: "column", gap: 4, minWidth: 140 }}>
              <span style={{ fontSize: 11, color: "var(--muted)" }}>Entidade</span>
              <select className="input" value={entityType} onChange={(e) => setEntityType(e.target.value)}>
                <option value="">Todas</option>
                {ENTITY_FILTER_OPTIONS.filter(Boolean).map((et) => (
                  <option key={et} value={et}>
                    {formatAuditEntityType(et)}
                  </option>
                ))}
              </select>
            </label>
            <label style={{ display: "flex", flexDirection: "column", gap: 4, minWidth: 140 }}>
              <span style={{ fontSize: 11, color: "var(--muted)" }}>Ação</span>
              <select className="input" value={actionFilter} onChange={(e) => setActionFilter(e.target.value)}>
                <option value="">Todas</option>
                {ACTION_FILTER_OPTIONS.filter(Boolean).map((a) => (
                  <option key={a} value={a}>
                    {formatAuditAction(a)}
                  </option>
                ))}
              </select>
            </label>
            <label style={{ display: "flex", flexDirection: "column", gap: 4, minWidth: 120 }}>
              <span style={{ fontSize: 11, color: "var(--muted)" }}>Utilizador</span>
              <input
                className="input"
                value={actorFilter}
                onChange={(e) => setActorFilter(e.target.value)}
                placeholder="key: ou sistema"
              />
            </label>
            <label style={{ display: "flex", flexDirection: "column", gap: 4, width: 72 }}>
              <span style={{ fontSize: 11, color: "var(--muted)" }}>Limite</span>
              <input className="input" value={lim} onChange={(e) => setLim(e.target.value)} aria-label="Limite de registos" />
            </label>
            <button type="button" className="btn btn--primary" onClick={applyFilters}>
              Buscar
            </button>
            <button type="button" className="btn" onClick={clearFilters}>
              Limpar
            </button>
            <button type="button" className="btn" onClick={() => void ops.refetch()}>
              Atualizar
            </button>
          </div>
          <p style={{ fontSize: 11, color: "var(--muted)", margin: "0 0 8px" }}>
            {(ops.data?.items ?? []).length} registo(s)
            {applied.q || applied.entityType || applied.action || applied.actor ? " (filtros activos)" : " — histórico completo"}
          </p>
          <AuditLogTable
            rows={ops.data?.items ?? []}
            emptyMessage="Nenhum registo encontrado com os filtros actuais."
          />
        </>
      ) : (
        <>
          <div className="row" style={{ marginBottom: 8, gap: 8, flexWrap: "wrap" }}>
            <input className="input" style={{ width: 80 }} value={lim} onChange={(e) => setLim(e.target.value)} aria-label="Limite de registos" />
            <button type="button" className="btn" onClick={() => void db.refetch()}>
              Atualizar
            </button>
          </div>
          <div className="table-wrap" style={{ maxHeight: "min(50vh, 400px)", overflow: "auto" }}>
            <table>
              <thead>
                <tr>
                  <th>Quando</th>
                  <th>OK</th>
                  <th>Fase</th>
                  <th>Mensagem</th>
                </tr>
              </thead>
              <tbody>
                {(db.data?.logs ?? []).map((l) => (
                  <tr key={l.id}>
                    <td className="mono">{new Date(l.created_at).toLocaleString("pt-PT")}</td>
                    <td>{l.ok ? "sim" : "não"}</td>
                    <td>{l.phase}</td>
                    <td>{l.message}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  );
}
