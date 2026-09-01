import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { Fragment, useMemo, useState } from "react";
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
import { apiFetch } from "../../lib/api";
import { formatBitrate } from "../../lib/formatBitrate";
import { EM_DASH, formatDateTime } from "./bgpFormat";

type TrafficPoint = { t: string; in_bps: number; out_bps: number };
type CarrierStats = { in_min: number; in_max: number; in_avg: number; out_min: number; out_max: number; out_avg: number };
type InterfaceTraffic = { interface_label: string; points: TrafficPoint[]; stats: CarrierStats };
type CarrierTraffic = {
  carrier_id: string;
  carrier_label: string;
  bandwidth_limit_mbps?: number;
  points: TrafficPoint[];
  stats: CarrierStats;
  interfaces: InterfaceTraffic[];
};
type TotalTraffic = { points: TrafficPoint[]; stats: CarrierStats; bandwidth_limit_mbps?: number };
type CarrierTrafficHistoryResponse = {
  device_id: string;
  since?: string;
  until?: string;
  bucket?: string;
  total: TotalTraffic | null;
  carriers: CarrierTraffic[];
};

const DAY_OPTIONS = [1, 3, 7, 15, 30, 60, 90, 150, 300] as const;
const CARRIER_COLORS = ["#3b82f6", "#f97316", "#22c55e", "#a855f7", "#eab308", "#ec4899", "#06b6d4", "#ef4444"];

// Converte um Date local para o valor aceito por <input type="datetime-local"> — mesmo helper
// usado em pages/olt/OltReportsTab.tsx (não exportado de lá, duplicado aqui de propósito).
function toLocalInputValue(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function statBlock(label: string, stats: { min: number; max: number; avg: number }) {
  return (
    <div style={{ minWidth: 150 }}>
      <div style={{ fontSize: 11, color: "var(--muted)", marginBottom: 2 }}>{label}</div>
      <div style={{ fontSize: 12, display: "flex", flexDirection: "column", gap: 1 }}>
        <span>
          <strong>máx</strong> {formatBitrate(stats.max)}
        </span>
        <span>
          <strong>mín</strong> {formatBitrate(stats.min)}
        </span>
        <span>
          <strong>média</strong> {formatBitrate(stats.avg)}
        </span>
      </div>
    </div>
  );
}

function buildTwoLineChartData(points: TrafficPoint[]) {
  return points.map((p) => ({ t: p.t, in_bps: p.in_bps, out_bps: p.out_bps }));
}

/** 1 gráfico simples (download + upload) — usado tanto no gráfico "TOTAL geral" como no gráfico
 * de cada operadora quando ela só tem 1 interface (ou está no modo "Somado"). */
function SimpleTrafficChart({ points, yDomain }: { points: TrafficPoint[]; yDomain: [number, number] | ["auto", "auto"] }) {
  const data = useMemo(() => buildTwoLineChartData(points), [points]);
  if (data.length < 2) {
    return <div className="msg" style={{ fontSize: 12 }}>Sem amostras suficientes neste período ainda.</div>;
  }
  return (
    <ResponsiveContainer width="100%" height={220}>
      <LineChart data={data} margin={{ top: 8, right: 8, left: 0, bottom: 4 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
        <XAxis dataKey="t" tick={{ fontSize: 9 }} interval="preserveStartEnd" minTickGap={24} tickFormatter={(v) => formatDateTime(v)} />
        <YAxis tick={{ fontSize: 10 }} width={64} domain={yDomain} tickFormatter={(v) => formatBitrate(v)} />
        <Tooltip
          labelFormatter={(v) => formatDateTime(String(v))}
          formatter={(v: number, name: string) => [formatBitrate(v), name]}
          contentStyle={{ background: "var(--panel2)", border: "1px solid var(--border)", fontSize: 12, color: "var(--text)" }}
          itemStyle={{ color: "var(--text)" }}
          labelStyle={{ color: "var(--text)" }}
        />
        <Legend wrapperStyle={{ fontSize: 11 }} />
        <Line type="monotone" dataKey="in_bps" name="Download" stroke="#3b82f6" strokeWidth={2} dot={false} />
        <Line type="monotone" dataKey="out_bps" name="Upload" stroke="#f97316" strokeWidth={2} dot={false} />
      </LineChart>
    </ResponsiveContainer>
  );
}

/** 1 gráfico com 1 par de linhas (download/upload) por interface — usado no gráfico de uma
 * operadora quando ela tem 2+ interfaces e o modo "Separado" está activo. */
function PerInterfaceChart({ interfaces, yDomain }: { interfaces: InterfaceTraffic[]; yDomain: [number, number] | ["auto", "auto"] }) {
  const data = useMemo(() => {
    const n = interfaces[0]?.points.length ?? 0;
    const rows: Array<Record<string, number | string>> = [];
    for (let i = 0; i < n; i++) {
      const row: Record<string, number | string> = { t: interfaces[0].points[i]?.t ?? "" };
      for (const ifc of interfaces) {
        row[`in_${ifc.interface_label}`] = ifc.points[i]?.in_bps ?? 0;
        row[`out_${ifc.interface_label}`] = ifc.points[i]?.out_bps ?? 0;
      }
      rows.push(row);
    }
    return rows;
  }, [interfaces]);
  if (data.length < 2) {
    return <div className="msg" style={{ fontSize: 12 }}>Sem amostras suficientes neste período ainda.</div>;
  }
  return (
    <ResponsiveContainer width="100%" height={220}>
      <LineChart data={data} margin={{ top: 8, right: 8, left: 0, bottom: 4 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
        <XAxis dataKey="t" tick={{ fontSize: 9 }} interval="preserveStartEnd" minTickGap={24} tickFormatter={(v) => formatDateTime(v)} />
        <YAxis tick={{ fontSize: 10 }} width={64} domain={yDomain} tickFormatter={(v) => formatBitrate(v)} />
        <Tooltip
          labelFormatter={(v) => formatDateTime(String(v))}
          formatter={(v: number, name: string) => [formatBitrate(v), name]}
          contentStyle={{ background: "var(--panel2)", border: "1px solid var(--border)", fontSize: 12, color: "var(--text)" }}
          itemStyle={{ color: "var(--text)" }}
          labelStyle={{ color: "var(--text)" }}
        />
        <Legend wrapperStyle={{ fontSize: 11 }} />
        {interfaces.map((ifc, i) => {
          const color = CARRIER_COLORS[i % CARRIER_COLORS.length];
          return (
            <Fragment key={ifc.interface_label}>
              <Line type="monotone" dataKey={`in_${ifc.interface_label}`} name={`Download (${ifc.interface_label})`} stroke={color} strokeWidth={2} dot={false} />
              <Line type="monotone" dataKey={`out_${ifc.interface_label}`} name={`Upload (${ifc.interface_label})`} stroke={color} strokeWidth={2} strokeDasharray="4 3" dot={false} />
            </Fragment>
          );
        })}
      </LineChart>
    </ResponsiveContainer>
  );
}

/** 1 gráfico dedicado de UMA operadora: título, toggle Separado/Somado (só quando ela tem 2+
 * interfaces), gráfico e cartões de máx/mín/média. */
function CarrierChart({ carrier }: { carrier: CarrierTraffic }) {
  const [mode, setMode] = useState<"separado" | "somado">("somado");
  const hasMultiple = carrier.interfaces.length > 1;

  const yDomain = useMemo((): [number, number] | ["auto", "auto"] => {
    if (!carrier.bandwidth_limit_mbps || carrier.bandwidth_limit_mbps <= 0) return ["auto", "auto"];
    return [0, carrier.bandwidth_limit_mbps * 1_000_000];
  }, [carrier.bandwidth_limit_mbps]);

  return (
    <div className="card" style={{ padding: 14 }}>
      <div className="row" style={{ justifyContent: "space-between", alignItems: "center", marginBottom: 8, flexWrap: "wrap", gap: 8 }}>
        <h4 style={{ margin: 0, fontSize: 14 }}>
          {carrier.carrier_label}
          {carrier.bandwidth_limit_mbps ? (
            <span style={{ fontWeight: 400, color: "var(--muted)", fontSize: 12, marginLeft: 8 }}>
              limite {carrier.bandwidth_limit_mbps >= 1000 ? `${carrier.bandwidth_limit_mbps / 1000} Gbps` : `${carrier.bandwidth_limit_mbps} Mbps`}
            </span>
          ) : null}
        </h4>
        {hasMultiple && (
          <div className="row" style={{ gap: 0 }}>
            <button type="button" className={`btn btn--sm${mode === "somado" ? " btn--primary" : ""}`} onClick={() => setMode("somado")}>
              Somado
            </button>
            <button type="button" className={`btn btn--sm${mode === "separado" ? " btn--primary" : ""}`} onClick={() => setMode("separado")}>
              Separado ({carrier.interfaces.length} interfaces)
            </button>
          </div>
        )}
      </div>

      {mode === "separado" && hasMultiple ? (
        <PerInterfaceChart interfaces={carrier.interfaces} yDomain={yDomain} />
      ) : (
        <SimpleTrafficChart points={carrier.points} yDomain={yDomain} />
      )}

      <div className="row" style={{ gap: 16, flexWrap: "wrap", marginTop: 12 }}>
        {statBlock("Download", { min: carrier.stats.in_min, max: carrier.stats.in_max, avg: carrier.stats.in_avg })}
        {statBlock("Upload", { min: carrier.stats.out_min, max: carrier.stats.out_max, avg: carrier.stats.out_avg })}
      </div>
    </div>
  );
}

type Props = { deviceId: string | null };

/**
 * Painel de tráfego por operadora: 1 gráfico "TOTAL geral" (soma de todas as interfaces de
 * todas as operadoras) no topo, seguido de 1 gráfico dedicado por operadora (empilhados) — cada
 * um com toggle Separado/Somado próprio quando a operadora tem 2+ interfaces. Período
 * seleccionável (presets 24h…300 dias ou intervalo específico), teto do eixo Y a partir do
 * limite de banda cadastrado em Configurações → BGP → Operadoras.
 */
export function BgpTrafficPanel({ deviceId }: Props) {
  const [days, setDays] = useState<(typeof DAY_OPTIONS)[number]>(1);
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
  const clearCustomRange = () => setAppliedRange(null);

  const queryParams = useMemo(() => {
    const p = new URLSearchParams();
    if (appliedRange) {
      p.set("from", appliedRange.from);
      p.set("to", appliedRange.to);
    } else {
      p.set("days", String(days));
    }
    return p.toString();
  }, [appliedRange, days]);

  const historyQ = useQuery({
    queryKey: ["bgp-carrier-traffic-history", deviceId, queryParams],
    enabled: !!deviceId,
    placeholderData: keepPreviousData,
    queryFn: () => apiFetch<CarrierTrafficHistoryResponse>(`/api/v1/bgp/devices/${deviceId}/carrier-traffic-history?${queryParams}`),
  });

  const carriers = useMemo(() => [...(historyQ.data?.carriers ?? [])].sort((a, b) => a.carrier_label.localeCompare(b.carrier_label)), [historyQ.data]);
  const total = historyQ.data?.total ?? null;

  const totalYDomain = useMemo((): [number, number] | ["auto", "auto"] => {
    if (!total?.bandwidth_limit_mbps || total.bandwidth_limit_mbps <= 0) return ["auto", "auto"];
    return [0, total.bandwidth_limit_mbps * 1_000_000];
  }, [total]);

  if (!deviceId) return null;

  return (
    <div className="card" style={{ padding: 14, marginBottom: 16 }}>
      <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Tráfego por operadora</h3>

      <div className="row" style={{ gap: 6, flexWrap: "wrap", alignItems: "center", marginBottom: 10 }}>
        <span style={{ fontSize: 11, color: "var(--muted)" }}>Período:</span>
        {DAY_OPTIONS.map((d) => (
          <button
            key={d}
            type="button"
            className={`btn btn--sm${!appliedRange && days === d ? " btn--primary" : ""}`}
            onClick={() => {
              setDays(d);
              setAppliedRange(null);
            }}
          >
            {d === 1 ? "24 h" : `${d} dias`}
          </button>
        ))}
        <button type="button" className={`btn btn--sm${appliedRange ? " btn--primary" : ""}`} onClick={() => setCustomOpen((v) => !v)}>
          Período específico…
        </button>
      </div>

      {customOpen && (
        <div className="row" style={{ gap: 8, flexWrap: "wrap", alignItems: "flex-end", marginBottom: 10 }}>
          <label style={{ fontSize: 11, color: "var(--muted)", display: "flex", flexDirection: "column", gap: 2 }}>
            De
            <input type="datetime-local" className="input" value={fromInput} onChange={(e) => setFromInput(e.target.value)} />
          </label>
          <label style={{ fontSize: 11, color: "var(--muted)", display: "flex", flexDirection: "column", gap: 2 }}>
            Até
            <input type="datetime-local" className="input" value={toInput} onChange={(e) => setToInput(e.target.value)} />
          </label>
          <button type="button" className="btn btn--sm btn--primary" onClick={applyCustomRange}>
            Aplicar
          </button>
          {appliedRange && (
            <button type="button" className="btn btn--sm" onClick={clearCustomRange}>
              Limpar
            </button>
          )}
        </div>
      )}

      {carriers.length === 0 ? (
        <div className="msg msg--warn">
          Nenhuma operadora configurada com tráfego principal ainda. Configure em Configurações → BGP → Operadoras.
        </div>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
          <div className="card" style={{ padding: 14, background: "var(--panel2)" }}>
            <h4 style={{ margin: "0 0 8px", fontSize: 14 }}>TOTAL geral (todas as operadoras)</h4>
            {total ? <SimpleTrafficChart points={total.points} yDomain={totalYDomain} /> : null}
            {total ? (
              <div className="row" style={{ gap: 16, flexWrap: "wrap", marginTop: 12 }}>
                {statBlock("Download", { min: total.stats.in_min, max: total.stats.in_max, avg: total.stats.in_avg })}
                {statBlock("Upload", { min: total.stats.out_min, max: total.stats.out_max, avg: total.stats.out_avg })}
              </div>
            ) : null}
          </div>

          {carriers.map((c) => (
            <CarrierChart key={c.carrier_id} carrier={c} />
          ))}
        </div>
      )}

      {historyQ.isError && <div className="msg msg--err" style={{ marginTop: 8 }}>{(historyQ.error as Error).message || EM_DASH}</div>}
    </div>
  );
}
