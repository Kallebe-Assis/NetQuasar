import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "../../lib/api";
import { can, isAdminUser } from "../../lib/auth";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { queryKeys } from "../../lib/queryKeys";

type Alert = {
  id: string;
  severity: string;
  alert_type: string;
  title: string;
  message: string;
  plate?: string | null;
  created_at: string;
  acknowledged_at?: string | null;
};

export function FleetAlertsPage() {
  const { push } = useAppToast();
  const qc = useQueryClient();
  const canMutate = can("fleet.manage") || isAdminUser();
  const q = useQuery({
    queryKey: queryKeys.fleetAlerts,
    queryFn: () => apiFetch<{ items: Alert[] }>("/api/v1/fleet/alerts?open=1"),
  });
  const settings = useQuery({
    queryKey: queryKeys.fleetSettings,
    queryFn: () => apiFetch<{ consumption_tolerance_pct: number; price_tolerance_pct: number; min_minutes_between_fuelings: number }>("/api/v1/fleet/settings"),
  });

  const ack = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/fleet/alerts/${id}/ack`, { method: "POST" }),
    onSuccess: async () => {
      toastOk(push, "Alerta reconhecido");
      await qc.invalidateQueries({ queryKey: queryKeys.fleetAlerts });
    },
    onError: (e) => toastErr(push, e),
  });

  const saveSettings = useMutation({
    mutationFn: (body: { consumption_tolerance_pct: number; price_tolerance_pct: number; min_minutes_between_fuelings: number }) =>
      apiFetch("/api/v1/fleet/settings", { method: "PATCH", body: JSON.stringify(body) }),
    onSuccess: async () => {
      toastOk(push, "Regras actualizadas");
      await qc.invalidateQueries({ queryKey: queryKeys.fleetSettings });
    },
    onError: (e) => toastErr(push, e),
  });

  return (
    <div className="fleet-page">
      <h1>Frota — Alertas</h1>
      {canMutate && settings.data ? (
        <div className="card" style={{ marginBottom: 12 }}>
          <h3>Regras de validação</h3>
          <div className="fleet-form-grid">
            <label>Tolerância consumo (%)
              <input className="input" id="tol-c" defaultValue={settings.data.consumption_tolerance_pct} />
            </label>
            <label>Tolerância preço (%)
              <input className="input" id="tol-p" defaultValue={settings.data.price_tolerance_pct} />
            </label>
            <label>Minutos mínimos entre abastecimentos
              <input className="input" id="tol-m" defaultValue={settings.data.min_minutes_between_fuelings} />
            </label>
          </div>
          <button
            type="button"
            className="btn btn--primary"
            style={{ marginTop: 10 }}
            onClick={() => {
              const c = Number((document.getElementById("tol-c") as HTMLInputElement).value);
              const p = Number((document.getElementById("tol-p") as HTMLInputElement).value);
              const m = Number((document.getElementById("tol-m") as HTMLInputElement).value);
              saveSettings.mutate({ consumption_tolerance_pct: c, price_tolerance_pct: p, min_minutes_between_fuelings: m });
            }}
          >
            Guardar regras
          </button>
        </div>
      ) : null}

      <div className="card">
        {(q.data?.items ?? []).length === 0 ? <p className="muted">Sem alertas abertos.</p> : (
          <ul className="fleet-alert-list">
            {(q.data?.items ?? []).map((a) => (
              <li key={a.id} className={`fleet-alert fleet-alert--${a.severity}`}>
                <div>
                  <strong>{a.title}</strong>
                  <span className="muted">{a.plate ? ` · ${a.plate}` : ""} · {new Date(a.created_at).toLocaleString("pt-BR")}</span>
                  <p>{a.message}</p>
                </div>
                {canMutate ? <button type="button" className="btn btn--sm" onClick={() => ack.mutate(a.id)}>Reconhecer</button> : null}
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
