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
type HistoryBucket = "minute" | "hour" | "day";
type HistoryResponse = {
  days: number;
  bucket: HistoryBucket;
  since: string;
  until: string;
  device_id: string;
  series: HistorySeries[];
  aggregate: { points: HistoryPoint[] };
  current_fleet?: { onu_total?: number; onu_online?: number; onu_offline?: number };
};

const DAY_OPTIONS = [1, 3, 7, 30] as const;
const CHART_COLORS = { total: "#58a6ff", online: "#3fb950", offline: "#f85149" };

function formatAxisTime(iso: string, bucket: HistoryBucket): string {
  const d = new Date(iso);
  if (!Number.isFinite(d.getTime())) return iso;
  if (bucket === "minute") {
    return d.toLocaleString("pt-PT", { day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" });
  }
  if (bucket === "hour") {
    return d.toLocaleString("pt-PT", { day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" });
  }
  return d.toLocaleDateString("pt-PT", { day: "2-digit", month: "short" });
}

function chartDataFromPoints(points: HistoryPoint[], bucket: HistoryBucket) {
  return points.map((p) => ({
    ...p,
    label: formatAxisTime(p.t, bucket),
  }));
}

function OnuHistoryChart({
  title,
  data,
  bucket,
  height = 220,
}: {
  title: string;
  data: HistoryPoint[];
  bucket: HistoryBucket;
  height?: number;
}) {
  const rows = chartDataFromPoints(data, bucket);
  if (rows.length === 0) {
    return (
      <div className="card" style={{ padding: 12 }}>
        <h3 style={{ margin: "0 0 8px", fontSize: 14 }}>{title}</h3>
        <p style={{ fontSize: 12, color: "var(--muted)", margin: 0 }}>Sem amostras no período. Atualize snapshots das OLTs para gerar histórico.</p>
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
          <Line type="monotone" dataKey="total" name="total" stroke={CHART_COLORS.total} strokeWidth={2} dot={rows.length <= 60} />
          <Line type="monotone" dataKey="online" name="online" stroke={CHART_COLORS.online} strokeWidth={2} dot={rows.length <= 60} />
          <Line type="monotone" dataKey="offline" name="offline" stroke={CHART_COLORS.offline} strokeWidth={2} dot={rows.length <= 60} />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}

// Converte um Date local para o valor aceito por <input type="datetime-local">.
function toLocalInputValue(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

type Props = { olts: Array<{ id: string; description?: string | null }> };

export function OltReportsTab({ olts }: Props) {
  const [days, setDays] = useState<(typeof DAY_OPTIONS)[number]>(7);
  const [deviceId, setDeviceId] = useState<string>("all");
  const [customOpen, setCustomOpen] = useState(false);
  const [fromInput, setFromInput] = useState(() => toLocalInputValue(new Date(Date.now() - 24 * 3600_000)));
  const [toInput, setToInput] = useState(() => toLocalInputValue(new Date()));
  const [appliedRange, setAppliedRange] = useState<{ from: string; to: string } | null>(null);

  const applyCustomRange = () => {
    const from = new Date(fromInput);
    const to = new Date(toInput);
    if (!Number.isFinite(from.getTime()) || !Number.isFinite(to.getTime()) || to <= from) return;
    setAppliedRange({ from: from.toISOString(), to: to.toISOString() });
  };

  const clearCustomRange = () => {
    setAppliedRange(null);
  };

  const queryParams = useMemo(() => {
    const p = new URLSearchParams();
    if (appliedRange) {
      p.set("from", appliedRange.from);
      p.set("to", appliedRange.to);
    } else {
      p.set("days", String(days));
    }
    if (deviceId !== "all") p.set("device_id", deviceId);
    return p.toString();
  }, [appliedRange, days, deviceId]);

  const q = useQuery({
    queryKey: ["olt-reports-history", queryParams],
    queryFn: () => apiFetch<HistoryResponse>(`/api/v1/olt/reports/history?${queryParams}`),
    staleTime: 30_000,
  });

  const aggPoints = q.data?.aggregate?.points ?? [];
  const series = q.data?.series ?? [];
  const fleet = q.data?.current_fleet;
  const bucket = q.data?.bucket ?? (days === 1 ? "minute" : "day");
  const hasAny = useMemo(() => aggPoints.length > 0 || series.some((s) => s.points.length > 0), [aggPoints, series]);
  const fmt = (v: unknown) => (Number.isFinite(Number(v)) ? Number(v).toLocaleString("pt-PT") : "—");
  const selectedOltLabel = useMemo(() => {
    if (deviceId === "all") return null;
    return olts.find((o) => o.id === deviceId)?.description || deviceId;
  }, [deviceId, olts]);

  return (
    <>
      <div className="card" style={{ marginBottom: 12 }}>
        <h2 style={{ display: "flex", alignItems: "center", flexWrap: "wrap", gap: 6, marginBottom: 8 }}>
          Relatórios ONU (histórico)
          <InfoHint label="Histórico de ONUs por OLT">
            <p>
              Cada <strong>atualização de snapshot OLT</strong> ou relatório mensal automático regista uma amostra (total, online, offline). Use os
              gráficos para acompanhar a evolução por equipamento e o total de todas as OLTs. Em períodos de até ~1 dia, cada coleta real aparece como
              um ponto (granularidade por minuto); períodos maiores agrupam por hora ou por dia.
            </p>
          </InfoHint>
        </h2>
        <div className="row" style={{ gap: 8, flexWrap: "wrap", alignItems: "center" }}>
          <span style={{ fontSize: 12, color: "var(--muted)" }}>Período:</span>
          {DAY_OPTIONS.map((d) => (
            <button
              key={d}
              type="button"
              className={`btn${!appliedRange && days === d ? " btn--primary" : ""}`}
              onClick={() => {
                setDays(d);
                setAppliedRange(null);
              }}
            >
              {d === 1 ? "24 h" : `${d} dias`}
            </button>
          ))}
          <button
            type="button"
            className={`btn${appliedRange ? " btn--primary" : ""}`}
            onClick={() => setCustomOpen((v) => !v)}
          >
            Período específico…
          </button>
          <span style={{ fontSize: 12, color: "var(--muted)" }}>OLT:</span>
          <select className="input" style={{ maxWidth: 220 }} value={deviceId} onChange={(e) => setDeviceId(e.target.value)}>
            <option value="all">Todas as OLTs</option>
            {olts.map((o) => (
              <option key={o.id} value={o.id}>
                {o.description || o.id}
              </option>
            ))}
          </select>
          <button type="button" className="btn" disabled={q.isFetching} onClick={() => q.refetch()} style={{ marginLeft: "auto" }}>
            {q.isFetching ? "A atualizar…" : "Atualizar gráficos"}
          </button>
        </div>

        {customOpen ? (
          <div className="row" style={{ gap: 8, flexWrap: "wrap", alignItems: "center", marginTop: 10 }}>
            <label style={{ fontSize: 12, color: "var(--muted)", display: "flex", flexDirection: "column", gap: 2 }}>
              De
              <input
                type="datetime-local"
                className="input"
                value={fromInput}
                onChange={(e) => setFromInput(e.target.value)}
              />
            </label>
            <label style={{ fontSize: 12, color: "var(--muted)", display: "flex", flexDirection: "column", gap: 2 }}>
              Até
              <input type="datetime-local" className="input" value={toInput} onChange={(e) => setToInput(e.target.value)} />
            </label>
            <button type="button" className="btn btn--primary" onClick={applyCustomRange} style={{ alignSelf: "flex-end" }}>
              Aplicar período
            </button>
            {appliedRange ? (
              <button type="button" className="btn" onClick={clearCustomRange} style={{ alignSelf: "flex-end" }}>
                Limpar
              </button>
            ) : null}
          </div>
        ) : null}

        {q.isError && <div className="msg msg--err" style={{ marginTop: 8 }}>{(q.error as Error).message}</div>}
        {fleet ? (
          <div className="row" style={{ gap: 12, marginTop: 10, flexWrap: "wrap" }}>
            <div className="stat" style={{ minWidth: 140 }}>
              <div className="stat__k">
                {selectedOltLabel ? `Total atual (${selectedOltLabel})` : "Total actual (última amostra por OLT)"}
              </div>
              <div className="stat__v">{fmt(fleet.onu_total)}</div>
            </div>
            <div className="stat" style={{ minWidth: 120 }}>
              <div className="stat__k">Online</div>
              <div className="stat__v">{fmt(fleet.onu_online)}</div>
            </div>
            <div className="stat" style={{ minWidth: 120 }}>
              <div className="stat__k">Offline</div>
              <div className="stat__v">{fmt(fleet.onu_offline)}</div>
            </div>
          </div>
        ) : null}
        {!q.isLoading && !hasAny && (
          <p style={{ fontSize: 12, color: "var(--muted)", marginTop: 8, marginBottom: 0 }}>
            Ainda não há histórico. Vá à lista de equipamentos e atualize snapshots, ou aguarde o relatório mensal automático.
          </p>
        )}
      </div>

      {q.isLoading ? (
        <p>A carregar histórico…</p>
      ) : (
        <>
          <OnuHistoryChart
            title={
              selectedOltLabel
                ? `${selectedOltLabel} — evolução no período`
                : "Total geral (soma da última amostra conhecida de cada OLT por período)"
            }
            data={aggPoints}
            bucket={bucket}
            height={260}
          />

          {series.length > 0 && deviceId === "all" && (
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
                  <OnuHistoryChart key={s.device_id} title={s.description || s.device_id} data={s.points} bucket={bucket} height={200} />
                ))}
              </div>
            </>
          )}
        </>
      )}
    </>
  );
}
