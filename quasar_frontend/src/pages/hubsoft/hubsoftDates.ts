/**
 * Data de hoje (+ deslocamento em dias) no formato YYYY-MM-DD, no fuso LOCAL.
 * `toISOString()` usa UTC e viraria "amanhã" depois das 21h no Brasil — por isso não é usado aqui.
 */
export function todayISO(offsetDays = 0): string {
  const d = new Date();
  d.setDate(d.getDate() + offsetDays);
  const mm = String(d.getMonth() + 1).padStart(2, "0");
  const dd = String(d.getDate()).padStart(2, "0");
  return `${d.getFullYear()}-${mm}-${dd}`;
}
