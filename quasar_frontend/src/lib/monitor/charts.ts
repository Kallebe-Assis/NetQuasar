
const MAX_HISTORY = 50;

/** Atualiza histórico de tráfego a partir de tabela de interfaces (formato tx/rx usado nas páginas NOC). */
export function trafficHistoryFromInterfaces(
  prev: Record<number, Array<{ ts: number; tx: number; rx: number }>>,
  rows: Array<{ if_index: number; in_bps?: number; out_bps?: number }>,
  ts = Date.now(),
): Record<number, Array<{ ts: number; tx: number; rx: number }>> {
  let next = { ...prev };
  let changed = false;
  for (const r of rows) {
    const rx = Number(r.in_bps ?? NaN);
    const tx = Number(r.out_bps ?? NaN);
    if (!Number.isFinite(rx) || !Number.isFinite(tx)) continue;
    const prevArr = next[r.if_index] ?? [];
    const last = prevArr[prevArr.length - 1];
    // Evita gráfico “a andar” com o mesmo valor do cache.
    if (last && last.tx === tx && last.rx === rx) continue;
    next[r.if_index] = [...prevArr, { ts, tx, rx }].slice(-MAX_HISTORY);
    changed = true;
  }
  return changed ? next : prev;
}
