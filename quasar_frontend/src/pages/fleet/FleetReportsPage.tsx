import { useState } from "react";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { downloadFleetReport, monthStartISO, todayISO } from "./fleetUtils";

const KINDS = [
  { id: "fuelings", label: "Abastecimentos" },
  { id: "by-vehicle", label: "Por veículo" },
  { id: "by-driver", label: "Por motorista" },
  { id: "by-station", label: "Por posto" },
  { id: "by-cost-center", label: "Por centro de custo" },
];

export function FleetReportsPage() {
  const { push } = useAppToast();
  const [from, setFrom] = useState(monthStartISO());
  const [to, setTo] = useState(todayISO());
  const [busy, setBusy] = useState<string | null>(null);

  async function exportKind(kind: string) {
    setBusy(kind);
    try {
      await downloadFleetReport(kind, from, to);
      toastOk(push, "CSV descarregado");
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
        <div className="row" style={{ marginBottom: 12 }}>
          <input className="input" type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
          <input className="input" type="date" value={to} onChange={(e) => setTo(e.target.value)} />
        </div>
        <div className="fleet-report-actions">
          {KINDS.map((k) => (
            <button key={k.id} type="button" className="btn btn--primary" disabled={busy === k.id} onClick={() => void exportKind(k.id)}>
              {busy === k.id ? "A exportar…" : `Exportar ${k.label} (CSV)`}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
