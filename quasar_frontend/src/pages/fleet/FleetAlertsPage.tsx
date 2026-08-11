import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { APP_ROUTES } from "../../app/routes";
import { apiFetch } from "../../lib/api";
import { can, isAdminUser } from "../../lib/auth";
import { useAppToast } from "../../lib/appToast";
import { toastErr, toastOk } from "../../lib/operationToast";
import { invalidateFleetOperationalQueries, queryKeys } from "../../lib/queryKeys";
import { formatFleetPlate } from "./fleetUtils";

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

  const ack = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/fleet/alerts/${id}/ack`, { method: "POST" }),
    onSuccess: async () => {
      toastOk(push, "Alerta reconhecido");
      await invalidateFleetOperationalQueries(qc);
    },
    onError: (e) => toastErr(push, e),
  });

  return (
    <div className="fleet-page">
      <h1>Frota — Alertas</h1>
      <p className="muted" style={{ marginTop: 0 }}>
        Anomalias de abastecimento. As tolerâncias de consumo e preço ficam em{" "}
        <Link to={`${APP_ROUTES.settings}?tab=fleet`}>Configurações → Frota</Link>.
      </p>
      <div className="card">
        {q.isLoading ? <p className="muted">A carregar…</p> : null}
        {q.isError ? <p className="err">Falha ao carregar alertas.</p> : null}
        {!q.isLoading && !q.isError && (q.data?.items ?? []).length === 0 ? <p className="muted">Sem alertas abertos.</p> : null}
        {(q.data?.items ?? []).length > 0 ? (
          <ul className="fleet-alert-list">
            {(q.data?.items ?? []).map((a) => (
              <li key={a.id} className={`fleet-alert fleet-alert--${a.severity}`}>
                <div>
                  <strong>{a.title}</strong>
                  <span className="muted">
                    {a.plate ? ` · ${formatFleetPlate(a.plate)}` : ""} · {new Date(a.created_at).toLocaleString("pt-BR")}
                  </span>
                  <p>{a.message}</p>
                </div>
                {canMutate ? (
                  <button
                    type="button"
                    className="btn btn--sm"
                    disabled={ack.isPending && ack.variables === a.id}
                    onClick={() => ack.mutate(a.id)}
                  >
                    {ack.isPending && ack.variables === a.id ? "…" : "Reconhecer"}
                  </button>
                ) : null}
              </li>
            ))}
          </ul>
        ) : null}
      </div>
    </div>
  );
}
