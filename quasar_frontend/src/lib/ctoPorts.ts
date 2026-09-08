/** Contagem de portas do splitter de uma CTO (ver backend ctoPortCounts,
 * handlers_network_infrastructure.go) — "ocupada"/"livre" seguem o vocabulário de
 * SplitterPortStatus (lib/fiberSplitter.ts). Ausência de ports_total = CTO sem splitter/portas
 * cadastradas ainda. */
export type CtoPortCounts = {
  ports_total?: number | null;
  ports_used?: number | null;
  ports_free?: number | null;
};

/** Escala de cores por ocupação de portas — usada na "Vista da CTO: Ocupação de portas" do mapa
 * (MapFilterModal) e em qualquer resumo/dashboard de ocupação. Uma CTO com capacidade conhecida
 * (ports_total > 0) mas nenhuma porta com status ainda preenchido (used=0 e free=0 — o splitter
 * nunca foi configurado no modal de fibras) cai num cinza "sem dados", distinto de "tudo livre"
 * (que seria enganoso: não sabemos se está tudo livre, só que ninguém registou nada ainda). */
export function ctoOccupancyColor(c: CtoPortCounts): string {
  const total = c.ports_total ?? 0;
  const used = c.ports_used ?? 0;
  const free = c.ports_free ?? 0;
  if (total <= 0) return "#94a3b8";
  if (used === 0 && free === 0) return "#94a3b8";
  const pct = used / total;
  if (pct >= 0.95) return "#dc2626";
  if (pct >= 0.8) return "#ea580c";
  if (pct >= 0.5) return "#eab308";
  return "#16a34a";
}

/** Legenda curta para popup/painel lateral da CTO. */
export function ctoOccupancyLabel(c: CtoPortCounts): string {
  const total = c.ports_total ?? 0;
  const used = c.ports_used ?? 0;
  const free = c.ports_free ?? 0;
  if (total <= 0) return "Sem informação de portas";
  if (used === 0 && free === 0) return `${total} porta${total === 1 ? "" : "s"} (sem status registado)`;
  return `${used} ocupada${used === 1 ? "" : "s"} · ${free} livre${free === 1 ? "" : "s"} de ${total}`;
}

export const CTO_OCCUPANCY_LEGEND = [
  { color: "#16a34a", label: "Maioria livre (<50%)" },
  { color: "#eab308", label: "50–79% ocupada" },
  { color: "#ea580c", label: "80–94% ocupada" },
  { color: "#dc2626", label: "≥95% ocupada / cheia" },
  { color: "#94a3b8", label: "Sem dados de portas" },
] as const;
