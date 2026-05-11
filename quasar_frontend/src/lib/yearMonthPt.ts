const MONTH_LABELS_PT = [
  "Janeiro",
  "Fevereiro",
  "Março",
  "Abril",
  "Maio",
  "Junho",
  "Julho",
  "Agosto",
  "Setembro",
  "Outubro",
  "Novembro",
  "Dezembro",
] as const;

/** Converte «2026-04» em «Abril de 2026». Devolve o texto original se não for AAAA-MM válido. */
export function formatYearMonthPt(ym: string): string {
  const s = String(ym ?? "").trim();
  if (!/^\d{4}-\d{2}$/.test(s)) return s || "—";
  const m = s.match(/^(\d{4})-(\d{2})$/);
  if (!m) return s;
  const y = m[1];
  const mo = Number(m[2]);
  if (mo < 1 || mo > 12) return s;
  const label = MONTH_LABELS_PT[mo - 1];
  return `${label} de ${y}`;
}

/** Garante que o valor seleccionado (AAAA-MM técnico) aparece sempre com etiqueta PT, mesmo fora da lista pré-definida. */
export function monthSelectChoicesWithFallback(
  predefined: { value: string; label: string }[],
  currentValue: string,
): { value: string; label: string }[] {
  const known = new Set(predefined.map((o) => o.value));
  const list = [...predefined];
  if (currentValue && !known.has(currentValue)) {
    list.unshift({ value: currentValue, label: formatYearMonthPt(currentValue) });
  }
  return list;
}

/** Opções para um <select>: mês corrente primeiro, depois meses anteriores. */
export function recentYearMonthChoices(monthsBack = 60): { value: string; label: string }[] {
  const out: { value: string; label: string }[] = [];
  const d = new Date();
  for (let i = 0; i <= monthsBack; i++) {
    const y = d.getFullYear();
    const mo = d.getMonth() + 1;
    const value = `${y}-${String(mo).padStart(2, "0")}`;
    out.push({ value, label: formatYearMonthPt(value) });
    d.setMonth(d.getMonth() - 1);
  }
  return out;
}
