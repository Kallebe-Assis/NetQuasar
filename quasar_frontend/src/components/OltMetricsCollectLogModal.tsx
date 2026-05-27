import { useMemo, useState } from "react";

export type MetricsWalkRow = Record<string, unknown>;

const METRIC_LABELS: Record<string, string> = {
  serial: "Serial",
  status: "Estado",
  rx_power: "RX da ONU",
  tx_power: "TX da ONU",
  pon_rx_power: "RX da PON (OLT)",
  pon_tx_power: "TX da PON (OLT)",
  temperature: "Temperatura",
  model: "Modelo",
};

function fmtMs(ms: unknown): string {
  const n = Number(ms);
  if (!Number.isFinite(n)) return "—";
  if (n < 1000) return `${n} ms`;
  return `${(n / 1000).toFixed(1)} s`;
}

function fmtCell(v: unknown): string {
  if (v == null || v === "") return "—";
  if (typeof v === "boolean") return v ? "sim" : "não";
  return String(v);
}

function rowSearchText(row: MetricsWalkRow): string {
  return [
    row.metric,
    row.oid,
    row.offline_oid,
    row.var_count,
    row.matched_rows,
    row.note,
    row.status,
  ]
    .map((x) => String(x ?? "").toLowerCase())
    .join(" ");
}

type Props = {
  open: boolean;
  onClose: () => void;
  walkRows: MetricsWalkRow[];
  elapsedMs?: number | null;
  note?: string | null;
};

export function OltMetricsCollectLogModal({ open, onClose, walkRows, elapsedMs, note }: Props) {
  const [view, setView] = useState<"tabela" | "json">("tabela");
  const [search, setSearch] = useState("");

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return walkRows;
    return walkRows.filter((row) => rowSearchText(row).includes(q));
  }, [walkRows, search]);

  if (!open) return null;

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="modal modal--wide"
        role="dialog"
        aria-modal="true"
        aria-labelledby="olt-metrics-log-title"
        onMouseDown={(e) => e.stopPropagation()}
        style={{ maxWidth: 1180, maxHeight: "90vh", overflow: "auto" }}
      >
        <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", gap: 8, flexWrap: "wrap" }}>
          <div>
            <h2 id="olt-metrics-log-title" style={{ margin: 0, fontSize: 16 }}>
              Logs da coleta SNMP (métricas)
            </h2>
            <p style={{ fontSize: 11, color: "var(--muted)", margin: "4px 0 0" }}>
              Um snmpwalk por métrica configurada no perfil OLT (serial, estado, RX, TX, etc.).
            </p>
          </div>
          <button type="button" className="btn" onClick={onClose}>
            Fechar
          </button>
        </div>

        <div className="row" style={{ gap: 8, marginTop: 12, flexWrap: "wrap", alignItems: "center" }}>
          <div className="tabs" style={{ margin: 0 }}>
            <button type="button" className={view === "tabela" ? "active" : ""} onClick={() => setView("tabela")}>
              Tabela
            </button>
            <button type="button" className={view === "json" ? "active" : ""} onClick={() => setView("json")}>
              JSON
            </button>
          </div>
          <input
            className="input"
            type="search"
            placeholder="Pesquisar métrica, OID, nota…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            style={{ flex: "1 1 220px", minWidth: 200, maxWidth: 420 }}
          />
          {Number.isFinite(Number(elapsedMs)) ? (
            <span style={{ fontSize: 12, color: "var(--muted)" }}>
              Duração total: <strong>{fmtMs(elapsedMs)}</strong>
            </span>
          ) : null}
        </div>

        {note ? (
          <p className="msg msg--off" style={{ fontSize: 11, marginTop: 10, marginBottom: 0 }}>
            {note}
          </p>
        ) : null}

        {walkRows.length === 0 ? (
          <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 14 }}>
            Sem registos de coleta por métricas. Execute «Atualizar ONUs» com um perfil que tenha MIBs SNMP configuradas.
          </p>
        ) : view === "json" ? (
          <pre
            className="mono"
            style={{
              fontSize: 11,
              marginTop: 12,
              padding: 10,
              borderRadius: 6,
              background: "var(--surface-2, rgba(0,0,0,0.04))",
              overflow: "auto",
              maxHeight: "60vh",
            }}
          >
            {JSON.stringify(filtered, null, 2)}
          </pre>
        ) : (
          <div className="table-wrap" style={{ marginTop: 12, maxHeight: "58vh", overflow: "auto" }}>
            <table className="table table--compact" style={{ width: "100%", fontSize: 12 }}>
              <thead>
                <tr>
                  <th>Métrica</th>
                  <th>OID</th>
                  <th className="mono">Vars</th>
                  <th className="mono">Corresp.</th>
                  <th className="mono">Tempo</th>
                  <th>Nota</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((row, i) => {
                  const metricKey = String(row.metric ?? "");
                  const oid =
                    row.offline_oid && String(row.offline_oid).trim()
                      ? `${fmtCell(row.oid)} · off: ${fmtCell(row.offline_oid)}`
                      : fmtCell(row.oid);
                  const status = String(row.status ?? "");
                  const errNote = row.note ? String(row.note) : "";
                  const noteText =
                    status === "empty"
                      ? [errNote, "walk vazio"].filter(Boolean).join(" · ")
                      : row.truncated
                        ? [errNote, "truncado"].filter(Boolean).join(" · ")
                        : errNote || "—";
                  return (
                    <tr key={`${metricKey}-${i}`}>
                      <td>
                        {METRIC_LABELS[metricKey] ?? (metricKey || "—")}
                        {status && status !== "ok" ? (
                          <span className="badge badge--err" style={{ marginLeft: 6, fontSize: 10 }}>
                            {status}
                          </span>
                        ) : null}
                      </td>
                      <td className="mono" style={{ maxWidth: 360, wordBreak: "break-all" }}>
                        {oid}
                      </td>
                      <td className="mono">{fmtCell(row.var_count)}</td>
                      <td className="mono">{fmtCell(row.matched_rows)}</td>
                      <td className="mono">{fmtMs(row.elapsed_ms)}</td>
                      <td style={{ fontSize: 11, maxWidth: 280, wordBreak: "break-word", color: errNote ? "var(--danger)" : undefined }}>
                        {noteText}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
            {filtered.length === 0 && search.trim() ? (
              <p style={{ fontSize: 12, color: "var(--muted)", padding: 8 }}>Nenhuma linha corresponde à pesquisa.</p>
            ) : null}
          </div>
        )}
      </div>
    </div>
  );
}
