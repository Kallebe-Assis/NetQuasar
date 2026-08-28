import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { Fragment, useEffect, useMemo, useState } from "react";
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
type CarrierTraffic = {
  carrier_label: string;
  bandwidth_limit_mbps?: number;
  points: TrafficPoint[];
  stats: CarrierStats;
};
type CarrierTrafficHistoryResponse = {
  device_id: string;
  since?: string;
  until?: string;
  bucket?: string;
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

type Props = { deviceId: string | null };

/**
 * Painel de tráfego por operadora (substitui o gráfico único da aba "Visão Geral"): período
 * seleccionável (presets 24h…300 dias ou intervalo específico), uma ou mais operadoras
 * seleccionadas (modo "Separado" — uma linha por operadora — ou "Somado" — soma numa linha só),
 * teto do eixo Y a partir do limite de banda configurado em Configurações → BGP → Operadoras, e
 * cartões de máx/mín/média de download/upload por operadora seleccionada.
 */
export function BgpTrafficPanel({ deviceId }: Props) {
  const [days, setDays] = useState<(typeof DAY_OPTIONS)[number]>(1);
  const [customOpen, setCustomOpen] = useState(false);
  const [fromInput, setFromInput] = useState(() => toLocalInputValue(new Date(Date.now() - 24 * 3600_000)));
  const [toInput, setToInput] = useState(() => toLocalInputValue(new Date()));
  const [appliedRange, setAppliedRange] = useState<{ from: string; to: string } | null>(null);
  const [selected, setSelected] = useState<string[]>([]);
  const [mode, setMode] = useState<"separado" | "somado">("separado");

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

  const carriers = historyQ.data?.carriers ?? [];

  // Selecciona todas as operadoras por omissão assim que a lista chega (1ª carga ou troca de
  // equipamento) — evita começar com o gráfico vazio.
  useEffect(() => {
    if (carriers.length === 0) return;
    setSelected((prev) => {
      const stillValid = prev.filter((l) => carriers.some((c) => c.carrier_label === l));
      return stillValid.length > 0 ? stillValid : carriers.map((c) => c.carrier_label);
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [deviceId, carriers.map((c) => c.carrier_label).join("|")]);

  const activeCarriers = useMemo(
    () => carriers.filter((c) => selected.includes(c.carrier_label)).sort((a, b) => a.carrier_label.localeCompare(b.carrier_label)),
    [carriers, selected],
  );

  const chartData = useMemo(() => {
    if (activeCarriers.length === 0) return [];
    const n = activeCarriers[0].points.length;
    const rows: Array<Record<string, number | string>> = [];
    for (let i = 0; i < n; i++) {
      const row: Record<string, number | string> = { t: activeCarriers[0].points[i]?.t ?? "" };
      if (mode === "separado") {
        for (const c of activeCarriers) {
          row[`in_${c.carrier_label}`] = c.points[i]?.in_bps ?? 0;
          row[`out_${c.carrier_label}`] = c.points[i]?.out_bps ?? 0;
        }
      } else {
        row.in_total = activeCarriers.reduce((acc, c) => acc + (c.points[i]?.in_bps ?? 0), 0);
        row.out_total = activeCarriers.reduce((acc, c) => acc + (c.points[i]?.out_bps ?? 0), 0);
      }
      rows.push(row);
    }
    return rows;
  }, [activeCarriers, mode]);

  // Teto do eixo Y a partir do(s) limite(s) de banda configurados (Configurações → BGP →
  // Operadoras): no modo Somado é a soma dos limites das operadoras seleccionadas (o gráfico
  // desenha a soma delas); no modo Separado é o maior limite entre as seleccionadas (cada linha
  // fica dentro do seu próprio tecto, mas partilham 1 só eixo). Sem nenhum limite configurado,
  // volta ao comportamento anterior (recharts escala automaticamente).
  const yDomain = useMemo((): [number, number] | ["auto", "auto"] => {
    const limitsBps = activeCarriers
      .map((c) => c.bandwidth_limit_mbps)
      .filter((v): v is number => typeof v === "number" && v > 0)
      .map((mbps) => mbps * 1_000_000);
    if (limitsBps.length === 0) return ["auto", "auto"];
    const yMax = mode === "somado" ? limitsBps.reduce((a, b) => a + b, 0) : Math.max(...limitsBps);
    return [0, yMax];
  }, [activeCarriers, mode]);

  function toggleCarrier(label: string) {
    setSelected((prev) => (prev.includes(label) ? prev.filter((l) => l !== label) : [...prev, label]));
  }

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
        <>
          <div className="row" style={{ gap: 10, flexWrap: "wrap", alignItems: "center", marginBottom: 12 }}>
            <span style={{ fontSize: 11, color: "var(--muted)" }}>Operadoras:</span>
            {carriers.map((c) => (
              <label key={c.carrier_label} style={{ display: "flex", alignItems: "center", gap: 4, fontSize: 12, cursor: "pointer" }}>
                <input type="checkbox" checked={selected.includes(c.carrier_label)} onChange={() => toggleCarrier(c.carrier_label)} />
                {c.carrier_label}
              </label>
            ))}
            {selected.length > 1 && (
              <div className="row" style={{ gap: 0, marginLeft: "auto" }}>
                <button
                  type="button"
                  className={`btn btn--sm${mode === "separado" ? " btn--primary" : ""}`}
                  onClick={() => setMode("separado")}
                >
                  Separado
                </button>
                <button
                  type="button"
                  className={`btn btn--sm${mode === "somado" ? " btn--primary" : ""}`}
                  onClick={() => setMode("somado")}
                >
                  Somado
                </button>
              </div>
            )}
          </div>

          {activeCarriers.length === 0 ? (
            <div className="msg" style={{ fontSize: 12 }}>Seleccione ao menos uma operadora.</div>
          ) : chartData.length < 2 ? (
            <div className="msg" style={{ fontSize: 12 }}>Sem amostras suficientes neste período ainda.</div>
          ) : (
            <>
              <ResponsiveContainer width="100%" height={260}>
                <LineChart data={chartData} margin={{ top: 8, right: 8, left: 0, bottom: 4 }}>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                  <XAxis
                    dataKey="t"
                    tick={{ fontSize: 9 }}
                    interval="preserveStartEnd"
                    minTickGap={24}
                    tickFormatter={(v) => formatDateTime(v)}
                  />
                  <YAxis tick={{ fontSize: 10 }} width={64} domain={yDomain} tickFormatter={(v) => formatBitrate(v)} />
                  <Tooltip
                    labelFormatter={(v) => formatDateTime(String(v))}
                    formatter={(v: number, name: string) => [formatBitrate(v), name]}
                    contentStyle={{ background: "var(--panel2)", border: "1px solid var(--border)", fontSize: 12, color: "var(--text)" }}
                    itemStyle={{ color: "var(--text)" }}
                    labelStyle={{ color: "var(--text)" }}
                  />
                  <Legend wrapperStyle={{ fontSize: 11 }} />
                  {mode === "somado" ? (
                    <>
                      <Line type="monotone" dataKey="in_total" name="Download total" stroke="#3b82f6" strokeWidth={2} dot={false} />
                      <Line type="monotone" dataKey="out_total" name="Upload total" stroke="#f97316" strokeWidth={2} dot={false} />
                    </>
                  ) : (
                    activeCarriers.map((c, i) => {
                      const color = CARRIER_COLORS[i % CARRIER_COLORS.length];
                      return (
                        <Fragment key={c.carrier_label}>
                          <Line
                            type="monotone"
                            dataKey={`in_${c.carrier_label}`}
                            name={`Download (${c.carrier_label})`}
                            stroke={color}
                            strokeWidth={2}
                            dot={false}
                          />
                          <Line
                            type="monotone"
                            dataKey={`out_${c.carrier_label}`}
                            name={`Upload (${c.carrier_label})`}
                            stroke={color}
                            strokeWidth={2}
                            strokeDasharray="4 3"
                            dot={false}
                          />
                        </Fragment>
                      );
                    })
                  )}
                </LineChart>
              </ResponsiveContainer>

              <div className="row" style={{ gap: 16, flexWrap: "wrap", marginTop: 14 }}>
                {activeCarriers.map((c) => (
                  <div key={c.carrier_label} style={{ display: "flex", gap: 16, alignItems: "flex-start" }}>
                    <div style={{ fontSize: 12, fontWeight: 600, minWidth: 70 }}>{c.carrier_label}</div>
                    {statBlock("Download", { min: c.stats.in_min, max: c.stats.in_max, avg: c.stats.in_avg })}
                    {statBlock("Upload", { min: c.stats.out_min, max: c.stats.out_max, avg: c.stats.out_avg })}
                  </div>
                ))}
              </div>
            </>
          )}
        </>
      )}

      {historyQ.isError && <div className="msg msg--err" style={{ marginTop: 8 }}>{(historyQ.error as Error).message || EM_DASH}</div>}
    </div>
  );
}
