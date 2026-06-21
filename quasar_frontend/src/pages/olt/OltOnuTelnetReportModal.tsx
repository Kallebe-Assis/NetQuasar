import { Copy } from "lucide-react";
import { useMemo, useState } from "react";
import {
  buildTelnetReportSections,
  buildUnifiedReportTable,
  formatTelnetReportPlainText,
  type OltTelnetReportStep,
} from "../../lib/oltTelnetReportFormat";
import { EM_DASH } from "../../lib/formatDisplay";

type Props = {
  open: boolean;
  loading: boolean;
  title: string;
  steps: OltTelnetReportStep[];
  onClose: () => void;
};

export function OltOnuTelnetReportModal({ open, loading, title, steps, onClose }: Props) {
  const [showRaw, setShowRaw] = useState(false);
  const sections = useMemo(() => buildTelnetReportSections(steps), [steps]);
  const rows = useMemo(() => buildUnifiedReportTable(sections), [sections]);

  if (!open) return null;

  async function copyAll() {
    const text = formatTelnetReportPlainText(rows, title);
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      /* ignore */
    }
  }

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={() => !loading && onClose()}>
      <div
        className="modal modal--wide"
        role="dialog"
        aria-modal="true"
        style={{ maxWidth: 640, maxHeight: "92vh", display: "flex", flexDirection: "column" }}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 12, flexShrink: 0 }}>
          <div style={{ minWidth: 0 }}>
            <h3 style={{ marginTop: 0, marginBottom: 4 }}>Relatório ONU</h3>
            <p style={{ margin: 0, fontSize: 12, color: "var(--muted)" }}>{title}</p>
          </div>
          <button
            type="button"
            className="btn btn--icon"
            title="Copiar"
            disabled={loading || rows.length === 0}
            onClick={() => void copyAll()}
          >
            <Copy size={16} />
          </button>
        </div>

        {loading ? (
          <p style={{ margin: "16px 0" }}>A recolher dados via telnet…</p>
        ) : (
          <div style={{ overflow: "auto", flex: 1, minHeight: 0, marginTop: 14 }}>
            {rows.length === 0 ? (
              <p style={{ color: "var(--muted)", fontSize: 12 }}>Nenhum dado estruturado na resposta.</p>
            ) : (
              <div className="table-wrap">
                <table className="conn-table" style={{ fontSize: 13 }}>
                  <tbody>
                    {rows.map((r) => (
                      <tr key={r.label}>
                        <td
                          style={{
                            width: "38%",
                            color: "var(--muted)",
                            fontWeight: 500,
                            verticalAlign: "top",
                            paddingRight: 16,
                            borderBottom: "1px solid var(--border)",
                          }}
                        >
                          {r.label}
                        </td>
                        <td style={{ wordBreak: "break-word", verticalAlign: "top", borderBottom: "1px solid var(--border)" }}>
                          {r.value || EM_DASH}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}

            {showRaw && sections.length > 0 ? (
              <details open style={{ marginTop: 16 }}>
                <summary style={{ cursor: "pointer", fontSize: 12, color: "var(--muted)", marginBottom: 8 }}>
                  Saída telnet original
                </summary>
                {sections.map((sec) => (
                  <div key={sec.id} style={{ marginBottom: 12 }}>
                    <div className="mono" style={{ fontSize: 10, color: "var(--muted)", marginBottom: 4 }}>
                      {sec.command}
                    </div>
                    <pre
                      style={{
                        margin: 0,
                        fontSize: 10,
                        whiteSpace: "pre-wrap",
                        wordBreak: "break-word",
                        background: "var(--surface-2, rgba(0,0,0,0.04))",
                        padding: 10,
                        borderRadius: 6,
                        border: "1px solid var(--border)",
                      }}
                    >
                      {sec.rawClean || EM_DASH}
                    </pre>
                  </div>
                ))}
              </details>
            ) : null}
          </div>
        )}

        <div className="row" style={{ justifyContent: "space-between", marginTop: 14, flexShrink: 0 }}>
          <label style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 12, cursor: "pointer" }}>
            <input type="checkbox" checked={showRaw} disabled={loading} onChange={(e) => setShowRaw(e.target.checked)} />
            Mostrar saída original
          </label>
          <button type="button" className="btn btn--primary" disabled={loading} onClick={onClose}>
            Fechar
          </button>
        </div>
      </div>
    </div>
  );
}
