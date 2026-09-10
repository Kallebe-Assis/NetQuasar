import { createPortal } from "react-dom";
import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "../lib/api";
import { formatCollectedPt } from "../lib/deviceReportHelpers";
import { EM_DASH, formatSnmpMetricCell } from "../lib/formatDisplay";
import { EmptyState } from "./EmptyState";

type OnuHistoryRow = {
  collected_at: string;
  row: {
    online?: boolean;
    rx_pwr?: string;
    tx_pwr?: string;
    voltage?: string;
    temp?: string;
    model?: string;
    serial?: string;
  };
};

function cell(v: unknown): string {
  if (v == null || (typeof v === "string" && v.trim() === "")) return EM_DASH;
  return formatSnmpMetricCell(v);
}

/** Últimas 10 colectas de uma ONU (pon+onu) — botão "Histórico" (3 pontinhos) na aba de ONUs.
 * Só existe histórico a partir de quando esta funcionalidade foi ligada (internal/oltsamples,
 * RecordOnuHistory) — não há como recuperar colectas anteriores, o snapshot antigo (olt_snapshots)
 * sempre foi só "o estado actual", sem histórico por ONU. */
export function OnuHistoryModal({
  open,
  deviceId,
  pon,
  onu,
  label,
  onClose,
}: {
  open: boolean;
  deviceId: string;
  pon: number | null;
  onu: number | null;
  label?: string;
  onClose: () => void;
}) {
  const q = useQuery({
    queryKey: ["olt-onu-history", deviceId, pon, onu],
    queryFn: () =>
      apiFetch<{ history: OnuHistoryRow[] }>(`/api/v1/olt/devices/${deviceId}/onu-history?pon=${pon}&onu=${onu}`),
    enabled: open && !!deviceId && pon != null && onu != null,
  });

  if (!open) return null;

  return createPortal(
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="modal"
        role="dialog"
        aria-modal="true"
        style={{ maxWidth: 640 }}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start" }}>
          <div>
            <h3 style={{ margin: 0 }}>Histórico da ONU</h3>
            <p style={{ fontSize: 12, color: "var(--muted)", margin: "2px 0 0" }}>
              {label ?? `PON ${pon} · ONU ${onu}`} — últimas {q.data?.history.length ?? 10} colecta(s)
            </p>
          </div>
          <button type="button" className="btn btn--icon" aria-label="Fechar" onClick={onClose}>
            ×
          </button>
        </div>

        {q.isLoading ? (
          <p style={{ fontSize: 12, color: "var(--muted)" }}>A carregar histórico…</p>
        ) : q.isError ? (
          <div className="msg msg--err">{(q.error as Error).message}</div>
        ) : !q.data || q.data.history.length === 0 ? (
          <EmptyState
            title="Sem histórico ainda para esta ONU"
            hint="O histórico começa a partir da próxima colecta — não há como recuperar colectas anteriores a esta funcionalidade."
          />
        ) : (
          <div className="table-wrap" style={{ marginTop: 10 }}>
            <table style={{ fontSize: 12, width: "100%" }}>
              <thead>
                <tr>
                  <th>Coleta</th>
                  <th>Status</th>
                  <th className="mono">RX</th>
                  <th className="mono">TX</th>
                  <th className="mono">Voltagem</th>
                  <th className="mono">Temp.</th>
                </tr>
              </thead>
              <tbody>
                {q.data.history.map((h, i) => (
                  <tr key={`${h.collected_at}-${i}`}>
                    <td className="mono">{formatCollectedPt(h.collected_at)}</td>
                    <td>
                      {h.row.online ? (
                        <span className="badge badge--ok">Online</span>
                      ) : (
                        <span className="badge badge--err">Offline</span>
                      )}
                    </td>
                    <td className="mono">{cell(h.row.rx_pwr)}</td>
                    <td className="mono">{cell(h.row.tx_pwr)}</td>
                    <td className="mono">{cell(h.row.voltage)}</td>
                    <td className="mono">{cell(h.row.temp)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        <div className="row" style={{ justifyContent: "flex-end", marginTop: 14 }}>
          <button type="button" className="btn" onClick={onClose}>
            Fechar
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
}
