import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { PageCountPill } from "../components/PageCountPill";
import { apiFetch } from "../lib/api";
import { displayEventType, displaySeverity } from "../lib/alertLabels";

type Ev = {
  id: string;
  created_at: string;
  event_type: string;
  severity?: string;
  device_id?: string | null;
  payload: unknown;
  probable_cause?: string;
};

export function EventsPage() {
  const [limit, setLimit] = useState("50");
  const lim = Math.min(200, Math.max(1, Number(limit) || 50));

  const ev = useQuery({
    queryKey: ["events", lim],
    queryFn: () => apiFetch<{ events: Ev[] }>(`/api/v1/events?limit=${lim}`),
  });

  if (ev.isLoading) return <p>Carregando eventos…</p>;
  if (ev.isError) return <div className="msg msg--err">{(ev.error as Error).message}</div>;

  return (
    <>
      <div className="page-heading">
        <h1>Eventos</h1>
        <PageCountPill label="Eventos" count={(ev.data?.events ?? []).length} />
      </div>
      <p style={{ color: "var(--muted)", marginTop: 0 }}>
        <code className="mono">GET /api/v1/events?limit=</code> (1–200)
      </p>
      <div className="row" style={{ marginBottom: 12 }}>
        <label className="row" style={{ gap: 8 }}>
          Limite
          <input className="input" style={{ width: 72 }} value={limit} onChange={(e) => setLimit(e.target.value)} />
        </label>
        <button type="button" className="btn" onClick={() => ev.refetch()}>
          Atualizar
        </button>
      </div>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Quando</th>
              <th>Tipo</th>
              <th>Severidade</th>
              <th>Device</th>
              <th>Causa provável</th>
              <th>Payload</th>
            </tr>
          </thead>
          <tbody>
            {(ev.data?.events ?? []).map((e) => (
              <tr key={e.id}>
                <td className="mono" style={{ whiteSpace: "nowrap" }}>
                  {e.created_at}
                </td>
                <td>{displayEventType(e.event_type)}</td>
                <td>{displaySeverity(e.severity)}</td>
                <td className="mono">{e.device_id ?? "—"}</td>
                <td style={{ maxWidth: 260 }}>{e.probable_cause ?? "—"}</td>
                <td style={{ maxWidth: 360, overflow: "hidden", textOverflow: "ellipsis" }} title={JSON.stringify(e.payload)}>
                  {typeof e.payload === "string" ? e.payload : JSON.stringify(e.payload)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}
