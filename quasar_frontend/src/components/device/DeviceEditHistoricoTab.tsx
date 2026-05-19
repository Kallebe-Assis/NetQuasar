import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { apiFetch } from "../../lib/api";
import { displayAlertMessage, displayEventType, displaySeverity, formatAlertDateTimePt } from "../../lib/alertLabels";

type HistRow = {
  id: string;
  kind: "alert" | "event";
  at: string;
  title: string;
  detail: string;
  severity?: string;
};

type Props = {
  deviceId: string;
};

export function DeviceEditHistoricoTab({ deviceId }: Props) {
  const alertsHist = useQuery({
    queryKey: ["device-hist-alerts", deviceId],
    queryFn: () => {
      const p = new URLSearchParams({ device_id: deviceId, limit: "80" });
      return apiFetch<{ events: Array<Record<string, unknown>> }>(`/api/v1/alerts/history?${p}`);
    },
    enabled: !!deviceId,
  });

  const alertsActive = useQuery({
    queryKey: ["device-active-alerts", deviceId],
    queryFn: () => apiFetch<{ alerts: Array<Record<string, unknown>> }>("/api/v1/alerts/active?limit=500"),
    enabled: !!deviceId,
    select: (data) => ({
      alerts: (data.alerts ?? []).filter((a) => String(a.device_id ?? "") === deviceId),
    }),
  });

  const events = useQuery({
    queryKey: ["device-hist-events", deviceId],
    queryFn: () => apiFetch<{ events: Array<Record<string, unknown>> }>(`/api/v1/events?device_id=${deviceId}&limit=80`),
    enabled: !!deviceId,
  });

  const rows = useMemo(() => {
    const out: HistRow[] = [];
    const seenAlert = new Set<string>();
    for (const e of alertsActive.data?.alerts ?? []) {
      const id = String(e.id);
      if (seenAlert.has(id)) continue;
      seenAlert.add(id);
      const at = String(e.active_since ?? "");
      out.push({
        id: `a-${id}`,
        kind: "alert",
        at,
        title: String(e.alert_type ?? e.type ?? "alerta"),
        detail: displayAlertMessage(String(e.message ?? ""), String(e.alert_type ?? e.type ?? "")),
        severity: String(e.severity ?? ""),
      });
    }
    for (const e of alertsHist.data?.events ?? []) {
      const id = String(e.id);
      if (seenAlert.has(id)) continue;
      seenAlert.add(id);
      const at = String(e.active_since ?? e.closed_at ?? "");
      out.push({
        id: `a-${id}`,
        kind: "alert",
        at,
        title: String(e.alert_type ?? e.type ?? "alerta"),
        detail: displayAlertMessage(String(e.message ?? ""), String(e.alert_type ?? e.type ?? "")),
        severity: String(e.severity ?? ""),
      });
    }
    for (const e of events.data?.events ?? []) {
      out.push({
        id: `e-${String(e.id)}`,
        kind: "event",
        at: String(e.created_at ?? ""),
        title: displayEventType(String(e.event_type ?? "")),
        detail:
          typeof e.probable_cause === "string" && e.probable_cause.trim()
            ? e.probable_cause
            : typeof e.payload === "string"
              ? e.payload
              : JSON.stringify(e.payload ?? {}),
        severity: e.severity != null ? String(e.severity) : undefined,
      });
    }
    out.sort((a, b) => new Date(b.at).getTime() - new Date(a.at).getTime());
    return out.slice(0, 100);
  }, [alertsHist.data, alertsActive.data, events.data]);

  const loading = alertsHist.isLoading || alertsActive.isLoading || events.isLoading;
  const err = alertsHist.error ?? alertsActive.error ?? events.error;

  if (loading) return <p style={{ padding: "8px 0" }}>A carregar histórico…</p>;
  if (err) return <div className="msg msg--err">{(err as Error).message}</div>;

  return (
    <>
      <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 0 }}>
        Últimos alertas e eventos associados a este equipamento (até 100 entradas mais recentes).
      </p>
      <div className="row" style={{ gap: 8, marginBottom: 10 }}>
        <button type="button" className="btn" onClick={() => { void alertsHist.refetch(); void alertsActive.refetch(); void events.refetch(); }}>
          Actualizar
        </button>
      </div>
      {rows.length === 0 ? (
        <p style={{ color: "var(--muted)", fontSize: 13 }}>Nenhum alerta ou evento registado para este equipamento.</p>
      ) : (
        <div className="table-wrap" style={{ maxHeight: 420, overflow: "auto" }}>
          <table>
            <thead>
              <tr>
                <th>Quando</th>
                <th>Tipo</th>
                <th>Origem</th>
                <th>Detalhe</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.id}>
                  <td className="mono" style={{ fontSize: 11, whiteSpace: "nowrap" }}>
                    {formatAlertDateTimePt(r.at)}
                  </td>
                  <td>{r.severity ? displaySeverity(r.severity) : "—"}</td>
                  <td>
                    <span className="badge badge--off" style={{ fontSize: 10 }}>
                      {r.kind === "alert" ? "Alerta" : "Evento"}
                    </span>{" "}
                    <span style={{ fontSize: 12 }}>{r.title}</span>
                  </td>
                  <td style={{ fontSize: 12, maxWidth: 360 }}>{r.detail}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}
