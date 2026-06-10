import {
  formatAuditActor,
  formatAuditDetailPreview,
  formatAuditEntityType,
  formatAuditAction,
  formatAuditSummary,
  resolveAuditEntityName,
  type AuditRowView,
} from "../lib/auditPresentation";
import { prettyAuditDiff } from "../lib/auditDisplay";

type Props = {
  rows: AuditRowView[];
  showDetailColumn?: boolean;
  emptyMessage?: string;
};

export function AuditLogTable({ rows, showDetailColumn = true, emptyMessage }: Props) {
  if (rows.length === 0) {
    return <p style={{ color: "var(--muted)", fontSize: 12 }}>{emptyMessage ?? "Nenhum registo."}</p>;
  }
  return (
    <div className="table-wrap audit-log-table" style={{ maxHeight: "min(65vh, 560px)", overflow: "auto" }}>
      <table style={{ fontSize: 12 }}>
        <thead>
          <tr>
            <th>Quando</th>
            <th>Resumo</th>
            <th>Entidade</th>
            <th>Ação</th>
            <th>Utilizador</th>
            {showDetailColumn ? <th>Detalhe</th> : null}
          </tr>
        </thead>
        <tbody>
          {rows.map((a) => {
            const preview = formatAuditDetailPreview(a);
            return (
              <tr key={a.id ?? `${a.created_at}-${a.entity_type}-${a.entity_id}-${a.action}`}>
                <td className="mono" style={{ whiteSpace: "nowrap", fontSize: 11 }}>
                  {new Date(a.created_at).toLocaleString("pt-PT")}
                </td>
                <td style={{ minWidth: 200 }}>{formatAuditSummary(a)}</td>
                <td>
                  <div>{resolveAuditEntityName(a)}</div>
                  <div style={{ fontSize: 10, color: "var(--muted)" }}>{formatAuditEntityType(a.entity_type)}</div>
                </td>
                <td>{formatAuditAction(a.action, a.entity_type)}</td>
                <td>{formatAuditActor(a.actor)}</td>
                {showDetailColumn ? (
                  <td style={{ maxWidth: 280 }}>
                    {preview ? <div style={{ fontSize: 11, color: "var(--muted)", marginBottom: 4 }}>{preview}</div> : null}
                    {(a.before_data && Object.keys(a.before_data).length > 0) ||
                    (a.after_data && Object.keys(a.after_data).length > 0) ? (
                      <details>
                        <summary style={{ cursor: "pointer", color: "var(--muted)", fontSize: 11 }}>Alterações</summary>
                        <pre className="mono" style={{ margin: "6px 0 0", fontSize: 10, whiteSpace: "pre-wrap" }}>
                          {prettyAuditDiff(a.before_data, a.after_data)}
                        </pre>
                      </details>
                    ) : (
                      !preview && <span style={{ color: "var(--muted)" }}>—</span>
                    )}
                  </td>
                ) : null}
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
