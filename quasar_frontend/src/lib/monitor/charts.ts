import type { MonitorGaugePoint, MonitorTrafficPoint } from "./types";

const MAX_HISTORY = 50;

/** Acrescenta ponto de tráfego ao histórico por if_index. */
export function appendTrafficPoint(
  prev: Record<number, MonitorTrafficPoint[]>,
  ifIndex: number,
  rxBps: number,
  txBps: number,
  ts = Date.now(),
): Record<number, MonitorTrafficPoint[]> {
  if (!Number.isFinite(rxBps) || !Number.isFinite(txBps)) return prev;
  const arr = [...(prev[ifIndex] ?? []), { ts, rx_bps: rxBps, tx_bps: txBps }];
  return { ...prev, [ifIndex]: arr.slice(-MAX_HISTORY) };
}

/** Acrescenta ponto de gauge (CPU/memória). */
export function appendGaugePoint(
  prev: MonitorGaugePoint[],
  value: number | null | undefined,
  ts = Date.now(),
): MonitorGaugePoint[] {
  if (value == null || !Number.isFinite(value)) return prev;
  return [...prev, { ts, value }].slice(-40);
}

/** Converte histórico de tráfego para Recharts (Mbps). */
export function trafficChartSeries(points: MonitorTrafficPoint[]) {
  return points.map((p) => ({
    t: new Date(p.ts).toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit", second: "2-digit" }),
    rx: p.rx_bps / 1_000_000,
    tx: p.tx_bps / 1_000_000,
  }));
}

/** Converte histórico de gauge para Recharts. */
export function gaugeChartSeries(points: MonitorGaugePoint[]) {
  return points.map((p) => ({
    t: new Date(p.ts).toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit", second: "2-digit" }),
    v: p.value,
  }));
}

/** Atualiza histórico de tráfego a partir de tabela de interfaces (formato tx/rx usado nas páginas NOC). */
export function trafficHistoryFromInterfaces(
  prev: Record<number, Array<{ ts: number; tx: number; rx: number }>>,
  rows: Array<{ if_index: number; in_bps?: number; out_bps?: number }>,
  ts = Date.now(),
): Record<number, Array<{ ts: number; tx: number; rx: number }>> {
  let next = { ...prev };
  for (const r of rows) {
    const rx = Number(r.in_bps ?? NaN);
    const tx = Number(r.out_bps ?? NaN);
    if (!Number.isFinite(rx) || !Number.isFinite(tx)) continue;
    const arr = [...(next[r.if_index] ?? []), { ts, tx, rx }];
    next[r.if_index] = arr.slice(-MAX_HISTORY);
  }
  return next;
}
