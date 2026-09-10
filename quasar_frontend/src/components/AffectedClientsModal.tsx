import { createPortal } from "react-dom";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Send } from "lucide-react";
import { apiFetch, downloadBlob } from "../lib/api";
import { buildExcelCsvBlob } from "../lib/excelCsv";
import { useAppToast } from "../lib/appToast";
import { EmptyState } from "./EmptyState";

type AffectedClient = { serial: string; client_name: string; pon: number; onu: number };
type AffectedClientsResp = {
  ok: boolean;
  olt_label: string;
  scope_label: string;
  onu_count: number;
  clients: AffectedClient[];
  unlinked_count: number;
};

/** Modal "Clientes afetados" (3 pontinhos → alerta de OLT offline ou PON DOWN): lista os
 * clientes com ONU vinculada dentro do alcance do alerta (a OLT inteira, ou só a PON caída),
 * com opção de exportar CSV e/ou enviar a mesma lista pelo bot de monitorização no Telegram. */
export function AffectedClientsModal({
  open,
  alertId,
  onClose,
}: {
  open: boolean;
  alertId: string | null;
  onClose: () => void;
}) {
  const { push: pushToast } = useAppToast();

  const q = useQuery({
    queryKey: ["alert-affected-clients", alertId],
    queryFn: () => apiFetch<AffectedClientsResp>(`/api/v1/alerts/${alertId}/affected-clients`),
    enabled: open && !!alertId,
  });

  const sendMut = useMutation({
    mutationFn: () =>
      apiFetch<{ ok: boolean; client_count: number; onu_count: number }>(
        `/api/v1/alerts/${alertId}/affected-clients-telegram`,
        { method: "POST", json: {} },
      ),
    onSuccess: (res) => {
      pushToast({ tone: "ok", text: `Enviado no Telegram: ${res.client_count} cliente(s) afetado(s) (${res.onu_count} ONU(s)).` });
    },
    onError: (e: unknown) => {
      pushToast({ tone: "err", text: e instanceof Error ? e.message : "Falha ao enviar clientes afetados." });
    },
  });

  function exportCsv() {
    if (!q.data) return;
    const rows: string[][] = [["Cliente", "Serial", "PON", "ONU"]];
    for (const c of q.data.clients) {
      rows.push([c.client_name, c.serial, String(c.pon || ""), String(c.onu || "")]);
    }
    downloadBlob(`clientes_afetados_${alertId}.csv`, buildExcelCsvBlob(rows));
  }

  if (!open) return null;

  const clients = q.data?.clients ?? [];

  return createPortal(
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="modal"
        role="dialog"
        aria-modal="true"
        style={{ maxWidth: 560 }}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start" }}>
          <div>
            <h3 style={{ margin: 0 }}>Clientes afetados</h3>
            <p style={{ fontSize: 12, color: "var(--muted)", margin: "2px 0 0" }}>
              {q.data ? `${q.data.olt_label} — ${q.data.scope_label}` : "A carregar…"}
            </p>
          </div>
          <button type="button" className="btn btn--icon" aria-label="Fechar" onClick={onClose}>
            ×
          </button>
        </div>

        {q.isLoading ? (
          <p style={{ fontSize: 12, color: "var(--muted)" }}>A carregar clientes afetados…</p>
        ) : q.isError ? (
          <div className="msg msg--err">{(q.error as Error).message}</div>
        ) : clients.length === 0 ? (
          <EmptyState
            title="Nenhum cliente vinculado às ONUs afetadas."
            hint={`Nenhuma das ${q.data?.onu_count ?? 0} ONU(s) em ${q.data?.scope_label ?? "—"} tem cliente vinculado. Vincule os clientes na aba Pesquisa da OLT.`}
          />
        ) : (
          <>
            <div className="table-wrap" style={{ marginTop: 10, maxHeight: 320, overflowY: "auto" }}>
              <table style={{ fontSize: 12, width: "100%" }}>
                <thead>
                  <tr>
                    <th>Cliente</th>
                    <th className="mono">Serial</th>
                    <th className="mono">PON/ONU</th>
                  </tr>
                </thead>
                <tbody>
                  {clients.map((c) => (
                    <tr key={c.serial}>
                      <td>{c.client_name}</td>
                      <td className="mono">{c.serial}</td>
                      <td className="mono">
                        {c.pon || "—"}/{c.onu || "—"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <p style={{ fontSize: 11, color: "var(--muted)", marginTop: 8 }}>
              Total: {clients.length} cliente(s) — {q.data?.unlinked_count ?? 0} ONU(s) sem vínculo não entraram na lista.
            </p>
          </>
        )}

        <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginTop: 14 }}>
          <button type="button" className="btn" onClick={onClose}>
            Fechar
          </button>
          <button type="button" className="btn" onClick={exportCsv} disabled={clients.length === 0}>
            Exportar CSV
          </button>
          <button
            type="button"
            className="btn btn--primary"
            onClick={() => sendMut.mutate()}
            disabled={clients.length === 0 || sendMut.isPending}
          >
            <Send size={13} style={{ marginRight: 4, verticalAlign: -2 }} />
            {sendMut.isPending ? "A enviar…" : "Enviar por Telegram"}
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
}
