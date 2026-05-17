import { useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { InfoHint } from "../../components/InfoHint";
import { apiFetch } from "../../lib/api";

type HistoryPoint = { t: string; total: number; online: number; offline: number };
type HistorySeries = { device_id: string; description: string; points: HistoryPoint[] };
type HistoryResponse = {
  days: number;
  bucket: string;
  since: string;
  series: HistorySeries[];
  aggregate: { points: HistoryPoint[] };
};

const DAY_OPTIONS = [1, 3, 7, 30] as const;
const CHART_COLORS = { total: "#58a6ff", online: "#3fb950", offline: "#f85149" };

function formatAxisTime(iso: string, days: number): string {
  const d = new Date(iso);
  if (!Number.isFinite(d.getTime())) return iso;
  if (days === 1) {
    return d.toLocaleString("pt-PT", { day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" });
  }
  return d.toLocaleDateString("pt-PT", { day: "2-digit", month: "short" });
}

function chartDataFromPoints(points: HistoryPoint[], days: number) {
  return points.map((p) => ({
    ...p,
    label: formatAxisTime(p.t, days),
  }));
}

function OnuHistoryChart({ title, data, days, height = 220 }: { title: string; data: HistoryPoint[]; days: number; height?: number }) {
  const rows = chartDataFromPoints(data, days);
  if (rows.length === 0) {
    return (
      <div className="card" style={{ padding: 12 }}>
        <h3 style={{ margin: "0 0 8px", fontSize: 14 }}>{title}</h3>
        <p style={{ fontSize: 12, color: "var(--muted)", margin: 0 }}>Sem amostras no período. Actualize snapshots das OLTs para gerar histórico.</p>
      </div>
    );
  }
  return (
    <div className="card" style={{ padding: 12 }}>
      <h3 style={{ margin: "0 0 8px", fontSize: 14 }}>{title}</h3>
      <ResponsiveContainer width="100%" height={height}>
        <LineChart data={rows} margin={{ top: 4, right: 8, left: 0, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
          <XAxis dataKey="label" tick={{ fontSize: 10 }} interval="preserveStartEnd" />
          <YAxis tick={{ fontSize: 10 }} width={40} allowDecimals={false} />
          <Tooltip
            formatter={(v: number, name: string) => [v.toLocaleString("pt-PT"), name === "total" ? "Total" : name === "online" ? "Online" : "Offline"]}
            labelFormatter={(l) => String(l)}
          />
          <Legend formatter={(v) => (v === "total" ? "Total" : v === "online" ? "Online" : "Offline")} />
          <Line type="monotone" dataKey="total" name="total" stroke={CHART_COLORS.total} strokeWidth={2} dot={false} />
          <Line type="monotone" dataKey="online" name="online" stroke={CHART_COLORS.online} strokeWidth={2} dot={false} />
          <Line type="monotone" dataKey="offline" name="offline" stroke={CHART_COLORS.offline} strokeWidth={2} dot={false} />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}

export function OltReportsTab() {
  const [days, setDays] = useState<(typeof DAY_OPTIONS)[number]>(7);
  const q = useQuery({
    queryKey: ["olt-reports-history", days],
    queryFn: () => apiFetch<HistoryResponse>(`/api/v1/olt/reports/history?days=${days}`),
    staleTime: 30_000,
  });

  const aggPoints = q.data?.aggregate?.points ?? [];
  const series = q.data?.series ?? [];
  const hasAny = useMemo(() => aggPoints.length > 0 || series.some((s) => s.points.length > 0), [aggPoints, series]);

  return (
    <>
      <div className="card" style={{ marginBottom: 12 }}>
        <h2 style={{ display: "flex", alignItems: "center", flexWrap: "wrap", gap: 6, marginBottom: 8 }}>
          Relatórios ONU (histórico)
          <InfoHint label="Histórico de ONUs por OLT">
            <p>
              Cada <strong>actualização de snapshot OLT</strong> ou relatório mensal automático regista uma amostra (total, online, offline). Use os
              gráficos para acompanhar a evolução por equipamento e o total de todas as OLTs.
            </p>
          </InfoHint>
        </h2>
        <div className="row" style={{ gap: 8, flexWrap: "wrap", alignItems: "center" }}>
          <span style={{ fontSize: 12, color: "var(--muted)" }}>Período:</span>
          {DAY_OPTIONS.map((d) => (
            <button key={d} type="button" className={`btn${days === d ? " btn--primary" : ""}`} onClick={() => setDays(d)}>
              {d === 1 ? "24 h" : `${d} dias`}
            </button>
          ))}
          <button type="button" className="btn" disabled={q.isFetching} onClick={() => q.refetch()} style={{ marginLeft: "auto" }}>
            {q.isFetching ? "A actualizar…" : "Actualizar gráficos"}
          </button>
        </div>
        {q.isError && <div className="msg msg--err" style={{ marginTop: 8 }}>{(q.error as Error).message}</div>}
        {!q.isLoading && !hasAny && (
          <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 8, marginBottom: 0 }}>
            Ainda não há histórico. Vá à lista de equipamentos e actualize snapshots, ou aguarde o relatório mensal automático.
          </p>
        )}
      </div>

      {q.isLoading ? (
        <p>A carregar histórico…</p>
      ) : (
        <>
          <OnuHistoryChart title="Total geral (todas as OLTs)" data={aggPoints} days={days} height={260} />

          {series.length > 0 && (
            <>
              <h2 style={{ marginTop: 20, marginBottom: 10, fontSize: 16 }}>Por OLT</h2>
              <div
                style={{
                  display: "grid",
                  gridTemplateColumns: "repeat(auto-fill, minmax(min(100%, 420px), 1fr))",
                  gap: 12,
                }}
              >
                {series.map((s) => (
                  <OnuHistoryChart key={s.device_id} title={s.description || s.device_id} data={s.points} days={days} height={200} />
                ))}
              </div>
            </>
          )}
        </>
      )}
    </>
  );
}
