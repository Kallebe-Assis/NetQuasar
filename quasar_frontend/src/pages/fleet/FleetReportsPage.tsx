import { useState } from "react";
import { SendIcon } from "../../components/icons/SendIcon";
import { apiFetch } from "../../lib/api";
import { can, isAdminUser } from "../../lib/auth";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { downloadFleetReport, monthStartISO, todayISO } from "./fleetUtils";

function rangeInvalid(from: string, to: string) {
  return !!from && !!to && from > to;
}

const KINDS = [
  { id: "fuelings", label: "Abastecimentos" },
  { id: "by-vehicle", label: "Por veículo" },
  { id: "by-driver", label: "Por motorista" },
  { id: "by-station", label: "Por posto" },
  { id: "by-cost-center", label: "Por centro de custo" },
];

export function FleetReportsPage() {
  const { push } = useAppToast();
  const canSend = can("fleet.manage") || can("reports.send") || isAdminUser();
  const [from, setFrom] = useState(monthStartISO());
  const [to, setTo] = useState(todayISO());
  const [busy, setBusy] = useState<string | null>(null);

  async function exportKind(kind: string) {
    if (rangeInvalid(from, to)) {
      toastErr(push, new Error("A data inicial não pode ser maior que a final"));
      return;
    }
    setBusy(`csv-${kind}`);
    try {
      await downloadFleetReport(kind, from, to);
      toastOk(push, "CSV baixado");
    } catch (e) {
      toastErr(push, e);
    } finally {
      setBusy(null);
    }
  }

  async function sendTelegram(kind: string) {
    if (rangeInvalid(from, to)) {
      toastErr(push, new Error("A data inicial não pode ser maior que a final"));
      return;
    }
    setBusy(`tg-${kind}`);
    try {
      const qs = new URLSearchParams({ from, to });
      await apiFetch(`/api/v1/fleet/reports/${encodeURIComponent(kind)}/telegram?${qs}`, { method: "POST" });
      toastOk(push, "Relatório enviado no Telegram");
    } catch (e) {
      toastErr(push, e);
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="fleet-page">
      <h1>Frota — Relatórios</h1>
      <div className="card">
        <div className="row" style={{ marginBottom: 12, gap: 8, flexWrap: "wrap" }}>
          <label>
            De
            <input className="input" type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
          </label>
          <label>
            Até
            <input className="input" type="date" value={to} onChange={(e) => setTo(e.target.value)} />
          </label>
        </div>
        {rangeInvalid(from, to) ? <p className="err">A data inicial não pode ser maior que a final.</p> : null}
        <div className="fleet-report-actions">
          {KINDS.map((k) => (
            <div key={k.id} className="row" style={{ gap: 8, flexWrap: "wrap" }}>
              <button type="button" className="btn btn--primary" disabled={busy === `csv-${k.id}`} onClick={() => void exportKind(k.id)}>
                {busy === `csv-${k.id}` ? "A exportar…" : `Exportar ${k.label} (CSV)`}
              </button>
              {canSend ? (
                <button
                  type="button"
                  className="btn btn--icon"
                  title={`Enviar ${k.label} no Telegram`}
                  aria-label={`Enviar ${k.label} no Telegram`}
                  disabled={busy === `tg-${k.id}`}
                  onClick={() => void sendTelegram(k.id)}
                >
                  <SendIcon size={16} />
                </button>
              ) : null}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
