import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { apiFetch } from "../lib/api";

export function MetricsPage() {
  const q = useQuery({
    queryKey: ["metrics-series"],
    queryFn: () => apiFetch<{ series: unknown[]; note?: string }>("/api/v1/metrics"),
  });

  return (
    <>
      <h1>Métricas</h1>
      <p style={{ color: "var(--muted)", fontSize: 12 }}>
        <code className="mono">GET /api/v1/metrics</code> — série genérica; históricos detalhados em{" "}
        <Link to="/devices">ping/telemetria por equipamento</Link> (<code className="mono">/ping/history</code>, <code className="mono">/telemetry/history</code>).
      </p>
      {q.isLoading && <p>A carregar…</p>}
      {q.isError && <div className="msg msg--err">{(q.error as Error).message}</div>}
      {q.data && (
        <div className="card">
          <pre className="mono" style={{ fontSize: 12, overflow: "auto" }}>{JSON.stringify(q.data, null, 2)}</pre>
        </div>
      )}
    </>
  );
}
