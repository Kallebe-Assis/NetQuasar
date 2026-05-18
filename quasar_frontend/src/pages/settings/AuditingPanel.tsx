import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { apiFetch } from "../../lib/api";
import { prettyAuditDiff } from "../../lib/auditDisplay";

type AuditRow = {
  id: number;
  entity_type: string;
  entity_id: string;
  action: string;
  actor?: string | null;
  before_data?: Record<string, unknown> | null;
  after_data?: Record<string, unknown> | null;
  created_at: string;
};

export function AuditingPanel() {
  const [sub, setSub] = useState<"ops" | "db">("ops");
  const [lim, setLim] = useState("150");
  const ops = useQuery({
    queryKey: ["ops-audit-settings", lim],
    queryFn: () => apiFetch<{ items: AuditRow[] }>(`/api/v1/ops/audit?limit=${encodeURIComponent(lim)}`),
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

  if (sub === "ops" && ops.isLoading && !ops.data) return <p>A carregar auditoria…</p>;
  if (sub === "db" && db.isLoading && !db.data) return <p>A carregar ligações…</p>;
  if (q.isError) return <div className="msg msg--err">{(q.error as Error).message}</div>;

  return (
    <div className="card">
      <h2>Auditoria</h2>
      <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>
        Registo de alterações no sistema (equipamentos, monitoramento, relatórios Telegram, utilizadores, etc.).
      </p>
      <div className="tabs" style={{ marginBottom: 12 }}>
        <button type="button" className={sub === "ops" ? "active" : ""} onClick={() => setSub("ops")}>
          Operações
        </button>
        <button type="button" className={sub === "db" ? "active" : ""} onClick={() => setSub("db")}>
          Ligações à base de dados
        </button>
      </div>
      <div className="row" style={{ marginBottom: 8, gap: 8, flexWrap: "wrap" }}>
        <input className="input" style={{ width: 80 }} value={lim} onChange={(e) => setLim(e.target.value)} aria-label="Limite de registos" />
        <button type="button" className="btn" onClick={() => void q.refetch()}>
          Atualizar
        </button>
      </div>
      {sub === "ops" ? (
        <div className="table-wrap" style={{ maxHeight: "min(60vh, 520px)", overflow: "auto" }}>
          <table style={{ fontSize: 11 }}>
            <thead>
              <tr>
                <th>Quando</th>
                <th>Entidade</th>
                <th>ID</th>
                <th>Ação</th>
                <th>Ator</th>
                <th>Detalhe</th>
              </tr>
            </thead>
            <tbody>
              {(ops.data?.items ?? []).map((a) => (
                <tr key={a.id}>
                  <td className="mono">{new Date(a.created_at).toLocaleString("pt-PT")}</td>
                  <td>{a.entity_type}</td>
                  <td className="mono">{a.entity_id}</td>
                  <td>{a.action}</td>
                  <td>{a.actor ?? "—"}</td>
                  <td>
                    <details>
                      <summary style={{ cursor: "pointer", color: "var(--muted)" }}>ver</summary>
                      <pre className="mono" style={{ margin: 0, fontSize: 10, whiteSpace: "pre-wrap" }}>
                        {prettyAuditDiff(a.before_data, a.after_data)}
                      </pre>
                    </details>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <div className="table-wrap" style={{ maxHeight: "min(50vh, 400px)", overflow: "auto" }}>
          <table>
            <thead>
              <tr>
                <th>ID</th>
                <th>Quando</th>
                <th>OK</th>
                <th>Fase</th>
                <th>Mensagem</th>
              </tr>
            </thead>
            <tbody>
              {(db.data?.logs ?? []).map((l) => (
                <tr key={l.id}>
                  <td>{l.id}</td>
                  <td className="mono">{l.created_at}</td>
                  <td>{l.ok ? "sim" : "não"}</td>
                  <td>{l.phase}</td>
                  <td>{l.message}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
